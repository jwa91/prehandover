package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jwa91/prehandover/internal/config"
	"github.com/jwa91/prehandover/internal/filter"
)

type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusTimeout Status = "timeout"
	StatusSkip    Status = "skip"
	StatusError   Status = "error"
)

type Result struct {
	ID       string
	Status   Status
	Duration time.Duration
	Budget   time.Duration
	Output   string
	Reason   string
}

type Run struct {
	Status   Status
	Duration time.Duration
	Budget   time.Duration
	Results  []Result
}

func Execute(ctx context.Context, cfg *config.Config, changed []string) (*Run, error) {
	start := time.Now()

	runCtx, cancel := context.WithTimeout(ctx, cfg.Budget.Duration)
	defer cancel()

	checks := make([]config.Check, len(cfg.Checks))
	copy(checks, cfg.Checks)
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].Priority < checks[j].Priority })

	workers := runtime.NumCPU()
	if cfg.Parallelism != "auto" {
		if n, err := strconv.Atoi(cfg.Parallelism); err == nil && n > 0 {
			workers = n
		}
	}

	globalExclude, err := filter.New(cfg.Files, cfg.Exclude)
	if err != nil {
		return nil, fmt.Errorf("global filter: %w", err)
	}
	changedFiltered := globalExclude.Filter(changed)

	type job struct {
		check config.Check
		files []string
	}
	var serial, parallel []job
	var results []Result

	for _, c := range checks {
		if c.ID == "" {
			return nil, errors.New("check missing id")
		}
		if c.Entry == "" {
			return nil, fmt.Errorf("check %q missing entry", c.ID)
		}
		m, err := filter.New(c.Files, c.Exclude)
		if err != nil {
			return nil, fmt.Errorf("check %q: %w", c.ID, err)
		}
		matched := m.Filter(changedFiltered)
		if !c.AlwaysRun && cfg.OnUnchanged == "skip" && len(matched) == 0 {
			results = append(results, Result{
				ID:     c.ID,
				Status: StatusSkip,
				Budget: c.Budget.Duration,
				Reason: "no matching changed files",
			})
			continue
		}
		j := job{check: c, files: matched}
		if c.RequireSerial {
			serial = append(serial, j)
		} else {
			parallel = append(parallel, j)
		}
	}

	failFastFired := false
	for _, j := range serial {
		if failFastFired {
			break
		}
		r := runCheck(runCtx, j.check, j.files, cfg.Budget.Duration-time.Since(start))
		results = append(results, r)
		if cfg.FailFast && (r.Status == StatusFail || r.Status == StatusError) {
			failFastFired = true
		}
	}

	if !failFastFired && len(parallel) > 0 {
		var mu sync.Mutex
		var wg sync.WaitGroup
		sem := make(chan struct{}, workers)
		for _, j := range parallel {
			wg.Add(1)
			sem <- struct{}{}
			go func(j job) {
				defer wg.Done()
				defer func() { <-sem }()
				remaining := cfg.Budget.Duration - time.Since(start)
				r := runCheck(runCtx, j.check, j.files, remaining)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}(j)
		}
		wg.Wait()
	}

	return &Run{
		Status:   aggregate(results),
		Duration: time.Since(start),
		Budget:   cfg.Budget.Duration,
		Results:  results,
	}, nil
}

func runCheck(ctx context.Context, c config.Check, files []string, remaining time.Duration) Result {
	budget := c.Budget.Duration
	if budget <= 0 {
		budget = remaining
	}
	if budget > remaining {
		budget = remaining
	}
	if budget <= 0 {
		return Result{ID: c.ID, Status: StatusTimeout, Budget: c.Budget.Duration, Reason: "total budget exhausted before check could start"}
	}

	cctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	var name string
	var args []string
	if c.Shell != "" {
		// Run entry through a shell. Filenames become positional args ($1, $2, ...).
		// The "_" is $0 (script name placeholder).
		name = c.Shell
		args = []string{"-c", c.Entry, "_"}
		args = append(args, c.Args...)
		if c.PassFilenames.Effective() {
			files = limitFiles(files, c.PassFilenames.Limit)
			args = append(args, files...)
		}
	} else {
		parts, err := splitEntry(c.Entry)
		if err != nil {
			return Result{ID: c.ID, Status: StatusError, Reason: err.Error(), Budget: budget}
		}
		name = parts[0]
		args = append([]string{}, parts[1:]...)
		args = append(args, c.Args...)
		if c.PassFilenames.Effective() {
			files = limitFiles(files, c.PassFilenames.Limit)
			args = append(args, files...)
		}
	}

	start := time.Now()
	cmd := exec.CommandContext(cctx, name, args...)
	cmd.Env = os.Environ()
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	runErr := cmd.Run()
	dur := time.Since(start)

	r := Result{ID: c.ID, Duration: dur, Budget: budget, Output: out.String()}
	if cctx.Err() == context.DeadlineExceeded {
		r.Status = StatusTimeout
		r.Reason = fmt.Sprintf("exceeded %s budget", budget)
		return r
	}
	if runErr != nil {
		r.Status = StatusFail
		return r
	}
	r.Status = StatusPass
	if !c.Verbose {
		r.Output = ""
	}
	return r
}

func limitFiles(files []string, limit int) []string {
	if limit > 0 && len(files) > limit {
		return files[:limit]
	}
	return files
}

func splitEntry(entry string) ([]string, error) {
	var parts []string
	var cur []byte
	var quote byte
	for i := 0; i < len(entry); i++ {
		ch := entry[i]
		if quote != 0 {
			if ch == quote {
				quote = 0
				continue
			}
			cur = append(cur, ch)
			continue
		}
		if ch == '"' || ch == '\'' {
			quote = ch
			continue
		}
		if ch == ' ' || ch == '\t' {
			if len(cur) > 0 {
				parts = append(parts, string(cur))
				cur = cur[:0]
			}
			continue
		}
		cur = append(cur, ch)
	}
	if len(cur) > 0 {
		parts = append(parts, string(cur))
	}
	if len(parts) == 0 {
		return nil, errors.New("empty entry")
	}
	return parts, nil
}

func aggregate(results []Result) Status {
	hasFail, hasTimeout := false, false
	for _, r := range results {
		switch r.Status {
		case StatusFail, StatusError:
			hasFail = true
		case StatusTimeout:
			hasTimeout = true
		}
	}
	if hasFail {
		return StatusFail
	}
	if hasTimeout {
		return StatusTimeout
	}
	return StatusPass
}

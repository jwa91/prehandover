package proof

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/jwa91/prehandover/internal/lifecycle"
	"github.com/jwa91/prehandover/internal/runner"
)

const LatestPath = ".prehandover/runs/latest.json"

type Artifact struct {
	Version             int          `json:"version"`
	GeneratedAt         string       `json:"generated_at"`
	Moment              string       `json:"moment"`
	Harness             string       `json:"harness"`
	CWD                 string       `json:"cwd"`
	SessionID           string       `json:"session_id,omitempty"`
	TurnID              string       `json:"turn_id,omitempty"`
	TranscriptPath      string       `json:"transcript_path,omitempty"`
	ConfigPath          string       `json:"config_path"`
	ConfigSHA256        string       `json:"config_sha256,omitempty"`
	ChangedFiles        []string     `json:"changed_files,omitempty"`
	Status              string       `json:"status"`
	Category            string       `json:"category"`
	DurationMs          int64        `json:"duration_ms,omitempty"`
	BudgetMs            int64        `json:"budget_ms,omitempty"`
	ContinuationMessage string       `json:"continuation_message,omitempty"`
	Error               string       `json:"error,omitempty"`
	Checks              []CheckProof `json:"checks,omitempty"`
}

type CheckProof struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	BudgetMs   int64  `json:"budget_ms"`
	Reason     string `json:"reason,omitempty"`
	Output     string `json:"output,omitempty"`
}

func FromRun(inv lifecycle.Invocation, configPath string, changed []string, outcome lifecycle.Outcome) Artifact {
	status := string(runner.StatusPass)
	category := "passed"
	var durationMs, budgetMs int64
	var checks []CheckProof
	if outcome.Run != nil {
		status = string(outcome.Run.Status)
		category = CategoryForRun(outcome.Run)
		durationMs = outcome.Run.Duration.Milliseconds()
		budgetMs = outcome.Run.Budget.Milliseconds()
		for _, res := range outcome.Run.Results {
			checks = append(checks, CheckProof{
				ID:         res.ID,
				Status:     string(res.Status),
				DurationMs: res.Duration.Milliseconds(),
				BudgetMs:   res.Budget.Milliseconds(),
				Reason:     res.Reason,
				Output:     res.Output,
			})
		}
	}
	return Artifact{
		Version:             1,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Moment:              string(inv.Moment),
		Harness:             inv.Harness,
		CWD:                 inv.CWD,
		SessionID:           inv.SessionID,
		TurnID:              inv.TurnID,
		TranscriptPath:      inv.TranscriptPath,
		ConfigPath:          configPath,
		ConfigSHA256:        fileSHA256(configPath),
		ChangedFiles:        changed,
		Status:              status,
		Category:            category,
		DurationMs:          durationMs,
		BudgetMs:            budgetMs,
		ContinuationMessage: outcome.ContinueMessage,
		Checks:              checks,
	}
}

func Failure(inv lifecycle.Invocation, configPath string, category string, err error) Artifact {
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Artifact{
		Version:             1,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		Moment:              string(inv.Moment),
		Harness:             inv.Harness,
		CWD:                 inv.CWD,
		SessionID:           inv.SessionID,
		TurnID:              inv.TurnID,
		TranscriptPath:      inv.TranscriptPath,
		ConfigPath:          configPath,
		ConfigSHA256:        fileSHA256(configPath),
		Status:              string(runner.StatusError),
		Category:            category,
		ContinuationMessage: message,
		Error:               message,
	}
}

func CategoryForRun(r *runner.Run) string {
	switch r.Status {
	case runner.StatusPass:
		return "passed"
	case runner.StatusTimeout:
		return "budget_exceeded"
	case runner.StatusFail:
		for _, res := range r.Results {
			if res.Status == runner.StatusError {
				return "validator_error"
			}
		}
		return "validator_failed"
	default:
		return "execution_error"
	}
}

func WriteLatest(a Artifact) error {
	if err := os.MkdirAll(filepath.Dir(LatestPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(LatestPath, data, 0644)
}

func fileSHA256(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ""
		}
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

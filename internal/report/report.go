package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/jwa91/prehandover/internal/runner"
)

func Human(w io.Writer, r *runner.Run) {
	var pass, fail, skip, timeout int
	for _, res := range r.Results {
		switch res.Status {
		case runner.StatusPass:
			pass++
		case runner.StatusFail, runner.StatusError:
			fail++
		case runner.StatusSkip:
			skip++
		case runner.StatusTimeout:
			timeout++
		}
		dur := "      —"
		if res.Duration > 0 {
			dur = fmt.Sprintf("%6dms", res.Duration.Milliseconds())
		}
		fmt.Fprintf(w, "%-20s %-8s %s", res.ID, res.Status, dur)
		if res.Reason != "" {
			fmt.Fprintf(w, "   %s", res.Reason)
		}
		fmt.Fprintln(w)
		if res.Output != "" && res.Status != runner.StatusPass {
			for _, line := range strings.Split(strings.TrimRight(res.Output, "\n"), "\n") {
				fmt.Fprintf(w, "  | %s\n", line)
			}
		}
	}
	fmt.Fprintf(w, "\n%d passed, %d failed, %d skipped, %d timed out — %dms / %dms budget\n",
		pass, fail, skip, timeout, r.Duration.Milliseconds(), r.Budget.Milliseconds())
}

type jsonResult struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	DurationMs int64  `json:"duration_ms"`
	BudgetMs   int64  `json:"budget_ms"`
	Output     string `json:"output,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type jsonRun struct {
	Status     string       `json:"status"`
	DurationMs int64        `json:"duration_ms"`
	BudgetMs   int64        `json:"budget_ms"`
	Checks     []jsonResult `json:"checks"`
}

func JSON(w io.Writer, r *runner.Run) error {
	out := jsonRun{
		Status:     string(r.Status),
		DurationMs: r.Duration.Milliseconds(),
		BudgetMs:   r.Budget.Milliseconds(),
	}
	for _, res := range r.Results {
		out.Checks = append(out.Checks, jsonResult{
			ID:         res.ID,
			Status:     string(res.Status),
			DurationMs: res.Duration.Milliseconds(),
			BudgetMs:   res.Budget.Milliseconds(),
			Output:     res.Output,
			Reason:     res.Reason,
		})
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

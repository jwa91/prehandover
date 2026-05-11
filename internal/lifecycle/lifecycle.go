package lifecycle

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/jwa91/prehandover/internal/runner"
)

type Moment string

const (
	MomentAgentStop         Moment = "agent_stop"
	MomentSessionContext    Moment = "session_context"
	MomentPromptIngress     Moment = "prompt_ingress"
	MomentToolPreflight     Moment = "tool_preflight"
	MomentToolResult        Moment = "tool_result"
	MomentWorkerStop        Moment = "worker_stop"
	MomentContextCompaction Moment = "context_compaction"
	MomentSessionEnd        Moment = "session_end"
)

var ReservedMoments = []Moment{
	MomentAgentStop,
	MomentSessionContext,
	MomentPromptIngress,
	MomentToolPreflight,
	MomentToolResult,
	MomentWorkerStop,
	MomentContextCompaction,
	MomentSessionEnd,
}

type Invocation struct {
	Harness              string
	Moment               Moment
	CWD                  string
	SessionID            string
	TurnID               string
	TranscriptPath       string
	LastAssistantMessage string
	Raw                  map[string]any
}

type Outcome struct {
	Allow           bool
	ContinueMessage string
	UserMessage     string
	Run             *runner.Run
}

type Adapter interface {
	Name() string
	Supports(Moment) bool
	Decode(Moment, []byte) (Invocation, error)
	Encode(Moment, Outcome, io.Writer) error
}

func ForHarness(name string) (Adapter, bool) {
	switch strings.ToLower(name) {
	case "claude":
		return decisionAdapter{name: "claude"}, true
	case "codex":
		return decisionAdapter{name: "codex"}, true
	case "cursor":
		return cursorAdapter{}, true
	default:
		return nil, false
	}
}

func OutcomeFromRun(r *runner.Run) Outcome {
	if r.Status == runner.StatusPass {
		return Outcome{Allow: true, Run: r}
	}
	return Outcome{
		Allow:           false,
		ContinueMessage: MessageFromRun(r),
		Run:             r,
	}
}

func FailureOutcome(message string) Outcome {
	return Outcome{
		Allow:           false,
		ContinueMessage: message,
		UserMessage:     message,
	}
}

func MessageFromRun(r *runner.Run) string {
	var b strings.Builder
	for _, res := range r.Results {
		if res.Status == runner.StatusPass || res.Status == runner.StatusSkip {
			continue
		}
		fmt.Fprintf(&b, "[%s] %s", res.ID, res.Status)
		if res.Reason != "" {
			fmt.Fprintf(&b, " - %s", res.Reason)
		}
		b.WriteString("\n")
		if res.Output != "" {
			b.WriteString(res.Output)
			if !strings.HasSuffix(res.Output, "\n") {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

type decisionAdapter struct {
	name string
}

func (a decisionAdapter) Name() string { return a.name }

func (a decisionAdapter) Supports(m Moment) bool { return m == MomentAgentStop }

func (a decisionAdapter) Decode(m Moment, input []byte) (Invocation, error) {
	raw, err := decodeRaw(input)
	if err != nil {
		return Invocation{}, err
	}
	cwdFallback := currentWD()
	if a.name == "claude" {
		cwdFallback = envOrWD("CLAUDE_PROJECT_DIR")
	}
	return Invocation{
		Harness:              a.name,
		Moment:               m,
		CWD:                  firstString(raw, "cwd", cwdFallback),
		SessionID:            stringField(raw, "session_id"),
		TurnID:               stringField(raw, "turn_id"),
		TranscriptPath:       stringField(raw, "transcript_path"),
		LastAssistantMessage: stringField(raw, "last_assistant_message"),
		Raw:                  raw,
	}, nil
}

func (a decisionAdapter) Encode(m Moment, out Outcome, w io.Writer) error {
	if !a.Supports(m) {
		return fmt.Errorf("%s does not support moment %q", a.name, m)
	}
	if out.Allow {
		return nil
	}
	return writeJSON(w, map[string]any{
		"decision": "block",
		"reason":   out.ContinueMessage,
	})
}

type cursorAdapter struct{}

func (a cursorAdapter) Name() string { return "cursor" }

func (a cursorAdapter) Supports(m Moment) bool { return m == MomentAgentStop }

func (a cursorAdapter) Decode(m Moment, input []byte) (Invocation, error) {
	raw, err := decodeRaw(input)
	if err != nil {
		return Invocation{}, err
	}
	cwd := firstString(raw, "cwd", "")
	if cwd == "" {
		cwd = firstWorkspaceRoot(raw)
	}
	if cwd == "" {
		cwd = envOrWD("CURSOR_PROJECT_DIR")
	}
	return Invocation{
		Harness:        "cursor",
		Moment:         m,
		CWD:            cwd,
		SessionID:      firstString(raw, "conversation_id", stringField(raw, "session_id")),
		TurnID:         stringField(raw, "generation_id"),
		TranscriptPath: stringField(raw, "transcript_path"),
		Raw:            raw,
	}, nil
}

func (a cursorAdapter) Encode(m Moment, out Outcome, w io.Writer) error {
	if !a.Supports(m) {
		return fmt.Errorf("cursor does not support moment %q", m)
	}
	if out.Allow {
		return nil
	}
	return writeJSON(w, map[string]any{
		"followup_message": out.ContinueMessage,
	})
}

func decodeRaw(input []byte) (map[string]any, error) {
	raw := map[string]any{}
	if len(strings.TrimSpace(string(input))) == 0 {
		return raw, nil
	}
	if err := json.Unmarshal(input, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		raw = map[string]any{}
	}
	return raw, nil
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func firstString(raw map[string]any, key string, fallback string) string {
	if v := stringField(raw, key); v != "" {
		return v
	}
	return fallback
}

func stringField(raw map[string]any, key string) string {
	if v, ok := raw[key].(string); ok {
		return v
	}
	return ""
}

func firstWorkspaceRoot(raw map[string]any) string {
	roots, ok := raw["workspace_roots"].([]any)
	if !ok || len(roots) == 0 {
		return ""
	}
	root, _ := roots[0].(string)
	return root
}

func envOrWD(env string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return currentWD()
}

func currentWD() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

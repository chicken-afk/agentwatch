// Package hook handles `agentwatch hook`: called by Claude Code on every event,
// it summarizes the payload and appends it to events.jsonl.
package hook

import (
	"encoding/json"
	"io"
	"time"

	"github.com/chicken-afk/agentwatch/internal/events"
	"github.com/chicken-afk/agentwatch/internal/format"
)

// Run reads one hook payload from stdin. Important rules:
//   - NEVER writes to stdout (for SessionStart/UserPromptSubmit,
//     stdout would be injected into Claude's context).
//   - Never fails or panics to the caller, and is fast, so it never
//     blocks Claude; the parent process always exits 0.
//
// agent names the tool that sent the event ("" = Claude Code); it is stamped as
// agentwatch_agent so the dashboard keeps sessions from different tools apart.
func Run(stdin io.Reader, agent string) {
	defer func() { _ = recover() }()

	raw, err := io.ReadAll(io.LimitReader(stdin, 32<<20))
	if err != nil || len(raw) == 0 {
		return
	}
	var in map[string]any
	if json.Unmarshal(raw, &in) != nil {
		return
	}

	out := Slim(in)
	out["agentwatch_ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	if agent != "" && agent != "claude" {
		out["agentwatch_agent"] = agent
	}
	record, err := json.Marshal(out)
	if err != nil {
		return
	}
	_ = events.Append(record)
}

var (
	eventKeys = []string{
		"session_id", "transcript_path", "cwd", "hook_event_name", "permission_mode",
		"tool_name", "tool_use_id", "agent_id", "agent_type", "agent_transcript_path",
		"source", "reason", "model", "stop_hook_active", "notification_type",
		"message", "trigger", "error", "is_interrupt",
		"input_tokens", "output_tokens", "cache_read_tokens", // generic Message events
	}
	toolInputKeys = []string{
		"file_path", "path", "pattern", "command", "description", "url", "query",
		"subagent_type", "model", "skill", "args", "notebook_path", "glob", "run_in_background",
	}
)

// Slim drops the large parts (file contents, tool output) and keeps
// only the fields the dashboard uses.
func Slim(in map[string]any) map[string]any {
	out := map[string]any{}
	copyFields(out, in, eventKeys, 300)
	if p, ok := in["prompt"].(string); ok {
		out["prompt"] = format.Clip(p, 200)
	}
	if ti, ok := in["tool_input"].(map[string]any); ok {
		slim := map[string]any{}
		copyFields(slim, ti, toolInputKeys, 160)
		if p, ok := ti["prompt"].(string); ok {
			slim["prompt"] = format.Clip(p, 120)
		}
		out["tool_input"] = slim
	}
	// model is sometimes an object in some versions
	if m, ok := in["model"].(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			out["model"] = id
		}
	}
	return out
}

// copyFields copies keys from src to dst; strings are truncated to maxLen.
func copyFields(dst, src map[string]any, keys []string, maxLen int) {
	for _, k := range keys {
		v, ok := src[k]
		if !ok {
			continue
		}
		if s, isStr := v.(string); isStr {
			v = format.Clip(s, maxLen)
		}
		dst[k] = v
	}
}

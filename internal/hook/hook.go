// Package hook menangani `ccwatch hook`: dipanggil Claude Code di tiap event,
// meringkas payload, lalu menambahkannya ke events.jsonl.
package hook

import (
	"encoding/json"
	"io"
	"time"

	"ccwatch/internal/events"
	"ccwatch/internal/format"
)

// Run membaca satu payload hook dari stdin. Aturan penting:
//   - TIDAK pernah menulis ke stdout (untuk SessionStart/UserPromptSubmit,
//     stdout akan disuntikkan ke konteks Claude).
//   - Tidak pernah gagal atau panic ke pemanggil dan cepat, supaya tidak
//     memblok Claude; proses induk selalu exit 0.
func Run(stdin io.Reader) {
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
	out["ccwatch_ts"] = time.Now().UTC().Format(time.RFC3339Nano)
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
	}
	toolInputKeys = []string{
		"file_path", "path", "pattern", "command", "description", "url", "query",
		"subagent_type", "model", "skill", "args", "notebook_path", "glob", "run_in_background",
	}
)

// Slim membuang bagian besar (isi file, output tool) dan hanya
// menyimpan field yang dipakai dashboard.
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
	// model kadang berupa object di beberapa versi
	if m, ok := in["model"].(map[string]any); ok {
		if id, ok := m["id"].(string); ok {
			out["model"] = id
		}
	}
	return out
}

// copyFields menyalin keys dari src ke dst; string dipotong maksimal maxLen.
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

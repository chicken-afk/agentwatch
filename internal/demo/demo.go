// Package demo replays a fake Claude Code session (hooks + transcript) in a temporary
// folder, so the dashboard can be tried without running Claude Code.
package demo

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ccwatch/internal/store"
	"ccwatch/internal/ui"
)

const (
	leadModel  = "claude-sonnet-5-5"
	haikuModel = "claude-haiku-5-5"
)

// Run runs the demo.
//
//	ccwatch demo                  play live
//	ccwatch demo --snapshot 140x45  print a single frame (for tests); --tab N picks the tab (0-3)
func Run(args []string) error {
	snapshot := argValue(args, "--snapshot")
	tab := 0
	if v := argValue(args, "--tab"); v != "" {
		fmt.Sscanf(v, "%d", &tab)
	}

	tmp, err := os.MkdirTemp("", "ccwatch-demo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := setupDemoConfig(tmp); err != nil {
		return err
	}

	sim := newSimulator(tmp)
	if snapshot != "" {
		var w, h int
		fmt.Sscanf(snapshot, "%dx%d", &w, &h)
		sim.fake = true
		sim.clock = time.Now().Add(-4 * time.Minute)
		sim.play()
		fmt.Println(ui.Snapshot(store.Options{}, w, h, tab))
		return nil
	}
	go func() {
		time.Sleep(800 * time.Millisecond)
		sim.play()
	}()
	return ui.Run(store.Options{})
}

// argValue: the argument after flag name ("" if absent). Unknown arguments are ignored.
func argValue(args []string, name string) string {
	for i, a := range args {
		if a == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// setupDemoConfig points ccwatch at a temporary folder and prepares settings + sample subagents.
func setupDemoConfig(tmp string) error {
	os.Setenv("CCWATCH_DIR", tmp)
	os.Setenv("CLAUDE_CONFIG_DIR", tmp)
	if err := os.WriteFile(filepath.Join(tmp, "settings.json"), []byte(`{"advisorModel":"claude-opus-5-5"}`), 0o644); err != nil {
		return err
	}
	// configured subagent that isn't called in the demo → shown as "ready"
	if err := os.MkdirAll(filepath.Join(tmp, "agents"), 0o755); err != nil {
		return err
	}
	reviewer := "---\nname: reviewer\ndescription: Review code changes before committing\nmodel: sonnet\n---\n"
	return os.WriteFile(filepath.Join(tmp, "agents", "reviewer.md"), []byte(reviewer), 0o644)
}

// simulator writes fake hook events and transcripts. In fake mode, time
// doesn't actually wait: a virtual clock is advanced.
type simulator struct {
	dir, sid, cwd, transcript string
	fake                      bool
	clock                     time.Time
	n                         int
}

func newSimulator(dir string) *simulator {
	s := &simulator{dir: dir, sid: "7f3c9a21-5b2e-4d1a-9c0e-demo00000001", cwd: "/home/dika/app"}
	s.transcript = filepath.Join(dir, "projects", "-home-dika-app", s.sid+".jsonl")
	_ = os.MkdirAll(filepath.Join(strings.TrimSuffix(s.transcript, ".jsonl"), "subagents"), 0o755)
	return s
}

func (d *simulator) now() time.Time {
	if d.fake {
		return d.clock
	}
	return time.Now()
}

func (d *simulator) wait(sec float64) {
	dd := time.Duration(sec * float64(time.Second))
	if d.fake {
		d.clock = d.clock.Add(dd)
		return
	}
	time.Sleep(dd)
}

func (d *simulator) appendJSON(path string, v any) {
	b, _ := json.Marshal(v)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	f.Write(append(b, '\n'))
	f.Close()
}

// ev writes one hook event to events.jsonl.
func (d *simulator) ev(name string, extra map[string]any) {
	e := map[string]any{
		"session_id": d.sid, "transcript_path": d.transcript, "cwd": d.cwd,
		"hook_event_name": name, "permission_mode": "acceptEdits",
		"ccwatch_ts": d.now().UTC().Format(time.RFC3339Nano),
	}
	for k, v := range extra {
		e[k] = v
	}
	d.appendJSON(filepath.Join(d.dir, "events.jsonl"), e)
}

func (d *simulator) subPath(agent string) string {
	return filepath.Join(strings.TrimSuffix(d.transcript, ".jsonl"), "subagents", "agent-"+agent+".jsonl")
}

// msg writes one assistant message to the transcript (agent "" = lead).
func (d *simulator) msg(agent, model string, in, out int, blocks []map[string]any, usageExtra map[string]any) {
	d.n++
	for i, b := range blocks {
		if _, ok := b["id"]; !ok && (b["type"] == "tool_use" || b["type"] == "server_tool_use") {
			b["id"] = fmt.Sprintf("toolu_%d_%d", d.n, i)
		}
	}
	usage := map[string]any{"input_tokens": in, "output_tokens": out,
		"cache_read_input_tokens": in * 9, "cache_creation_input_tokens": in / 2}
	for k, v := range usageExtra {
		usage[k] = v
	}
	p := d.transcript
	if agent != "" {
		p = d.subPath(agent)
	}
	d.appendJSON(p, map[string]any{
		"type": "assistant", "timestamp": d.now().UTC().Format(time.RFC3339Nano),
		"isSidechain": agent != "",
		"message": map[string]any{"id": fmt.Sprintf("msg_%04d", d.n), "model": model,
			"role": "assistant", "content": blocks, "usage": usage},
	})
}

func toolBlock(name string) map[string]any { return map[string]any{"type": "tool_use", "name": name} }
func textBlock() map[string]any            { return map[string]any{"type": "text", "text": "…"} }

// advisor: the lead calls the advisor (opus) with the given in/out tokens.
func (d *simulator) advisor(in, out int) {
	d.msg("", leadModel, 900, 60,
		[]map[string]any{{"type": "server_tool_use", "name": "advisor", "input": map[string]any{}}, textBlock()},
		map[string]any{"iterations": []any{
			map[string]any{"type": "message", "input_tokens": 900, "output_tokens": 40},
			map[string]any{"type": "advisor_message", "model": "claude-opus-5-5", "input_tokens": in, "output_tokens": out},
		}})
}

func (d *simulator) leadTool(name string, input map[string]any) {
	d.msg("", leadModel, 1200, 180, []map[string]any{textBlock(), toolBlock(name)}, nil)
	d.ev("PreToolUse", map[string]any{"tool_name": name, "tool_input": input})
}

func (d *simulator) subTool(agent, model, name string, input map[string]any) {
	d.msg(agent, model, 2400, 90, []map[string]any{toolBlock(name)}, nil)
	d.ev("PreToolUse", map[string]any{"tool_name": name, "tool_input": input, "agent_id": agent})
	d.ev("PostToolUse", map[string]any{"tool_name": name, "agent_id": agent})
}

func (d *simulator) subStop(agent, typ string) {
	d.ev("SubagentStop", map[string]any{"agent_id": "agent-" + agent, "agent_type": typ,
		"agent_transcript_path": d.subPath(agent)})
}

// play runs the whole scenario: an earlier prompt with a slash command, a second prompt with
// the advisor + three haiku subagents, then the lead writes a migration and runs the tests.
func (d *simulator) play() {
	d.ev("SessionStart", map[string]any{"source": "startup", "model": leadModel})
	d.wait(1)
	d.previousPrompt()
	d.startTask()
	d.dispatchSubagents()
	d.implementAndTest()
	d.auditTail()
}

// previousPrompt: the earlier prompt, with a skill via slash command.
func (d *simulator) previousPrompt() {
	d.ev("UserPromptSubmit", map[string]any{"prompt": "/code-tour internal/leave"})
	d.wait(1)
	d.msg("", leadModel, 900, 300, []map[string]any{textBlock()}, nil)
	d.ev("Stop", nil)
	d.wait(1.5)
}

// startTask: the current prompt; the lead reads code, then asks the advisor.
func (d *simulator) startTask() {
	d.ev("UserPromptSubmit", map[string]any{"prompt": "add leave carry-over migration + 1 test, check all LeaveBalance call sites"})
	d.wait(2)
	d.leadTool("Read", map[string]any{"file_path": "/home/dika/app/internal/leave/service.go"})
	d.wait(1)
	d.ev("PostToolUse", map[string]any{"tool_name": "Read"})
	d.wait(1.5)
	d.advisor(42000, 820) // before drafting a plan
	d.wait(2)
}

// dispatchSubagents: the lead dispatches 3 haiku subagents working in parallel.
func (d *simulator) dispatchSubagents() {
	d.msg("", leadModel, 1500, 400, []map[string]any{toolBlock("Agent"), toolBlock("Agent"), toolBlock("Agent")}, nil)
	agents := []struct{ id, typ, desc string }{
		{"a1f0c2", "search", "petakan struktur repo"},
		{"b7e913", "grep", "find LeaveBalance call sites"},
		{"c94d5e", "audit", "audit migration in worktree"},
	}
	for _, a := range agents {
		d.ev("PreToolUse", map[string]any{"tool_name": "Agent", "tool_input": map[string]any{
			"subagent_type": a.typ, "description": a.desc, "model": "haiku"}})
	}
	d.wait(0.6)
	for _, a := range agents {
		d.ev("SubagentStart", map[string]any{"agent_id": "agent-" + a.id, "agent_type": a.typ})
		d.wait(0.3)
	}
	d.subTool("b7e913", haikuModel, "Grep", map[string]any{"pattern": "LeaveBalance"})
	d.subTool("a1f0c2", haikuModel, "Glob", map[string]any{"pattern": "**/*.go"})
	d.wait(1.2)
	d.subTool("c94d5e", haikuModel, "Bash", map[string]any{"command": "git worktree add ../audit-wt"})
	d.subTool("b7e913", haikuModel, "Grep", map[string]any{"pattern": "CarryOver\\("})
	d.wait(1.2)
	d.subTool("a1f0c2", haikuModel, "Read", map[string]any{"file_path": "/home/dika/app/go.mod"})
	d.subTool("b7e913", haikuModel, "Read", map[string]any{"file_path": "/home/dika/app/internal/leave/repo.go"})
	d.wait(1)
	d.msg("b7e913", haikuModel, 3000, 700, []map[string]any{textBlock()}, nil)
	d.subStop("b7e913", "grep")
	d.wait(1.5)
	d.subTool("a1f0c2", haikuModel, "Glob", map[string]any{"pattern": "migrations/*.sql"})
	d.subTool("c94d5e", haikuModel, "Read", map[string]any{"file_path": "/home/dika/app/migrations/0042_leave.sql"})
	d.wait(1.5)
}

// implementAndTest: the lead writes the migration, tests fail 2x → advisor, then done.
func (d *simulator) implementAndTest() {
	d.leadTool("Skill", map[string]any{"skill": "golang-testing"})
	d.ev("PostToolUse", map[string]any{"tool_name": "Skill"})
	d.subTool("c94d5e", haikuModel, "Skill", map[string]any{"skill": "database-migrations"})
	d.wait(0.5)
	d.leadTool("Write", map[string]any{"file_path": "/home/dika/app/migrations/0043_carry_over.sql"})
	d.wait(1)
	d.ev("PostToolUse", map[string]any{"tool_name": "Write"})
	d.leadTool("Bash", map[string]any{"command": "go test ./internal/leave/..."})
	d.wait(2)
	d.ev("PostToolUseFailure", map[string]any{"tool_name": "Bash", "error": "FAIL: TestCarryOver — expected 5, got 0"})
	d.wait(1)
	d.leadTool("Bash", map[string]any{"command": "go test ./internal/leave/... -run CarryOver"})
	d.wait(1.5)
	d.ev("PostToolUseFailure", map[string]any{"tool_name": "Bash", "error": "FAIL: TestCarryOver — expected 5, got 0"})
	d.wait(1)
	d.advisor(51000, 640) // same error 2x
	d.wait(1.5)
	d.msg("a1f0c2", haikuModel, 2800, 900, []map[string]any{textBlock()}, nil)
	d.subStop("a1f0c2", "search")
	d.leadTool("Edit", map[string]any{"file_path": "/home/dika/app/internal/leave/service.go"})
	d.wait(1)
	d.ev("PostToolUse", map[string]any{"tool_name": "Edit"})
	d.leadTool("Bash", map[string]any{"command": "go test ./internal/leave/..."})
	d.wait(2)
	d.ev("PostToolUse", map[string]any{"tool_name": "Bash"})
	d.subTool("c94d5e", haikuModel, "Bash", map[string]any{"command": "go vet ./..."})
	d.wait(1)
	d.advisor(38000, 300) // before finishing
	d.wait(1.5)
	d.msg("", leadModel, 1100, 650, []map[string]any{textBlock()}, nil)
	d.ev("Stop", nil)
}

// auditTail: the audit subagent keeps running for a while after the lead finishes.
func (d *simulator) auditTail() {
	for i := 0; i < 3; i++ {
		d.wait(2)
		d.subTool("c94d5e", haikuModel, "Read", map[string]any{"file_path": fmt.Sprintf("/home/dika/app/internal/leave/file%d.go", i)})
	}
}

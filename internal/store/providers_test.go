package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolate points every agent home at empty temp folders so tests never read real sessions.
func isolate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for env, sub := range map[string]string{
		"CLAUDE_CONFIG_DIR":    "claude",
		"CCWATCH_DIR":          "ccwatch",
		"CODEX_HOME":           "codex",
		"CCWATCH_GEMINI_DIR":   "gemini",
		"CCWATCH_OPENCODE_DIR": "opencode",
	} {
		t.Setenv(env, filepath.Join(root, sub))
	}
	return root
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func only(t *testing.T, st *Store, agent string) *Session {
	t.Helper()
	var got []*Session
	for _, s := range st.Sessions {
		if s.Agent == agent {
			got = append(got, s)
		}
	}
	if len(got) != 1 {
		t.Fatalf("want 1 %s session, got %d", agent, len(got))
	}
	return got[0]
}

func TestCodexRollout(t *testing.T) {
	root := isolate(t)
	id := "0199a213-81c0-7800-8aa1-bbab2a035a53"
	rollout := `{"timestamp":"2026-11-17T10:00:00.000Z","type":"session_meta","payload":{"id":"` + id + `","cwd":"/work/pos","cli_version":"0.60.0"}}
{"timestamp":"2026-11-17T10:00:01.000Z","type":"turn_context","payload":{"cwd":"/work/pos","model":"gpt-5-codex","approval_policy":"on-request"}}
{"timestamp":"2026-11-17T10:00:01.500Z","type":"event_msg","payload":{"type":"user_message","message":"<environment_context>\\n  <cwd>/work/pos</cwd>\\n</environment_context>"}}
{"timestamp":"2026-11-17T10:00:02.000Z","type":"event_msg","payload":{"type":"user_message","message":"fix the login bug","images":[]}}
{"timestamp":"2026-11-17T10:00:02.100Z","type":"event_msg","payload":{"type":"task_started","model_context_window":272000}}
{"timestamp":"2026-11-17T10:00:03.000Z","type":"response_item","payload":{"type":"function_call","name":"shell","arguments":"{\"command\":[\"bash\",\"-lc\",\"cat ~/.codex/skills/impeccable/SKILL.md\"]}","call_id":"call_1"}}
{"timestamp":"2026-11-17T10:00:03.500Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_1","output":"{\"output\":\"ok\",\"metadata\":{\"exit_code\":0}}"}}
{"timestamp":"2026-11-17T10:00:04.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":80,"total_tokens":1280},"last_token_usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":80,"total_tokens":1280}}}}
{"timestamp":"2026-11-17T10:00:04.100Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":80,"total_tokens":1280},"last_token_usage":{"input_tokens":1200,"cached_input_tokens":1000,"output_tokens":80,"total_tokens":1280}}}}
{"timestamp":"2026-11-17T10:00:05.000Z","type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","call_id":"call_2","input":"*** Begin Patch\n*** Update File: src/login.go\n@@\n-a\n+b\n*** End Patch"}}
{"timestamp":"2026-11-17T10:00:05.500Z","type":"response_item","payload":{"type":"function_call_output","call_id":"call_3","output":"{\"output\":\"boom\",\"metadata\":{\"exit_code\":2}}"}}
{"timestamp":"2026-11-17T10:00:06.000Z","type":"event_msg","payload":{"type":"task_complete","last_agent_message":"done"}}
`
	write(t, filepath.Join(root, "codex", "sessions", "2026", "11", "17", "rollout-2026-11-17T10-00-00-"+id+".jsonl"), rollout)
	childID := "0199a213-81c0-7800-8aa1-000000000c01"
	child := `{"timestamp":"2026-11-17T10:00:03.000Z","type":"session_meta","payload":{"id":"` + childID + `","parent_thread_id":"` + id + `","agent_role":"explorer","cwd":"/work/pos"}}
{"timestamp":"2026-11-17T10:00:03.100Z","type":"turn_context","payload":{"cwd":"/work/pos","model":"gpt-5-mini"}}
{"timestamp":"2026-11-17T10:00:03.200Z","type":"event_msg","payload":{"type":"turn_started"}}
{"timestamp":"2026-11-17T10:00:03.500Z","type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"rg login\"}","call_id":"k1"}}
{"timestamp":"2026-11-17T10:00:04.000Z","type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":50,"cached_input_tokens":0,"cache_write_input_tokens":7,"output_tokens":9,"total_tokens":59},"last_token_usage":{"input_tokens":50,"cached_input_tokens":0,"cache_write_input_tokens":7,"output_tokens":9,"total_tokens":59}}}}
{"timestamp":"2026-11-17T10:00:04.500Z","type":"event_msg","payload":{"type":"turn_complete"}}
`
	write(t, filepath.Join(root, "codex", "sessions", "2026", "11", "17", "rollout-2026-11-17T10-00-03-"+childID+".jsonl"), child)

	st := New(Options{})
	st.Refresh()
	s := only(t, st, AgentCodex)

	if s.ID != id || s.Cwd != "/work/pos" || s.Lead != "gpt-5-codex" || s.PermMode != "on-request" {
		t.Errorf("meta: id=%q cwd=%q lead=%q mode=%q", s.ID, s.Cwd, s.Lead, s.PermMode)
	}
	if s.Prompts != 1 || s.LastPrompt != "fix the login bug" || s.Turns != 1 || s.Status != "idle" {
		t.Errorf("turns: prompts=%d last=%q turns=%d status=%s", s.Prompts, s.LastPrompt, s.Turns, s.Status)
	}
	if s.MainMsgs != 1 || s.MainUsage.In != 200 || s.MainUsage.CacheR != 1000 || s.MainUsage.Out != 80 {
		t.Errorf("usage (repeated token_count must count once): msgs=%d %+v", s.MainMsgs, s.MainUsage)
	}
	if s.MainTools != 2 || s.Failures != 1 {
		t.Errorf("tools=%d failures=%d", s.MainTools, s.Failures)
	}
	if len(s.Skills) != 1 || s.Skills[0].Name != "impeccable" {
		t.Errorf("skills from SKILL.md read: %+v", s.Skills)
	}
	if len(s.Agents) != 1 {
		t.Fatalf("child rollout should be 1 subagent, got %d", len(s.Agents))
	}
	for _, a := range s.Agents {
		if a.Type != "explorer" || a.Model() != "gpt-5-mini" || a.Status != "done" || a.Tools != 1 || a.Usage.CacheW != 7 || a.Usage.Out != 9 {
			t.Errorf("codex subagent: %+v", a)
		}
	}
	var sawPatch bool
	for _, e := range s.Log {
		sawPatch = sawPatch || e.Text == "apply_patch: login.go"
	}
	if !sawPatch {
		t.Error("apply_patch summary missing from log")
	}
}

func TestGeminiChat(t *testing.T) {
	root := isolate(t)
	proj := filepath.Join(root, "gemini", "tmp", "pos")
	write(t, filepath.Join(proj, ".project_root"), "/work/pos\n")
	write(t, filepath.Join(proj, "chats", "session-2026-11-17T10-00-ab12cd34.json"), `{
  "sessionId": "ab12cd34-0000-4000-8000-000000000000",
  "projectHash": "deadbeef",
  "startTime": "2026-11-17T10:00:00.000Z",
  "lastUpdated": "2026-11-17T10:00:09.000Z",
  "messages": [
    {"id":"1","timestamp":"2026-11-17T10:00:01.000Z","type":"user","content":"explain main.go"},
    {"id":"2","timestamp":"2026-11-17T10:00:05.000Z","type":"gemini","content":"","model":"gemini-2.5-pro",
     "tokens":{"input":900,"output":40,"cached":600,"thoughts":10,"tool":0,"total":950},
     "toolCalls":[
       {"id":"t1","name":"read_file","args":{"absolute_path":"/work/pos/main.go"},"status":"success","timestamp":"2026-11-17T10:00:03.000Z"},
       {"id":"t2","name":"activate_skill","args":{"name":"code-tour"},"status":"success","timestamp":"2026-11-17T10:00:04.000Z"},
       {"id":"t3","name":"run_shell_command","args":{"command":"false"},"status":"error","timestamp":"2026-11-17T10:00:04.500Z"}
     ]},
    {"id":"3","timestamp":"2026-11-17T10:00:09.000Z","type":"gemini","content":[{"text":"It starts the server."}],"model":"gemini-2.5-flash",
     "tokens":{"input":100,"output":20,"cached":0,"total":120}}
  ]
}`)
	st := New(Options{})
	st.Refresh()
	s := only(t, st, AgentGemini)
	if s.Cwd != "/work/pos" || s.Lead != "gemini-2.5-flash" || s.Prompts != 1 {
		t.Errorf("cwd=%q lead=%q prompts=%d", s.Cwd, s.Lead, s.Prompts)
	}
	if s.MainMsgs != 2 || s.MainUsage.In != 400 || s.MainUsage.CacheR != 600 || s.MainUsage.Out != 70 {
		t.Errorf("usage: msgs=%d %+v", s.MainMsgs, s.MainUsage)
	}
	if s.MainTools != 3 || s.Failures != 1 || len(s.Skills) != 1 || s.Skills[0].Name != "code-tour" {
		t.Errorf("tools=%d failures=%d skills=%+v", s.MainTools, s.Failures, s.Skills)
	}
	if len(s.Stats) != 2 {
		t.Errorf("want 2 models in stats, got %d", len(s.Stats))
	}

	// Rescan without changes must not duplicate anything (sessions are rebuilt, not appended).
	st.lastProviderScan = time.Time{}
	st.Refresh()
	if s := only(t, st, AgentGemini); s.MainMsgs != 2 {
		t.Errorf("rescan duplicated messages: %d", s.MainMsgs)
	}
}

func TestOpenCodeStorage(t *testing.T) {
	root := isolate(t)
	sto := filepath.Join(root, "opencode", "storage")
	write(t, filepath.Join(sto, "session", "proj1", "ses_parent.json"),
		`{"id":"ses_parent","projectID":"proj1","directory":"/work/pos","title":"Fix auth","time":{"created":1763373600000,"updated":1763373660000}}`)
	write(t, filepath.Join(sto, "session", "proj1", "ses_child.json"),
		`{"id":"ses_child","projectID":"proj1","directory":"/work/pos","parentID":"ses_parent","title":"Find auth code","time":{"created":1763373610000,"updated":1763373620000}}`)
	write(t, filepath.Join(sto, "message", "ses_parent", "msg_001.json"),
		`{"id":"msg_001","sessionID":"ses_parent","role":"user","time":{"created":1763373600000}}`)
	write(t, filepath.Join(sto, "part", "msg_001", "prt_001.json"),
		`{"id":"prt_001","type":"text","text":"fix the auth flow"}`)
	write(t, filepath.Join(sto, "message", "ses_parent", "msg_002.json"),
		`{"id":"msg_002","sessionID":"ses_parent","role":"assistant","modelID":"claude-sonnet-5-5","providerID":"anthropic","mode":"build",
		  "time":{"created":1763373601000,"completed":1763373650000},"tokens":{"input":50,"output":30,"reasoning":5,"cache":{"read":700,"write":100}}}`)
	write(t, filepath.Join(sto, "part", "msg_002", "prt_002.json"),
		`{"id":"prt_002","type":"tool","tool":"bash","callID":"c1","state":{"status":"completed","input":{"command":"go test ./..."}}}`)
	write(t, filepath.Join(sto, "part", "msg_002", "prt_003.json"),
		`{"id":"prt_003","type":"tool","tool":"skill","callID":"c2","state":{"status":"completed","input":{"name":"golang-testing"}}}`)
	write(t, filepath.Join(sto, "message", "ses_child", "msg_100.json"),
		`{"id":"msg_100","sessionID":"ses_child","role":"assistant","modelID":"gpt-5-mini","agent":"explore",
		  "time":{"created":1763373611000,"completed":1763373619000},"tokens":{"input":10,"output":5,"reasoning":0,"cache":{"read":0,"write":0}}}`)

	st := New(Options{Window: 100000 * time.Hour})
	st.Refresh()
	s := only(t, st, AgentOpenCode)
	if s.ID != "ses_parent" || s.Cwd != "/work/pos" || s.Lead != "claude-sonnet-5-5" || s.LastPrompt != "fix the auth flow" {
		t.Errorf("id=%q cwd=%q lead=%q prompt=%q", s.ID, s.Cwd, s.Lead, s.LastPrompt)
	}
	if s.MainUsage.Out != 35 || s.MainUsage.CacheR != 700 || s.MainTools != 2 || len(s.Skills) != 1 {
		t.Errorf("usage=%+v tools=%d skills=%+v", s.MainUsage, s.MainTools, s.Skills)
	}
	if len(s.Agents) != 1 {
		t.Fatalf("child session should be 1 subagent, got %d", len(s.Agents))
	}
	for _, a := range s.Agents {
		if a.Type != "explore" || a.Model() != "gpt-5-mini" || a.Status != "done" || a.Desc != "Find auth code" {
			t.Errorf("subagent: %+v", a)
		}
	}
}

func TestGenericHookAgent(t *testing.T) {
	isolate(t)
	st := New(Options{})
	lines := []string{
		`{"session_id":"s1","ccwatch_agent":"aider","hook_event_name":"SessionStart","cwd":"/work/pos","model":"gpt-5","ccwatch_ts":"2026-11-17T10:00:00Z"}`,
		`{"session_id":"s1","ccwatch_agent":"aider","hook_event_name":"UserPromptSubmit","prompt":"add tests","ccwatch_ts":"2026-11-17T10:00:01Z"}`,
		`{"session_id":"s1","ccwatch_agent":"aider","hook_event_name":"BeforeTool","tool_name":"shell","tool_input":{"command":"go test"},"ccwatch_ts":"2026-11-17T10:00:02Z"}`,
		`{"session_id":"s1","ccwatch_agent":"aider","hook_event_name":"Message","model":"gpt-5","input_tokens":100,"output_tokens":20,"ccwatch_ts":"2026-11-17T10:00:03Z"}`,
		`{"session_id":"s1","ccwatch_agent":"aider","hook_event_name":"AfterAgent","ccwatch_ts":"2026-11-17T10:00:04Z"}`,
		// same id from Claude Code must stay a separate session
		`{"session_id":"s1","hook_event_name":"SessionStart","cwd":"/work/pos","ccwatch_ts":"2026-11-17T10:00:05Z"}`,
	}
	for _, l := range lines {
		st.ingestEvent([]byte(l))
	}
	if len(st.Sessions) != 2 {
		t.Fatalf("want aider + claude sessions, got %d", len(st.Sessions))
	}
	s := only(t, st, "aider")
	if s.Lead != "gpt-5" || s.Prompts != 1 || s.Turns != 1 || s.MainMsgs != 1 || s.MainUsage.Out != 20 || !strings.HasPrefix(s.Log[len(s.Log)-1].Text, "turn #1") {
		t.Errorf("lead=%q prompts=%d turns=%d msgs=%d usage=%+v", s.Lead, s.Prompts, s.Turns, s.MainMsgs, s.MainUsage)
	}
	if s.IsClaude() || !only(t, st, AgentClaude).IsClaude() {
		t.Error("IsClaude mismatch")
	}
	if got := (Options{Agents: []string{"claude"}}); got.wantsAgent("aider") || !got.wantsAgent("") {
		t.Error("agent filter")
	}
}

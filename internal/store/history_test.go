package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestHistoryReport(t *testing.T) {
	root := isolate(t)
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	ts := func(ago time.Duration) string { return now.Add(-ago).Format(time.RFC3339Nano) }

	// Claude: the same message id on two lines counts once; advisor iterations count under their
	// own model; a message from 40 days ago is outside every period.
	claudeLine := func(ago time.Duration, id, model, extra string) string {
		return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"message":{"id":%q,"model":%q,"content":[],"usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":100,"cache_creation_input_tokens":5%s}}}`,
			ts(ago), id, model, extra) + "\n"
	}
	write(t, filepath.Join(root, "claude", "projects", "p", "s1.jsonl"),
		claudeLine(30*time.Minute, "m1", "claude-opus-5-5", "")+
			claudeLine(30*time.Minute, "m1", "claude-opus-5-5", "")+ // second block of the same message
			claudeLine(3*time.Hour, "m2", "claude-sonnet-5-5", `,"iterations":[{"type":"advisor_message","model":"claude-opus-5-5","input_tokens":1000,"output_tokens":50}]`)+
			claudeLine(40*24*time.Hour, "m3", "claude-opus-5-5", "")+
			`{"type":"user","timestamp":"`+ts(time.Minute)+`","message":{"role":"user","content":"hi"}}`+"\n")
	write(t, filepath.Join(root, "claude", "projects", "p", "s1", "subagents", "agent-a1.jsonl"),
		claudeLine(2*24*time.Hour, "sub1", "claude-haiku-5-5", ""))

	// Codex: a repeated token_count is counted once.
	tc := func(ago time.Duration, total int) string {
		return fmt.Sprintf(`{"timestamp":%q,"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"total_tokens":%d},"last_token_usage":{"input_tokens":300,"cached_input_tokens":200,"output_tokens":40}}}}`, ts(ago), total) + "\n"
	}
	write(t, filepath.Join(root, "codex", "sessions", "2026", "11", "18", "rollout-x-0199a213-81c0-7800-8aa1-bbab2a035a53.jsonl"),
		`{"timestamp":"`+ts(10*24*time.Hour)+`","type":"turn_context","payload":{"model":"gpt-5-codex"}}`+"\n"+tc(10*24*time.Hour, 340)+tc(10*24*time.Hour, 340))

	// omp: an assistant message and a side call (model_usage).
	write(t, filepath.Join(root, "omp", "agent", "sessions", "-work", "2026_s.jsonl"),
		`{"type":"message","timestamp":"`+ts(20*time.Minute)+`","message":{"role":"assistant","model":"claude-only","usage":{"input":7,"output":3,"cacheRead":0,"cacheWrite":0}}}`+"\n"+
			`{"type":"model_usage","timestamp":"`+ts(20*time.Minute)+`","purpose":"title","model":"p/claude-haiku-5-5","usage":{"input":1,"output":1,"cacheRead":0,"cacheWrite":0}}`+"\n")

	h := NewHistory()
	h.Scan(now)
	rep := h.Report(now, nil)

	agent := func(name string) AgentUsage {
		for _, a := range rep.Agents {
			if a.Agent == name {
				return a
			}
		}
		t.Fatalf("agent %s missing from report", name)
		return AgentUsage{}
	}
	model := func(a AgentUsage, name string) ModelUsage {
		for _, m := range a.Models {
			if m.Model == name {
				return m
			}
		}
		t.Fatalf("model %s missing for %s", name, a.Agent)
		return ModelUsage{}
	}

	c := agent(AgentClaude)
	// 1h: m1 only · 24h: + m2 + advisor · 7d: + subagent · 30d: same (m3 is 40 days old)
	wantMsgs := []int{1, 3, 4, 4}
	for i, w := range wantMsgs {
		if got := c.Periods[i].Msgs; got != w {
			t.Errorf("claude %s msgs = %d, want %d", UsagePeriods[i].Label, got, w)
		}
	}
	if opus := model(c, "claude-opus-5-5"); opus.Periods[1].Msgs != 2 || opus.Periods[1].In != 1010 {
		t.Errorf("opus 24h should hold m1 + the advisor call: %+v", opus.Periods[1])
	}
	if got := c.Periods[0].Tokens(); got != 135 {
		t.Errorf("claude last-hour tokens = %d, want 135 (10+20+100+5)", got)
	}

	cx := agent(AgentCodex)
	if p := cx.Periods[3]; p.Msgs != 1 || p.In != 100 || p.CacheR != 200 || p.Out != 40 {
		t.Errorf("codex 30d (repeated token_count counted once, cached split out): %+v", p)
	}
	if cx.Periods[2].Msgs != 0 {
		t.Error("a 10-day-old codex message must not be in the last 7 days")
	}

	o := agent(AgentOmp)
	if o.Periods[0].Msgs != 2 || model(o, "claude-haiku-5-5").Periods[0].Msgs != 1 {
		t.Errorf("omp should count the message and the side call: %+v", o.Periods[0])
	}

	if rep.Total[0].Msgs != 3 || rep.Total[3].Msgs != 7 {
		t.Errorf("totals: 1h=%d 30d=%d, want 3 and 7", rep.Total[0].Msgs, rep.Total[3].Msgs)
	}
	if len(rep.Agents) != 3 || rep.Agents[0].Agent != AgentClaude {
		t.Errorf("agents should be sorted busiest first: %+v", rep.Agents)
	}

	// filtering and incremental rescans
	if only := h.Report(now, []string{"codex"}); len(only.Agents) != 1 || only.Total[3].Msgs != 1 {
		t.Errorf("--agent filter: %+v", only)
	}
	h.Scan(now)
	if again := h.Report(now, nil); again.Total[3].Msgs != 7 {
		t.Errorf("a rescan without new lines must not double count: %d", again.Total[3].Msgs)
	}
}

package store

import (
	"os"
	"regexp"
	"strconv"
	"time"

	"ccwatch/internal/format"
)

// Polled providers: agents other than Claude Code (Codex, Gemini CLI, OpenCode) are read
// straight from the session files they write, so they need no hooks or install step.
// Codex writes append-only JSONL (tailed incrementally); Gemini and OpenCode rewrite JSON
// files, so their sessions are rebuilt from scratch whenever a file changes.

const (
	AgentCodex    = "codex"
	AgentGemini   = "gemini"
	AgentOpenCode = "opencode"

	providerScanEvery = 1500 * time.Millisecond
	// staleWorking: a polled session still marked "working" whose files haven't changed this long
	// is shown as idle (the CLI was closed or crashed mid-turn).
	staleWorking = 5 * time.Minute
)

// scanProviders discovers and refreshes sessions from the polled agents.
func (st *Store) scanProviders(now time.Time) {
	if now.Sub(st.lastProviderScan) >= providerScanEvery {
		st.lastProviderScan = now
		since := now.Add(-st.opts.window())
		if st.opts.polls(AgentCodex) {
			st.discoverCodex(since)
		}
		if st.opts.polls(AgentGemini) {
			st.scanGemini(since)
		}
		if st.opts.polls(AgentOpenCode) {
			st.scanOpenCode(since)
		}
	}
	// Codex rollouts are append-only: tail them on every refresh for a live view.
	for _, s := range st.Sessions {
		if s.Agent != AgentCodex {
			continue
		}
		if s.Transcript != "" {
			s.tailWith(s.Transcript, s.ingestCodex)
			s.settleStale(s.Transcript, now)
		}
		for _, a := range s.Agents {
			if a.Transcript != "" {
				s.tailWith(a.Transcript, func(l []byte) { s.ingestCodexAgent(a, l) })
				if a.Status == "running" {
					if fi, err := os.Stat(a.Transcript); err == nil && now.Sub(fi.ModTime()) > staleWorking {
						a.Status = "done"
					}
				}
			}
		}
	}
}

// tailWith reads new lines of path with a custom line handler.
func (s *Session) tailWith(path string, fn func([]byte)) {
	t := s.tailers[path]
	if t == nil {
		t = &tailer{path: path}
		s.tailers[path] = t
	}
	t.read(fn)
}

// settleStale: a session stuck in "working" with a quiet file becomes idle.
func (s *Session) settleStale(path string, now time.Time) {
	if s.Status != "working" {
		return
	}
	if fi, err := os.Stat(path); err == nil && now.Sub(fi.ModTime()) > staleWorking {
		s.Status, s.CurrentTool = "idle", ""
	}
}

// beginPrompt records a prompt typed by the user.
func (s *Session) beginPrompt(t time.Time, text string) {
	s.Prompts++
	s.Status = "working"
	s.LastPrompt = format.OneLine(text)
	s.PromptTimes = append(s.PromptTimes, t)
	if name, args, ok := slashSkill(text); ok {
		s.addSkill(SkillUse{T: t, Name: name, Args: args, Source: "slash", Who: "user", Model: s.Lead})
	}
	s.log(t, "user", "", "prompt #"+strconv.Itoa(s.Prompts)+": "+s.LastPrompt, false)
}

// leadTool records a tool call by the lead. skill, if set, is also recorded as a skill use
// (a skill tool call, or reading a SKILL.md file, which is how Codex loads skills).
func (s *Session) leadTool(t time.Time, id, tool, summary, skill string) {
	s.MainTools++
	s.CurrentTool = summary
	s.lastTool = tool
	s.Status = "working"
	if skill != "" {
		s.addSkill(SkillUse{T: t, Name: skill, Source: "tool", Who: "lead", Model: s.Lead, ID: id})
	}
	s.log(t, "lead", s.Lead, summary, false)
}

// toolFailed records a failed tool call.
func (s *Session) toolFailed(t time.Time, who, mdl, msg string) {
	s.Failures++
	s.log(t, who, mdl, msg, true)
}

// endTurn: the agent finished answering.
func (s *Session) endTurn(t time.Time) {
	s.Turns++
	s.Status = "idle"
	s.CurrentTool = ""
	s.log(t, "lead", s.Lead, "turn #"+strconv.Itoa(s.Turns)+" finished", false)
}

// leadReply records one model response with its token usage.
func (s *Session) leadReply(t time.Time, model string, u Usage) {
	if model == "" {
		model = s.Lead
	}
	if model == "" {
		model = s.Agent
	}
	s.MainMsgs++
	s.MainUsage.add(u)
	s.recordUsage(model, "lead", t, u)
	s.Points = append(s.Points, point{T: t, Role: "lead", Model: model})
}

// setLead changes the lead model, logging a switch.
func (s *Session) setLead(t time.Time, m string) {
	if m == "" || m == s.Lead {
		return
	}
	old := s.Lead
	s.Lead = m
	if old != "" {
		s.log(t, "sys", m, "lead model changed: "+format.PrettyModel(old)+" → "+format.PrettyModel(m), false)
	}
}

var skillPathRe = regexp.MustCompile(`([A-Za-z0-9_.:-]+)/SKILL\.md`)

// skillFromText: the skill name if text reads a SKILL.md file (".../impeccable/SKILL.md" → "impeccable").
func skillFromText(text string) string {
	if m := skillPathRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	return ""
}

// msTime: a Unix timestamp in milliseconds (OpenCode) as time.Time.
func msTime(ms float64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms))
}

// fileSig: a cheap change signature for a set of files (count + newest mtime + total size).
type fileSig struct {
	n    int
	mod  time.Time
	size int64
}

func (f *fileSig) add(fi os.FileInfo) {
	f.n++
	f.size += fi.Size()
	if fi.ModTime().After(f.mod) {
		f.mod = fi.ModTime()
	}
}

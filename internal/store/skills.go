package store

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"ccwatch/internal/format"
)

// SkillUse: one skill use, either called by Claude via the Skill tool
// or typed by the user as a slash command (/name).
type SkillUse struct {
	T       time.Time
	Name    string
	Sub     string // subcommand: first word of args (e.g. "layout" in /impeccable layout …)
	Args    string // remaining args after the subcommand
	Source  string // tool | slash
	AgentID string // "" = lead
	Who     string // lead | subagent type | user
	Model   string
	ID      string // tool_use_id (to dedupe hook vs transcript)
}

// fill: fill in missing data from the other source (old hooks without args, etc.).
func (c *SkillUse) fill(u SkillUse) {
	if c.Model == "" {
		c.Model = u.Model
	}
	if c.Sub == "" && c.Args == "" {
		c.Sub, c.Args = u.Sub, u.Args
	}
}

// Claude Code built-in slash commands: not skills, don't record them.
var builtinSlash = map[string]bool{
	"clear": true, "compact": true, "model": true, "help": true, "config": true, "cost": true,
	"exit": true, "quit": true, "resume": true, "login": true, "logout": true, "mcp": true,
	"permissions": true, "hooks": true, "agents": true, "memory": true, "status": true,
	"doctor": true, "reload-skills": true, "reload-plugins": true, "plugin": true, "plugins": true,
	"context": true, "usage": true, "add-dir": true, "vim": true, "theme": true, "terminal-setup": true,
	"bug": true, "rewind": true, "export": true, "ide": true, "fast": true, "effort": true,
}

// slashSkill: "/name args" → name, args. ok=false if it isn't a skill slash command.
func slashSkill(prompt string) (name, args string, ok bool) {
	p := strings.TrimSpace(prompt)
	if !strings.HasPrefix(p, "/") || len(p) < 2 {
		return "", "", false
	}
	name, args, _ = strings.Cut(p[1:], " ")
	name = strings.TrimPrefix(strings.TrimSpace(name), "skill:") // omp: /skill:<name>
	if name == "" || strings.ContainsAny(name, "/\\") || builtinSlash[name] {
		return "", "", false
	}
	return name, strings.TrimSpace(args), true
}

// systemPrompt: if the "prompt" is actually an automatic message from Claude Code
// (not typed by the user), return its kind; otherwise "".
func systemPrompt(p string) string {
	p = strings.TrimSpace(p)
	for _, tag := range []string{"task-notification", "agent-message", "bash-notification", "local-command-stdout", "system-reminder"} {
		if strings.HasPrefix(p, "<"+tag+">") || strings.HasPrefix(p, "<"+tag+" ") {
			return tag
		}
	}
	return ""
}

var subRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,31}$`)

// splitSub: "layout page http://…" → ("layout", "page http://…").
// The first word counts as a subcommand if it's a plain word (not a path/url/flag).
func splitSub(args string) (sub, rest string) {
	args = strings.TrimSpace(args)
	first, rest, _ := strings.Cut(args, " ")
	if subRe.MatchString(first) {
		return first, strings.TrimSpace(rest)
	}
	return "", args
}

var (
	cmdNameRe = regexp.MustCompile(`<command-name>\s*/?([^<\s]+)\s*</command-name>`)
	cmdArgsRe = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
)

// transcriptSlash: a slash command from a user message in the transcript
// (<command-name>/impeccable</command-name><command-args>layout …</command-args>).
func transcriptSlash(raw []byte) (name, args string, ok bool) {
	if len(raw) == 0 || !strings.Contains(string(raw), "command-name") {
		return "", "", false
	}
	var txt string
	if json.Unmarshal(raw, &txt) != nil {
		var blocks []map[string]any
		_ = json.Unmarshal(raw, &blocks)
		for _, b := range blocks {
			if t, _ := b["text"].(string); strings.Contains(t, "command-name") {
				txt = t
				break
			}
		}
	}
	mm := cmdNameRe.FindStringSubmatch(txt)
	if mm == nil {
		return "", "", false
	}
	if a := cmdArgsRe.FindStringSubmatch(txt); a != nil {
		args = strings.TrimSpace(a[1])
	}
	return slashSkill("/" + mm[1] + " " + args)
}

// dedupeWindow: two records of the same skill from different paths (hook vs transcript) within this window are merged.
const dedupeWindow = 5 * time.Second

// addSkill: record a skill use, deduping between hook and transcript.
func (s *Session) addSkill(u SkillUse) {
	if u.Name == "" {
		return
	}
	if u.Sub == "" && u.Args != "" {
		u.Sub, u.Args = splitSub(u.Args)
	}
	u.Args = format.OneLine(u.Args)
	for i := range s.Skills {
		c := &s.Skills[i]
		if u.ID != "" && c.ID == u.ID {
			c.fill(u)
			return
		}
		// same source from another path (hook without id / transcript) within a short window
		if c.Name == u.Name && c.AgentID == u.AgentID && c.Source == u.Source &&
			(c.ID == "" || u.ID == "") && absDur(c.T.Sub(u.T)) < dedupeWindow {
			if c.ID == "" {
				c.ID = u.ID
			}
			c.fill(u)
			return
		}
	}
	s.Skills = append(s.Skills, u)
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// promptOf: the (1-based) prompt number running at time t; 0 if none yet.
func (s *Session) promptOf(t time.Time) int {
	n := 0
	for _, pt := range s.PromptTimes {
		if !pt.After(t) {
			n++
		}
	}
	return n
}

// CurrentSkills: skills used since the last prompt (the current task).
func (s *Session) CurrentSkills() []SkillUse {
	var start time.Time
	if n := len(s.PromptTimes); n > 0 {
		start = s.PromptTimes[n-1]
	}
	var out []SkillUse
	for _, u := range s.Skills {
		if !u.T.Before(start) {
			out = append(out, u)
		}
	}
	return out
}

// SkillActive: a skill is still "running" while its caller is still working in this prompt.
func (s *Session) SkillActive(u SkillUse) bool {
	if s.Status == "ended" {
		return false
	}
	if u.AgentID != "" {
		a, ok := s.Agents[u.AgentID]
		return ok && a.Status == "running"
	}
	return s.Status == "working"
}

// ContextSkill: skills from earlier prompts, grouped by name.
type ContextSkill struct {
	Name, Model string
	Who         string   // lead or subagent type
	Subs        []string // unique subcommands, in order of first use
	SubCount    map[string]int
	Count       int
	Last        time.Time
	LastPrompt  int
	FromCompact bool // known only from invoked_skills (preserved after a compact)
}

// SubsLabel: "layout ×2, extract".
func (c ContextSkill) SubsLabel() string {
	var ps []string
	for _, sb := range c.Subs {
		if n := c.SubCount[sb]; n > 1 {
			ps = append(ps, sb+" ×"+strconv.Itoa(n))
		} else {
			ps = append(ps, sb)
		}
	}
	return strings.Join(ps, ", ")
}

// ContextSkills: skills from earlier prompts.
//   - lead: skills the lead invoked stay in the lead's context for the whole session
//     (Claude Code re-attaches them via invoked_skills after a compact).
//   - sub: skills a subagent invoked only live in that subagent's context → history.
func (s *Session) ContextSkills() (lead, sub []ContextSkill) {
	cur := len(s.PromptTimes)
	if cur == 0 {
		// no prompt boundary: everything is already shown as "current"
		return nil, nil
	}
	curNames := map[string]bool{}
	for _, u := range s.CurrentSkills() {
		if u.AgentID == "" {
			curNames[u.Name] = true
		}
	}
	li, si := map[string]int{}, map[string]int{}
	for _, u := range s.Skills {
		p := s.promptOf(u.T)
		if p >= cur {
			continue // current prompt, already shown above
		}
		if u.AgentID == "" {
			if curNames[u.Name] {
				continue // used again in this prompt
			}
			addContextSkill(&lead, li, u.Name, u, p)
		} else {
			addContextSkill(&sub, si, u.Who+"\x00"+u.Name, u, p)
		}
	}
	// skills known only from invoked_skills (e.g. invoked before hooks/tailing started)
	for name := range s.InvokedSkills {
		if _, ok := li[name]; ok || curNames[name] {
			continue
		}
		li[name] = len(lead)
		lead = append(lead, ContextSkill{Name: name, Who: "lead", Model: s.Lead, FromCompact: true})
	}
	byLast := func(x []ContextSkill) {
		sort.SliceStable(x, func(i, j int) bool { return x[i].Last.After(x[j].Last) })
	}
	byLast(lead)
	byLast(sub)
	return lead, sub
}

// addContextSkill adds use u (in prompt p) to group key in list.
func addContextSkill(list *[]ContextSkill, idx map[string]int, key string, u SkillUse, p int) {
	i, ok := idx[key]
	if !ok {
		idx[key] = len(*list)
		*list = append(*list, ContextSkill{Name: u.Name, Who: u.Who, SubCount: map[string]int{}})
		i = len(*list) - 1
	}
	c := &(*list)[i]
	c.Count++
	if u.Sub != "" {
		if c.SubCount[u.Sub] == 0 {
			c.Subs = append(c.Subs, u.Sub)
		}
		c.SubCount[u.Sub]++
	}
	if !u.T.Before(c.Last) {
		c.Last, c.LastPrompt, c.Model = u.T, p, u.Model
	}
}

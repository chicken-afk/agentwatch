package store

import (
	"strings"
	"time"
)

// Options controls which sessions are tracked and how the advisor is labeled.
type Options struct {
	Session string
	Advisor string // global advisor label (all sessions)
	// AdvisorBySession: advisor label per session id (key = id prefix).
	AdvisorBySession map[string]string
	Recent           time.Duration
	// Dir: if set, only show sessions whose cwd is this folder (or a subfolder of it).
	Dir string
	// Agents: only show sessions from these agents (claude, codex, gemini, opencode, …); empty = all.
	Agents []string
	// Window: how far back the polled agents (Codex, Gemini, OpenCode) are scanned. 0 = DefaultWindow.
	Window time.Duration
}

// DefaultWindow: polled agents have no hooks, so only sessions active within this window are loaded.
const DefaultWindow = 24 * time.Hour

func (o Options) window() time.Duration {
	if o.Window > 0 {
		return o.Window
	}
	return DefaultWindow
}

// wantsAgent: whether sessions from agent a pass the --agent filter.
func (o Options) wantsAgent(a string) bool {
	if len(o.Agents) == 0 {
		return true
	}
	if a == "" {
		a = AgentClaude
	}
	for _, x := range o.Agents {
		if strings.EqualFold(x, a) {
			return true
		}
	}
	return false
}

// polls: whether provider p should be scanned at all.
func (o Options) polls(p string) bool { return o.wantsAgent(p) }

// advisorFor: the manual advisor label for session id, "" if none.
// A per-session label (longest matching prefix) wins over the global label.
func (o Options) advisorFor(id string) string {
	best, bestLen := "", -1
	for p, mdl := range o.AdvisorBySession {
		if strings.HasPrefix(id, p) && len(p) > bestLen {
			best, bestLen = mdl, len(p)
		}
	}
	if bestLen >= 0 {
		return best
	}
	return o.Advisor
}

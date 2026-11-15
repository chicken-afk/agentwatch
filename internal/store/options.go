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
}

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

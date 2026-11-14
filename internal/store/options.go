package store

import (
	"strings"
	"time"
)

// Options mengatur sesi mana yang dilacak dan bagaimana advisor dilabeli.
type Options struct {
	Session string
	Advisor string // label advisor global (semua sesi)
	// AdvisorBySession: label advisor per session id (key = prefix id).
	AdvisorBySession map[string]string
	Recent           time.Duration
	// Dir: kalau di-set, hanya tampilkan sesi yang cwd-nya folder ini (atau subfolder-nya).
	Dir string
}

// advisorFor: label advisor manual untuk sesi id, "" kalau tidak ada.
// Label per sesi (prefix terpanjang yang cocok) menang atas label global.
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

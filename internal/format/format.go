// Package format holds text, duration, and number formatting helpers for display.
package format

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Clip truncates s to at most n runes, ending with "…" if cut.
func Clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// OneLine collapses whitespace into a single line (max 200 runes).
func OneLine(s string) string {
	return Clip(strings.Join(strings.Fields(s), " "), 200)
}

// Short: the first 8 characters of a session id.
func Short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func Dur(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	default:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	}
}

// Human: 1234 → "1.2k", 2_500_000 → "2.5M".
func Human(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%dk", n/1000)
	case n >= 1000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	default:
		return strconv.FormatInt(n, 10)
	}
}

func Clock(t time.Time) string {
	if t.IsZero() {
		return "--:--:--"
	}
	return t.Local().Format("15:04:05")
}

// BaseDir: "~/<folder name>" for compact display.
func BaseDir(p string) string {
	if p == "" {
		return "?"
	}
	return "~/" + filepath.Base(p)
}

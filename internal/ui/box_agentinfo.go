package ui

import (
	"path/filepath"

	"github.com/chicken-afk/agentwatch/internal/format"
	"github.com/chicken-afk/agentwatch/internal/store"
)

// agentInfoBox: for non-Claude sessions, which agent this is and where agentwatch reads it from.
func (m *model) agentInfoBox(s *store.Session, w, h int) string {
	src := "hooks (agentwatch hook --agent " + s.Agent + ")"
	if s.Transcript != "" {
		src = filepath.Base(s.Transcript)
	}
	lines := []string{
		kv("agent", agentBadge(s.Agent)),
		kv("model", mcb(s.Lead, format.PrettyModel(s.Lead))),
		kv("folder", sText.Render(format.BaseDir(s.Cwd))),
		kv("source", sDim.Render(src)),
		"",
		sDim.Render("advisor: Claude Code only"),
	}
	if !s.Start.IsZero() {
		lines = append(lines[:4], append([]string{kv("started", sText.Render(format.Clock(s.Start)))}, lines[4:]...)...)
	}
	title := sBold.Render("AGENT") + sDim.Render(" · ") + agentBadge(s.Agent)
	return box(title, agentColor(s.Agent), w, lines, max(0, h-2))
}

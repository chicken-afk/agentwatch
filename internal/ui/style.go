// Package ui: terminal dashboard (Bubble Tea) for AI coding agent sessions (Claude Code, Codex, Gemini CLI, Antigravity CLI, OpenCode, omp, …).
package ui

import (
	"github.com/chicken-afk/agentwatch/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// Short aliases for package theme so the render code stays compact.
var (
	sDim   = theme.Dim
	sText  = theme.Text
	sBold  = theme.Bold
	sGreen = theme.Green
	sRed   = theme.Red
	sYel   = theme.Yellow
)

var (
	cGreen  = theme.ColorGreen
	cText   = theme.ColorText
	cBorder = theme.ColorBorder
)

func modelColor(m string) lipgloss.Color { return theme.ModelColor(m) }

// mc / mcb: color text with the model family's color (mcb = bold).
func mc(m, text string) string  { return theme.Model(m, text) }
func mcb(m, text string) string { return theme.ModelBold(m, text) }

func agentColor(a string) lipgloss.Color { return theme.AgentColor(a) }

// agentBadge: the agent name in its color ("claude" when unset).
func agentBadge(a string) string {
	if a == "" {
		a = "claude"
	}
	return lipgloss.NewStyle().Foreground(agentColor(a)).Bold(true).Render(a)
}

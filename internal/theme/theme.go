// Package theme: color palette and lipgloss styles used by the dashboard and the status line.
package theme

import (
	"ccwatch/internal/format"

	"github.com/charmbracelet/lipgloss"
)

var (
	Opus   = lipgloss.Color("#B48EF0")
	Sonnet = lipgloss.Color("#E8875B")
	Haiku  = lipgloss.Color("#6CA0F5")
	Fable  = lipgloss.Color("#5FC9A8")
	Mythos = lipgloss.Color("#E06C75")
	GPT    = lipgloss.Color("#56B6C2")
	Gemini = lipgloss.Color("#F28FAD")
	Other  = lipgloss.Color("#A8A8A8")

	ColorGreen  = lipgloss.Color("#6CC48A")
	ColorRed    = lipgloss.Color("#E06C75")
	ColorYellow = lipgloss.Color("#E5C07B")
	ColorDim    = lipgloss.Color("#6B7280")
	ColorText   = lipgloss.Color("#D4D4D8")
	ColorBorder = lipgloss.Color("#3F3F46")

	Dim    = lipgloss.NewStyle().Foreground(ColorDim)
	Text   = lipgloss.NewStyle().Foreground(ColorText)
	Bold   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FAFAFA")).Bold(true)
	Green  = lipgloss.NewStyle().Foreground(ColorGreen)
	Red    = lipgloss.NewStyle().Foreground(ColorRed)
	Yellow = lipgloss.NewStyle().Foreground(ColorYellow)
)

// ModelColor: color per model family (opus purple, sonnet orange, haiku blue, fable green).
func ModelColor(m string) lipgloss.Color {
	switch format.ModelFamily(m) {
	case "opus":
		return Opus
	case "sonnet":
		return Sonnet
	case "haiku":
		return Haiku
	case "fable":
		return Fable
	case "mythos":
		return Mythos
	case "gpt":
		return GPT
	case "gemini":
		return Gemini
	}
	return Other
}

// AgentColor: color per agent (claude orange like sonnet, codex like gpt, gemini pink, others gray).
func AgentColor(agent string) lipgloss.Color {
	switch agent {
	case "", "claude":
		return Sonnet
	case "codex":
		return GPT
	case "gemini":
		return Gemini
	case "opencode":
		return Haiku
	case "omp":
		return Opus
	}
	return Other
}

// Model colors text with the color of model m's family.
func Model(m, text string) string {
	return lipgloss.NewStyle().Foreground(ModelColor(m)).Render(text)
}

// ModelBold is like Model, but bold.
func ModelBold(m, text string) string {
	return lipgloss.NewStyle().Foreground(ModelColor(m)).Bold(true).Render(text)
}

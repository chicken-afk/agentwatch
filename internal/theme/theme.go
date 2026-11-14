// Package theme: palet warna dan style lipgloss yang dipakai dashboard dan status line.
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

// ModelColor: warna per keluarga model (opus ungu, sonnet oranye, haiku biru, fable hijau).
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
	}
	return Other
}

// Model mewarnai text dengan warna keluarga model m.
func Model(m, text string) string {
	return lipgloss.NewStyle().Foreground(ModelColor(m)).Render(text)
}

// ModelBold sama seperti Model, tebal.
func ModelBold(m, text string) string {
	return lipgloss.NewStyle().Foreground(ModelColor(m)).Bold(true).Render(text)
}

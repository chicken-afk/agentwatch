// Package ui: terminal dashboard (Bubble Tea) for Claude Code sessions.
package ui

import (
	"ccwatch/internal/theme"

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

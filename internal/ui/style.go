// Package ui: dashboard terminal (Bubble Tea) untuk sesi Claude Code.
package ui

import (
	"ccwatch/internal/theme"

	"github.com/charmbracelet/lipgloss"
)

// Singkatan dari package theme supaya kode render tetap ringkas.
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

// mc / mcb: warnai teks dengan warna keluarga model (mcb = tebal).
func mc(m, text string) string  { return theme.Model(m, text) }
func mcb(m, text string) string { return theme.ModelBold(m, text) }

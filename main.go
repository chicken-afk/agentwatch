// ccwatch — live dashboard untuk sesi Claude Code.
//
// Subcommand:
//
//	ccwatch                 buka dashboard (ikuti sesi paling aktif)
//	ccwatch watch [flags]   sama seperti di atas
//	ccwatch hook            dipanggil oleh hooks Claude Code (baca JSON dari stdin)
//	ccwatch statusline      status line ringkas untuk Claude Code
//	ccwatch install         pasang hooks ke ~/.claude/settings.json + aturan skill ke ~/.claude/CLAUDE.md
//	ccwatch uninstall       lepas hooks dan aturan skill ccwatch
//	ccwatch rules           pasang/perbarui aturan skill saja (--remove, --show)
//	ccwatch demo            jalankan dashboard dengan sesi palsu (untuk coba tampilan)
//
// Logika ada di paket internal/*; file ini hanya titik masuk.
package main

import (
	"os"

	"ccwatch/internal/cli"
)

func main() { os.Exit(cli.Run(os.Args[1:])) }

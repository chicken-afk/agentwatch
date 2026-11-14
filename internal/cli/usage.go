package cli

const usage = `ccwatch — live dashboard Claude Code

  ccwatch                  buka dashboard (hanya sesi di folder saat ini + subfolder)
  ccwatch --all            sesi dari semua folder
  ccwatch --dir ~/Works/personal/pos   hanya sesi di folder tertentu
  ccwatch watch --session <id> --advisor opus --recent 30m
  ccwatch --advisor opus                  label advisor untuk semua sesi
  ccwatch --advisor 3f2a=opus             label advisor hanya untuk sesi 3f2a… (prefix boleh)
  ccwatch --advisor 3f2a=opus --advisor 9c1b=sonnet
  ccwatch --session 3f2a --advisor opus   pin sesi 3f2a… + label advisor hanya untuk sesi itu
  ccwatch install [--statusline] [--no-rules]   hooks + aturan skill di ~/.claude/CLAUDE.md
  ccwatch uninstall                              lepas hooks + aturan skill
  ccwatch rules [--remove | --show]              aturan skill saja
  ccwatch demo             coba tampilan dengan data palsu
  ccwatch hook             (dipanggil oleh Claude Code)
  ccwatch statusline       (dipanggil oleh Claude Code)`

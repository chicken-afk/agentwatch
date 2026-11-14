# ccwatch — live dashboard Claude Code

Dashboard terminal yang menampilkan **model apa saja yang dipakai dalam satu sesi Claude Code dan urutannya**:
lead (main session), advisor, dan setiap subagent, live di split Ghostty sebelah Claude Code.

```
╭─ ADVISOR · opus 5.5 ──────╮ ╭─ LEAD · sonnet 5.5 · main session ───────────────────────────╮
│ calls   3                 │ │ status  ◓ working  ⟶ Bash: go test ./internal/leave/...     │
│ ◇ 00:27:15 lead · Bash    │ ╰──────────────────────────────────────────────────────────────╯
│ ◇ 00:27:10 lead · Bash    │ ╭─ URUTAN MODEL · 20 langkah ──────────────────────────────────╮
│ ◇ 00:26:53 lead · Read    │ │ … sonnet 5.5 → opus 5.5 (advisor) → sonnet 5.5 → haiku 5.5   │
│                           │ │ 00:27:10  ● opus 5.5    advisor   1 saran                    │
│                           │ │ 00:27:11  ● haiku 5.5   search    1 pesan                    │
╰───────────────────────────╯ ╰──────────────────────────────────────────────────────────────╯
```

## Isi tampilan

| Tab | Isi |
|---|---|
| **1 dashboard** | Kotak LEAD (model, status, tool yang sedang jalan, prompt, token), ADVISOR (model, jumlah panggilan, riwayat + konteks pemicunya), SKILLS (skill yang dipakai di prompt saat ini: siapa yang memanggil, masih aktif `●` atau selesai `✓`, plus ringkasan prompt sebelumnya), URUTAN MODEL (rantai + langkah terakhir), kartu SUBAGENT (model, running/done, durasi, tool, token, tugas), tabel PEMAKAIAN MODEL (peran, pesan, token in/out/cache, porsi output, jam aktif), SESSION LOG |
| **2 urutan model** | Semua langkah model dari awal sesi, berurutan, termasuk jeda |
| **3 log** | Semua event: prompt, tool call, kegagalan, dispatch subagent, advisor, compact |
| **4 subagent** | Tabel semua subagent di sesi itu, plus subagent terkonfigurasi yang belum dipanggil (status `siap`) |

Tombol: `1-4`/`tab` ganti tampilan · `[` `]` ganti sesi · `f` ikuti sesi paling aktif · `j/k`/scroll mouse untuk scroll · `q` keluar.

Warna per keluarga model: opus ungu, sonnet oranye, haiku biru, fable hijau.

## Cara kerja

1. **Hooks** (`ccwatch hook`): Claude Code memanggilnya di tiap event (SessionStart, UserPromptSubmit, Pre/PostToolUse, SubagentStart/Stop, Stop, dll.). Payload diringkas lalu ditambahkan ke `~/.claude/ccwatch/events.jsonl`. Hook tidak pernah menulis ke stdout dan selalu exit 0, jadi tidak mengganggu Claude.
2. **Transcript**: dashboard membaca (tail) transcript sesi `~/.claude/projects/<proj>/<session>.jsonl` dan transcript subagent `…/<session>/subagents/agent-*.jsonl`. Dari sini didapat **model sebenarnya** per pesan, token, jumlah tool, dan panggilan advisor (`server_tool_use` bernama `advisor`).
3. **Skills**: dicatat dari tool `Skill` yang dipanggil Claude (hook PreToolUse + transcript, lead maupun subagent) dan dari slash command yang kamu ketik (`/nama args`, kecuali perintah bawaan seperti `/clear`, `/model`). Kata pertama setelah nama dibaca sebagai **subcommand**, mis. `/impeccable layout page …` → `impeccable layout` dengan sisa args sebagai konteks. Skill yang dipanggil **lead** di prompt sebelumnya ditampilkan sebagai **masih di konteks (lead)** (`◆`), karena isinya tetap ada di konteks lead sepanjang sesi; Claude Code bahkan menyertakannya lagi setelah compact (attachment `invoked_skills` di transcript, yang juga dibaca ccwatch). Skill yang dipanggil **subagent** hanya hidup di konteks subagent itu, jadi tampil sebagai **riwayat subagent** (`◇`). Catatan: "di konteks" berarti instruksi skill tersedia bagi Claude, bukan berarti Claude benar-benar mengikutinya di prompt ini; yang pasti dipakai hanya yang muncul di bagian atas (dipanggil di prompt saat ini). Skill sejak prompt terakhir = task saat ini; statusnya aktif selama lead/subagent pemanggilnya masih bekerja.
4. **Definisi subagent**: `.claude/agents/*.md` di proyek (dicari dari folder sesi naik ke atas) dan `~/.claude/agents/*.md`. Nama, `model`, dan `description` dari frontmatter dipakai untuk menampilkan subagent yang **siap tapi belum dipanggil**. Kalau namanya sama, versi proyek menang. Dibaca ulang tiap 15 detik, jadi agent baru langsung muncul.
5. **Status line** (opsional, `ccwatch statusline`): menampilkan model lead + advisor di bawah prompt Claude Code, dan mencatat pergantian `/model` ke dashboard.

## Instalasi

### Pakai binary jadi
```bash
# macOS Apple Silicon (ganti sesuai OS/arsitektur kamu)
mkdir -p ~/.local/bin
cp dist/ccwatch-darwin-arm64 ~/.local/bin/ccwatch
chmod +x ~/.local/bin/ccwatch
xattr -d com.apple.quarantine ~/.local/bin/ccwatch 2>/dev/null   # macOS: lepas karantina Gatekeeper
```
Pastikan `~/.local/bin` ada di `PATH`.

### Atau build sendiri (Go 1.22+)
```bash
go mod tidy
go build -o ~/.local/bin/ccwatch .
```

### Pasang hooks
```bash
ccwatch install --statusline
```
- Menambah hook ccwatch ke `~/.claude/settings.json` **tanpa menghapus** hook/setting yang sudah ada, dan membuat backup `settings.json.ccwatch-backup-*`.
- `--statusline` hanya dipasang kalau kamu belum punya `statusLine`.
- Lepas lagi: `ccwatch uninstall`.
- Kalau binary dipindah, jalankan `ccwatch install` lagi supaya path-nya diperbarui.

### Aturan skill (otomatis untuk semua proyek)
`ccwatch install` juga menambah blok aturan singkat ke `~/.claude/CLAUDE.md`, memori level user yang dimuat Claude Code di **semua proyek**. Isinya meminta Claude benar-benar memanggil skill lewat tool `Skill` (bukan hanya menyebut namanya) dan hanya melaporkan skill yang memang dipanggil di task itu. Dengan begitu yang Claude klaim cocok dengan yang tampil di kotak SKILLS, tanpa perlu edit `AGENTS.md`/`CLAUDE.md` di tiap proyek.

```bash
ccwatch rules            # pasang/perbarui aturan saja (tanpa hooks)
ccwatch rules --show     # lihat isinya tanpa menulis apa pun
ccwatch rules --remove   # lepas aturan
ccwatch install --no-rules   # pasang hooks tanpa aturan
```
- Blok ditandai `<!-- ccwatch:skills:start -->` … `<!-- ccwatch:skills:end -->`; isi lain di `CLAUDE.md` tidak disentuh, dan backup `CLAUDE.md.ccwatch-backup-*` dibuat setiap kali file yang sudah ada diubah.
- `ccwatch uninstall` ikut melepas blok ini.
- Ini tetap instruksi, bukan paksaan: Claude kadang masih melewatinya. Untuk memastikan skill jalan, panggil langsung dengan slash command (mis. `/impeccable polish …`).

## Pakai di Ghostty

1. Buka split: **macOS** `Cmd+D` (kanan) / `Cmd+Shift+D` (bawah), **Linux** `Ctrl+Shift+O` (kanan) / `Ctrl+Shift+E` (bawah).
2. Pane kiri: `claude --model sonnet --advisor opus`
3. Pane kanan: `ccwatch`

Secara default dashboard **hanya menampilkan sesi Claude Code di folder tempat `ccwatch` dijalankan** (termasuk subfolder-nya), lalu otomatis mengikuti sesi yang paling baru aktif di antaranya. Contoh: `cd ~/Works/personal/pos && ccwatch` → hanya sesi yang dibuka di `pos`, `pos/frontend`, `pos/backend`, dst. Lebar ideal ≥ 110 kolom (advisor di kiri). Di pane sempit, tata letaknya otomatis bertumpuk.

Coba tampilannya tanpa Claude Code: `ccwatch demo`.

### Opsi watch
```bash
ccwatch                        # hanya sesi di folder saat ini (+ subfolder)
ccwatch --all                  # sesi dari semua folder
ccwatch --dir ~/Works/personal/pos   # hanya sesi di folder tertentu, dari mana pun ccwatch dijalankan
ccwatch --session 7f3c9a21     # kunci ke sesi tertentu (prefix id), tanpa filter folder
ccwatch --advisor opus         # label advisor kalau dipakai via flag --advisor (tidak tersimpan di settings)
ccwatch --recent 30m           # ikut tampilkan sesi yang aktif 30 menit terakhir walau hooks belum terpasang
```

### Label advisor per sesi
`--advisor opus` berlaku untuk **semua** sesi. Kalau tiap sesi pakai advisor berbeda, beri label per session id (prefix id boleh):

```bash
ccwatch --advisor 7f3c=opus                          # hanya sesi 7f3c…
ccwatch --advisor 7f3c=opus --advisor 9c1b=sonnet    # beberapa sesi (flag boleh diulang)
ccwatch --advisor 7f3c=opus,9c1b=sonnet              # sama, dipisah koma
ccwatch --advisor opus --advisor 9c1b=sonnet         # default opus, kecuali sesi 9c1b… pakai sonnet
ccwatch --session 7f3c --advisor opus                # pin sesi 7f3c… dan label advisor hanya untuk sesi itu
```

Urutan prioritas label advisor:
1. Model dari `usage.iterations` di transcript (kalau advisor sudah pernah dipanggil).
2. Label per sesi `--advisor <id>=<model>` (prefix terpanjang yang cocok menang).
3. Label global `--advisor <model>`. Kalau `--session` di-set, label ini hanya berlaku untuk sesi tersebut.
4. `advisorModel` dari settings: `~/.claude/settings.json`, lalu `<proyek>/.claude/settings.json`, lalu `<proyek>/.claude/settings.local.json` (yang terakhir ditemukan menang).

Session id (versi pendek) terlihat di header dashboard (`sesi 7f3c9a21`) dan bisa langsung dipakai sebagai prefix. Id lengkap ada di nama file transcript `~/.claude/projects/<proj>/<session-id>.jsonl`.

## Catatan jujur

- **Model advisor**: nama model diambil dari `usage.iterations` di transcript kalau tersedia, kalau tidak dari `advisorModel` di settings. Kalau kamu memakai `claude --advisor opus` (flag, bukan settings), jalankan `ccwatch --advisor opus` supaya labelnya benar (atau `ccwatch --advisor <session-id>=opus` kalau hanya untuk satu sesi).
- Format transcript Claude Code bukan API resmi dan bisa berubah antar versi. Kalau ada angka yang kosong setelah update Claude Code, kemungkinan besar formatnya berubah.
- Subagent di `.claude/agents` hanya *tersedia*; Claude yang memutuskan kapan mendelegasikan. Kalau statusnya terus `siap`, minta langsung, mis. "use the search agent to find …".
- Claude Code kadang mengirim `SubagentStop` "hantu" untuk summarizer internal (tanpa `agent_type`, transcript tidak ada). ccwatch mengabaikannya.
- Gambar yang kamu lihat di internet punya "JEV fork layer" dengan skor p=0.81 dan sebagainya. Itu bukan data yang dikeluarkan Claude Code, jadi tidak ditampilkan. Fork subagent tetap muncul sebagai subagent biasa, dengan tipe sesuai yang dilaporkan Claude Code.
- `events.jsonl` dirotasi otomatis saat > 20 MB.

## Struktur kode

`main.go` hanya titik masuk; logika ada di `internal/`:

| Paket | Tanggung jawab |
|---|---|
| `cli` | dispatch subcommand dan parsing flag |
| `config` | lokasi file (`~/.claude`, `CCWATCH_DIR`) dan pembacaan `advisorModel` |
| `hook`, `statusline`, `events` | `ccwatch hook` / `ccwatch statusline` dan penulisan `events.jsonl` |
| `install` | pasang/lepas hooks, status line, dan aturan skill (`settings.json`, `CLAUDE.md`) |
| `store` | state sesi dari `events.jsonl` + transcript (model, subagent, skill, advisor) |
| `ui`, `theme` | dashboard Bubble Tea dan palet warna |
| `demo` | sesi palsu untuk `ccwatch demo` |
| `format`, `pathmatch` | helper format teks/angka dan pencocokan folder sesi |

Cek cepat sebelum commit: `go vet ./... && go test ./...`

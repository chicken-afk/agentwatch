package ui

import (
	"strings"
	"time"

	"ccwatch/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	tabCount     = 4
	tickEvery    = 100 * time.Millisecond // animasi
	refreshEvery = 300 * time.Millisecond // baca data
	wheelStep    = 3
	pageStep     = 15
)

var spinFrames = []string{"◐", "◓", "◑", "◒"}

type tickMsg time.Time

type model struct {
	st     *store.Store
	w, h   int
	tab    int
	pinned string // prefix session id yang dikunci; "" = ikuti sesi paling aktif

	refreshedAt time.Time
	scroll      int
	frame       int
}

func tick() tea.Cmd {
	return tea.Tick(tickEvery, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// Run membuka dashboard live sampai user keluar.
func Run(o store.Options) error {
	m := &model{st: store.New(o), pinned: o.Session}
	m.st.Refresh()
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

// Snapshot merender satu frame dashboard tanpa terminal interaktif (dipakai `ccwatch demo --snapshot`).
func Snapshot(o store.Options, w, h, tab int) string {
	m := &model{st: store.New(o), w: w, h: h, tab: tab, frame: 1}
	m.st.Refresh()
	return m.View()
}

func (m *model) Init() tea.Cmd { return tick() }

// spin: frame spinner, ganti tiap ~300ms walau tick UI 100ms.
func (m *model) spin() string { return spinFrames[(m.frame/3)%len(spinFrames)] }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		if now := time.Time(msg); now.Sub(m.refreshedAt) >= refreshEvery {
			m.refreshedAt = now
			m.st.Refresh()
		}
		m.frame++
		return m, tick()
	case tea.MouseMsg:
		m.onMouse(msg)
	case tea.KeyMsg:
		return m, m.onKey(msg.String())
	}
	return m, nil
}

func (m *model) onMouse(msg tea.MouseMsg) {
	if msg.Action != tea.MouseActionPress {
		return
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scroll += wheelStep
	case tea.MouseButtonWheelDown:
		m.scroll = max(0, m.scroll-wheelStep)
	}
}

func (m *model) onKey(key string) tea.Cmd {
	switch key {
	case "q", "ctrl+c":
		return tea.Quit
	case "1", "2", "3", "4":
		m.setTab(int(key[0] - '1'))
	case "tab":
		m.setTab((m.tab + 1) % tabCount)
	case "shift+tab":
		m.setTab((m.tab + tabCount - 1) % tabCount)
	case "]", "n":
		m.cycle(1)
	case "[", "p":
		m.cycle(-1)
	case "f":
		m.pinned = ""
	case "k", "up":
		m.scroll++
	case "j", "down":
		m.scroll = max(0, m.scroll-1)
	case "pgup":
		m.scroll += pageStep
	case "pgdown":
		m.scroll = max(0, m.scroll-pageStep)
	case "g", "home":
		m.scroll = 1 << 20
	case "G", "end":
		m.scroll = 0
	}
	return nil
}

func (m *model) setTab(t int) {
	m.tab = t
	m.scroll = 0
}

// current: sesi yang sedang ditampilkan beserta posisinya (index, total).
func (m *model) current() (*store.Session, int, int) {
	ss := m.st.Visible()
	if len(ss) == 0 {
		return nil, 0, 0
	}
	if m.pinned != "" {
		for i, s := range ss {
			if strings.HasPrefix(s.ID, m.pinned) {
				return s, i, len(ss)
			}
		}
	}
	return ss[0], 0, len(ss)
}

// cycle pindah ke sesi sebelum/sesudahnya (d = -1/+1) dan mengunci sesi itu.
func (m *model) cycle(d int) {
	ss := m.st.Visible()
	if len(ss) == 0 {
		return
	}
	_, i, n := m.current()
	m.pinned = ss[((i+d)%n+n)%n].ID
	m.scroll = 0
}

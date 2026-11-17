package ui

import (
	"strings"
	"time"

	"github.com/chicken-afk/agentwatch/internal/store"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	tabCount     = 4
	tickEvery    = 100 * time.Millisecond // animation
	refreshEvery = 300 * time.Millisecond // data reads
	wheelStep    = 3
	pageStep     = 15
)

var spinFrames = []string{"◐", "◓", "◑", "◒"}

type tickMsg time.Time

type model struct {
	st     *store.Store
	w, h   int
	tab    int
	pinned string // pinned session id prefix; "" = follow the most active session

	refreshedAt time.Time
	scroll      int
	frame       int

	splash      bool // startup animation still playing
	splashStart time.Time
}

func tick(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// nextTick: ~30 fps while the splash animates, slower while it waits for Enter,
// then the normal pace.
func (m *model) nextTick() tea.Cmd {
	switch {
	case m.splash && time.Since(m.splashStart) < splashTotal+promptFade:
		return tick(splashTick)
	case m.splash:
		return tick(splashIdle)
	}
	return tick(tickEvery)
}

// splashKey handles a key while the splash shows: it can't be skipped, Enter opens the
// dashboard once the animation is done, q / ctrl+c still quit.
func (m *model) splashKey(key string) tea.Cmd {
	switch key {
	case "ctrl+c", "q":
		return tea.Quit
	case "enter":
		if splashReady(time.Since(m.splashStart)) {
			m.splash = false
		}
	}
	return nil
}

// Run opens the live dashboard until the user quits. With splash, the startup
// animation plays first and waits for Enter; sessions load in the background meanwhile.
func Run(o store.Options, splash bool) error {
	m := &model{st: store.New(o), pinned: o.Session, splash: splash, splashStart: time.Now()}
	m.st.Refresh()
	_, err := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion()).Run()
	return err
}

// Snapshot renders one dashboard frame without an interactive terminal (used by `agentwatch demo --snapshot`).
func Snapshot(o store.Options, w, h, tab int) string {
	m := &model{st: store.New(o), w: w, h: h, tab: tab, frame: 1}
	m.st.Refresh()
	return m.View()
}

func (m *model) Init() tea.Cmd { return m.nextTick() }

// spin: spinner frame, advancing every ~300ms even though the UI ticks every 100ms.
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
		return m, m.nextTick()
	case tea.MouseMsg:
		if !m.splash {
			m.onMouse(msg)
		}
	case tea.KeyMsg:
		if m.splash {
			return m, m.splashKey(msg.String())
		}
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

// current: the session being shown plus its position (index, total).
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

// cycle moves to the previous/next session (d = -1/+1) and pins it.
func (m *model) cycle(d int) {
	ss := m.st.Visible()
	if len(ss) == 0 {
		return
	}
	_, i, n := m.current()
	m.pinned = ss[((i+d)%n+n)%n].ID
	m.scroll = 0
}

// Package tui is the Bubble Tea front end: the combat table, row selection
// and turn controls. It changes the encounter only through session.Apply, so
// every action is autosaved and logged the same way (D19).
package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
)

// Default size until the terminal reports its own (R: works in 80×24).
const (
	defaultWidth  = 80
	defaultHeight = 24
)

// chrome is the number of screen lines that aren't table rows: the title,
// the table's top border, header and separator, its bottom border, the
// status line and the key hints.
const chrome = 7

type msgLevel int

const (
	levelInfo msgLevel = iota
	levelWarn
	levelError
)

// Model is the Bubble Tea model for the combat screen.
type Model struct {
	sess     *session.Session
	width    int
	height   int
	selected combat.RowID // the row hotkeys act on; may differ from the turn
	selIdx   int          // last known index of selected, to land nearby if it disappears
	offset   int          // first visible row when the table scrolls
	msg      string
	level    msgLevel
}

// New returns a model for s. resumedFrom, if non-empty, is shown as a notice
// that a saved fight was picked up.
func New(s *session.Session, resumedFrom string) *Model {
	m := &Model{sess: s, width: defaultWidth, height: defaultHeight}
	if resumedFrom != "" {
		m.info("Resumed from " + resumedFrom)
	}
	m.selected = s.Encounter().Current
	m.syncSelection()
	return m
}

// Selected returns the ID of the selected row, or zero when the table is empty.
func (m *Model) Selected() combat.RowID { return m.selected }

func (m *Model) Init() tea.Cmd { return nil }

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.syncSelection()
	case tea.KeyPressMsg:
		return m, m.handleKey(msg.String())
	}
	return m, nil
}

func (m *Model) handleKey(key string) tea.Cmd {
	switch key {
	case "q", "ctrl+c":
		return tea.Quit
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "home", "g":
		m.moveTo(0)
	case "end", "G":
		m.moveTo(len(m.sess.Encounter().Order()) - 1)
	case "n", "space":
		m.turnOp(func(e *combat.Encounter) ([]combat.Event, error) { return e.Next() })
	case "p":
		m.turnOp(func(e *combat.Encounter) ([]combat.Event, error) { return e.Previous() })
	case "S":
		m.turnOp(func(e *combat.Encounter) ([]combat.Event, error) { return e.Start() })
	}
	return nil
}

// turnOp applies a turn change and moves the selection to the new turn
// holder, since that's usually who the DM acts on next (D22).
func (m *Model) turnOp(op session.Op) {
	if m.apply(op) {
		m.selected = m.sess.Encounter().Current
		m.syncSelection()
	}
}

// apply runs op through the session and reports what happened in the status
// line. It returns whether the operation succeeded.
func (m *Model) apply(op session.Op) bool {
	m.msg = ""
	out, err := m.sess.Apply(op)
	if err != nil {
		m.fail(err)
		return false
	}
	var problems []string
	if out.SaveErr != nil {
		problems = append(problems, "Autosave failed: "+out.SaveErr.Error())
	}
	if out.LogErr != nil {
		problems = append(problems, "Log write failed: "+out.LogErr.Error())
	}
	if len(problems) > 0 {
		m.msg, m.level = strings.Join(problems, " · "), levelError
	}
	return true
}

func (m *Model) info(s string) { m.msg, m.level = s, levelInfo }

func (m *Model) fail(err error) {
	text := err.Error()
	switch {
	case errors.Is(err, combat.ErrEmpty):
		text = "No combatants to start with"
	case errors.Is(err, combat.ErrNotActive):
		text = "Combat hasn't started; press S to start"
	case errors.Is(err, combat.ErrNotSetup):
		text = "Combat has already started"
	case errors.Is(err, combat.ErrNothingToUndo):
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	m.msg, m.level = text, levelWarn
}

func (m *Model) move(delta int) {
	m.syncSelection()
	m.moveTo(m.selIdx + delta)
}

func (m *Model) moveTo(idx int) {
	order := m.sess.Encounter().Order()
	if len(order) == 0 {
		return
	}
	idx = max(0, min(idx, len(order)-1))
	m.selected = order[idx].ID()
	m.syncSelection()
}

// syncSelection keeps the selection on a row that exists (falling back to
// the row now at its old position) and scrolls so it is visible.
func (m *Model) syncSelection() {
	order := m.sess.Encounter().Order()
	if len(order) == 0 {
		m.selected, m.selIdx, m.offset = 0, 0, 0
		return
	}
	found := false
	for i, r := range order {
		if r.ID() == m.selected {
			m.selIdx, found = i, true
			break
		}
	}
	if !found {
		m.selIdx = max(0, min(m.selIdx, len(order)-1))
		m.selected = order[m.selIdx].ID()
	}
	visible := m.visibleRows()
	if m.selIdx < m.offset {
		m.offset = m.selIdx
	}
	if m.selIdx >= m.offset+visible {
		m.offset = m.selIdx - visible + 1
	}
	m.offset = max(0, min(m.offset, len(order)-visible))
}

func (m *Model) visibleRows() int { return max(1, m.height-chrome) }

// Styles. Kept simple; colours degrade to the terminal's profile.
var (
	styleTitle    = lipgloss.NewStyle().Bold(true)
	styleHeader   = lipgloss.NewStyle().Bold(true)
	styleSelected = lipgloss.NewStyle().Reverse(true)
	styleCurrent  = lipgloss.NewStyle().Bold(true)
	styleDown     = lipgloss.NewStyle().Faint(true)
	styleEffect   = lipgloss.NewStyle().Foreground(lipgloss.Magenta)
	styleTie      = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	styleBloodied = lipgloss.NewStyle().Foreground(lipgloss.Red)
	styleHint     = lipgloss.NewStyle().Faint(true)
	styleWarn     = lipgloss.NewStyle().Foreground(lipgloss.Yellow)
	styleError    = lipgloss.NewStyle().Foreground(lipgloss.Red).Bold(true)
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "dmct"
	return v
}

// render draws the whole screen as a string.
func (m *Model) render() string {
	enc := m.sess.Encounter()
	l := computeLayout(m.width)
	rows := buildRows(enc)

	var b strings.Builder
	b.WriteString(fit(m.title(enc), m.width))
	b.WriteString("\n")
	b.WriteString(l.border("┌", "┬", "┐") + "\n")
	b.WriteString(l.line(func(c column) string { return headers[c] }, func(_ column, s string) string { return styleHeader.Render(s) }) + "\n")
	b.WriteString(l.border("├", "┼", "┤") + "\n")

	visible := m.visibleRows()
	end := min(len(rows), m.offset+visible)
	for _, rc := range rows[m.offset:end] {
		b.WriteString(l.line(rc.get, m.cellStyle(rc)) + "\n")
	}
	b.WriteString(l.border("└", "┴", "┘") + "\n")
	b.WriteString(m.statusLine(enc, rows, end) + "\n")
	b.WriteString(styleHint.Render(fit(m.hints(enc), m.width)))
	return b.String()
}

func (m *Model) title(enc *combat.Encounter) string {
	switch enc.Status {
	case combat.Setup:
		return " Setup ─ press S to start combat"
	default:
		holder := "—"
		if r, ok := enc.Row(enc.Current); ok {
			holder = r.Name()
		}
		return styleTitle.Render(fmt.Sprintf(" Round %d ─ Turn: %s", enc.Round, holder))
	}
}

func (m *Model) cellStyle(rc rowCells) func(column, string) string {
	selected := rc.id == m.selected
	return func(c column, s string) string {
		st := lipgloss.NewStyle()
		switch {
		case rc.down:
			st = st.Inherit(styleDown)
		case rc.effect:
			st = st.Inherit(styleEffect)
		}
		if rc.current {
			st = st.Inherit(styleCurrent)
		}
		if c == colInit && rc.tied {
			st = st.Inherit(styleTie)
		}
		if c == colHP && strings.Contains(rc.hp, "▼") {
			st = st.Inherit(styleBloodied)
		}
		if selected {
			st = st.Inherit(styleSelected)
		}
		return st.Render(s)
	}
}

// statusLine shows the latest message, else a tie warning, else a scroll hint.
func (m *Model) statusLine(enc *combat.Encounter, rows []rowCells, end int) string {
	text, style := m.msg, styleHint
	switch m.level {
	case levelWarn:
		style = styleWarn
	case levelError:
		style = styleError
	}
	if text == "" && len(rows) == 0 {
		text = "No combatants yet"
	}
	if text == "" && len(enc.Ties()) > 0 {
		text, style = "! Tied initiative: enter a decimal (e.g. 15.2 / 15.1) to set the order", styleWarn
	}
	if text == "" && (m.offset > 0 || end < len(rows)) {
		text = fmt.Sprintf("Rows %d–%d of %d", m.offset+1, end, len(rows))
	}
	return style.Render(fit(" "+text, m.width))
}

func (m *Model) hints(enc *combat.Encounter) string {
	if enc.Status == combat.Setup {
		return " S start · ↑↓/jk select · q quit"
	}
	return " n/space next · p previous · ↑↓/jk select · q quit"
}

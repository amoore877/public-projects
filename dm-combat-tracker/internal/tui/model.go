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
	"github.com/charmbracelet/x/ansi"

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
// status line and the key hints. The error banner, when shown, adds one.
const chrome = 7

// minHeight is the shortest terminal that shows at least one table row
// with the banner up.
const minHeight = chrome + 2

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
	overlay  overlay  // the open prompt, if any; it gets every key
	chain    overlay  // set by an action to open another prompt when the current one closes
	history  []string // command-bar lines, oldest first
	help     bool     // the help screen is showing
	helpOff  int      // its scroll position
	quitting bool     // set by the quit confirmation; Update then quits

	// Sticky problems shown in the error banner until a later change
	// succeeds where they failed.
	saveErr error // the latest autosave failed
	logErr  error // the latest log write failed
	logLost int   // changes whose log entries were lost since logging last worked
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
		if m.tooSmall() {
			// Nothing on screen to act on: only quitting works.
			if k := msg.String(); k == "q" || k == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.help {
			m.updateHelp(msg)
			return m, nil
		}
		if m.overlay != nil {
			m.overlay = m.overlay.update(m, msg)
			if m.chain != nil {
				m.overlay, m.chain = m.chain, nil
			}
			if m.quitting {
				return m, tea.Quit
			}
			return m, nil
		}
		return m, m.handleKey(msg.String())
	case tea.PasteMsg:
		if p, ok := m.overlay.(*inputPrompt); ok {
			p.paste(msg.Content)
		}
	}
	return m, nil
}

// open shows an overlay; a nil overlay (the action warned instead) is a no-op.
func (m *Model) open(o overlay) {
	if o != nil {
		m.msg = ""
	}
	m.overlay = o
}

func (m *Model) handleKey(key string) tea.Cmd {
	switch key {
	case "q", "ctrl+c":
		if m.saveErr != nil {
			m.open(confirm("Autosave is failing: quit anyway? Changes since it failed are lost.", func(m *Model) { m.quitting = true }))
			return nil
		}
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
	case "a":
		m.open(m.promptAdd())
	case "x":
		if fx, ok := m.sess.Encounter().Effect(m.selected); ok {
			m.open(m.promptEndEffect(fx))
		} else {
			m.open(m.promptRemove())
		}
	case "e":
		m.open(m.promptNewEffect())
	case "T":
		m.open(m.promptTargets())
	case "d", "h", "t", "D", "M":
		m.open(m.promptHP(key))
	case "c":
		m.open(m.promptAddCondition())
	case "C":
		m.open(m.promptRemoveCondition())
	case "i":
		m.open(m.promptInitiative())
	case "N":
		m.open(m.promptNote())
	case "l":
		m.open(m.promptLog())
	case "E":
		m.open(m.promptEndCombat())
	case ":":
		m.open(m.promptCommand())
	case "?":
		m.help, m.helpOff = true, 0
	}
	return nil
}

// turnOp applies a turn change and moves the selection to the new turn
// holder, since that's usually who the DM acts on next (D22).
func (m *Model) turnOp(op session.Op) {
	if m.apply(op) {
		m.selected = m.sess.Encounter().Current
		m.syncSelection()
		m.turnReminder()
	}
}

// turnReminder points out, in the status line, a turn holder whose health
// state asks for it (3.5 dying). It only reminds; the DM applies the rule (RS3).
func (m *Model) turnReminder() {
	enc := m.sess.Encounter()
	c, ok := enc.Combatant(enc.Current)
	if !ok || m.msg != "" {
		return
	}
	if st := enc.HealthState(c); st != nil && st.Remind {
		m.info(fmt.Sprintf("%s is %s (%d HP)", c.Name, st.Name, c.HP))
	}
}

// apply runs op through the session and reports what happened in the status
// line. It returns whether the operation succeeded.
func (m *Model) apply(op session.Op) bool {
	if _, err := m.run(op); err != nil {
		m.fail(err)
		return false
	}
	return true
}

// run applies op and reports save and log problems, leaving a refused
// operation's error to the caller.
func (m *Model) run(op session.Op) (session.Outcome, error) {
	m.msg = ""
	out, err := m.sess.Apply(op)
	if err != nil {
		return out, err
	}
	m.track(out)
	if m.msg == "" && out.Logged {
		// A manual note changes nothing on screen, so say where it went
		// (the same for the l key and :log).
		for _, ev := range out.Events {
			if _, ok := ev.(combat.NoteLogged); ok {
				m.info("Logged to " + m.sess.Encounter().LogPath)
			}
		}
	}
	m.syncSelection() // the banner coming or going changes how many rows fit
	return out, nil
}

// track updates the error banner from an operation's outcome. A problem
// stays up until a later change saves (or logs) successfully, and its
// recovery is reported once in the status line.
func (m *Model) track(out session.Outcome) {
	if len(out.Events) == 0 {
		return // nothing was saved or logged
	}
	switch {
	case out.SaveErr != nil:
		m.saveErr = out.SaveErr
	case m.saveErr != nil:
		m.saveErr = nil
		m.info("Autosave is working again; the state file is up to date")
	}
	switch {
	case out.LogErr != nil:
		m.logErr = out.LogErr
		m.logLost++
	case out.Logged && m.logErr != nil:
		m.warn(fmt.Sprintf("The log is writing again; %s before this weren't logged", plural(m.logLost, "change", "changes")))
		m.logErr, m.logLost = nil, 0
	}
}

// banner is the error banner's text, or "" when nothing is failing. The
// summary comes first and the raw error last, since narrow terminals cut
// the end off.
func (m *Model) banner() string {
	switch {
	case m.saveErr != nil && m.logErr != nil:
		return " ✖ Autosave and log failing: changes aren't on disk or in the log (retrying): " + m.saveErr.Error()
	case m.saveErr != nil:
		return " ✖ Autosave failing: changes aren't on disk (retrying): " + m.saveErr.Error()
	case m.logErr != nil:
		return fmt.Sprintf(" ✖ Log failing: %s not logged (retrying): %s", plural(m.logLost, "change", "changes"), m.logErr.Error())
	}
	return ""
}

func (m *Model) bannerLines() int {
	if m.banner() != "" {
		return 1
	}
	return 0
}

// tooSmall reports whether the terminal can't fit the table.
func (m *Model) tooSmall() bool { return m.width < minWidth || m.height < minHeight }

func (m *Model) renderTooSmall() string {
	lines := []string{
		" Terminal too small",
		fmt.Sprintf(" Need %d×%d, have %d×%d.", minWidth, minHeight, m.width, m.height),
		" Make the window bigger; q quits (state is saved).",
	}
	if m.saveErr != nil {
		lines[2] = " Make the window bigger: autosave is failing."
	}
	out := make([]string, m.height)
	for i := range out {
		s := ""
		if i < len(lines) {
			s = lines[i]
		}
		out[i] = fit(s, m.width)
	}
	if b := m.banner(); b != "" && m.height > len(lines) {
		out[len(lines)] = styleBanner.Render(fit(b, m.width))
	}
	return strings.Join(out, "\n")
}

func (m *Model) info(s string) { m.msg, m.level = s, levelInfo }
func (m *Model) warn(s string) { m.msg, m.level = s, levelWarn }

func (m *Model) fail(err error) { m.warn(errText(err)) }

// errText turns a domain error into a sentence for the status line.
func errText(err error) string {
	text := err.Error()
	switch {
	case errors.Is(err, combat.ErrEmpty):
		text = "No combatants to start with"
	case errors.Is(err, combat.ErrNotActive):
		text = "Combat hasn't started; press S to start"
	case errors.Is(err, combat.ErrInvalid):
		text = strings.TrimPrefix(text, combat.ErrInvalid.Error()+": ")
	}
	if text != "" {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	return text
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

func (m *Model) visibleRows() int { return max(1, m.height-chrome-m.bannerLines()) }

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
	styleCursor   = lipgloss.NewStyle().Reverse(true)
	styleBanner   = lipgloss.NewStyle().Foreground(lipgloss.BrightWhite).Background(lipgloss.Red).Bold(true)
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "dmct"
	return v
}

// render draws the whole screen as a string.
func (m *Model) render() string {
	if m.tooSmall() {
		return m.renderTooSmall()
	}
	if m.help {
		return m.renderHelp()
	}
	enc := m.sess.Encounter()
	l := computeLayout(m.width)
	rows := buildRows(enc)
	if p, ok := m.overlay.(*targetPicker); ok {
		for i := range rows {
			rows[i].marker = p.marker(enc, rows[i].id)
		}
	}

	var b strings.Builder
	b.WriteString(m.title(enc))
	b.WriteString("\n")
	if text := m.banner(); text != "" {
		b.WriteString(styleBanner.Render(fit(text, m.width)) + "\n")
	}
	b.WriteString(l.border("┌", "┬", "┐") + "\n")
	b.WriteString(l.line(func(c column) string { return headers[c] }, func(_ column, s string) string { return styleHeader.Render(s) }) + "\n")
	b.WriteString(l.border("├", "┼", "┤") + "\n")

	visible := m.visibleRows()
	end := min(len(rows), m.offset+visible)
	for _, rc := range rows[m.offset:end] {
		b.WriteString(l.line(rc.get, m.cellStyle(rc)) + "\n")
	}
	b.WriteString(l.border("└", "┴", "┘") + "\n")
	if m.overlay != nil {
		b.WriteString(fit(m.overlay.line(m.width), m.width) + "\n")
		text, warn := m.overlay.hint()
		style := styleHint
		if warn {
			style = styleWarn
		}
		b.WriteString(style.Render(fit(text, m.width)))
		return b.String()
	}
	b.WriteString(m.statusLine(enc, l, rows, end) + "\n")
	b.WriteString(styleHint.Render(fit(m.hints(enc), m.width)))
	return b.String()
}

// title is the top line: the round and turn on the left and the ruleset on
// the right (RS1). The ruleset gives way when the line is too narrow for both.
func (m *Model) title(enc *combat.Encounter) string {
	left, style := " Setup ─ press S to start combat", lipgloss.NewStyle()
	if enc.Status != combat.Setup {
		holder := "—"
		if r, ok := enc.Row(enc.Current); ok {
			holder = r.Name()
		}
		left, style = fmt.Sprintf(" Round %d ─ Turn: %s", enc.Round, holder), styleTitle
	}
	label := enc.Rules().Name + " "
	room := m.width - ansi.StringWidth(label)
	if ansi.StringWidth(left)+2 > room {
		return style.Render(fit(left, m.width))
	}
	return style.Render(fit(left, room)) + styleHint.Render(label)
}

func (m *Model) cellStyle(rc rowCells) func(column, string) string {
	selected := rc.id == m.selected
	return func(c column, s string) string {
		st := lipgloss.NewStyle()
		// A state that stops the creature acting dims its row; one it can
		// still act in (3.5 disabled) only marks the HP cell.
		switch {
		case rc.state != nil && !rc.state.Acts:
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
		if rc.state != nil {
			switch {
			case c == colHP && rc.state.Acts:
				st = st.Inherit(styleWarn)
			case c == colHP && rc.state.Remind:
				st = st.Inherit(styleBloodied)
			case c == colName && rc.state.Name == "dead":
				st = st.Strikethrough(true)
			}
		}
		if selected {
			st = st.Inherit(styleSelected)
		}
		return st.Render(s)
	}
}

// statusLine shows the latest message, else the selected row's details when
// the table cuts them off, else a tie warning, else a scroll hint.
func (m *Model) statusLine(enc *combat.Encounter, l layout, rows []rowCells, end int) string {
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
	if text == "" {
		text = m.selectedDetail(l, rows)
	}
	if text == "" && len(enc.Ties()) > 0 {
		text, style = "! Tied initiative: enter a decimal (e.g. 15.2 / 15.1) to set the order", styleWarn
	}
	if text == "" && (m.offset > 0 || end < len(rows)) {
		text = fmt.Sprintf("Rows %d–%d of %d", m.offset+1, end, len(rows))
	}
	return style.Render(fit(" "+text, m.width))
}

// hints lists the keys that apply, most useful first, dropping from the end
// (but always keeping the command bar, help and quit) until they fit the width.
func (m *Model) hints(enc *combat.Encounter) string {
	keys := []string{"n next", "p prev", "d/h dmg/heal", "c/C cond", "e effect", "a add", "x remove", "T targets", "t temp HP", "i init", "N note", "l log", "E end"}
	if r, ok := enc.Row(m.selected); ok && r.Effect != nil {
		keys = []string{"n next", "p prev", "T targets", "x end early", "e effect", "i init", "N note", "l log", "E end"}
	}
	if enc.Status == combat.Setup {
		keys = []string{"a add", "S start", "e effect", "d/h dmg/heal", "c/C cond", "x remove", "i init", "N note", "l log", "t temp HP"}
	}
	keys = append([]string{}, keys...)
	for {
		s := " " + strings.Join(append(keys[:len(keys):len(keys)], ": cmd", "? help", "q quit"), " · ")
		if ansi.StringWidth(s) <= m.width || len(keys) == 0 {
			return s
		}
		keys = keys[:len(keys)-1]
	}
}

// selectedDetail spells out the selected row's name, conditions and note
// when the table truncates or hides any of them (narrow terminals), or "".
func (m *Model) selectedDetail(l layout, rows []rowCells) string {
	for _, rc := range rows {
		if rc.id != m.selected {
			continue
		}
		if !l.cut(colName, rc.name) && !l.cut(colHP, rc.hp) && !l.cut(colConds, rc.conds) && !l.cut(colNote, rc.note) {
			return ""
		}
		parts := []string{rc.name}
		if l.cut(colHP, rc.hp) {
			parts = append(parts, "HP "+rc.hp)
		}
		if rc.conds != "" {
			parts = append(parts, rc.conds)
		}
		if rc.note != "" {
			parts = append(parts, "Note: "+rc.note)
		}
		return strings.Join(parts, " · ")
	}
	return ""
}

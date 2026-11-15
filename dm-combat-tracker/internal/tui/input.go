package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// overlay is an input step that takes over the bottom two lines of the
// screen (the status line and the key hints) while it is open, so the table
// never moves (D24).
type overlay interface {
	// update handles a key and returns the overlay to show next: itself to
	// stay open, another to move to the next step, or nil to close.
	update(m *Model, k tea.KeyPressMsg) overlay
	// line is the prompt line, drawn in place of the status line.
	line(width int) string
	// hint is drawn in place of the key hints.
	hint() (text string, warn bool)
}

// lineEdit is a minimal single-line text editor.
type lineEdit struct {
	text []rune
	pos  int
}

func newLineEdit(s string) lineEdit {
	r := []rune(s)
	return lineEdit{text: r, pos: len(r)}
}

func (l *lineEdit) String() string { return string(l.text) }

// handle applies an editing key and reports whether the text changed.
func (l *lineEdit) handle(k tea.KeyPressMsg) (changed, handled bool) {
	switch k.String() {
	case "left", "ctrl+b":
		l.pos = max(0, l.pos-1)
	case "right", "ctrl+f":
		l.pos = min(len(l.text), l.pos+1)
	case "home", "ctrl+a":
		l.pos = 0
	case "end", "ctrl+e":
		l.pos = len(l.text)
	case "backspace", "ctrl+h":
		if l.pos == 0 {
			return false, true
		}
		l.text = append(l.text[:l.pos-1], l.text[l.pos:]...)
		l.pos--
		return true, true
	case "delete", "ctrl+d":
		if l.pos == len(l.text) {
			return false, true
		}
		l.text = append(l.text[:l.pos], l.text[l.pos+1:]...)
		return true, true
	case "ctrl+u":
		l.text, l.pos = l.text[l.pos:], 0
		return true, true
	case "ctrl+k":
		l.text = l.text[:l.pos]
		return true, true
	case "ctrl+w":
		i := l.pos
		for i > 0 && l.text[i-1] == ' ' {
			i--
		}
		for i > 0 && l.text[i-1] != ' ' {
			i--
		}
		l.text = append(l.text[:i], l.text[l.pos:]...)
		l.pos = i
		return true, true
	default:
		if k.Text == "" || k.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) != 0 {
			return false, false
		}
		ins := []rune(k.Text)
		l.text = append(l.text[:l.pos], append(ins, l.text[l.pos:]...)...)
		l.pos += len(ins)
		return true, true
	}
	return false, true
}

// render draws the text with a block cursor, scrolled so the cursor fits in
// width cells.
func (l *lineEdit) render(width int) string {
	before, at, after := string(l.text[:l.pos]), " ", ""
	if l.pos < len(l.text) {
		at, after = string(l.text[l.pos]), string(l.text[l.pos+1:])
	}
	for width > 1 && ansi.StringWidth(before)+ansi.StringWidth(at) > width {
		r := []rune(before)
		before = "…" + string(r[min(2, len(r)):])
	}
	rest := width - ansi.StringWidth(before) - ansi.StringWidth(at)
	if rest < 0 {
		rest = 0
	}
	if ansi.StringWidth(after) > rest {
		after = ansi.Truncate(after, rest, "…")
	}
	return before + styleCursor.Render(at) + after
}

// inputPrompt asks for one line of text.
type inputPrompt struct {
	label string
	ed    lineEdit
	help  string // shown in the hint line
	// suggest, if set, lists completions for the text; tab cycles them.
	suggest func(text string) []string
	// submit handles enter. An error keeps the prompt open and is shown in
	// the hint line; otherwise the returned overlay (or nil) comes next.
	submit func(m *Model, text string) (overlay, error)

	// history, if set, is browsed with up/down (the command bar).
	history *[]string
	histIdx int

	err     string
	tabBase *string // text before the first tab of a cycle
	tabIdx  int
}

func (p *inputPrompt) update(m *Model, k tea.KeyPressMsg) overlay {
	switch k.String() {
	case "esc", "ctrl+c":
		return nil
	case "enter":
		next, err := p.submit(m, p.ed.String())
		if err != nil {
			p.err = errText(err)
			return p
		}
		return next
	case "up", "down":
		if p.history == nil || len(*p.history) == 0 {
			return p
		}
		h := *p.history
		if k.String() == "up" {
			p.histIdx = max(0, p.histIdx-1)
		} else {
			p.histIdx = min(len(h), p.histIdx+1)
		}
		text := ""
		if p.histIdx < len(h) {
			text = h[p.histIdx]
		}
		p.ed, p.tabBase, p.err = newLineEdit(text), nil, ""
		return p
	case "tab", "shift+tab":
		if p.suggest == nil {
			return p
		}
		if p.tabBase == nil {
			base := p.ed.String()
			p.tabBase, p.tabIdx = &base, -1
		}
		opts := p.suggest(*p.tabBase)
		if len(opts) == 0 {
			return p
		}
		if k.String() == "tab" {
			p.tabIdx = (p.tabIdx + 1) % len(opts)
		} else {
			p.tabIdx = (p.tabIdx - 1 + len(opts)) % len(opts)
		}
		p.ed = newLineEdit(opts[p.tabIdx])
		p.err = ""
		return p
	}
	if changed, _ := p.ed.handle(k); changed {
		p.tabBase, p.err = nil, ""
	}
	return p
}

func (p *inputPrompt) line(width int) string {
	label := " " + p.label + " "
	return styleTitle.Render(label) + p.ed.render(max(1, width-ansi.StringWidth(label)))
}

func (p *inputPrompt) hint() (string, bool) {
	if p.err != "" {
		return " " + p.err, true
	}
	parts := []string{}
	if p.suggest != nil {
		base := p.ed.String()
		if p.tabBase != nil {
			base = *p.tabBase
		}
		if opts := p.suggest(base); len(opts) > 0 {
			parts = append(parts, "tab: "+strings.Join(opts, " · "))
		}
	}
	if p.help != "" {
		parts = append(parts, p.help)
	}
	parts = append(parts, "enter ok · esc cancel")
	return " " + strings.Join(parts, " · "), false
}

// choice is one key of a choicePrompt.
type choice struct {
	key   string // as tea.KeyPressMsg.String() reports it
	label string
	run   func(m *Model) overlay
}

// choicePrompt asks a question answered with a single key, e.g. y/n.
type choicePrompt struct {
	question string
	choices  []choice
	// enter, if set, is the key that enter stands for.
	enter string
}

func (c *choicePrompt) update(m *Model, k tea.KeyPressMsg) overlay {
	key := k.String()
	if key == "esc" || key == "ctrl+c" {
		return nil
	}
	if key == "enter" && c.enter != "" {
		key = c.enter
	}
	for _, ch := range c.choices {
		if strings.EqualFold(key, ch.key) {
			return ch.run(m)
		}
	}
	return c
}

func (c *choicePrompt) line(width int) string {
	return styleTitle.Render(fit(" "+c.question, width))
}

func (c *choicePrompt) hint() (string, bool) {
	parts := make([]string, 0, len(c.choices)+1)
	for _, ch := range c.choices {
		s := ch.key + " " + ch.label
		if ch.key == c.enter {
			s += " (enter)"
		}
		parts = append(parts, s)
	}
	parts = append(parts, "esc cancel")
	return " " + strings.Join(parts, " · "), false
}

// confirm is a yes/no choicePrompt; yes runs then closes.
func confirm(question string, yes func(m *Model)) *choicePrompt {
	return &choicePrompt{
		question: question,
		choices: []choice{
			{key: "y", label: "yes", run: func(m *Model) overlay { yes(m); return nil }},
			{key: "n", label: "no", run: func(*Model) overlay { return nil }},
		},
	}
}

// paste inserts pasted text, flattened to one line.
func (p *inputPrompt) paste(s string) {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	p.ed.text = append(p.ed.text[:p.ed.pos], append(r, p.ed.text[p.ed.pos:]...)...)
	p.ed.pos += len(r)
	p.tabBase, p.err = nil, ""
}

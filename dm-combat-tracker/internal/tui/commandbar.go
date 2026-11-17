package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/command"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
)

// This file holds the ":" command bar and the "?" help screen (milestone 6).

func (m *Model) promptCommand() overlay {
	p := &inputPrompt{label: ":", help: "? lists commands", history: &m.history}
	p.histIdx = len(m.history)
	p.suggest = func(text string) []string {
		if strings.Contains(text, " ") {
			return nil
		}
		var out []string
		for _, v := range command.Verbs {
			if strings.HasPrefix(v, strings.ToLower(strings.TrimPrefix(text, ":"))) {
				out = append(out, v)
			}
		}
		return out
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		cmd, err := command.Parse(m.sess.Encounter(), text)
		if err != nil {
			return p, err
		}
		m.remember(text)
		return m.runCommand(cmd, p)
	}
	return p
}

// remember adds a line to the command history, skipping repeats.
func (m *Model) remember(line string) {
	line = strings.TrimSpace(line)
	if n := len(m.history); n > 0 && m.history[n-1] == line {
		return
	}
	m.history = append(m.history, line)
	if len(m.history) > 100 {
		m.history = m.history[1:]
	}
}

// runCommand runs a parsed command. A refused operation keeps the prompt
// open with the reason, like a typo would.
func (m *Model) runCommand(cmd command.Command, p *inputPrompt) (overlay, error) {
	switch cmd.Kind {
	case command.Remove:
		name := ""
		if r, ok := m.sess.Encounter().Row(cmd.Target); ok {
			name = r.Name()
		}
		m.remove(cmd.Target, name, combat.AskAboutEffects)
		return nil, nil
	case command.EndCombat:
		m.endCombat()
		return nil, nil
	}
	if _, err := m.run(session.Op(cmd.Op)); err != nil {
		return p, err
	}
	switch {
	case cmd.Kind == command.Turn:
		m.selected = m.sess.Encounter().Current
		m.turnReminder()
	case cmd.Created != nil && *cmd.Created != 0:
		m.selected = *cmd.Created
	case cmd.Focus != 0:
		m.selected = cmd.Focus
	}
	m.syncSelection()
	return nil, nil
}

// --- help -------------------------------------------------------------------

var keyHelp = [][2]string{
	{"n / space", "next turn"},
	{"p", "previous turn (restores durations, not other edits)"},
	{"↑↓ / j k", "move the selection"},
	{"g / G", "first / last row"},
	{"a", "add a combatant, or a group: Goblin x4"},
	{"x", "remove the combatant / end the effect early"},
	{"d / h", "damage / heal (=N sets HP)"},
	{"t", "temp HP (N replaces, +N adds)"},
	{"D", "nonlethal damage (=N sets the total; rulesets that track it)"},
	{"M", "edit max HP"},
	{"c / C", "add / remove a condition"},
	{"e", "new effect (source: the selected combatant)"},
	{"T", "change an effect's targets"},
	{"i", "edit initiative"},
	{"N", "edit the side note"},
	{"l", "write a log entry"},
	{":", "command bar (↑↓ history, tab completes)"},
	{"S / E", "start / end combat"},
	{"?", "this help"},
	{"q", "quit (state is autosaved; asks first if saving fails)"},
}

// helpLines is the help screen's content.
func helpLines() []string {
	lines := []string{styleTitle.Render(" Keys") + styleHint.Render(" act on the selected row; in a prompt, tab completes and esc cancels")}
	for _, k := range keyHelp {
		lines = append(lines, fmt.Sprintf("   %-10s %s", k[0], k[1]))
	}
	lines = append(lines, "", styleTitle.Render(" Commands")+styleHint.Render(" names match loosely (goblin 2, gob2, g2); TARGETS is a comma list"))
	for _, s := range command.Specs {
		lines = append(lines, "   "+s.Usage)
		detail := styleHint.Render("       " + s.Summary)
		if s.Example != "" {
			detail += styleHint.Render(" · e.g. ") + s.Example
		}
		lines = append(lines, detail)
	}
	return lines
}

// helpBody is the number of help lines that fit between the title and the
// hint line.
func (m *Model) helpBody() int { return max(1, m.height-2) }

func (m *Model) updateHelp(k tea.KeyPressMsg) {
	limit := max(0, len(helpLines())-m.helpBody())
	switch k.String() {
	case "down", "j":
		m.helpOff++
	case "up", "k":
		m.helpOff--
	case "pgdown", "space", "f":
		m.helpOff += m.helpBody()
	case "pgup", "b":
		m.helpOff -= m.helpBody()
	case "home", "g":
		m.helpOff = 0
	case "end", "G":
		m.helpOff = limit
	default:
		m.help = false
		return
	}
	m.helpOff = max(0, min(m.helpOff, limit))
}

func (m *Model) renderHelp() string {
	lines := helpLines()
	body := m.helpBody()
	end := min(len(lines), m.helpOff+body)
	var b strings.Builder
	b.WriteString(styleTitle.Render(fit(" Help", m.width)) + "\n")
	for i := m.helpOff; i < m.helpOff+body; i++ {
		s := ""
		if i < end {
			s = lines[i]
		}
		b.WriteString(fit(s, m.width) + "\n")
	}
	hint := " any key closes"
	if len(lines) > body {
		hint = fmt.Sprintf(" ↑↓/jk scroll · lines %d–%d of %d · any other key closes", m.helpOff+1, end, len(lines))
	}
	b.WriteString(styleHint.Render(fit(hint, m.width)))
	return b.String()
}

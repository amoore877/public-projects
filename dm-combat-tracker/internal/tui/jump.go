package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// jumpPrompt lets the DM jump selection to any row (combatant or effect) by
// typing part of its name (FJ1–FJ3, D62–D63). Selection follows the active
// match live; enter keeps it and esc restores the starting selection.
type jumpPrompt struct {
	ed         lineEdit
	origSel    combat.RowID
	origOffset int
	matches    []combat.RowID
	idx        int
}

func (m *Model) promptJump() overlay {
	if len(m.sess.Encounter().Order()) == 0 {
		m.warn("Nothing to jump to")
		return nil
	}
	return &jumpPrompt{
		origSel:    m.selected,
		origOffset: m.offset,
	}
}

func (j *jumpPrompt) update(m *Model, k tea.KeyPressMsg) overlay {
	switch k.String() {
	case "esc", "ctrl+c":
		m.selected = j.origSel
		m.offset = j.origOffset
		m.syncSelection()
		return nil
	case "enter":
		return nil
	case "tab", "down", "ctrl+n":
		if len(j.matches) > 0 {
			j.idx = (j.idx + 1) % len(j.matches)
			j.jump(m)
		}
		return j
	case "shift+tab", "up", "ctrl+p":
		if len(j.matches) > 0 {
			j.idx = (j.idx - 1 + len(j.matches)) % len(j.matches)
			j.jump(m)
		}
		return j
	}
	if changed, _ := j.ed.handle(k); changed {
		j.refresh(m)
	}
	return j
}

func (j *jumpPrompt) paste(m *Model, s string) {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	j.ed.text = append(j.ed.text[:j.ed.pos], append(r, j.ed.text[j.ed.pos:]...)...)
	j.ed.pos += len(r)
	j.refresh(m)
}

func (j *jumpPrompt) refresh(m *Model) {
	q := strings.TrimSpace(j.ed.String())
	if q == "" {
		j.matches = nil
		j.idx = 0
		m.selected = j.origSel
		m.offset = j.origOffset
		m.syncSelection()
		return
	}
	j.matches = matchRows(m.sess.Encounter().Order(), q)
	j.idx = 0
	if len(j.matches) > 0 {
		j.jump(m)
	} else {
		m.selected = j.origSel
		m.offset = j.origOffset
		m.syncSelection()
	}
}

func (j *jumpPrompt) jump(m *Model) {
	m.selected = j.matches[j.idx]
	m.syncSelection()
}

func (j *jumpPrompt) line(width int) string {
	label := " / "
	return styleTitle.Render(label) + j.ed.render(max(1, width-ansi.StringWidth(label)))
}

func (j *jumpPrompt) hint() (string, bool) {
	q := strings.TrimSpace(j.ed.String())
	if q == "" {
		return " type a name to jump · enter ok · esc cancel", false
	}
	if len(j.matches) == 0 {
		return " No match · esc cancel", true
	}
	return fmt.Sprintf(" %d/%d · tab/↑↓ next · enter jump · esc cancel", j.idx+1, len(j.matches)), false
}

// matchRows returns the IDs of rows matching query in three ranked tiers,
// preserving table order within each tier (D63):
//  1. prefix (case-insensitive, spaces optional)
//  2. substring (case-insensitive, spaces optional)
//  3. subsequence / fuzzy (case-insensitive, spaces ignored)
func matchRows(rows []combat.Row, query string) []combat.RowID {
	q := strings.ToLower(strings.TrimSpace(query))
	qCompact := stripSpaces(q)
	if qCompact == "" {
		return nil
	}
	var prefix, substr, fuzzy []combat.RowID
	for _, r := range rows {
		name := strings.ToLower(r.Name())
		compact := stripSpaces(name)
		switch {
		case strings.HasPrefix(name, q) || strings.HasPrefix(compact, qCompact):
			prefix = append(prefix, r.ID())
		case strings.Contains(name, q) || strings.Contains(compact, qCompact):
			substr = append(substr, r.ID())
		case isSubsequence(compact, qCompact):
			fuzzy = append(fuzzy, r.ID())
		}
	}
	out := make([]combat.RowID, 0, len(prefix)+len(substr)+len(fuzzy))
	out = append(out, prefix...)
	out = append(out, substr...)
	out = append(out, fuzzy...)
	return out
}

func stripSpaces(s string) string {
	return strings.ReplaceAll(s, " ", "")
}

func isSubsequence(haystack, needle string) bool {
	nr := []rune(needle)
	i := 0
	for _, r := range haystack {
		if r == nr[i] {
			i++
			if i == len(nr) {
				return true
			}
		}
	}
	return false
}

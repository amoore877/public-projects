package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// rowCells is one table row's plain text; styling is applied at render time.
type rowCells struct {
	id      combat.RowID
	marker  string
	init    string
	name    string
	kind    string
	hp      string
	conds   string
	note    string
	current bool
	state   *combat.HealthState // nil when the creature is fine
	effect  bool
	tied    bool
}

// buildRows turns the encounter's turn order into display cells.
func buildRows(enc *combat.Encounter) []rowCells {
	ties := enc.Ties()
	order := enc.Order()
	out := make([]rowCells, 0, len(order))
	for _, r := range order {
		rc := rowCells{id: r.ID(), init: r.Initiative().String(), current: r.ID() == enc.Current}
		if rc.current {
			rc.marker = "▶"
		}
		if c := r.Combatant; c != nil {
			rc.name = c.Name
			rc.kind = strings.ToUpper(string(c.Kind))
			rc.state = enc.HealthState(c)
			rc.hp = hpText(c, rc.state)
			rc.conds = conditionsText(enc, c)
			rc.note = c.Note
			rc.tied = ties[c.ID]
			if rc.tied {
				rc.init += " !"
			}
		} else {
			fx := r.Effect
			rc.effect = true
			rc.name = "◆ " + fx.Name
			rc.kind = "FX"
			rc.conds = effectSummary(fx)
			rc.note = fx.Note
		}
		out = append(out, rc)
	}
	return out
}

// hpText is the HP cell: the health state's label ("DYING -4/22"), or a
// bloodied mark, then any temp HP (+5) and nonlethal total (nl6).
func hpText(c *combat.Combatant, state *combat.HealthState) string {
	s := fmt.Sprintf("%d/%d", c.HP, c.MaxHP)
	switch {
	case state != nil:
		s = state.Label() + " " + s
	case c.Bloodied():
		s += " ▼"
	}
	if c.TempHP > 0 {
		s += fmt.Sprintf(" +%d", c.TempHP)
	}
	if c.Nonlethal > 0 {
		s += fmt.Sprintf(" nl%d", c.Nonlethal)
	}
	return s
}

func conditionsText(enc *combat.Encounter, c *combat.Combatant) string {
	var parts []string
	for _, cond := range c.Conditions {
		if cond.Remaining != nil {
			parts = append(parts, fmt.Sprintf("%s (%d)", cond.Name, *cond.Remaining))
		} else {
			parts = append(parts, cond.Name)
		}
	}
	for _, fx := range enc.LinkedEffects(c.ID) {
		parts = append(parts, "◆ "+fx.Name)
	}
	return strings.Join(parts, ", ")
}

func effectSummary(fx *combat.Effect) string {
	var parts []string
	if fx.Remaining != nil {
		parts = append(parts, plural(*fx.Remaining, "rd", "rds"))
	}
	parts = append(parts, plural(len(fx.Targets), "target", "targets"))
	return strings.Join(parts, " · ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}

// column identifies a table column.
type column int

const (
	colMarker column = iota
	colInit
	colName
	colKind
	colHP
	colConds
	colNote
)

var headers = map[column]string{
	colMarker: "", colInit: "Init", colName: "Name", colKind: "Kind",
	colHP: "HP", colConds: "Conditions", colNote: "Note",
}

func (rc rowCells) get(c column) string {
	switch c {
	case colMarker:
		return rc.marker
	case colInit:
		return rc.init
	case colName:
		return rc.name
	case colKind:
		return rc.kind
	case colHP:
		return rc.hp
	case colConds:
		return rc.conds
	default:
		return rc.note
	}
}

// layout picks the visible columns and their content widths for a terminal
// width. Narrow terminals drop Note first, then Kind, then narrow the
// remaining columns; below minWidth nothing fits and the TUI shows a
// "too small" screen instead of the table.
type layout struct {
	cols   []column
	widths map[column]int
}

// tier is one way of laying out the table, tried widest first.
type tier struct {
	cols  []column
	fixed map[column]int // columns with a set width
	mins  map[column]int // flexible columns' minimum widths
}

var (
	fullFixed  = map[column]int{colMarker: 1, colInit: 6, colKind: 4, colHP: 11}
	fullMins   = map[column]int{colName: 10, colConds: 12, colNote: 8}
	tightFixed = map[column]int{colMarker: 1, colInit: 5, colHP: 9}
	tightMins  = map[column]int{colName: 6, colConds: 6}

	tiers = []tier{
		{[]column{colMarker, colInit, colName, colKind, colHP, colConds, colNote}, fullFixed, fullMins},
		{[]column{colMarker, colInit, colName, colKind, colHP, colConds}, fullFixed, fullMins},
		{[]column{colMarker, colInit, colName, colHP, colConds}, fullFixed, fullMins},
		{[]column{colMarker, colInit, colName, colHP, colConds}, tightFixed, tightMins},
	}
)

// need is the narrowest width the tier fits in.
func (t tier) need() int {
	n := 1 + 3*len(t.cols) // the left edge "│" plus " … │" around each cell
	for _, c := range t.cols {
		n += t.fixed[c] + t.mins[c]
	}
	return n
}

// minWidth is the narrowest terminal the table can be drawn in.
var minWidth = tiers[len(tiers)-1].need()

func computeLayout(total int) layout {
	t := tiers[len(tiers)-1]
	for _, cand := range tiers {
		if cand.need() <= total {
			t = cand
			break
		}
	}
	widths := map[column]int{}
	var flex []column
	for _, c := range t.cols {
		if w, ok := t.fixed[c]; ok {
			widths[c] = w
		} else {
			widths[c] = t.mins[c]
			flex = append(flex, c)
		}
	}
	// Share the spare width: Conditions gets the most, then Name, then Note.
	spare := max(0, total-t.need())
	share := map[column]int{colName: 3, colConds: 4, colNote: 2}
	parts := 0
	for _, c := range flex {
		parts += share[c]
	}
	given := 0
	for i, c := range flex {
		extra := spare * share[c] / parts
		if i == len(flex)-1 {
			extra = spare - given
		}
		given += extra
		widths[c] += extra
	}
	return layout{cols: t.cols, widths: widths}
}

// has reports whether the layout shows column c.
func (l layout) has(c column) bool {
	for _, lc := range l.cols {
		if lc == c {
			return true
		}
	}
	return false
}

// cut reports whether text doesn't show in full in column c: the column is
// dropped or too narrow.
func (l layout) cut(c column, text string) bool {
	return text != "" && (!l.has(c) || ansi.StringWidth(text) > l.widths[c])
}

// fit pads or truncates s to exactly w cells.
func fit(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
}

// trailingNumber matches a group member's number, as in "Goblin 12".
var trailingNumber = regexp.MustCompile(` \d+$`)

// fitName is fit for the Name column: when a name like "Goblin 12" doesn't
// fit, it shortens the base and keeps the number ("Go… 12"), so group
// members stay distinguishable in narrow terminals.
func fitName(s string, w int) string {
	num := trailingNumber.FindString(s)
	base := strings.TrimSuffix(s, num)
	if ansi.StringWidth(s) <= w || num == "" || w-len(num) < 3 {
		return fit(s, w)
	}
	return fit(ansi.Truncate(base, w-len(num), "…")+num, w)
}

func (l layout) border(left, mid, right string) string {
	var b strings.Builder
	b.WriteString(left)
	for i, c := range l.cols {
		b.WriteString(strings.Repeat("─", l.widths[c]+2))
		if i < len(l.cols)-1 {
			b.WriteString(mid)
		}
	}
	b.WriteString(right)
	return b.String()
}

// line renders one row. style, if non-nil, is applied to each cell's padded
// content (not the borders), so a highlighted row stays inside the grid.
func (l layout) line(text func(column) string, style func(column, string) string) string {
	var b strings.Builder
	b.WriteString("│")
	for _, c := range l.cols {
		var cell string
		if c == colName {
			cell = " " + fitName(text(c), l.widths[c]) + " "
		} else {
			cell = " " + fit(text(c), l.widths[c]) + " "
		}
		if style != nil {
			cell = style(c, cell)
		}
		b.WriteString(cell)
		b.WriteString("│")
	}
	return b.String()
}

package tui

import (
	"fmt"
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
	down    bool
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
			rc.hp = hpText(c)
			rc.conds = conditionsText(enc, c)
			rc.note = c.Note
			rc.down = c.Down()
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

func hpText(c *combat.Combatant) string {
	s := fmt.Sprintf("%d/%d", c.HP, c.MaxHP)
	switch {
	case c.Down():
		s = "DOWN " + s
	case c.Bloodied():
		s += " ▼"
	}
	if c.TempHP > 0 {
		s += fmt.Sprintf(" +%d", c.TempHP)
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
// width. Narrow terminals drop Note first, then Kind.
type layout struct {
	cols   []column
	widths map[column]int
}

const (
	minName  = 10
	minConds = 12
	minNote  = 8
)

func computeLayout(total int) layout {
	fixed := map[column]int{colMarker: 1, colInit: 6, colKind: 4, colHP: 11}
	try := func(cols []column) (layout, bool) {
		// Each column takes its width plus " │" padding; the left edge is "│ ".
		overhead := 1 + 3*len(cols)
		used := overhead
		var flex []column
		for _, c := range cols {
			if w, ok := fixed[c]; ok {
				used += w
			} else {
				flex = append(flex, c)
			}
		}
		mins := map[column]int{colName: minName, colConds: minConds, colNote: minNote}
		need := used
		for _, c := range flex {
			need += mins[c]
		}
		if need > total {
			return layout{}, false
		}
		widths := map[column]int{}
		for c, w := range fixed {
			widths[c] = w
		}
		// Share the spare width: Conditions gets the most, then Name, then Note.
		spare := total - need
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
			widths[c] = mins[c] + extra
		}
		return layout{cols: cols, widths: widths}, true
	}
	full := []column{colMarker, colInit, colName, colKind, colHP, colConds, colNote}
	if l, ok := try(full); ok {
		return l
	}
	noNote := []column{colMarker, colInit, colName, colKind, colHP, colConds}
	if l, ok := try(noNote); ok {
		return l
	}
	minimal := []column{colMarker, colInit, colName, colHP, colConds}
	if l, ok := try(minimal); ok {
		return l
	}
	// Too narrow for anything sensible: squeeze the flexible columns.
	return layout{cols: minimal, widths: map[column]int{colMarker: 1, colInit: 6, colName: minName, colHP: 11, colConds: minConds}}
}

// fit pads or truncates s to exactly w cells.
func fit(s string, w int) string {
	if ansi.StringWidth(s) > w {
		s = ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-ansi.StringWidth(s))
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
		cell := " " + fit(text(c), l.widths[c]) + " "
		if style != nil {
			cell = style(c, cell)
		}
		b.WriteString(cell)
		b.WriteString("│")
	}
	return b.String()
}

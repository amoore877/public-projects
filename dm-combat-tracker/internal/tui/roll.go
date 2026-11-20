package tui

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/dice"
)

// This file holds the dice roller (milestone 14a). A roll is a tool beside
// the fight: it never goes through session.Apply, so nothing is saved,
// logged or put on the undo stack (D45).

// errNoRoll refuses a repeat before anything has been rolled.
var errNoRoll = errors.New("nothing to roll yet; type dice, e.g. 2d6+3")

// newRand is the roller's random source, seeded from the clock.
func newRand() *rand.Rand {
	n := uint64(time.Now().UnixNano())
	return rand.New(rand.NewPCG(n, n>>1|1))
}

// promptRoll opens the r prompt. Enter rolls what was typed; enter on an
// empty prompt rolls the last expression again (D47).
func (m *Model) promptRoll() overlay {
	p := &inputPrompt{label: "Roll:", help: "e.g. 2d6+2+1d4 · d20 is 1d20"}
	if m.lastRoll != nil {
		p.help = "enter alone rolls " + m.lastRoll.String() + " again · e.g. 2d6+2+1d4"
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		var e *dice.Expr
		if !blank(text) {
			parsed, err := dice.Parse(text)
			if err != nil {
				return p, err
			}
			e = &parsed
		}
		if err := m.roll(e); err != nil {
			return p, err
		}
		return nil, nil
	}
	return p
}

// roll rolls e, or the last expression when e is nil, and shows the result
// in the status line. Nothing in the fight changes.
func (m *Model) roll(e *dice.Expr) error {
	if e == nil {
		e = m.lastRoll
	}
	if e == nil {
		return errNoRoll
	}
	m.lastRoll = e
	m.rolled = e.Roll(m.rng)
	m.info(rollText(m.rolled))
	return nil
}

// rollText is a roll as the status line shows it when there's room:
// "2d6+2+1d4 = 14 · [5 3] +2 [4]" (DR4, D48).
func rollText(r dice.Result) string { return r.String() + " · " + r.Breakdown() }

// rollLine fits a roll into width cells. The breakdown is cut first; if
// even "EXPR = TOTAL" doesn't fit, the expression is cut and the total kept.
func rollLine(r dice.Result, width int) string {
	full, head := rollText(r), r.String()
	switch {
	case ansi.StringWidth(full) <= width:
		return full
	case ansi.StringWidth(head+" · …") <= width:
		return ansi.Truncate(full, width, "…")
	case ansi.StringWidth(head) <= width:
		return head
	}
	total := fmt.Sprintf(" = %d", r.Total)
	return ansi.Truncate(r.Expr.String(), max(1, width-len(total)), "…") + total
}

func blank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

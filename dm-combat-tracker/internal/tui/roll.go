package tui

import (
	"errors"
	"math/rand/v2"
	"time"

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
	m.info(e.Roll(m.rng).String())
	return nil
}

func blank(s string) bool {
	for _, r := range s {
		if r != ' ' && r != '\t' {
			return false
		}
	}
	return true
}

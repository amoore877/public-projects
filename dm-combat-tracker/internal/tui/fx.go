package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// This file holds the effect keys (milestone 5): e starts an effect, T edits
// its targets, x on an effect row ends it early.

// targetPicker is a checklist drawn on the table itself (D26): the selection
// moves as usual and space toggles the selected combatant, so the DM picks
// targets while seeing their HP and conditions.
type targetPicker struct {
	enc     func() *combat.Encounter
	title   string
	checked map[combat.RowID]bool
	back    combat.RowID // the selection to put back if the picker is cancelled
	done    func(m *Model, targets []combat.RowID) overlay
}

func newTargetPicker(m *Model, title string, initial []combat.RowID, done func(*Model, []combat.RowID) overlay) *targetPicker {
	p := &targetPicker{enc: m.sess.Encounter, title: title, checked: map[combat.RowID]bool{}, back: m.selected, done: done}
	for _, id := range initial {
		p.checked[id] = true
	}
	return p
}

func (p *targetPicker) update(m *Model, k tea.KeyPressMsg) overlay {
	switch k.String() {
	case "esc", "ctrl+c":
		m.selected = p.back
		m.syncSelection()
		return nil
	case "up", "k":
		m.move(-1)
	case "down", "j":
		m.move(1)
	case "home", "g":
		m.moveTo(0)
	case "end", "G":
		m.moveTo(len(m.sess.Encounter().Order()) - 1)
	case "space":
		if _, ok := m.sess.Encounter().Combatant(m.selected); ok {
			p.checked[m.selected] = !p.checked[m.selected]
			m.move(1) // like a checklist: tick and move on
		}
	case "enter":
		return p.done(m, p.targets(m.sess.Encounter()))
	}
	return p
}

// targets returns the checked combatants in turn order.
func (p *targetPicker) targets(enc *combat.Encounter) []combat.RowID {
	var out []combat.RowID
	for _, r := range enc.Order() {
		if r.Combatant != nil && p.checked[r.ID()] {
			out = append(out, r.ID())
		}
	}
	return out
}

func (p *targetPicker) names(enc *combat.Encounter) string {
	var out []string
	for _, id := range p.targets(enc) {
		if c, ok := enc.Combatant(id); ok {
			out = append(out, c.Name)
		}
	}
	if len(out) == 0 {
		return "none"
	}
	return strings.Join(out, ", ")
}

func (p *targetPicker) line(width int) string {
	return fit(styleTitle.Render(" "+p.title+": ")+p.names(p.enc()), width)
}

func (p *targetPicker) hint() (string, bool) {
	return " ↑↓/jk move · space toggle · enter done · esc cancel", false
}

// marker is what the picker shows in the marker column of a row.
func (p *targetPicker) marker(enc *combat.Encounter, id combat.RowID) string {
	if _, ok := enc.Combatant(id); !ok {
		return ""
	}
	if p.checked[id] {
		return "✓"
	}
	return "·"
}

// --- new effect -----------------------------------------------------------

func (m *Model) promptNewEffect() overlay {
	enc := m.sess.Encounter()
	if len(enc.Combatants) == 0 {
		m.warn("Add combatants first")
		return nil
	}
	back := m.selected
	var (
		spec   combat.EffectSpec
		source string // name, "" for none
	)
	var suggested *combat.Combatant
	if c, ok := enc.Combatant(m.selected); ok {
		suggested = c
	}

	finish := func(m *Model, targets []combat.RowID) overlay {
		spec.Targets = targets
		var id combat.RowID
		ok := m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			var evs []combat.Event
			var err error
			id, evs, err = e.AddEffect(spec)
			return evs, err
		})
		if ok {
			m.selected = id
		} else {
			m.selected = back
		}
		m.syncSelection()
		return nil
	}

	who := func() string {
		if source == "" {
			return spec.Name
		}
		return spec.Name + " (" + source + ")"
	}

	dur := &inputPrompt{help: "rounds (e.g. 10 or 1e for end of turn); blank = until ended"}
	dur.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		spec.Rounds = nil
		spec.EndOfTurn = false
		if text != "" {
			n, eot, err := combat.ParseDuration(text)
			if err != nil {
				return dur, err
			}
			spec.Rounds, spec.EndOfTurn = &n, eot
		}
		return newTargetPicker(m, who()+" · pick targets", nil, finish), nil
	}

	initStep := &inputPrompt{help: "no source, so it needs its own place in the order (lair actions: 20)"}
	initStep.submit = func(m *Model, text string) (overlay, error) {
		v, err := combat.ParseInitiative(text)
		if err != nil {
			return initStep, err
		}
		spec.Initiative = &v
		dur.label = who() + " · duration:"
		return dur, nil
	}

	noSource := func(m *Model) overlay {
		spec.SourceID, source = 0, ""
		initStep.label = who() + " · initiative:"
		return initStep
	}
	withSource := func(m *Model) overlay {
		spec.SourceID, source = suggested.ID, suggested.Name
		dur.label = who() + " · duration:"
		return dur
	}

	name := &inputPrompt{label: "New effect · name:", help: "e.g. Spirit Guardians, Wall of Fire"}
	name.submit = func(m *Model, text string) (overlay, error) {
		spec.Name = strings.TrimSpace(text)
		if spec.Name == "" {
			return name, fmt.Errorf("Enter a name")
		}
		if suggested == nil {
			return noSource(m), nil
		}
		return &choicePrompt{
			question: spec.Name + " · source?",
			enter:    "s",
			choices: []choice{
				{key: "s", label: suggested.Name + ", right after their turn", run: withSource},
				{key: "n", label: "none (lair / environment)", run: noSource},
			},
		}, nil
	}
	return name
}

// --- edit targets ---------------------------------------------------------

func (m *Model) promptTargets() overlay {
	fx, ok := m.sess.Encounter().Effect(m.selected)
	if !ok {
		m.warn("Select an effect row to change its targets")
		return nil
	}
	id, name := fx.ID, fx.Name
	before := append([]combat.RowID(nil), fx.Targets...)
	return newTargetPicker(m, name+" · targets", before, func(m *Model, after []combat.RowID) overlay {
		added, removed := diffIDs(before, after)
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			var evs []combat.Event
			if len(added) > 0 {
				a, err := e.AddTargets(id, added)
				if err != nil {
					return nil, err
				}
				evs = append(evs, a...)
			}
			if len(removed) > 0 {
				r, err := e.RemoveTargets(id, removed)
				if err != nil {
					return evs, err
				}
				evs = append(evs, r...)
			}
			return evs, nil
		})
		m.selected = id
		m.syncSelection()
		return nil
	})
}

// diffIDs returns the IDs in after but not before, and in before but not after.
func diffIDs(before, after []combat.RowID) (added, removed []combat.RowID) {
	in := func(ids []combat.RowID, id combat.RowID) bool {
		for _, x := range ids {
			if x == id {
				return true
			}
		}
		return false
	}
	for _, id := range after {
		if !in(before, id) {
			added = append(added, id)
		}
	}
	for _, id := range before {
		if !in(after, id) {
			removed = append(removed, id)
		}
	}
	return added, removed
}

// --- end early ------------------------------------------------------------

func (m *Model) promptEndEffect(fx *combat.Effect) overlay {
	id, name := fx.ID, fx.Name
	p := &inputPrompt{label: "End " + name + " early · reason:", help: "optional, e.g. concentration broken"}
	p.submit = func(m *Model, text string) (overlay, error) {
		wasTurn := m.sess.Encounter().Current == id
		if m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.EndEffect(id, text) }) && wasTurn {
			m.selected = m.sess.Encounter().Current // the turn passed on (D22)
		}
		m.syncSelection()
		return nil, nil
	}
	return p
}

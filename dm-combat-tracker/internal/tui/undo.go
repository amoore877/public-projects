package tui

import (
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/journal"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
)

// undo takes back the last change, or with redo makes the last undone one
// again (UR1–UR3, D44). The status line names the change, and the
// selection moves to the row it touched.
func (m *Model) undo(redo bool) {
	m.msg = ""
	var out session.Outcome
	var err error
	if redo {
		out, err = m.sess.Redo()
	} else {
		out, err = m.sess.Undo()
	}
	if err != nil {
		if redo {
			m.info("Nothing to redo")
		} else {
			m.info("Nothing to undo")
		}
		return
	}
	m.track(out)
	if id := m.undoFocus(out.Events); id != 0 {
		m.selected = id
	}
	m.syncSelection()
	if m.msg != "" {
		return // a save or log recovery to report instead
	}
	summary := journal.Summary(out.Events)
	if redo {
		m.info("Redid: " + summary + " · u undoes")
	} else {
		m.info("Undid: " + summary + " · U redoes")
	}
}

// undoFocus picks the row to select after an undo or redo: the turn holder
// for a change of turn (D22), otherwise the first row the change named, if
// it still exists. Zero leaves the selection alone.
func (m *Model) undoFocus(evs []combat.Event) combat.RowID {
	enc := m.sess.Encounter()
	for _, ev := range evs {
		switch ev.(type) {
		case combat.TurnStarted, combat.TurnRewound, combat.CombatStarted:
			return enc.Current
		}
	}
	for _, ev := range evs {
		if id := eventRow(ev); id != 0 {
			if _, ok := enc.Row(id); ok {
				return id
			}
		}
	}
	return 0
}

// eventRow is the row an event is about, or zero.
func eventRow(ev combat.Event) combat.RowID {
	switch e := ev.(type) {
	case combat.CombatantAdded:
		return e.ID
	case combat.CombatantRemoved:
		return e.ID
	case combat.InitiativeChanged:
		return e.ID
	case combat.RowNoteSet:
		return e.ID
	case combat.RowRenamed:
		return e.ID
	case combat.Damaged:
		return e.ID
	case combat.Healed:
		return e.ID
	case combat.NonlethalDamaged:
		return e.ID
	case combat.NonlethalSet:
		return e.ID
	case combat.MaxHPSet:
		return e.ID
	case combat.HPSet:
		return e.ID
	case combat.TempHPSet:
		return e.ID
	case combat.HealthStateChanged:
		return e.ID
	case combat.ConditionApplied:
		return e.TargetID
	case combat.ConditionRemoved:
		return e.TargetID
	case combat.ConditionExpired:
		return e.TargetID
	case combat.EffectStarted:
		return e.ID
	case combat.EffectTargetsChanged:
		return e.ID
	case combat.EffectEnded:
		return e.ID
	case combat.EffectExpired:
		return e.ID
	case combat.ResourceChanged:
		return e.ID
	case combat.ResourceRemoved:
		return e.ID
	}
	return 0
}

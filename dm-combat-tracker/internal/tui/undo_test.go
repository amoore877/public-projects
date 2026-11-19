package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
)

func hpOf(t *testing.T, s *session.Session, name string) int {
	t.Helper()
	c, ok := s.Encounter().Combatant(idOf(t, s, name))
	if !ok {
		t.Fatalf("no %s", name)
	}
	return c.HP
}

// u takes back the last change and U (or ctrl+r) makes it again; the status
// line names it and the selection follows it (UR1–UR3, D44).
func TestUndoRedoKeys(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	g1 := idOf(t, s, "Goblin 1")
	m.selected = g1
	press(m, "d")
	typeText(m, "4")
	press(m, "enter")
	if hpOf(t, s, "Goblin 1") != 3 {
		t.Fatalf("damage: HP %d", hpOf(t, s, "Goblin 1"))
	}

	m.selected = idOf(t, s, "Thorin")
	press(m, "u")
	if hpOf(t, s, "Goblin 1") != 7 || m.Selected() != g1 || m.level != levelInfo ||
		m.msg != "Undid: Goblin 1 took 4 damage · U redoes" {
		t.Errorf("undo: HP %d, selected %d, msg %q", hpOf(t, s, "Goblin 1"), m.Selected(), m.msg)
	}
	if !strings.Contains(screen(m), "Undid: Goblin 1 took 4 damage") {
		t.Errorf("status line:\n%s", screen(m))
	}
	press(m, "U")
	if hpOf(t, s, "Goblin 1") != 3 || m.msg != "Redid: Goblin 1 took 4 damage · u undoes" {
		t.Errorf("redo: HP %d, msg %q", hpOf(t, s, "Goblin 1"), m.msg)
	}
	press(m, "U")
	if m.msg != "Nothing to redo" {
		t.Errorf("msg %q", m.msg)
	}

	// Undoing a turn selects the turn holder it gives back.
	press(m, "n")
	m.selected = idOf(t, s, "Mira")
	press(m, "u")
	if enc := s.Encounter(); enc.Current != idOf(t, s, "Thorin") || m.Selected() != enc.Current ||
		!strings.HasPrefix(m.msg, "Undid: Turn passed to Goblin 1") {
		t.Errorf("undo n: current %d, selected %d, msg %q", enc.Current, m.Selected(), m.msg)
	}
	press(m, "ctrl+r")
	if enc := s.Encounter(); enc.Current != idOf(t, s, "Goblin 1") || m.Selected() != enc.Current {
		t.Errorf("ctrl+r: current %d, selected %d", enc.Current, m.Selected())
	}

	// A removed combatant comes back, with its effect, and is selected.
	m.selected = idOf(t, s, "Mira")
	press(m, "x", "y", "e") // remove, and end the effects she is the source of
	if _, ok := s.Encounter().Combatant(idOf0(s, "Mira")); ok {
		t.Fatalf("Mira should be gone:\n%s", screen(m))
	}
	press(m, "u")
	if m.Selected() != idOf(t, s, "Mira") || !strings.HasPrefix(m.msg, "Undid: Mira removed from combat") {
		t.Errorf("undo remove: selected %d, msg %q", m.Selected(), m.msg)
	}
	if len(s.Encounter().Effects) != 1 || !strings.Contains(screen(m), "Spirit") {
		t.Errorf("effects after undo: %d", len(s.Encounter().Effects))
	}
}

// idOf0 is idOf without failing: zero when the row is gone.
func idOf0(s *session.Session, name string) combat.RowID {
	for _, r := range s.Encounter().Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	return 0
}

func TestNothingToUndo(t *testing.T) {
	s := newSession(t)
	m := New(s, "")
	press(m, "u")
	if m.msg != "Nothing to undo" || m.level != levelInfo {
		t.Errorf("msg %q", m.msg)
	}
}

func TestUndoCommands(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44))
	m := New(s, "")
	runCmd(m, "add Goblin x3 12 7")
	if len(s.Encounter().Combatants) != 4 {
		t.Fatalf("combatants %d", len(s.Encounter().Combatants))
	}
	runCmd(m, "undo")
	if len(s.Encounter().Combatants) != 1 || m.msg != "Undid: Goblin 1 joined at initiative 12.3 (7/7 HP) (and 2 more) · U redoes" {
		t.Errorf("combatants %d, msg %q", len(s.Encounter().Combatants), m.msg)
	}
	if m.Selected() != idOf(t, s, "Thorin") {
		t.Errorf("the goblins are gone, so the selection stays on a row that exists, got %d", m.Selected())
	}
	runCmd(m, "redo")
	if len(s.Encounter().Combatants) != 4 || m.Selected() != idOf(t, s, "Goblin 1") {
		t.Errorf("combatants %d, selected %d", len(s.Encounter().Combatants), m.Selected())
	}
}

// A save failure during an undo shows the banner as for any change.
func TestUndoWithSaveFailing(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), add("Goblin", combat.NPC, "12", 7), start)
	m := New(s, "")
	press(m, "n")
	m.saveErr = errFake
	press(m, "u")
	if m.saveErr != nil || !strings.Contains(m.msg, "Autosave is working again") {
		t.Errorf("save err %v, msg %q", m.saveErr, m.msg)
	}
}

var errFake = errors.New("disk full")

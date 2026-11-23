package session

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// rowID finds a row by name in whatever encounter the session holds now.
func rowID(e *combat.Encounter, name string) combat.RowID {
	for _, r := range e.Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	panic("no row " + name)
}

func rounds(n int) *int { return &n }

func evsOnly[T any](_ T, evs []combat.Event, err error) ([]combat.Event, error) { return evs, err }

// state is the session's encounter as JSON, without the log path, which an
// undo keeps once it is bound.
func state(t *testing.T, s *Session) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(snapshot(s.Encounter()), &m); err != nil {
		t.Fatal(err)
	}
	delete(m, "logPath")
	return m
}

// Every kind of change, in a 3.5e fight so nonlethal and fixed-count effects
// are covered too: after each one, undo must give back exactly the state
// before it, and redo exactly the state after it (UR1, UR2, D39).
func TestUndoRedoEveryChange(t *testing.T) {
	c, cfg := setup(t)
	s, _ := open(t, cfg)
	type change struct {
		name string
		op   Op
	}
	id := rowID
	changes := []change{
		{"ruleset", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetRuleset("3.5e") }},
		{"setup note", note("the bridge is out")},
		{"add", addOp("Mira", 15, 27)},
		{"add group", func(e *combat.Encounter) ([]combat.Event, error) {
			return evsOnly(e.AddGroup(combat.GroupSpec{CombatantSpec: combat.CombatantSpec{Name: "Goblin", Initiative: combat.InitiativeFromInt(12), MaxHP: 7}, Count: 3}))
		}},
		{"add wolf", addOp("Wolf", 5, 13)},
		{"condition in setup", func(e *combat.Encounter) ([]combat.Event, error) {
			return evsOnly(e.AddCondition(id(e, "Wolf"), "Prone", nil, ""))
		}},
		{"start", start},
		{"damage", damage("Goblin 1", 4)},
		{"damage to dying", damage("Goblin 2", 9)},
		{"heal", func(e *combat.Encounter) ([]combat.Event, error) { return e.Heal(id(e, "Goblin 1"), 2) }},
		{"set hp", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetHP(id(e, "Goblin 3"), 0) }},
		{"temp hp", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetTempHP(id(e, "Mira"), 5) }},
		{"add temp hp", func(e *combat.Encounter) ([]combat.Event, error) { return e.AddTempHP(id(e, "Mira"), 3) }},
		{"max hp", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetMaxHP(id(e, "Mira"), 20) }},
		{"nonlethal", func(e *combat.Encounter) ([]combat.Event, error) { return e.NonlethalDamage(id(e, "Wolf"), 13) }},
		{"set nonlethal", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetNonlethal(id(e, "Wolf"), 2) }},
		{"sourced condition", func(e *combat.Encounter) ([]combat.Event, error) {
			return evsOnly(e.AddConditionFrom(id(e, "Goblin 1"), "Dazed", rounds(1), id(e, "Mira"), ""))
		}},
		{"remove condition", func(e *combat.Encounter) ([]combat.Event, error) {
			w, _ := e.Combatant(id(e, "Wolf"))
			return e.RemoveCondition(w.ID, w.Conditions[0].ID)
		}},
		{"effect", func(e *combat.Encounter) ([]combat.Event, error) {
			return evsOnly(e.AddEffect(combat.EffectSpec{Name: "Fog", SourceID: id(e, "Mira"), Rounds: rounds(3), Targets: []combat.RowID{id(e, "Goblin 1")}}))
		}},
		{"add targets", func(e *combat.Encounter) ([]combat.Event, error) {
			return e.AddTargets(id(e, "Fog"), []combat.RowID{id(e, "Goblin 3"), id(e, "Wolf")})
		}},
		{"remove targets", func(e *combat.Encounter) ([]combat.Event, error) {
			return e.RemoveTargets(id(e, "Fog"), []combat.RowID{id(e, "Wolf")})
		}},
		{"note on a row", func(e *combat.Encounter) ([]combat.Event, error) { return e.SetNote(id(e, "Wolf"), "hungry") }},
		{"log note", note("Mira shouts a warning")},
		{"delay moving effects", func(e *combat.Encounter) ([]combat.Event, error) {
			v, _ := combat.ParseInitiative("4")
			return e.SetInitiative(id(e, "Mira"), v, combat.LeaveEffects)
		}},
		{"next", next},
		{"next again", next},
		{"previous", func(e *combat.Encounter) ([]combat.Event, error) { return e.Previous() }},
		{"next into round 2", next},
		{"next", next},
		{"end effect", func(e *combat.Encounter) ([]combat.Event, error) { return e.EndEffect(id(e, "Fog"), "dispelled") }},
		{"remove", func(e *combat.Encounter) ([]combat.Event, error) {
			return e.Remove(id(e, "Goblin 2"), combat.EndEffects)
		}},
	}
	for _, ch := range changes {
		before := state(t, s)
		c.now = c.now.Add(60e9)
		apply(t, s, ch.op)
		after := state(t, s)
		if reflect.DeepEqual(before, after) && !strings.HasSuffix(ch.name, " note") { // a log note changes only the log
			t.Fatalf("%s: changed nothing", ch.name)
		}
		out, err := s.Undo()
		if err != nil || !out.Undid || len(out.Events) == 0 || out.SaveErr != nil || out.LogErr != nil {
			t.Fatalf("%s: undo %+v, %v", ch.name, out, err)
		}
		if got := state(t, s); !reflect.DeepEqual(got, before) {
			t.Fatalf("%s: undo left\n%v\nwant\n%v", ch.name, got, before)
		}
		if saved := readFile(t, DefaultStatePath); !strings.Contains(saved, `"schemaVersion"`) {
			t.Fatalf("%s: state file %q", ch.name, saved)
		}
		out, err = s.Redo()
		if err != nil || !out.Redid || out.SaveErr != nil || out.LogErr != nil {
			t.Fatalf("%s: redo %+v, %v", ch.name, out, err)
		}
		if got := state(t, s); !reflect.DeepEqual(got, after) {
			t.Fatalf("%s: redo left\n%v\nwant\n%v", ch.name, got, after)
		}
	}
	if u, r := s.UndoDepth(); u != len(changes) || r != 0 {
		t.Errorf("depth %d/%d", u, r)
	}

	// Undo all the way back to an empty Setup, then forward again.
	for range changes {
		if _, err := s.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if e := s.Encounter(); e.Status != combat.Setup || len(e.Combatants) != 0 || e.Ruleset != "5e" {
		t.Errorf("back at the start: %+v", e)
	}
	if _, err := s.Undo(); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("err %v", err)
	}
	for range changes {
		if _, err := s.Redo(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Redo(); !errors.Is(err, ErrNothingToRedo) {
		t.Errorf("err %v", err)
	}
	if len(s.Encounter().Combatants) != 4 {
		t.Errorf("after redoing everything: %d combatants", len(s.Encounter().Combatants))
	}
}

// A new change drops what's left to redo; a refused or empty change keeps
// it (UR2, D40).
func TestNewChangeDropsRedo(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 27))
	apply(t, s, addOp("Orc", 12, 15))
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(damage("Nobody", 3)); err == nil {
		t.Fatal("expected a refusal")
	}
	if _, err := s.Apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.SetRuleset("5e") }); err != nil {
		t.Fatal(err) // no events: nothing recorded
	}
	if u, r := s.UndoDepth(); u != 1 || r != 1 {
		t.Fatalf("depth %d/%d", u, r)
	}
	apply(t, s, addOp("Wolf", 5, 13))
	if u, r := s.UndoDepth(); u != 2 || r != 0 {
		t.Errorf("depth %d/%d", u, r)
	}
}

func TestUndoLimit(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 1000))
	apply(t, s, start)
	for range UndoLimit + 5 {
		apply(t, s, damage("Mira", 1))
	}
	if u, _ := s.UndoDepth(); u != UndoLimit {
		t.Fatalf("depth %d", u)
	}
	for range UndoLimit {
		if _, err := s.Undo(); err != nil {
			t.Fatal(err)
		}
	}
	if c := s.Encounter().Combatants[0]; c.HP != 995 {
		t.Errorf("oldest kept step leaves HP %d, want 995", c.HP)
	}
}

// Ending combat and restarting both start the history again (D43).
func TestUndoBarriers(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 27))
	apply(t, s, start)
	apply(t, s, end)
	if u, r := s.UndoDepth(); u != 0 || r != 0 {
		t.Errorf("after End: %d/%d", u, r)
	}
	apply(t, s, addOp("Orc", 12, 15))
	s.Close()

	s2, resumed := open(t, cfg)
	if u, _ := s2.UndoDepth(); !resumed || u != 0 {
		t.Errorf("after restart: resumed %v, depth %d", resumed, u)
	}
}

func TestApplyMerged(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, addOp("Thorin", 0, 44))
	apply(t, s, addOp("Mira", 0, 27))
	apply(t, s, addOp("Orc", 0, 15))
	baseDepth, _ := s.UndoDepth()

	setInit := func(idx int, v int) Op {
		return func(e *combat.Encounter) ([]combat.Event, error) {
			return e.SetInitiativeDirect(e.Combatants[idx].ID, combat.InitiativeFromInt(v))
		}
	}
	if _, err := s.Apply(setInit(0, 18)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyMerged(setInit(1, 14)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ApplyMerged(setInit(2, 9)); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.UndoDepth(); u != baseDepth+1 {
		t.Fatalf("merged pass should add 1 undo step, got %d (base %d)", u, baseDepth)
	}
	out, err := s.Undo()
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Events) != 3 {
		t.Errorf("undid %d events, want 3", len(out.Events))
	}
	for _, c := range s.Encounter().Combatants {
		if c.Initiative != 0 {
			t.Errorf("%s initiative = %s after undo, want 0", c.Name, c.Initiative)
		}
	}
	if _, err := s.Redo(); err != nil {
		t.Fatal(err)
	}
	if s.Encounter().Combatants[0].Initiative != combat.InitiativeFromInt(18) ||
		s.Encounter().Combatants[1].Initiative != combat.InitiativeFromInt(14) ||
		s.Encounter().Combatants[2].Initiative != combat.InitiativeFromInt(9) {
		t.Errorf("redo didn't restore all initiatives")
	}
}

// The log stays append-only: an undone step that wrote lines gets an
// "Undone" line, a redo writes them again marked, and quiet steps stay
// quiet (D42).
func TestUndoInTheLog(t *testing.T) {
	c, cfg := setup(t)
	cfg.LogPath = "log.md"
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 27))
	apply(t, s, addOp("Orc", 12, 5))
	apply(t, s, start)
	apply(t, s, next) // Orc's turn: nothing logged
	if _, err := s.Undo(); err != nil {
		t.Fatal(err)
	}
	c.now = c.now.Add(3 * 60e9)
	apply(t, s, damage("Orc", 5))  // drops: one line
	apply(t, s, damage("Mira", 3)) // arithmetic: no line
	s.Undo()
	s.Undo()
	c.now = c.now.Add(60e9)
	s.Redo()
	apply(t, s, next)
	apply(t, s, next) // round 2: a heading
	s.Undo()
	apply(t, s, next) // the heading is written again

	got := readFile(t, "log.md")
	want := []string{
		"- `19:42` **[R1 · Mira]** Combat started: Mira (15), Orc (12)",
		"- `19:45` **[R1 · Mira]** Orc dropped to 0 HP",
		"- `19:45` **[R1 · Mira]** Undone: Orc dropped to 0 HP",
		"- `19:46` **[R1 · Mira]** Orc dropped to 0 HP (redone)",
		"## Round 2",
		"- `19:46` **[R2 · Mira]** Undone: Round 2 started",
		"## Round 2",
	}
	at := 0
	for _, w := range want {
		i := strings.Index(got[at:], w)
		if i < 0 {
			t.Fatalf("log is missing %q after offset %d:\n%s", w, at, got)
		}
		at += i + len(w)
	}
	if strings.Count(got, "Undone") != 2 || strings.Contains(got, "Mira took") {
		t.Errorf("log:\n%s", got)
	}
}

// Undo and redo save the state; a save failure is reported, not fatal.
func TestUndoSaveFailure(t *testing.T) {
	_, cfg := setup(t)
	cfg.StatePath = "state/fight.json"
	if err := os.Mkdir("state", 0o755); err != nil {
		t.Fatal(err)
	}
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 27))
	if err := os.Chmod("state", 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod("state", 0o755) })
	out, err := s.Undo()
	if err != nil || (out.SaveErr == nil && os.Getuid() != 0) {
		t.Errorf("undo: %+v, %v", out, err)
	}
	if len(s.Encounter().Combatants) != 0 {
		t.Error("the undo should still apply in memory")
	}
}

// An undo line for a log that can't be opened is reported as a log error.
func TestUndoLogFailure(t *testing.T) {
	_, cfg := setup(t)
	cfg.LogPath = "logs/log.md" // the folder doesn't exist
	s, _ := open(t, cfg)
	apply(t, s, addOp("Mira", 15, 27))
	out, _ := s.Apply(start)
	if out.LogErr == nil {
		t.Fatal("expected the log to fail")
	}
	// The start wasn't logged, so its undo writes nothing either.
	if out, err := s.Undo(); err != nil || out.LogErr != nil || out.Logged {
		t.Errorf("undo: %+v, %v", out, err)
	}
	if err := os.Mkdir("logs", 0o755); err != nil {
		t.Fatal(err)
	}
	apply(t, s, start)
	s.closeLog()
	if err := os.RemoveAll("logs"); err != nil {
		t.Fatal(err)
	}
	if out, err := s.Undo(); err != nil || out.LogErr == nil {
		t.Errorf("undo with the log gone: %+v, %v", out, err)
	}
}

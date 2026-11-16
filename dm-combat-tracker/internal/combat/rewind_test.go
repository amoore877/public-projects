package combat

import (
	"errors"
	"strings"
	"testing"
)

// A rewind restores an expired effect, but not onto rows that have left
// combat since: removed targets are dropped.
func TestPreviousRestoresExpiredEffectWithoutRemovedRows(t *testing.T) {
	e := newEnc()
	mira := add(t, e, "Mira", "15", 27)
	gob := add(t, e, "Goblin", "12", 7)
	orc := add(t, e, "Orc", "10", 15)
	fx, _, err := e.AddEffect(EffectSpec{Name: "Web", SourceID: mira, Rounds: rounds(1), Targets: []RowID{gob, orc}})
	if err != nil {
		t.Fatal(err)
	}
	must(t)(e.Start())
	for range 4 { // Web, Goblin, Orc, then Mira's round-2 turn, where Web expires
		must(t)(e.Next())
	}
	if _, ok := e.Effect(fx); ok || currentName(e) != "Mira" || e.Round != 2 {
		t.Fatalf("web should have expired as Mira's turn began, turn is %s", currentName(e))
	}
	// Orc (the previous turn holder) must stay, or the rewind is refused.
	must(t)(e.Remove(gob, AskAboutEffects))
	must(t)(e.Previous())
	web, ok := e.Effect(fx)
	if !ok || *web.Remaining != 1 || len(web.Targets) != 1 || web.Targets[0] != orc || web.SourceID != mira {
		t.Fatalf("restored web: %+v", web)
	}
	if currentName(e) != "Orc" || e.Round != 1 {
		t.Errorf("turn = %s, round %d; want Orc in round 1", currentName(e), e.Round)
	}
}

// A state file saved by an earlier build can hold a turn record whose expired
// effect names a source that has since left; the rewind restores it unsourced.
func TestPreviousClearsRemovedSourceOfRestoredEffect(t *testing.T) {
	e := newEnc()
	thorin := add(t, e, "Thorin", "14", 44)
	gob := add(t, e, "Goblin", "12", 7)
	must(t)(e.Start())
	must(t)(e.Next()) // Goblin
	e.History[0].ExpiredEffects = []Effect{{ID: 90, Name: "Web", SourceID: 91, Remaining: rounds(1), Targets: []RowID{gob, 92}}}
	must(t)(e.Previous())
	web, ok := e.Effect(90)
	if !ok || web.SourceID != 0 || len(web.Targets) != 1 || e.Current != thorin {
		t.Fatalf("restored web: %+v, turn %s", web, currentName(e))
	}
}

func ptrInit(v Initiative) *Initiative { return &v }

// Conditions and effects that were removed by hand since the advance are
// skipped when its countdowns are put back.
func TestPreviousSkipsItemsRemovedSince(t *testing.T) {
	e := newEnc()
	thorin := add(t, e, "Thorin", "18", 44)
	gob := add(t, e, "Goblin", "12", 7)
	cond, _, _ := e.AddCondition(gob, "Prone", rounds(3), "")
	_, _, _ = e.AddCondition(gob, "Stunned", rounds(1), "")
	fx, _, _ := e.AddEffect(EffectSpec{Name: "Bless", SourceID: thorin, Initiative: ptrInit(ini(t, "5")), Rounds: rounds(3)})
	must(t)(e.Start())
	must(t)(e.Next()) // Goblin: Prone 3→2, Stunned expires
	must(t)(e.Next()) // Bless
	must(t)(e.RemoveCondition(gob, cond))
	must(t)(e.EndEffect(fx, "")) // Bless held the turn: it passes to Thorin and history clears
	if _, err := e.Previous(); !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("err = %v, want nothing to undo after the turn holder was removed", err)
	}

	// Same again, but end things that don't hold the turn.
	e = newEnc()
	thorin = add(t, e, "Thorin", "18", 44)
	gob = add(t, e, "Goblin", "12", 7)
	cond, _, _ = e.AddCondition(gob, "Prone", rounds(3), "")
	must(t)(e.Start())
	must(t)(e.Next()) // Goblin: Prone 3→2
	must(t)(e.RemoveCondition(gob, cond))
	must(t)(e.Previous())
	if g, _ := e.Combatant(gob); len(g.Conditions) != 0 || e.Current != thorin {
		t.Errorf("removed condition came back: %+v", g.Conditions)
	}
}

// An expired condition whose owner has left is not restored.
func TestPreviousSkipsConditionOfRemovedOwner(t *testing.T) {
	e := newEnc()
	add(t, e, "Thorin", "18", 44)
	gob := add(t, e, "Goblin", "12", 7)
	add(t, e, "Orc", "10", 15)
	_, _, _ = e.AddCondition(gob, "Stunned", rounds(1), "")
	must(t)(e.Start())
	must(t)(e.Next()) // Goblin: Stunned expires
	must(t)(e.Next()) // Orc
	must(t)(e.Remove(gob, AskAboutEffects))
	if _, err := e.Previous(); !errors.Is(err, ErrNothingToUndo) || !strings.Contains(err.Error(), "removed") {
		t.Fatalf("err = %v, want a refusal naming the removed row", err)
	}
	if len(e.History) != 0 {
		t.Error("history should be cleared after a refused rewind")
	}
}

func TestTurnControlsNeedActiveCombat(t *testing.T) {
	e := newEnc()
	_, err := e.Next()
	wantErr(t, err, ErrNotActive)
	_, err = e.Previous()
	wantErr(t, err, ErrNotActive)
	_, err = e.Start()
	wantErr(t, err, ErrEmpty)
	add(t, e, "Thorin", "18", 44)
	must(t)(e.Start())
	_, err = e.Start()
	wantErr(t, err, ErrNotSetup)
	_, err = e.Previous()
	wantErr(t, err, ErrNothingToUndo)
}

// Removing the last row leaves nobody holding the turn; a later add and
// advance picks the fight back up.
func TestRemoveLastRowThenCarryOn(t *testing.T) {
	e := newEnc()
	thorin := add(t, e, "Thorin", "18", 44)
	must(t)(e.Start())
	must(t)(e.Remove(thorin, AskAboutEffects))
	if e.Current != 0 || len(e.Order()) != 0 {
		t.Fatalf("current = %d, order = %s", e.Current, orderNames(e))
	}
	must(t)(e.Next()) // nothing to advance to, and no panic
	gob := add(t, e, "Goblin", "12", 7)
	must(t)(e.Next())
	if e.Current != gob {
		t.Errorf("current = %d, want Goblin", e.Current)
	}
}

func TestRowLookupsAndErrors(t *testing.T) {
	e := newEnc() // a zero clock falls back to time.Now
	e.SetClock(nil)
	if e.now().IsZero() {
		t.Error("nil clock should mean time.Now")
	}
	if (&Encounter{}).now().IsZero() {
		t.Error("an encounter without a clock should use time.Now")
	}
	e.SetClock(fixedClock())
	thorin := add(t, e, "Thorin", "18", 44)
	fx, evs, err := e.AddEffect(EffectSpec{Name: "Fog", Initiative: ptrInit(ini(t, "20")), Note: " heavily obscured "})
	if err != nil {
		t.Fatal(err)
	}
	if m := evs[0].Meta(); m.Round != 0 || !m.At.Equal(t0) {
		t.Errorf("meta = %+v", m)
	}
	if r, _ := e.Row(fx); r.Name() != "Fog" || r.ID() != fx || r.Effect.Note != "heavily obscured" {
		t.Errorf("effect row: %+v", r.Effect)
	}
	const missing = RowID(99)

	// Operations on a combatant refuse effect rows and unknown rows.
	for name, op := range map[string]func(RowID) error{
		"Damage":          func(id RowID) error { _, err := e.Damage(id, 1); return err },
		"SetHP":           func(id RowID) error { _, err := e.SetHP(id, 1); return err },
		"AddCondition":    func(id RowID) error { _, _, err := e.AddCondition(id, "Prone", nil, ""); return err },
		"RemoveCondition": func(id RowID) error { _, err := e.RemoveCondition(id, 1); return err },
		"Remove":          func(id RowID) error { _, err := e.Remove(id, AskAboutEffects); return err },
	} {
		if err := op(fx); !errors.Is(err, ErrNotCombatant) {
			t.Errorf("%s on an effect: %v", name, err)
		}
		if err := op(missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s on a missing row: %v", name, err)
		}
	}
	// Operations on an effect refuse combatant rows and unknown rows.
	for name, op := range map[string]func(RowID) error{
		"AddTargets":    func(id RowID) error { _, err := e.AddTargets(id, nil); return err },
		"RemoveTargets": func(id RowID) error { _, err := e.RemoveTargets(id, nil); return err },
		"EndEffect":     func(id RowID) error { _, err := e.EndEffect(id, ""); return err },
	} {
		if err := op(thorin); !errors.Is(err, ErrNotEffect) {
			t.Errorf("%s on a combatant: %v", name, err)
		}
		if err := op(missing); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s on a missing row: %v", name, err)
		}
	}
	if _, err := e.SetInitiative(missing, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetInitiative: %v", err)
	}
	if _, err := e.SetNote(missing, "x"); !errors.Is(err, ErrNotFound) {
		t.Errorf("SetNote: %v", err)
	}
	if _, err := e.RemoveCondition(thorin, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("RemoveCondition: %v", err)
	}

	// Effects can be re-slotted and annotated like combatants.
	must(t)(e.SetInitiative(fx, ini(t, "3")))
	must(t)(e.SetNote(fx, " thick "))
	if f, _ := e.Effect(fx); f.Initiative.String() != "3" || f.Note != "thick" || orderNames(e) != "Thorin,Fog" {
		t.Errorf("effect after edits: %+v, order %s", f, orderNames(e))
	}

	// Bad effect and condition specs.
	for _, spec := range []EffectSpec{
		{Name: " ", SourceID: thorin},
		{Name: "Web", SourceID: thorin, Rounds: rounds(0)},
		{Name: "Web"},
	} {
		if _, _, err := e.AddEffect(spec); !errors.Is(err, ErrInvalid) {
			t.Errorf("AddEffect(%+v): %v", spec, err)
		}
	}
	if _, _, err := e.AddEffect(EffectSpec{Name: "Web", SourceID: fx}); !errors.Is(err, ErrNotCombatant) {
		t.Errorf("effect sourced from an effect: %v", err)
	}
	if _, _, err := e.AddEffect(EffectSpec{Name: "Web", SourceID: thorin, Targets: []RowID{missing}}); !errors.Is(err, ErrNotFound) {
		t.Errorf("effect on a missing target: %v", err)
	}
	if _, err := e.AddTargets(fx, []RowID{missing}); !errors.Is(err, ErrNotFound) {
		t.Errorf("AddTargets with a missing target: %v", err)
	}
	if _, _, err := e.AddCondition(thorin, " ", nil, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("blank condition: %v", err)
	}
	if _, _, err := e.AddCondition(thorin, "Prone", rounds(0), ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("zero-round condition: %v", err)
	}
	if _, _, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "Orc", MaxHP: 5}, Count: 2, Initiatives: []Initiative{1}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("group with too few initiatives: %v", err)
	}
	if _, _, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "Orc", MaxHP: 5}, Count: 0}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty group: %v", err)
	}
	if _, _, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{MaxHP: 5}, Count: 2}); !errors.Is(err, ErrInvalid) {
		t.Errorf("unnamed group: %v", err)
	}

	// Find is case-insensitive and ignores surrounding spaces.
	_, _, _ = e.AddCondition(thorin, "Prone", nil, "")
	c, _ := e.Combatant(thorin)
	if _, ok := c.FindCondition(" prone "); !ok {
		t.Error("FindCondition should match case-insensitively")
	}
	if _, ok := c.FindCondition("stunned"); ok {
		t.Error("FindCondition matched a condition that isn't there")
	}

	nd := &NeedsDecisionError{Combatant: thorin, Effects: []RowID{fx}}
	if !strings.Contains(nd.Error(), "1 effect") {
		t.Errorf("NeedsDecisionError: %s", nd)
	}
}

// SetHP crossing zero in either direction reports it.
func TestSetHPCrossingZero(t *testing.T) {
	e := newEnc()
	id := add(t, e, "Goblin", "12", 7)
	wantKinds(t, must(t)(e.SetHP(id, -3)), "HPSet", "Downed")
	wantKinds(t, must(t)(e.SetHP(id, 99)), "HPSet", "Revived")
	if c, _ := e.Combatant(id); c.HP != 7 {
		t.Errorf("HP = %d, want clamped to 7", c.HP)
	}
}

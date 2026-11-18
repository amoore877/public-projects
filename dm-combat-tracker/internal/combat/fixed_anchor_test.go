package combat

import (
	"errors"
	"testing"
)

func addFX(t *testing.T, e *Encounter, name string, src RowID, n int) RowID {
	t.Helper()
	id, _, err := e.AddEffect(EffectSpec{Name: name, SourceID: src, Rounds: rounds(n)})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func remainingFX(t *testing.T, e *Encounter, id RowID) int {
	t.Helper()
	fx, ok := e.Effect(id)
	if !ok {
		t.Fatalf("effect %d is gone", id)
	}
	return *fx.Remaining
}

// In 3.5e a sourced effect's row sorts just before its source on a tie
// (D32); in 5e it stays after, as in v1. Other ties are unchanged.
func TestPinnedEffectSortsBeforeSource(t *testing.T) {
	for _, tc := range []struct {
		ruleset, want string
	}{
		{"5e", "Thorin,Mira,Bless,Fog,Goblin"},
		{"3.5e", "Thorin,Bless,Mira,Fog,Goblin"},
	} {
		e := newEnc()
		must(t)(e.SetRuleset(tc.ruleset))
		add(t, e, "Thorin", "15", 40)
		mira := add(t, e, "Mira", "15", 27)
		add(t, e, "Goblin", "12", 7)
		addFX(t, e, "Bless", mira, 3)
		at := ini(t, "15")
		if _, _, err := e.AddEffect(EffectSpec{Name: "Fog", Initiative: &at}); err != nil {
			t.Fatal(err)
		}
		if got := orderNames(e); got != tc.want {
			t.Errorf("%s: order = %s, want %s", tc.ruleset, got, tc.want)
		}
	}
}

// A 1-round effect Mira starts on her turn counts down on its own row and
// expires just before her next turn, which then starts (D32).
func TestPinnedEffectCountsDownOnItsRow(t *testing.T) {
	e := newEnc35(t)
	mira := add(t, e, "Mira", "15", 27)
	add(t, e, "Goblin", "12", 7)
	must(t)(e.Start())
	fx := addFX(t, e, "Bless", mira, 1)
	if currentName(e) != "Mira" {
		t.Fatalf("turn moved to %s", currentName(e))
	}
	must(t)(e.Next()) // Goblin
	if remainingFX(t, e, fx) != 1 {
		t.Error("the effect shouldn't count down on the goblin's turn")
	}
	evs := must(t)(e.Next())
	wantKinds(t, evs, "RoundStarted", "EffectExpired", "TurnStarted")
	if currentName(e) != "Mira" || e.Round != 2 {
		t.Errorf("turn = %s round %d, want Mira round 2", currentName(e), e.Round)
	}
	// Rewind puts it back on its row, still at 1.
	must(t)(e.Previous())
	if currentName(e) != "Goblin" || remainingFX(t, e, fx) != 1 {
		t.Errorf("after rewind: turn %s, effect at %d", currentName(e), remainingFX(t, e, fx))
	}
}

// When the caster delays, the effect stays on its count and keeps counting
// down there; the caster's own turn no longer touches it (G7).
func TestDelayLeavesEffectOnItsCount(t *testing.T) {
	e := newEnc35(t)
	add(t, e, "Thorin", "18", 40)
	mira := add(t, e, "Mira", "15", 27)
	add(t, e, "Goblin", "12", 7)
	must(t)(e.Start())
	must(t)(e.Next()) // Mira casts
	fx := addFX(t, e, "Bless", mira, 2)
	_, err := e.SetInitiative(mira, ini(t, "10"), AskToMoveEffects)
	var me *MoveEffectsError
	if !errors.As(err, &me) || me.Combatant != mira || me.To != ini(t, "10") || len(me.Effects) != 1 || me.Effects[0] != fx {
		t.Fatalf("err = %v, want a move-effects question", err)
	}
	if c, _ := e.Combatant(mira); c.Initiative != ini(t, "15") {
		t.Error("asking must change nothing")
	}
	evs := must(t)(e.SetInitiative(mira, ini(t, "10"), LeaveEffects))
	wantKinds(t, evs, "InitiativeChanged")
	if got := orderNames(e); got != "Thorin,Bless,Goblin,Mira" {
		t.Errorf("order = %s", got)
	}
	must(t)(e.Next()) // round 2, Thorin
	must(t)(e.Next()) // Bless counts down on its own row
	if currentName(e) != "Bless" || remainingFX(t, e, fx) != 1 {
		t.Errorf("turn %s, effect at %d; want Bless at 1", currentName(e), remainingFX(t, e, fx))
	}
	must(t)(e.Next()) // Goblin
	must(t)(e.Next()) // Mira
	if remainingFX(t, e, fx) != 1 {
		t.Error("Mira's turn shouldn't count her effect down")
	}
	must(t)(e.Next()) // round 3, Thorin
	evs = must(t)(e.Next())
	wantKinds(t, evs, "EffectExpired", "TurnStarted")
	if currentName(e) != "Goblin" {
		t.Errorf("turn = %s, want Goblin", currentName(e))
	}
}

// A corrected initiative moves the source's effects by the same amount, so
// one sitting on the source's count stays pinned to it (D33).
func TestCorrectionMovesEffects(t *testing.T) {
	e := newEnc35(t)
	mira := add(t, e, "Mira", "15", 27)
	add(t, e, "Goblin", "12", 7)
	bless := addFX(t, e, "Bless", mira, 3)
	at := ini(t, "13")
	wall, _, err := e.AddEffect(EffectSpec{Name: "Wall", SourceID: mira, Initiative: &at})
	if err != nil {
		t.Fatal(err)
	}
	evs := must(t)(e.SetInitiative(mira, ini(t, "8"), MoveEffects))
	wantKinds(t, evs, "InitiativeChanged", "InitiativeChanged", "InitiativeChanged")
	if ch := evs[1].(InitiativeChanged); ch.Name != "Bless" || ch.From != ini(t, "15") || ch.To != ini(t, "8") {
		t.Errorf("bless change = %+v", ch)
	}
	b, _ := e.Effect(bless)
	w, _ := e.Effect(wall)
	if b.Initiative != ini(t, "8") || w.Initiative != ini(t, "6") {
		t.Errorf("effects at %s and %s, want 8 and 6", b.Initiative, w.Initiative)
	}
	if got := orderNames(e); got != "Goblin,Bless,Mira,Wall" {
		t.Errorf("order = %s", got)
	}
}

// The question is only asked when the answer matters.
func TestMoveEffectsQuestionOnlyWhenItMatters(t *testing.T) {
	// 5e: effects follow their source's turn, so nothing to ask.
	e := newEnc()
	mira := add(t, e, "Mira", "15", 27)
	fx := addFX(t, e, "Bless", mira, 3)
	must(t)(e.SetInitiative(mira, ini(t, "8"), AskToMoveEffects))
	if f, _ := e.Effect(fx); f.Initiative != ini(t, "15") {
		t.Error("5e must not move the effect row")
	}

	e = newEnc35(t)
	mira = add(t, e, "Mira", "15", 27)
	gob := add(t, e, "Goblin", "12", 7)
	fx = addFX(t, e, "Bless", mira, 3)
	must(t)(e.SetInitiative(mira, ini(t, "15"), AskToMoveEffects)) // unchanged value
	must(t)(e.SetInitiative(gob, ini(t, "11"), AskToMoveEffects))  // no effects
	must(t)(e.SetInitiative(fx, ini(t, "14"), AskToMoveEffects))   // the effect row itself
	// Leave on a creature with no effects is a plain change.
	must(t)(e.SetInitiative(gob, ini(t, "10"), MoveEffects))
	if g, _ := e.Combatant(gob); g.Initiative != ini(t, "10") {
		t.Error("goblin initiative not set")
	}
}

// Removing the source and keeping its effect leaves the row where it was.
func TestRemoveSourceKeepsPinnedRow(t *testing.T) {
	e := newEnc35(t)
	add(t, e, "Thorin", "18", 40)
	mira := add(t, e, "Mira", "15", 27)
	add(t, e, "Goblin", "12", 7)
	fx := addFX(t, e, "Bless", mira, 2)
	must(t)(e.Start())
	must(t)(e.Remove(mira, KeepEffects))
	if got := orderNames(e); got != "Thorin,Bless,Goblin" {
		t.Errorf("order = %s", got)
	}
	must(t)(e.Next()) // Bless's own row: ticks unsourced
	if currentName(e) != "Bless" || remainingFX(t, e, fx) != 1 {
		t.Errorf("turn %s, effect at %d", currentName(e), remainingFX(t, e, fx))
	}
}

func TestMoveEffectsErrorMessage(t *testing.T) {
	err := &MoveEffectsError{Effects: []RowID{1, 2}}
	if err.Error() != "combatant is the source of 2 effect(s): move them too?" {
		t.Error(err.Error())
	}
}

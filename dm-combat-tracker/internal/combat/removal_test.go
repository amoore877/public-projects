package combat

import (
	"errors"
	"testing"
)

func TestRemoveNotCurrent(t *testing.T) {
	e := newEnc()
	add(t, e, "A", "20", 5)
	b := add(t, e, "B", "10", 5)
	c := add(t, e, "C", "5", 5)
	fx, _, _ := e.AddEffect(EffectSpec{Name: "Web", Initiative: ptr(InitiativeFromInt(1)), Targets: []RowID{b, c}})
	must(t)(e.Start())
	evs := must(t)(e.Remove(b, AskAboutEffects))
	wantKinds(t, evs, "CombatantRemoved")
	if currentName(e) != "A" || orderNames(e) != "A,C,Web" {
		t.Errorf("current=%s order=%s", currentName(e), orderNames(e))
	}
	w, _ := e.Effect(fx)
	if len(w.Targets) != 1 || w.Targets[0] != c {
		t.Errorf("targets = %v", w.Targets)
	}
	_, err := e.Remove(b, AskAboutEffects)
	wantErr(t, err, ErrNotFound)
	_, err = e.Remove(fx, AskAboutEffects)
	wantErr(t, err, ErrNotCombatant)
}

func TestRemoveCurrentPassesTurn(t *testing.T) {
	e := newEnc()
	add(t, e, "A", "20", 5)
	b := add(t, e, "B", "10", 5)
	c := add(t, e, "C", "5", 5)
	must(t)(e.Start())
	must(t)(e.Next()) // B
	e.AddCondition(c, "Slow", rounds(2), "")

	evs := must(t)(e.Remove(b, AskAboutEffects))
	wantKinds(t, evs, "CombatantRemoved", "TurnStarted")
	if currentName(e) != "C" || e.Round != 1 {
		t.Errorf("current=%s round=%d", currentName(e), e.Round)
	}
	cc, _ := e.Combatant(c)
	if *cc.Conditions[0].Remaining != 1 { // C's turn started: ticks exactly once
		t.Errorf("Slow remaining = %d", *cc.Conditions[0].Remaining)
	}
	_, err := e.Previous()
	wantErr(t, err, ErrNothingToUndo)

	// Removing the last row in order wraps into a new round.
	evs = must(t)(e.Remove(c, AskAboutEffects))
	wantKinds(t, evs, "CombatantRemoved", "RoundStarted", "TurnStarted")
	if currentName(e) != "A" || e.Round != 2 {
		t.Errorf("current=%s round=%d", currentName(e), e.Round)
	}

	// Removing the only row leaves nobody up.
	a := e.Current
	must(t)(e.Remove(a, AskAboutEffects))
	if e.Current != 0 || len(e.Order()) != 0 {
		t.Errorf("current=%d", e.Current)
	}
}

func TestRemoveSourceAsks(t *testing.T) {
	setup := func() (*Encounter, RowID, RowID, RowID) {
		e := newEnc()
		m := add(t, e, "Mira", "9", 27)
		g := add(t, e, "Gob", "5", 7)
		fx, _, _ := e.AddEffect(EffectSpec{Name: "SG", SourceID: m, Rounds: rounds(10), Targets: []RowID{g}})
		must(t)(e.Start())
		return e, m, g, fx
	}

	e, m, _, fx := setup()
	_, err := e.Remove(m, AskAboutEffects)
	var nd *NeedsDecisionError
	if !errors.As(err, &nd) || len(nd.Effects) != 1 || nd.Effects[0] != fx {
		t.Fatalf("err = %v", err)
	}
	if _, ok := e.Combatant(m); !ok {
		t.Fatal("asking must not change state")
	}

	// Keep: effect stays at its initiative, unsourced.
	evs := must(t)(e.Remove(m, KeepEffects))
	wantKinds(t, evs, "CombatantRemoved", "TurnStarted")
	sg, ok := e.Effect(fx)
	if !ok || sg.SourceID != 0 || sg.Initiative.String() != "9" {
		t.Errorf("kept effect = %+v", sg)
	}
	if currentName(e) != "SG" {
		t.Errorf("current = %s", currentName(e))
	}

	// End: effect and its linked conditions go too; the turn skips past it.
	e, m, g, fx := setup()
	evs = must(t)(e.Remove(m, EndEffects))
	wantKinds(t, evs, "CombatantRemoved", "EffectEnded", "TurnStarted")
	if _, ok := e.Effect(fx); ok || len(e.LinkedEffects(g)) != 0 {
		t.Error("effect should be gone")
	}
	if ee := evs[1].(EffectEnded); ee.Reason != "Mira left combat" {
		t.Errorf("reason = %q", ee.Reason)
	}
	if currentName(e) != "Gob" {
		t.Errorf("current = %s", currentName(e))
	}
}

func TestRemoveDuringSetup(t *testing.T) {
	e := newEnc()
	a := add(t, e, "A", "1", 1)
	must(t)(e.Remove(a, AskAboutEffects))
	if e.Current != 0 || len(e.Combatants) != 0 {
		t.Error("expected empty")
	}
}

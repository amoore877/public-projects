package combat

import (
	"reflect"
	"testing"
)

func TestConditions(t *testing.T) {
	e := newEnc()
	g := add(t, e, "Gob", "1", 7)
	_, _, err := e.AddCondition(g, " ", nil, "")
	wantErr(t, err, ErrInvalid)
	_, _, err = e.AddCondition(g, "Prone", rounds(0), "")
	wantErr(t, err, ErrInvalid)

	n := 3
	id, evs, err := e.AddCondition(g, " Poisoned ", &n, "DC 14 CON save ends")
	if err != nil {
		t.Fatal(err)
	}
	n = 99 // caller's pointer must not alias the stored value
	ca := evs[0].(ConditionApplied)
	if ca.Condition != "Poisoned" || *ca.Remaining != 3 || ca.Note != "DC 14 CON save ends" {
		t.Errorf("applied = %+v", ca)
	}
	c, _ := e.Combatant(g)
	cond, ok := c.FindCondition("poisoned")
	if !ok || cond.ID != id || *cond.Remaining != 3 {
		t.Errorf("find = %+v %v", cond, ok)
	}

	evs = must(t)(e.RemoveCondition(g, id))
	wantKinds(t, evs, "ConditionRemoved")
	if len(c.Conditions) != 0 {
		t.Error("not removed")
	}
	_, err = e.RemoveCondition(g, id)
	wantErr(t, err, ErrNotFound)
}

func TestEffects(t *testing.T) {
	e := newEnc()
	m := add(t, e, "Mira", "9", 27)
	g1 := add(t, e, "Goblin 1", "12.4", 7)
	g2 := add(t, e, "Goblin 2", "12.3", 7)
	g3 := add(t, e, "Goblin 3", "12.2", 7)

	_, _, err := e.AddEffect(EffectSpec{Name: "Fog"})
	wantErr(t, err, ErrInvalid)
	_, _, err = e.AddEffect(EffectSpec{Name: "X", SourceID: m, Targets: []RowID{77}})
	wantErr(t, err, ErrNotFound)

	fx, evs, err := e.AddEffect(EffectSpec{Name: "Spirit Guardians", SourceID: m, Rounds: rounds(10), Targets: []RowID{g1, g3, g1}, Note: "3d8 radiant"})
	if err != nil {
		t.Fatal(err)
	}
	es := evs[0].(EffectStarted)
	if es.Source != "Mira" || !reflect.DeepEqual(es.Targets, []string{"Goblin 1", "Goblin 3"}) || *es.Remaining != 10 {
		t.Errorf("started = %+v", es)
	}
	if got := orderNames(e); got != "Goblin 1,Goblin 2,Goblin 3,Mira,Spirit Guardians" {
		t.Errorf("order = %s", got)
	}
	if len(e.LinkedEffects(g1)) != 1 || len(e.LinkedEffects(g2)) != 0 {
		t.Error("linked effects wrong")
	}
	if got := e.EffectsFrom(m); len(got) != 1 || got[0].ID != fx {
		t.Errorf("effects from = %v", got)
	}

	evs = must(t)(e.AddTargets(fx, []RowID{g2, g1}))
	if tc := evs[0].(EffectTargetsChanged); !reflect.DeepEqual(tc.Added, []string{"Goblin 2"}) {
		t.Errorf("added = %v", tc.Added)
	}
	if evs := must(t)(e.AddTargets(fx, []RowID{g2})); len(evs) != 0 {
		t.Error("no-op add should emit nothing")
	}
	evs = must(t)(e.RemoveTargets(fx, []RowID{g1, m}))
	if tc := evs[0].(EffectTargetsChanged); !reflect.DeepEqual(tc.Removed, []string{"Goblin 1"}) {
		t.Errorf("removed = %v", tc.Removed)
	}
	_, err = e.AddTargets(g1, []RowID{g2})
	wantErr(t, err, ErrNotEffect)

	// Environmental effect with an explicit initiative, and source + override.
	lair, _, err := e.AddEffect(EffectSpec{Name: "Lair", Initiative: ptr(InitiativeFromInt(20))})
	if err != nil {
		t.Fatal(err)
	}
	if l, _ := e.Effect(lair); l.SourceID != 0 || l.Initiative.String() != "20" {
		t.Errorf("lair = %+v", l)
	}

	evs = must(t)(e.EndEffect(fx, " concentration broken "))
	if ee := evs[0].(EffectEnded); ee.Name != "Spirit Guardians" || ee.Reason != "concentration broken" {
		t.Errorf("ended = %+v", ee)
	}
	for _, id := range []RowID{g1, g2, g3} {
		if len(e.LinkedEffects(id)) != 0 {
			t.Errorf("%d still linked", id)
		}
	}
	_, err = e.EndEffect(fx, "")
	wantErr(t, err, ErrNotFound)
}

func TestEndCurrentEffectPassesTurn(t *testing.T) {
	e := newEnc()
	m := add(t, e, "Mira", "9", 27)
	add(t, e, "Gob", "5", 7)
	fx, _, _ := e.AddEffect(EffectSpec{Name: "SG", SourceID: m})
	must(t)(e.Start())
	must(t)(e.Next()) // SG's row
	evs := must(t)(e.EndEffect(fx, ""))
	wantKinds(t, evs, "EffectEnded", "TurnStarted")
	if currentName(e) != "Gob" {
		t.Errorf("current = %s", currentName(e))
	}
}

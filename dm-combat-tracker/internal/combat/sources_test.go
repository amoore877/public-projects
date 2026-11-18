package combat

import (
	"errors"
	"strings"
	"testing"
)

func remainingOf(t *testing.T, c *Combatant, name string) int {
	t.Helper()
	cond, ok := c.FindCondition(name)
	if !ok {
		t.Fatalf("%s has no %s", c.Name, name)
	}
	return *cond.Remaining
}

// A condition with a source counts down at the start of the source's turn,
// not the bearer's (RS7, G8): Mira's 1-round daze on the goblin lasts until
// Mira's next turn, through the goblin's turn.
func TestSourcedConditionCountsDownOnSourcesTurn(t *testing.T) {
	e := newEnc()
	mira := add(t, e, "Mira", "15", 27)
	gob := add(t, e, "Goblin", "12", 7)
	orc := add(t, e, "Orc", "10", 15)
	must(t)(e.Start())
	_, evs, err := e.AddConditionFrom(gob, "Dazed", rounds(1), mira, "")
	if err != nil {
		t.Fatal(err)
	}
	if ap := evs[0].(ConditionApplied); ap.Source != "Mira" {
		t.Errorf("event source = %q", ap.Source)
	}
	// An unsourced 1-round condition next to it expires on the goblin's turn.
	_, _, _ = e.AddCondition(gob, "Prone", rounds(1), "")
	_, _, _ = e.AddConditionFrom(orc, "Frightened", rounds(2), mira, "")

	must(t)(e.Next()) // Goblin
	c, _ := e.Combatant(gob)
	if _, ok := c.FindCondition("Prone"); ok {
		t.Error("prone should expire on the goblin's turn")
	}
	if remainingOf(t, c, "Dazed") != 1 {
		t.Error("dazed should not count down on the goblin's turn")
	}
	must(t)(e.Next()) // Orc
	o, _ := e.Combatant(orc)
	if remainingOf(t, o, "Frightened") != 2 {
		t.Error("frightened should not count down on the orc's turn")
	}
	evs = must(t)(e.Next()) // Mira, round 2
	if _, ok := c.FindCondition("Dazed"); ok {
		t.Error("dazed should expire as Mira's turn starts")
	}
	if remainingOf(t, o, "Frightened") != 1 {
		t.Error("frightened should count down on Mira's turn")
	}
	var expired int
	for _, ev := range evs {
		if x, ok := ev.(ConditionExpired); ok && x.Condition == "Dazed" && x.Target == "Goblin" {
			expired++
		}
	}
	if expired != 1 {
		t.Errorf("want one Dazed expiry event, got %v", evs)
	}
}

// Previous puts back a sourced condition that expired, on the right bearer
// and in its old place, and undoes the countdowns of the others.
func TestPreviousRestoresSourcedConditions(t *testing.T) {
	e := newEnc()
	mira := add(t, e, "Mira", "15", 27)
	gob := add(t, e, "Goblin", "12", 7)
	must(t)(e.Start())
	_, _, _ = e.AddCondition(gob, "Poisoned", nil, "")
	_, _, _ = e.AddConditionFrom(gob, "Dazed", rounds(1), mira, "")
	_, _, _ = e.AddCondition(gob, "Blessed", nil, "")
	_, _, _ = e.AddConditionFrom(gob, "Slowed", rounds(3), mira, "")
	must(t)(e.Next()) // Goblin
	must(t)(e.Next()) // Mira: Dazed expires, Slowed 3 → 2
	c, _ := e.Combatant(gob)
	if len(c.Conditions) != 3 || remainingOf(t, c, "Slowed") != 2 {
		t.Fatalf("after Mira's turn: %+v", c.Conditions)
	}
	must(t)(e.Previous())
	var names []string
	for _, cond := range c.Conditions {
		names = append(names, cond.Name)
	}
	if got := strings.Join(names, ","); got != "Poisoned,Dazed,Blessed,Slowed" {
		t.Errorf("conditions after rewind = %s", got)
	}
	d, _ := c.FindCondition("Dazed")
	if d.SourceID != mira || *d.Remaining != 1 || remainingOf(t, c, "Slowed") != 3 {
		t.Errorf("restored: dazed %+v, slowed %d", d, remainingOf(t, c, "Slowed"))
	}
}

// When the source leaves combat, its conditions stay and count down on the
// bearer's turn again. A rewind that restores one whose source has gone
// does the same.
func TestSourceLeavingHandsConditionsToBearer(t *testing.T) {
	e := newEnc()
	mira := add(t, e, "Mira", "15", 27)
	gob := add(t, e, "Goblin", "12", 7)
	must(t)(e.Start())
	_, _, _ = e.AddConditionFrom(gob, "Dazed", rounds(2), mira, "")
	must(t)(e.Remove(mira, AskAboutEffects)) // no effects, so no question; turn passes to Goblin
	c, _ := e.Combatant(gob)
	d, _ := c.FindCondition("Dazed")
	if d.SourceID != 0 || *d.Remaining != 1 {
		t.Fatalf("after source left: %+v (the goblin's turn started and counted it down)", d)
	}

	e2 := newEnc()
	thorin := add(t, e2, "Thorin", "14", 44)
	gob2 := add(t, e2, "Goblin", "12", 7)
	must(t)(e2.Start())
	must(t)(e2.Next())
	e2.History[0].ExpiredConditions = []ExpiredCondition{{OwnerID: gob2, Condition: Condition{ID: 90, Name: "Dazed", Remaining: rounds(1), SourceID: 91}}}
	must(t)(e2.Previous())
	g, _ := e2.Combatant(gob2)
	if d, ok := g.FindCondition("Dazed"); !ok || d.SourceID != 0 || e2.Current != thorin {
		t.Errorf("restored dazed: %+v", d)
	}
}

// A source that is the bearer is the same as no source; an unknown source is
// refused with nothing changed.
func TestConditionSourceValidation(t *testing.T) {
	e := newEnc()
	gob := add(t, e, "Goblin", "12", 7)
	_, _, err := e.AddConditionFrom(gob, "Raging", rounds(10), gob, "")
	if err != nil {
		t.Fatal(err)
	}
	c, _ := e.Combatant(gob)
	if c.Conditions[0].SourceID != 0 {
		t.Error("self-sourced condition should be stored unsourced")
	}
	if _, _, err := e.AddConditionFrom(gob, "Dazed", rounds(1), 999, ""); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown source: err = %v", err)
	}
	if len(c.Conditions) != 1 {
		t.Error("refused add changed the conditions")
	}
}

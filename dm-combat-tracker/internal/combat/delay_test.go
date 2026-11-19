package combat

import (
	"encoding/json"
	"testing"
)

// delayFight: Thorin 18, Mira 15 (Bless 3 rounds), Goblin 12, Orc 10, with
// the turn on Mira in round 1 (Bless already at 2).
func delayFight(t *testing.T) (e *Encounter, mira RowID) {
	t.Helper()
	e = newEnc()
	add(t, e, "Thorin", "18", 44)
	mira = add(t, e, "Mira", "15", 27)
	add(t, e, "Goblin", "12", 7)
	add(t, e, "Orc", "10", 15)
	if _, _, err := e.AddCondition(mira, "Blessed", rounds(3), ""); err != nil {
		t.Fatal(err)
	}
	must(t)(e.Start())
	must(t)(e.Next())
	c, _ := e.Combatant(mira)
	if currentName(e) != "Mira" || remainingOf(t, c, "Blessed") != 2 {
		t.Fatalf("setup: turn %s", currentName(e))
	}
	return e, mira
}

// Moving the turn holder past the next row is a Delay: the turn passes on,
// the creatures in between still act, and the mover acts again at its new
// count this round without its durations counting down twice (RS10, D36).
func TestDelayPassesTheTurn(t *testing.T) {
	e, mira := delayFight(t)
	c, _ := e.Combatant(mira)
	evs := must(t)(e.SetInitiative(mira, ini(t, "11"), AskToMoveEffects))
	wantKinds(t, evs, "InitiativeChanged", "TurnStarted")
	if currentName(e) != "Goblin" || e.Round != 1 {
		t.Fatalf("turn %s round %d, want Goblin round 1", currentName(e), e.Round)
	}
	must(t)(e.Next())
	if currentName(e) != "Mira" || e.Round != 1 || remainingOf(t, c, "Blessed") != 2 {
		t.Errorf("Mira's second turn: turn %s round %d blessed %d", currentName(e), e.Round, remainingOf(t, c, "Blessed"))
	}
	if len(e.Delayed) != 0 {
		t.Errorf("delayed = %v after Mira's turn came", e.Delayed)
	}
	must(t)(e.Next()) // Orc
	must(t)(e.Next()) // round 2, Thorin
	must(t)(e.Next()) // Goblin
	must(t)(e.Next()) // Mira at 11: counts down as normal
	if currentName(e) != "Mira" || e.Round != 2 || remainingOf(t, c, "Blessed") != 1 {
		t.Errorf("round 2: turn %s round %d blessed %d", currentName(e), e.Round, remainingOf(t, c, "Blessed"))
	}
}

// p steps back over the pass, one turn at a time; the new initiative stays,
// like other edits.
func TestDelayRewinds(t *testing.T) {
	e, mira := delayFight(t)
	c, _ := e.Combatant(mira)
	must(t)(e.SetInitiative(mira, ini(t, "11"), AskToMoveEffects))
	must(t)(e.Next()) // Mira's second turn
	must(t)(e.Previous())
	if currentName(e) != "Goblin" || !containsID(e.Delayed, mira) {
		t.Fatalf("one step back: turn %s delayed %v", currentName(e), e.Delayed)
	}
	must(t)(e.Next()) // her turn again: still no second countdown
	if remainingOf(t, c, "Blessed") != 2 {
		t.Errorf("blessed %d after redo", remainingOf(t, c, "Blessed"))
	}
	must(t)(e.Previous())
	must(t)(e.Previous())
	if currentName(e) != "Mira" || len(e.Delayed) != 0 || c.Initiative != ini(t, "11") {
		t.Errorf("two steps back: turn %s delayed %v init %s", currentName(e), e.Delayed, c.Initiative)
	}
}

// The turn stays put when the edit isn't a Delay.
func TestInitiativeEditsThatKeepTheTurn(t *testing.T) {
	for name, edit := range map[string]func(e *Encounter, mira RowID) ([]Event, error){
		"earlier": func(e *Encounter, mira RowID) ([]Event, error) {
			return e.SetInitiative(mira, ini(t, "20"), AskToMoveEffects)
		},
		"later, not past the next row": func(e *Encounter, mira RowID) ([]Event, error) {
			return e.SetInitiative(mira, ini(t, "13"), AskToMoveEffects)
		},
		"another row": func(e *Encounter, _ RowID) ([]Event, error) {
			return e.SetInitiative(e.Combatants[2].ID, ini(t, "5"), AskToMoveEffects)
		},
	} {
		e, mira := delayFight(t)
		wantKinds(t, must(t)(edit(e, mira)), "InitiativeChanged")
		if currentName(e) != "Mira" {
			t.Errorf("%s: turn moved to %s", name, currentName(e))
		}
	}

	// The last row moving later passes nobody.
	e, _ := delayFight(t)
	must(t)(e.Next())
	must(t)(e.Next()) // Orc
	orc := e.Current
	wantKinds(t, must(t)(e.SetInitiative(orc, ini(t, "1"), AskToMoveEffects)), "InitiativeChanged")
	if e.Current != orc {
		t.Error("the last row keeps its turn")
	}

	// In Setup there is no turn to pass.
	s := newEnc()
	a := add(t, s, "A", "15", 5)
	add(t, s, "B", "12", 5)
	wantKinds(t, must(t)(s.SetInitiative(a, ini(t, "1"), AskToMoveEffects)), "InitiativeChanged")
}

// A delayer removed before its turn comes again is forgotten at the next
// round, and Delayed survives a save.
func TestDelayedClearsAndPersists(t *testing.T) {
	e, mira := delayFight(t)
	must(t)(e.SetInitiative(mira, ini(t, "11"), AskToMoveEffects))
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back Encounter
	if err := json.Unmarshal(raw, &back); err != nil || !containsID(back.Delayed, mira) {
		t.Fatalf("delayed after a round trip: %v %v", back.Delayed, err)
	}
	must(t)(e.Remove(mira, AskAboutEffects))
	must(t)(e.Next()) // Orc
	must(t)(e.Next()) // round 2
	if e.Delayed != nil {
		t.Errorf("delayed = %v in a new round", e.Delayed)
	}
}

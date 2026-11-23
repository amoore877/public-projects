package combat

import "testing"

func TestParseDuration(t *testing.T) {
	for _, tc := range []struct {
		in  string
		n   int
		eot bool
	}{
		{"1", 1, false},
		{"10", 10, false},
		{"1e", 1, true},
		{"2E", 2, true},
		{" 3e ", 3, true},
	} {
		n, eot, err := ParseDuration(tc.in)
		if err != nil || n != tc.n || eot != tc.eot {
			t.Errorf("ParseDuration(%q) = (%d, %v, %v), want (%d, %v, nil)", tc.in, n, eot, err, tc.n, tc.eot)
		}
	}
	for _, bad := range []string{"", "0", "0e", "-1", "-2e", "e", "+2", "abc"} {
		if _, _, err := ParseDuration(bad); err == nil {
			t.Errorf("ParseDuration(%q) should fail", bad)
		}
	}
}

func TestEndOfTurnConditionsEffectsDelayAndRewind(t *testing.T) {
	e := newEnc()
	mira := add(t, e, "Mira", "18", 27)
	gob := add(t, e, "Goblin", "12", 7)
	must(t)(e.Start()) // Round 1, Mira's turn

	// Sourced condition "until end of Mira's next turn" = 2e by Mira;
	// Bearer condition "until end of Goblin's turn" = 1e on Goblin;
	// Sourced effect "until end of Mira's current turn" = 1e from Mira.
	if _, _, err := e.AddConditionEnd(gob, "Stunned", rounds(2), true, mira, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddConditionEnd(gob, "Dodging", rounds(1), true, 0, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddEffect(EffectSpec{Name: "Shield", SourceID: mira, Rounds: rounds(1), EndOfTurn: true, Targets: []RowID{mira}}); err != nil {
		t.Fatal(err)
	}

	// Mira finishes her turn: Shield expires (1e -> 0), Stunned ticks (2e -> 1e),
	// Dodging (anchored to Goblin) untouched. Turn moves straight past expired Shield to Goblin!
	evs := must(t)(e.Next())
	wantKinds(t, evs, "EffectExpired", "TurnStarted")
	if evs[0].Meta().TurnHolder != "Mira" || evs[0].(EffectExpired).Name != "Shield" {
		t.Errorf("Shield expiry meta = %+v", evs[0])
	}
	if e.Current != gob {
		t.Fatalf("current = %d, want Goblin %d", e.Current, gob)
	}
	g, _ := e.Combatant(gob)
	if len(g.Conditions) != 2 || *g.Conditions[0].Remaining != 1 || !g.Conditions[0].EndOfTurn {
		t.Fatalf("Goblin conditions on Goblin's turn start: %+v", g.Conditions)
	}

	// Rewind p back to Mira: Shield is back with EndOfTurn=true, Stunned is back to 2e!
	must(t)(e.Previous())
	if e.Current != mira || len(e.Effects) != 1 || !e.Effects[0].EndOfTurn || *e.Effects[0].Remaining != 1 {
		t.Fatalf("after Previous: current=%d effects=%+v", e.Current, e.Effects)
	}
	if g, _ := e.Combatant(gob); *g.Conditions[0].Remaining != 2 {
		t.Fatalf("after Previous, Stunned remaining = %d, want 2", *g.Conditions[0].Remaining)
	}

	// Advance past Mira again -> Goblin's turn -> Goblin finishes turn: Dodging expires at end of Goblin's turn.
	must(t)(e.Next()) // Goblin's turn
	evs = must(t)(e.Next()) // Round 2, Mira's turn
	wantKinds(t, evs, "ConditionExpired", "RoundStarted", "TurnStarted")
	if ce := evs[0].(ConditionExpired); ce.Condition != "Dodging" || ce.Meta().TurnHolder != "Goblin" {
		t.Errorf("Dodging expiry = %+v", ce)
	}
	// Round 2 Mira's turn: Stunned is still at 1e (doesn't tick on Mira's start).
	if g, _ := e.Combatant(gob); len(g.Conditions) != 1 || *g.Conditions[0].Remaining != 1 {
		t.Fatalf("Round 2 start Goblin conds: %+v", g.Conditions)
	}
	// Mira Delays past Goblin: Delay does NOT tick end-of-turn Stunned (ET2, D73).
	must(t)(e.SetInitiative(mira, ini(t, "5"), LeaveEffects))
	if e.Current != gob {
		t.Fatalf("after Delay, current = %d, want Goblin", e.Current)
	}
	if g, _ := e.Combatant(gob); len(g.Conditions) != 1 || *g.Conditions[0].Remaining != 1 {
		t.Fatalf("Delay shouldn't tick end-of-turn Stunned: %+v", g.Conditions)
	}
	// Goblin finishes turn -> Mira's delayed turn starts -> Mira finishes turn: Stunned expires!
	must(t)(e.Next()) // Mira's delayed turn
	evs = must(t)(e.Next())
	wantKinds(t, evs, "ConditionExpired", "RoundStarted", "TurnStarted")
	if evs[0].(ConditionExpired).Condition != "Stunned" {
		t.Errorf("expected Stunned expiry, got %+v", evs[0])
	}
}

func TestUnsourcedEndOfTurnEffectSelfExpiry(t *testing.T) {
	e := newEnc()
	thorin := add(t, e, "Thorin", "18", 44)
	init10 := ini(t, "10")
	fx1, _, err := e.AddEffect(EffectSpec{Name: "Hazard Mid", Initiative: &init10, Rounds: rounds(1), EndOfTurn: true})
	if err != nil {
		t.Fatal(err)
	}
	gob := add(t, e, "Goblin", "5", 7)
	init1 := ini(t, "1")
	fx2, _, err := e.AddEffect(EffectSpec{Name: "Hazard End", Initiative: &init1, Rounds: rounds(1), EndOfTurn: true})
	if err != nil {
		t.Fatal(err)
	}
	must(t)(e.Start()) // Thorin

	// Advance to Hazard Mid, then Next: Hazard Mid expires at end of its own turn, landing on Goblin!
	must(t)(e.Next()) // Hazard Mid
	if e.Current != fx1 {
		t.Fatalf("want fx1, got %d", e.Current)
	}
	evs := must(t)(e.Next())
	wantKinds(t, evs, "EffectExpired", "TurnStarted")
	if e.Current != gob {
		t.Fatalf("after mid-round self-expiry, want Goblin, got %d", e.Current)
	}
	// Advance to Hazard End (last row of round), then Next: expires and wraps cleanly to Round 2 Thorin!
	must(t)(e.Next()) // Hazard End
	if e.Current != fx2 {
		t.Fatalf("want fx2, got %d", e.Current)
	}
	evs = must(t)(e.Next())
	wantKinds(t, evs, "EffectExpired", "RoundStarted", "TurnStarted")
	if e.Round != 2 || e.Current != thorin || len(e.Effects) != 0 {
		t.Fatalf("after end-of-round self-expiry: round=%d cur=%d effects=%d", e.Round, e.Current, len(e.Effects))
	}
	// Rewind p brings Hazard End back cleanly!
	must(t)(e.Previous())
	if e.Round != 1 || e.Current != fx2 || len(e.Effects) != 1 {
		t.Fatalf("after Previous: round=%d cur=%d effects=%d", e.Round, e.Current, len(e.Effects))
	}
}

package combat

import (
	"testing"
)

func TestStartAndAdvance(t *testing.T) {
	e := newEnc()
	_, err := e.Start()
	wantErr(t, err, ErrEmpty)
	_, err = e.Next()
	wantErr(t, err, ErrNotActive)

	add(t, e, "Mira", "9", 27)
	add(t, e, "Thorin", "18", 44)
	add(t, e, "Gob", "12", 7)

	evs := must(t)(e.Start())
	wantKinds(t, evs, "CombatStarted", "RoundStarted", "TurnStarted")
	cs := evs[0].(CombatStarted)
	if len(cs.Order) != 3 || cs.Order[0].Name != "Thorin" || cs.Order[2].Initiative.String() != "9" {
		t.Errorf("order = %+v", cs.Order)
	}
	if e.Round != 1 || currentName(e) != "Thorin" || cs.TurnHolder != "Thorin" {
		t.Errorf("round=%d current=%s", e.Round, currentName(e))
	}
	_, err = e.Start()
	wantErr(t, err, ErrNotSetup)

	wantKinds(t, must(t)(e.Next()), "TurnStarted")
	if currentName(e) != "Gob" {
		t.Errorf("current = %s", currentName(e))
	}
	must(t)(e.Next())
	evs = must(t)(e.Next()) // wraps
	wantKinds(t, evs, "RoundStarted", "TurnStarted")
	if e.Round != 2 || currentName(e) != "Thorin" {
		t.Errorf("round=%d current=%s", e.Round, currentName(e))
	}
	if rs := evs[0].(RoundStarted); rs.Round != 2 || rs.TurnHolder != "Thorin" {
		t.Errorf("RoundStarted = %+v", rs)
	}
}

func TestAddMidCombatKeepsTurn(t *testing.T) {
	e := newEnc()
	add(t, e, "A", "20", 5)
	b := add(t, e, "B", "10", 5)
	must(t)(e.Start())
	must(t)(e.Next())
	add(t, e, "C", "15", 5) // slots between A and B
	if e.Current != b {
		t.Fatalf("current = %s", currentName(e))
	}
	must(t)(e.Next())
	if currentName(e) != "A" || e.Round != 2 {
		t.Errorf("current=%s round=%d", currentName(e), e.Round)
	}
	must(t)(e.Next())
	if currentName(e) != "C" {
		t.Errorf("current=%s", currentName(e))
	}
}

func TestConditionCountdown(t *testing.T) {
	e := newEnc()
	a := add(t, e, "A", "20", 5)
	add(t, e, "B", "10", 5)
	must(t)(e.Start()) // A's turn; no tick on the opening turn
	_, _, err := e.AddCondition(a, "Blessed", rounds(2), "")
	if err != nil {
		t.Fatal(err)
	}
	e.AddCondition(a, "Prone", nil, "")
	e.AddCondition(a, "Dazed", rounds(1), "")

	must(t)(e.Next()) // B: A's durations untouched
	ca, _ := e.Combatant(a)
	if *ca.Conditions[0].Remaining != 2 {
		t.Fatalf("ticked on someone else's turn")
	}

	evs := must(t)(e.Next()) // A, round 2: Blessed 2→1, Dazed expires
	wantKinds(t, evs, "RoundStarted", "ConditionExpired", "TurnStarted")
	if x := evs[1].(ConditionExpired); x.Condition != "Dazed" || x.Target != "A" || x.TurnHolder != "A" || x.Round != 2 {
		t.Errorf("expired = %+v", x)
	}
	if len(ca.Conditions) != 2 || *ca.Conditions[0].Remaining != 1 || ca.Conditions[1].Remaining != nil {
		t.Errorf("conditions = %v", condSummary(ca))
	}

	must(t)(e.Next())
	evs = must(t)(e.Next()) // A, round 3: Blessed expires
	wantKinds(t, evs, "RoundStarted", "ConditionExpired", "TurnStarted")
	if got := condSummary(ca); got != "Prone" {
		t.Errorf("conditions = %s", got)
	}
}

func condSummary(c *Combatant) string {
	s := ""
	for i, cond := range c.Conditions {
		if i > 0 {
			s += ","
		}
		s += cond.Name
		if cond.Remaining != nil {
			s += "(" + itoa(*cond.Remaining) + ")"
		}
	}
	return s
}

func itoa(n int) string { return Initiative(int64(n) * 1000).String() }

// A sourced effect counts down at the start of its caster's turns, not on its
// own row's turn: cast on Mira's turn for 2 rounds, it lasts until the start
// of her turn two rounds later.
func TestSourcedEffectCountsDownOnCastersTurn(t *testing.T) {
	e := newEnc()
	m := add(t, e, "Mira", "9", 27)
	g := add(t, e, "Gob", "5", 7)
	must(t)(e.Start()) // Mira
	fxID, _, err := e.AddEffect(EffectSpec{Name: "SG", SourceID: m, Rounds: rounds(2), Targets: []RowID{g}})
	if err != nil {
		t.Fatal(err)
	}
	if got := orderNames(e); got != "Mira,SG,Gob" {
		t.Fatalf("order = %s", got)
	}

	must(t)(e.Next()) // SG's own row: a reminder only, no countdown
	fx, _ := e.Effect(fxID)
	if currentName(e) != "SG" || *fx.Remaining != 2 {
		t.Fatalf("current=%s remaining=%d", currentName(e), *fx.Remaining)
	}
	must(t)(e.Next())                                              // Gob
	wantKinds(t, must(t)(e.Next()), "RoundStarted", "TurnStarted") // Mira r2: 2→1
	if *fx.Remaining != 1 {
		t.Fatalf("remaining=%d after Mira's second turn began", *fx.Remaining)
	}
	must(t)(e.Next()) // SG
	must(t)(e.Next()) // Gob
	// Mira r3: SG expires as her turn starts, and its row is gone.
	wantKinds(t, must(t)(e.Next()), "RoundStarted", "EffectExpired", "TurnStarted")
	if currentName(e) != "Mira" || e.Round != 3 || len(e.Effects) != 0 || len(e.LinkedEffects(g)) != 0 || orderNames(e) != "Mira,Gob" {
		t.Errorf("current=%s round=%d effects=%d", currentName(e), e.Round, len(e.Effects))
	}
	// Stepping back restores it, still on Gob and right after Mira.
	must(t)(e.Previous())
	fx, ok := e.Effect(fxID)
	if !ok || *fx.Remaining != 1 || currentName(e) != "Gob" || orderNames(e) != "Mira,SG,Gob" || len(e.LinkedEffects(g)) != 1 {
		t.Errorf("after rewind: %+v, current=%s, order=%s", fx, currentName(e), orderNames(e))
	}
}

// Once its caster has left (and the effect was kept), an effect counts down
// on its own row's turn, like a lair effect.
func TestKeptEffectCountsDownOnItsOwnTurn(t *testing.T) {
	e := newEnc()
	m := add(t, e, "Mira", "9", 27)
	add(t, e, "Gob", "5", 7)
	fxID, _, _ := e.AddEffect(EffectSpec{Name: "SG", SourceID: m, Rounds: rounds(2)})
	must(t)(e.Start())
	must(t)(e.Next()) // SG
	must(t)(e.Next()) // Gob
	must(t)(e.Remove(m, KeepEffects))
	must(t)(e.Next()) // SG r2: 2→1, on its own row now
	fx, _ := e.Effect(fxID)
	if currentName(e) != "SG" || *fx.Remaining != 1 || fx.SourceID != 0 {
		t.Fatalf("current=%s effect=%+v", currentName(e), fx)
	}
}

func TestEffectExpiringAtEndOfOrderWraps(t *testing.T) {
	e := newEnc()
	add(t, e, "A", "10", 5)
	must(t)(e.Start())
	e.AddEffect(EffectSpec{Name: "Fog", Initiative: ptr(InitiativeFromInt(1)), Rounds: rounds(1)})
	evs := must(t)(e.Next()) // Fog expires on its turn → wrap to A, round 2
	wantKinds(t, evs, "EffectExpired", "RoundStarted", "TurnStarted")
	if currentName(e) != "A" || e.Round != 2 {
		t.Errorf("current=%s round=%d", currentName(e), e.Round)
	}
	must(t)(e.Previous())
	if currentName(e) != "A" || e.Round != 1 || len(e.Effects) != 1 || *e.Effects[0].Remaining != 1 {
		t.Errorf("after rewind: current=%s round=%d effects=%d", currentName(e), e.Round, len(e.Effects))
	}
}

func TestPreviousReversesDurations(t *testing.T) {
	e := newEnc()
	a := add(t, e, "A", "20", 10)
	b := add(t, e, "B", "10", 10)
	must(t)(e.Start())
	e.AddCondition(a, "X", rounds(1), "")
	e.AddCondition(a, "Keep", nil, "")
	e.AddCondition(a, "Y", rounds(3), "")
	e.AddCondition(a, "Z", rounds(1), "")
	fxID, _, _ := e.AddEffect(EffectSpec{Name: "FX", SourceID: b, Rounds: rounds(5), Targets: []RowID{a}})

	must(t)(e.Next()) // B: FX 5→4 (B is its source)
	must(t)(e.Next()) // FX
	must(t)(e.Next()) // A r2: X,Z expire, Y 3→2
	ca, _ := e.Combatant(a)
	if got := condSummary(ca); got != "Keep,Y(2)" {
		t.Fatalf("conditions = %s", got)
	}
	must(t)(e.Damage(a, 4)) // not undone by rewind

	evs := must(t)(e.Previous())
	wantKinds(t, evs, "TurnRewound")
	if tr := evs[0].(TurnRewound); tr.Name != "FX" {
		t.Errorf("rewound to %s", tr.Name)
	}
	if got := condSummary(ca); got != "X(1),Keep,Y(3),Z(1)" {
		t.Errorf("conditions after rewind = %s", got)
	}
	if e.Round != 1 || ca.HP != 6 {
		t.Errorf("round=%d hp=%d", e.Round, ca.HP)
	}

	must(t)(e.Previous()) // back to B; FX was counted down when B's turn began
	fx, _ := e.Effect(fxID)
	if *fx.Remaining != 4 || currentName(e) != "B" {
		t.Errorf("fx remaining=%d current=%s", *fx.Remaining, currentName(e))
	}
	must(t)(e.Previous()) // back to A, FX 4→5
	if *fx.Remaining != 5 || currentName(e) != "A" {
		t.Errorf("fx remaining=%d current=%s", *fx.Remaining, currentName(e))
	}
	_, err := e.Previous()
	wantErr(t, err, ErrNothingToUndo)

	// Replaying forward gives the same result as before.
	must(t)(e.Next())
	must(t)(e.Next())
	must(t)(e.Next())
	if got := condSummary(ca); got != "Keep,Y(2)" {
		t.Errorf("replay conditions = %s", got)
	}
}

func TestHistoryCap(t *testing.T) {
	e := newEnc()
	add(t, e, "A", "1", 1)
	must(t)(e.Start())
	for range historyCap + 10 {
		must(t)(e.Next())
	}
	if len(e.History) != historyCap {
		t.Errorf("history = %d", len(e.History))
	}
	for range historyCap {
		must(t)(e.Previous())
	}
	_, err := e.Previous()
	wantErr(t, err, ErrNothingToUndo)
	if e.Round != 11 {
		t.Errorf("round = %d", e.Round)
	}
}

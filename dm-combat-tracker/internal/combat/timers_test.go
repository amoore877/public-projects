package combat

import "testing"

func TestTimersTickRewindAndCancel(t *testing.T) {
	e := newEnc()
	add(t, e, "Thorin", "18", 44)
	add(t, e, "Goblin", "12", 7)

	// Setup: r3 fires at start of Round 3 (Remaining = 2), relative 1 fires at start of Round 2.
	id1, evs, err := e.AddTimer("Bridge groans", 1)
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, evs, "TimerAdded")
	id2, _, err := e.AddTimerAtRound("Reinforcements", 3)
	if err != nil {
		t.Fatal(err)
	}
	if tm, ok := e.Timer(id2); !ok || tm.Remaining != 2 {
		t.Fatalf("r3 in Setup: %+v", tm)
	}
	// Target round <= 1 in Setup refused.
	if _, _, err := e.AddTimerAtRound("Too soon", 1); err == nil {
		t.Error("r1 in Setup should fail")
	}

	// Start combat (Round 1): opening turn does not tick timers (D15).
	must(t)(e.Start())
	if len(e.Timers) != 2 {
		t.Fatalf("Start shouldn't tick timers: %d", len(e.Timers))
	}
	// Advance to Goblin (still Round 1): no tick.
	must(t)(e.Next())
	if len(e.Timers) != 2 {
		t.Fatalf("mid-round Next shouldn't tick timers: %d", len(e.Timers))
	}
	// Wrap to Round 2: "Bridge groans" fires and is removed; "Reinforcements" drops 2 -> 1.
	evs = must(t)(e.Next())
	wantKinds(t, evs, "RoundStarted", "TimerExpired", "TurnStarted")
	if evs[1].(TimerExpired).Name != "Bridge groans" {
		t.Errorf("expired = %+v", evs[1])
	}
	if len(e.Timers) != 1 || e.Timers[0].ID != id2 || e.Timers[0].Remaining != 1 {
		t.Fatalf("timers at Round 2: %+v", e.Timers)
	}

	// Rewind p back to Round 1: both timers are restored to their pre-wrap state!
	must(t)(e.Previous())
	if len(e.Timers) != 2 || e.Timers[0].ID != id1 || e.Timers[0].Remaining != 1 || e.Timers[1].ID != id2 || e.Timers[1].Remaining != 2 {
		t.Fatalf("after Previous: %+v", e.Timers)
	}

	// Cancel id1 early.
	evs = must(t)(e.RemoveTimer(id1))
	wantKinds(t, evs, "TimerRemoved")
	if len(e.Timers) != 1 {
		t.Fatalf("after cancel: %+v", e.Timers)
	}
	// AddTimerAtRound during Active Round 1 with target 4 -> Remaining = 3.
	id3, _, err := e.AddTimerAtRound("Lava rises", 4)
	if err != nil {
		t.Fatal(err)
	}
	if tm, _ := e.Timer(id3); tm.Remaining != 3 {
		t.Errorf("r4 in Round 1: remaining=%d, want 3", tm.Remaining)
	}
	if _, _, err := e.AddTimerAtRound("Past", 1); err == nil {
		t.Error("r1 in Round 1 should fail")
	}
	if _, _, err := e.AddTimer("", 2); err == nil {
		t.Error("blank name should fail")
	}
	if _, _, err := e.AddTimer("Bad", 0); err == nil {
		t.Error("0 rounds should fail")
	}
	if _, err := e.RemoveTimer(999); err == nil {
		t.Error("unknown timer id should fail")
	}

	// Timers do not carry over on End.
	must(t)(e.EndWith(true))
	if next := e.NextEncounter(true); len(next.Timers) != 0 {
		t.Errorf("timers shouldn't carry over: %+v", next.Timers)
	}
}

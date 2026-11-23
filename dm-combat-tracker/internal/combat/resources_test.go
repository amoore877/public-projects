package combat

import "testing"

func TestResourcesCRUDAndTurnReset(t *testing.T) {
	e := newEnc()
	boss, _, err := e.Add(CombatantSpec{Name: "Strahd", Kind: NPC, Initiative: ini(t, "18"), MaxHP: 144})
	if err != nil {
		t.Fatal(err)
	}
	thorin, _, err := e.Add(CombatantSpec{Name: "Thorin", Kind: PC, Initiative: ini(t, "12"), MaxHP: 44})
	if err != nil {
		t.Fatal(err)
	}
	fx, _, err := e.AddEffect(EffectSpec{Name: "Fog", SourceID: boss})
	if err != nil {
		t.Fatal(err)
	}

	// Bounded with turn reset + unbounded counter.
	three := 3
	evs := must(t)(e.SetResource(boss, "LA", 3, &three, true))
	wantKinds(t, evs, "ResourceChanged")
	evs = must(t)(e.SetResourceCurrent(thorin, "Arrows", 14))
	wantKinds(t, evs, "ResourceChanged")
	evs = must(t)(e.SetResource(thorin, "Rxn", 1, intPtr(1), true))
	wantKinds(t, evs, "ResourceChanged")

	// Spend Legendary Actions and Arrows.
	evs = must(t)(e.AdjustResource(boss, "la", -2))
	wantKinds(t, evs, "ResourceChanged")
	if b, _ := e.Combatant(boss); b.Resources[0].Current != 1 {
		t.Fatalf("LA = %d, want 1", b.Resources[0].Current)
	}
	must(t)(e.AdjustResource(thorin, "arrows", -3))
	must(t)(e.SetResourceCurrent(thorin, "rxn", 0))
	// Clamping at 0 and max; no-op when already at bound.
	must(t)(e.AdjustResource(boss, "LA", -10))
	if b, _ := e.Combatant(boss); b.Resources[0].Current != 0 {
		t.Fatalf("LA clamped = %d, want 0", b.Resources[0].Current)
	}
	if evs := must(t)(e.AdjustResource(boss, "LA", -1)); len(evs) != 0 {
		t.Errorf("adjust at 0 should be no-op, got %v", evs)
	}
	if evs := must(t)(e.SetResourceCurrent(boss, "LA", 99)); len(evs) != 1 || evs[0].(ResourceChanged).Current != 3 {
		t.Errorf("set current should clamp to max 3: %+v", evs)
	}
	must(t)(e.AdjustResource(boss, "LA", -2)) // now 1/3

	// Start combat: opening turn doesn't reset (D15).
	must(t)(e.Start())
	if b, _ := e.Combatant(boss); b.Resources[0].Current != 1 {
		t.Fatalf("opening turn shouldn't reset LA: %d", b.Resources[0].Current)
	}
	// Advance past Fog -> Thorin's turn: Thorin's Rxn resets 0 -> 1, Arrows stays 11.
	must(t)(e.Next()) // Fog
	must(t)(e.Next()) // Thorin
	if th, _ := e.Combatant(thorin); th.Resources[1].Current != 1 || th.Resources[0].Current != 11 {
		t.Fatalf("Thorin resources on turn start: Arrows=%d Rxn=%d", th.Resources[0].Current, th.Resources[1].Current)
	}
	// Rewind p restores Thorin's Rxn back to 0 (D67).
	must(t)(e.Previous())
	if th, _ := e.Combatant(thorin); th.Resources[1].Current != 0 {
		t.Fatalf("after Previous, Rxn = %d, want 0", th.Resources[1].Current)
	}
	// Advance to Thorin, then wrap to Round 2 (Strahd): Strahd's LA resets 1 -> 3.
	must(t)(e.Next()) // Thorin
	must(t)(e.Next()) // Strahd (Round 2)
	if b, _ := e.Combatant(boss); b.Resources[0].Current != 3 {
		t.Fatalf("Round 2 Strahd LA = %d, want 3", b.Resources[0].Current)
	}

	// Carry-over PCs preserves Arrows (11) and resets Rxn (even if spent) to max (1) (D68).
	must(t)(e.SetResourceCurrent(thorin, "Rxn", 0))
	must(t)(e.EndWith(true))
	next := e.NextEncounter(true)
	if len(next.Combatants) != 1 || len(next.Combatants[0].Resources) != 2 {
		t.Fatalf("carried resources: %+v", next.Combatants)
	}
	r0, r1 := next.Combatants[0].Resources[0], next.Combatants[0].Resources[1]
	if r0.Name != "Arrows" || r0.Current != 11 || r0.Max != nil {
		t.Errorf("carried Arrows: %+v", r0)
	}
	if r1.Name != "Rxn" || r1.Current != 1 || !r1.ResetOnTurn {
		t.Errorf("carried Rxn: %+v", r1)
	}

	// Removal and error cases on fresh encounter.
	e2 := newEnc()
	id := add(t, e2, "Orc", "10", 15)
	must(t)(e2.SetResource(id, "Rage", 2, intPtr(2), false))
	evs = must(t)(e2.RemoveResource(id, "rage"))
	wantKinds(t, evs, "ResourceRemoved")
	if _, err := e2.RemoveResource(id, "rage"); err == nil {
		t.Error("removing missing resource should fail")
	}
	if _, err := e2.AdjustResource(id, "rage", -1); err == nil {
		t.Error("adjusting missing resource should fail")
	}
	if _, err := e2.SetResource(id, "LA", 3, nil, true); err == nil {
		t.Error("reset without max should fail")
	}
	if _, err := e2.SetResource(id, "", 1, nil, false); err == nil {
		t.Error("blank name should fail")
	}
	if _, err := e2.SetResource(id, "LA", -1, nil, false); err == nil {
		t.Error("negative value should fail")
	}
	if _, err := e2.SetResource(id, "LA", 0, intPtr(0), false); err == nil {
		t.Error("zero max should fail")
	}
	if _, err := e.SetResource(fx, "X", 1, nil, false); err == nil {
		t.Error("ended encounter should fail")
	}
}

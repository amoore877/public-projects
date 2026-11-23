package combat

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestOrdering(t *testing.T) {
	e := newEnc()
	add(t, e, "Mira", "9", 27)
	add(t, e, "Thorin", "18", 44)
	a := add(t, e, "Ash", "12", 10)
	add(t, e, "Bree", "12", 10) // exact tie with Ash: insertion order breaks it
	add(t, e, "Cole", "12.5", 10)
	mira, _ := e.Combatant(1)
	if _, _, err := e.AddEffect(EffectSpec{Name: "Spirit Guardians", SourceID: mira.ID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddEffect(EffectSpec{Name: "Lair", Initiative: ptr(InitiativeFromInt(20))}); err != nil {
		t.Fatal(err)
	}
	add(t, e, "Late", "9", 5) // same init as Mira and her effect: combatants come before effects

	if got, want := orderNames(e), "Lair,Thorin,Cole,Ash,Bree,Mira,Late,Spirit Guardians"; got != want {
		t.Errorf("order = %s, want %s", got, want)
	}

	ties := e.Ties()
	if !ties[a] || len(ties) != 4 { // Ash+Bree, Mira+Late
		t.Errorf("ties = %v", ties)
	}
}

func ptr[T any](v T) *T { return &v }

func TestAddValidation(t *testing.T) {
	e := newEnc()
	bad := []CombatantSpec{
		{Name: "", MaxHP: 5},
		{Name: "x", MaxHP: 0},
		{Name: "x", MaxHP: 5, HP: ptr(6)},
		{Name: "x", MaxHP: 5, TempHP: -1},
		{Name: "x", MaxHP: 5, Kind: "boss"},
	}
	for _, s := range bad {
		if _, _, err := e.Add(s); !errors.Is(err, ErrInvalid) {
			t.Errorf("Add(%+v): err = %v", s, err)
		}
	}
	id, evs, err := e.Add(CombatantSpec{Name: " Thorin ", Kind: PC, Initiative: 18000, MaxHP: 44, HP: ptr(30), TempHP: 5})
	if err != nil {
		t.Fatal(err)
	}
	wantKinds(t, evs, "CombatantAdded")
	c, _ := e.Combatant(id)
	if c.Name != "Thorin" || c.HP != 30 || c.TempHP != 5 || c.Kind != PC {
		t.Errorf("combatant = %+v", c)
	}
	id2 := add(t, e, "Gob", "3", 7)
	c2, _ := e.Combatant(id2)
	if c2.Kind != NPC || c2.HP != 7 {
		t.Errorf("defaults: %+v", c2)
	}
}

func TestAddGroup(t *testing.T) {
	e := newEnc()
	ids, evs, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "Goblin", Initiative: 12000, MaxHP: 7}, Count: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 4 || len(evs) != 4 {
		t.Fatalf("ids=%v evs=%d", ids, len(evs))
	}
	if got := orderNames(e); got != "Goblin 1,Goblin 2,Goblin 3,Goblin 4" {
		t.Errorf("order = %s", got)
	}
	g1, _ := e.Combatant(ids[0])
	if g1.Initiative.String() != "12.4" {
		t.Errorf("Goblin 1 init = %s", g1.Initiative)
	}

	// A second group continues the numbering; separate initiatives are honored.
	_, _, err = e.AddGroup(GroupSpec{
		CombatantSpec: CombatantSpec{Name: "goblin", MaxHP: 7},
		Count:         2,
		Initiatives:   []Initiative{20000, 1000},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := orderNames(e); got != "goblin 5,Goblin 1,Goblin 2,Goblin 3,Goblin 4,goblin 6" {
		t.Errorf("order = %s", got)
	}

	if _, _, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "x", MaxHP: 1}, Count: 2, Initiatives: []Initiative{1}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("mismatched initiatives: %v", err)
	}
	if _, _, err := e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "x", MaxHP: 1}, Count: 0}); !errors.Is(err, ErrInvalid) {
		t.Errorf("count 0: %v", err)
	}
}

func TestGroupCollisionShowsAsTie(t *testing.T) {
	e := newEnc()
	add(t, e, "Thorin", "12.2", 44)
	e.AddGroup(GroupSpec{CombatantSpec: CombatantSpec{Name: "Goblin", Initiative: 12000, MaxHP: 7}, Count: 3})
	ties := e.Ties()
	if len(ties) != 2 {
		t.Errorf("want Thorin and Goblin 2 flagged, got %v", ties)
	}
}

func TestSetInitiativeKeepsTurn(t *testing.T) {
	e := newEnc()
	a := add(t, e, "A", "20", 5)
	b := add(t, e, "B", "10", 5)
	must(t)(e.Start())
	must(t)(e.Next()) // B's turn
	must(t)(e.SetInitiative(b, 30000, AskToMoveEffects))
	if e.Current != b || orderNames(e) != "B,A" {
		t.Errorf("current=%s order=%s", currentName(e), orderNames(e))
	}
	evs := must(t)(e.Next())
	if e.Current != a || e.Round != 1 {
		t.Errorf("after next: current=%s round=%d", currentName(e), e.Round)
	}
	wantKinds(t, evs, "TurnStarted")
}

func TestSerializationRoundTrip(t *testing.T) {
	e := newEnc()
	m := add(t, e, "Mira", "9", 27)
	g := add(t, e, "Gob", "12", 7)
	e.AddCondition(g, "Prone", nil, "")
	e.AddCondition(m, "Blessed", rounds(3), "")
	e.AddEffect(EffectSpec{Name: "SG", SourceID: m, Rounds: rounds(10), Targets: []RowID{g}})
	must(t)(e.Start())
	must(t)(e.Next())

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var back Encounter
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	back.SetClock(fixedClock())
	back.clock, e.clock = nil, nil
	if !reflect.DeepEqual(&back, e) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", back, *e)
	}
	// IDs keep counting from where they left off.
	id := add(t, &back, "New", "1", 1)
	if int(id) != e.NextID+1 {
		t.Errorf("new id = %d after NextID %d", id, e.NextID)
	}
}

func TestEndedIsReadOnly(t *testing.T) {
	e := newEnc()
	a := add(t, e, "A", "1", 5)
	must(t)(e.Start())
	evs := must(t)(e.End())
	wantKinds(t, evs, "CombatEnded")
	if evs[0].(CombatEnded).Rounds != 1 {
		t.Errorf("rounds = %d", evs[0].(CombatEnded).Rounds)
	}
	_, _, err := e.Add(CombatantSpec{Name: "x", MaxHP: 1})
	wantErr(t, err, ErrEnded)
	_, err = e.Damage(a, 1)
	wantErr(t, err, ErrEnded)
	_, err = e.Next()
	wantErr(t, err, ErrNotActive)
	_, err = e.End()
	wantErr(t, err, ErrEnded)
}

func TestLogNote(t *testing.T) {
	e := newEnc()
	add(t, e, "Thorin", "18", 44)
	must(t)(e.Start())
	evs := must(t)(e.LogNote("  Thorin shoves the goblin off the bridge "))
	n := evs[0].(NoteLogged)
	if n.Text != "Thorin shoves the goblin off the bridge" || n.Round != 1 || n.TurnHolder != "Thorin" || !n.At.Equal(t0) {
		t.Errorf("note = %+v", n)
	}
	_, err := e.LogNote("  ")
	wantErr(t, err, ErrInvalid)
}

func TestSetNote(t *testing.T) {
	e := newEnc()
	g := add(t, e, "Gob", "1", 5)
	must(t)(e.SetNote(g, " fleeing "))
	c, _ := e.Combatant(g)
	if c.Note != "fleeing" {
		t.Errorf("note = %q", c.Note)
	}
	_, err := e.SetNote(99, "x")
	wantErr(t, err, ErrNotFound)
}

func TestNextEncounterCarryOverPCs(t *testing.T) {
	e := newEnc()
	must(t)(e.SetRuleset("3.5e"))
	if e.HasPCs() {
		t.Fatal("empty encounter shouldn't have PCs")
	}
	thorin, _, err := e.Add(CombatantSpec{Name: "Thorin", Kind: PC, Initiative: ini(t, "18"), MaxHP: 44, HP: intPtr(31), TempHP: 5, Note: "cursed"})
	if err != nil {
		t.Fatal(err)
	}
	gob := add(t, e, "Goblin", "12", 7)
	mira, _, err := e.Add(CombatantSpec{Name: "Mira", Kind: PC, Initiative: ini(t, "9"), MaxHP: 27, HP: intPtr(-4), Note: "shield"})
	if err != nil {
		t.Fatal(err)
	}
	must(t)(e.NonlethalDamage(thorin, 6))
	// Indefinite condition with a source (source should be cleared) + timed condition (dropped).
	if _, _, err := e.AddConditionFrom(thorin, "Exhausted", nil, gob, "level 1"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddCondition(thorin, "Prone", intPtr(2), ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddCondition(mira, "Stable", nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.AddEffect(EffectSpec{Name: "Fog Cloud", SourceID: mira, Rounds: intPtr(5), Targets: []RowID{gob, thorin}}); err != nil {
		t.Fatal(err)
	}
	if !e.HasPCs() {
		t.Fatal("HasPCs should be true")
	}
	must(t)(e.Start())
	evs := must(t)(e.EndWith(true))
	if !evs[0].(CombatEnded).KeepPCs {
		t.Error("CombatEnded.KeepPCs should be true")
	}

	// Empty next encounter drops everything except the ruleset.
	empty := e.NextEncounter(false)
	if empty.Status != Setup || empty.Ruleset != "3.5e" || len(empty.Combatants) != 0 {
		t.Fatalf("empty next: %+v", empty)
	}

	// Carried-over next encounter keeps only PCs in AddedSeq order, initiative 0.
	next := e.NextEncounter(true)
	if next.Status != Setup || next.Ruleset != "3.5e" || len(next.Effects) != 0 {
		t.Fatalf("carried next: status=%s ruleset=%q effects=%d", next.Status, next.Ruleset, len(next.Effects))
	}
	if len(next.Combatants) != 2 {
		t.Fatalf("expected 2 PCs, got %d", len(next.Combatants))
	}
	c0, c1 := next.Combatants[0], next.Combatants[1]
	if c0.Name != "Thorin" || c0.Kind != PC || c0.Initiative != 0 || c0.HP != 31 || c0.MaxHP != 44 || c0.TempHP != 5 || c0.Nonlethal != 6 || c0.Note != "cursed" {
		t.Errorf("carried Thorin: %+v", c0)
	}
	if len(c0.Conditions) != 1 || c0.Conditions[0].Name != "Exhausted" || c0.Conditions[0].Note != "level 1" || c0.Conditions[0].Remaining != nil || c0.Conditions[0].SourceID != 0 {
		t.Errorf("carried Thorin conditions: %+v", c0.Conditions)
	}
	if c1.Name != "Mira" || c1.Kind != PC || c1.Initiative != 0 || c1.HP != -4 || c1.MaxHP != 27 || c1.Note != "shield" {
		t.Errorf("carried Mira: %+v", c1)
	}
	if len(c1.Conditions) != 1 || c1.Conditions[0].Name != "Stable" {
		t.Errorf("carried Mira conditions: %+v", c1.Conditions)
	}
	if c0.AddedSeq >= c1.AddedSeq {
		t.Errorf("AddedSeq order lost: %d >= %d", c0.AddedSeq, c1.AddedSeq)
	}
	// Ties should flag both carried-over PCs at 0.
	if ties := next.Ties(); !ties[c0.ID] || !ties[c1.ID] {
		t.Errorf("expected tie at initiative 0: %v", ties)
	}
	// Mutating next must not mutate the archived encounter.
	must(t)(next.Heal(c0.ID, 10))
	orig, _ := e.Combatant(thorin)
	if orig.HP != 31 {
		t.Errorf("mutating next changed old encounter HP to %d", orig.HP)
	}
}

func TestRename(t *testing.T) {
	e := newEnc()
	fig := add(t, e, "Hooded Figure", "15", 50)
	fx, _, err := e.AddEffect(EffectSpec{Name: "Mist", SourceID: fig, Rounds: rounds(5), Targets: []RowID{fig}})
	if err != nil {
		t.Fatal(err)
	}
	// Same name is a no-op.
	if evs := must(t)(e.Rename(fig, " Hooded Figure ")); len(evs) != 0 {
		t.Fatalf("same name should emit nothing, got %v", evs)
	}
	// Rename combatant.
	evs := must(t)(e.Rename(fig, " Strahd "))
	wantKinds(t, evs, "RowRenamed")
	rn := evs[0].(RowRenamed)
	if rn.ID != fig || rn.From != "Hooded Figure" || rn.To != "Strahd" {
		t.Errorf("event = %+v", rn)
	}
	c, _ := e.Combatant(fig)
	if c.Name != "Strahd" {
		t.Errorf("name = %q", c.Name)
	}
	// Rename effect.
	evs = must(t)(e.Rename(fx, "Fog Cloud"))
	wantKinds(t, evs, "RowRenamed")
	f, _ := e.Effect(fx)
	if f.Name != "Fog Cloud" {
		t.Errorf("effect name = %q", f.Name)
	}
	// Blank or unknown refused.
	_, err = e.Rename(fig, "  ")
	wantErr(t, err, ErrInvalid)
	_, err = e.Rename(999, "Ghost")
	wantErr(t, err, ErrNotFound)
}

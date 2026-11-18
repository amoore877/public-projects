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

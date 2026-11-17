package combat

import (
	"strings"
	"testing"
)

func TestHP(t *testing.T) {
	type step struct {
		op        string
		n         int
		hp, temp  int
		wantKinds []string
	}
	tests := []struct {
		name  string
		start CombatantSpec
		steps []step
	}{
		{
			name:  "temp absorbs first",
			start: CombatantSpec{MaxHP: 20, TempHP: 5},
			steps: []step{
				{"dmg", 3, 20, 2, []string{"Damaged"}},
				{"dmg", 4, 18, 0, []string{"Damaged"}},
			},
		},
		{
			name:  "down and revive",
			start: CombatantSpec{MaxHP: 10},
			steps: []step{
				{"dmg", 25, 0, 0, []string{"Damaged", "HealthStateChanged"}},
				{"dmg", 3, 0, 0, []string{"Damaged"}}, // already down: no second state change
				{"heal", 4, 4, 0, []string{"Healed", "HealthStateChanged"}},
				{"heal", 100, 10, 0, []string{"Healed"}},
			},
		},
		{
			name:  "set hp clamps and crosses zero",
			start: CombatantSpec{MaxHP: 10},
			steps: []step{
				{"set", 50, 10, 0, []string{"HPSet"}},
				{"set", -5, 0, 0, []string{"HPSet", "HealthStateChanged"}},
				{"set", 3, 3, 0, []string{"HPSet", "HealthStateChanged"}},
			},
		},
		{
			name:  "temp hp replaces and heal leaves it alone",
			start: CombatantSpec{MaxHP: 10, HP: ptr(5), TempHP: 8},
			steps: []step{
				{"temp", 3, 5, 3, []string{"TempHPSet"}},
				{"heal", 2, 7, 3, []string{"Healed"}},
				{"dmg", 0, 7, 3, []string{"Damaged"}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newEnc()
			tt.start.Name = "X"
			id, _, err := e.Add(tt.start)
			if err != nil {
				t.Fatal(err)
			}
			c, _ := e.Combatant(id)
			for i, s := range tt.steps {
				var evs []Event
				switch s.op {
				case "dmg":
					evs, err = e.Damage(id, s.n)
				case "heal":
					evs, err = e.Heal(id, s.n)
				case "set":
					evs, err = e.SetHP(id, s.n)
				case "temp":
					evs, err = e.SetTempHP(id, s.n)
				}
				if err != nil {
					t.Fatalf("step %d: %v", i, err)
				}
				if c.HP != s.hp || c.TempHP != s.temp {
					t.Errorf("step %d (%s %d): hp=%d temp=%d, want %d/%d", i, s.op, s.n, c.HP, c.TempHP, s.hp, s.temp)
				}
				wantKinds(t, evs, s.wantKinds...)
			}
		})
	}
}

func TestHPEvents(t *testing.T) {
	e := newEnc()
	id, _, _ := e.Add(CombatantSpec{Name: "X", MaxHP: 10, HP: ptr(8), TempHP: 3})
	evs := must(t)(e.Damage(id, 5))
	d := evs[0].(Damaged)
	if d.Amount != 5 || d.AbsorbedByTemp != 3 || d.HP != 6 || d.TempHP != 0 {
		t.Errorf("damaged = %+v", d)
	}
	evs = must(t)(e.Heal(id, 10))
	if h := evs[0].(Healed); h.Amount != 4 || h.HP != 10 {
		t.Errorf("healed = %+v", h)
	}
}

func TestHPErrors(t *testing.T) {
	e := newEnc()
	id := add(t, e, "X", "1", 10)
	fx, _, _ := e.AddEffect(EffectSpec{Name: "F", Initiative: ptr(Initiative(0))})
	_, err := e.Damage(id, -1)
	wantErr(t, err, ErrInvalid)
	_, err = e.Heal(fx, 1)
	wantErr(t, err, ErrNotCombatant)
	_, err = e.SetTempHP(42, 1)
	wantErr(t, err, ErrNotFound)
}

func TestBloodied(t *testing.T) {
	c := &Combatant{MaxHP: 7}
	for hp, want := range map[int]bool{7: false, 4: false, 3: true, 1: true, 0: false} {
		c.HP = hp
		if c.Bloodied() != want {
			t.Errorf("hp %d/7 bloodied = %v", hp, !want)
		}
	}
}

func newEnc35(t *testing.T) *Encounter {
	t.Helper()
	e := newEnc()
	must(t)(e.SetRuleset("3.5e"))
	return e
}

// 3.5e: HP runs below zero to -10, healing counts up from the real value,
// and each move between disabled, dying and dead is reported (RS2, RS3).
func TestHP35(t *testing.T) {
	e := newEnc35(t)
	id := add(t, e, "Orc", "12", 22)
	c, _ := e.Combatant(id)
	state := func() string { return stateName(e.HealthState(c)) }
	change := func(evs []Event) string {
		t.Helper()
		if len(evs) != 2 {
			t.Fatalf("want a state change, got %v", kinds(evs))
		}
		ch := evs[1].(HealthStateChanged)
		if ch.HP != c.HP || ch.ID != id || ch.Name != "Orc" {
			t.Errorf("change %+v", ch)
		}
		return ch.From + ">" + ch.To
	}

	if got := change(must(t)(e.Damage(id, 22))); got != ">disabled" || c.HP != 0 {
		t.Errorf("%s at %d", got, c.HP)
	}
	if got := change(must(t)(e.Damage(id, 4))); got != "disabled>dying" || c.HP != -4 {
		t.Errorf("%s at %d", got, c.HP)
	}
	wantKinds(t, must(t)(e.Damage(id, 3)), "Damaged") // -7: still dying
	evs := must(t)(e.Heal(id, 5))
	wantKinds(t, evs, "Healed")
	if h := evs[0].(Healed); c.HP != -2 || h.Amount != 5 || h.HP != -2 {
		t.Errorf("healing 5 at -7 should give -2: hp %d, %+v", c.HP, h)
	}
	if got := change(must(t)(e.Heal(id, 2))); got != "dying>disabled" {
		t.Error(got)
	}
	if got := change(must(t)(e.Heal(id, 1))); got != "disabled>" || state() != "" {
		t.Error(got)
	}
	// Straight past disabled and dying; the floor is -10.
	if got := change(must(t)(e.Damage(id, 500))); got != ">dead" || c.HP != -10 {
		t.Errorf("%s at %d", got, c.HP)
	}
	wantKinds(t, must(t)(e.Damage(id, 5)), "Damaged")
	if got := change(must(t)(e.SetHP(id, -9))); got != "dead>dying" {
		t.Error(got)
	}
	must(t)(e.SetHP(id, -99))
	if c.HP != -10 || state() != "dead" {
		t.Errorf("set below the floor: %d %s", c.HP, state())
	}
	if got := change(must(t)(e.SetHP(id, 99))); got != "dead>" || c.HP != 22 {
		t.Errorf("%s at %d", got, c.HP)
	}
}

func TestHealthStates(t *testing.T) {
	five, _ := LookupRuleset("5e")
	three, _ := LookupRuleset("3.5e")
	for hp, want := range map[int]string{5: "", 1: "", 0: "down"} {
		if got := stateName(five.State(hp)); got != want {
			t.Errorf("5e at %d: %q, want %q", hp, got, want)
		}
	}
	for hp, want := range map[int]string{1: "", 0: "disabled", -1: "dying", -9: "dying", -10: "dead"} {
		if got := stateName(three.State(hp)); got != want {
			t.Errorf("3.5e at %d: %q, want %q", hp, got, want)
		}
	}
	if s := three.State(0); !s.Acts || s.Remind {
		t.Errorf("disabled still acts: %+v", s)
	}
	if s := three.State(-3); s.Acts || !s.Remind {
		t.Errorf("dying: %+v", s)
	}
	if five.HPFloor != 0 || three.HPFloor != -10 {
		t.Errorf("floors %d %d", five.HPFloor, three.HPFloor)
	}
}

// A combatant can join already below zero where the ruleset allows it.
func TestAddBelowZero(t *testing.T) {
	e := newEnc()
	_, _, err := e.Add(CombatantSpec{Name: "X", MaxHP: 10, HP: ptr(-1)})
	wantErr(t, err, ErrInvalid)

	e = newEnc35(t)
	id, _, err := e.Add(CombatantSpec{Name: "X", MaxHP: 10, HP: ptr(-4)})
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := e.Combatant(id); c.HP != -4 || stateName(e.HealthState(c)) != "dying" || c.Bloodied() {
		t.Errorf("joined at %d", c.HP)
	}
	_, _, err = e.Add(CombatantSpec{Name: "Y", MaxHP: 10, HP: ptr(-11)})
	wantErr(t, err, ErrInvalid)
	if err == nil || !strings.Contains(err.Error(), "between -10 and max HP") {
		t.Errorf("err %v", err)
	}
}

// Nonlethal damage keeps its own total; staggered and unconscious come from
// comparing it with HP, and healing lowers it too (RS4).
func TestNonlethal(t *testing.T) {
	e := newEnc()
	id := add(t, e, "Orc", "12", 10)
	_, err := e.NonlethalDamage(id, 3)
	wantErr(t, err, ErrNoNonlethal)
	_, err = e.SetNonlethal(id, 3)
	wantErr(t, err, ErrNoNonlethal)

	e = newEnc35(t)
	id = add(t, e, "Orc", "12", 10)
	c, _ := e.Combatant(id)
	state := func() string { return stateName(e.HealthState(c)) }
	must(t)(e.Damage(id, 4)) // 6 HP
	evs := must(t)(e.NonlethalDamage(id, 5))
	wantKinds(t, evs, "NonlethalDamaged")
	if d := evs[0].(NonlethalDamaged); d.Amount != 5 || d.Total != 5 || c.HP != 6 {
		t.Errorf("%+v, hp %d", d, c.HP)
	}
	evs = must(t)(e.NonlethalDamage(id, 1))
	wantKinds(t, evs, "NonlethalDamaged", "HealthStateChanged")
	if ch := evs[1].(HealthStateChanged); ch.To != "staggered" || ch.Nonlethal != 6 || ch.HP != 6 {
		t.Errorf("%+v", ch)
	}
	if s := e.HealthState(c); !s.Acts || s.Label() != "STAGR" {
		t.Errorf("staggered %+v", s)
	}
	wantKinds(t, must(t)(e.NonlethalDamage(id, 1)), "NonlethalDamaged", "HealthStateChanged")
	if state() != "unconscious" || e.HealthState(c).Acts {
		t.Errorf("state %s", state())
	}

	// Healing 2 brings HP to 8 and nonlethal to 5: no state.
	evs = must(t)(e.Heal(id, 2))
	wantKinds(t, evs, "Healed", "HealthStateChanged")
	if h := evs[0].(Healed); h.Amount != 2 || h.NonlethalHealed != 2 || c.Nonlethal != 5 || state() != "" {
		t.Errorf("%+v, nonlethal %d, state %s", h, c.Nonlethal, state())
	}
	// Healing at full HP still removes nonlethal damage.
	must(t)(e.SetHP(id, 10))
	if h := must(t)(e.Heal(id, 3))[0].(Healed); h.Amount != 0 || h.NonlethalHealed != 3 || c.Nonlethal != 2 {
		t.Errorf("%+v, nonlethal %d", h, c.Nonlethal)
	}
	if h := must(t)(e.Heal(id, 9))[0].(Healed); h.NonlethalHealed != 2 || c.Nonlethal != 0 {
		t.Errorf("%+v, nonlethal %d", h, c.Nonlethal)
	}

	// An HP state wins over a nonlethal one; temp HP doesn't absorb nonlethal.
	must(t)(e.SetTempHP(id, 5))
	must(t)(e.NonlethalDamage(id, 20))
	if c.TempHP != 5 || state() != "unconscious" {
		t.Errorf("temp %d, state %s", c.TempHP, state())
	}
	if ch := must(t)(e.SetHP(id, -3))[1].(HealthStateChanged); ch.From != "unconscious" || ch.To != "dying" {
		t.Errorf("%+v", ch)
	}
	evs = must(t)(e.SetNonlethal(id, 0))
	wantKinds(t, evs, "NonlethalSet") // still dying
	if s := evs[0].(NonlethalSet); s.From != 20 || s.To != 0 {
		t.Errorf("%+v", s)
	}
	_, err = e.NonlethalDamage(id, -1)
	wantErr(t, err, ErrInvalid)
}

func TestSetMaxHP(t *testing.T) {
	e := newEnc()
	id := add(t, e, "Orc", "12", 30)
	c, _ := e.Combatant(id)
	must(t)(e.Damage(id, 5)) // 25/30
	evs := must(t)(e.SetMaxHP(id, 40))
	wantKinds(t, evs, "MaxHPSet")
	if s := evs[0].(MaxHPSet); s.From != 30 || s.To != 40 || s.HP != 25 || c.HP != 25 {
		t.Errorf("raising max doesn't heal: %+v", s)
	}
	if s := must(t)(e.SetMaxHP(id, 24))[0].(MaxHPSet); s.HP != 24 || c.HP != 24 || c.MaxHP != 24 {
		t.Errorf("lowering max brings HP down: %+v", s)
	}
	wantKinds(t, must(t)(e.SetMaxHP(id, 24))) // no change, no event
	_, err := e.SetMaxHP(id, 0)
	wantErr(t, err, ErrInvalid)

	// Below zero, a lower max leaves HP where it is.
	e = newEnc35(t)
	id = add(t, e, "Orc", "12", 30)
	c, _ = e.Combatant(id)
	must(t)(e.SetHP(id, -4))
	wantKinds(t, must(t)(e.SetMaxHP(id, 10)), "MaxHPSet")
	if c.HP != -4 {
		t.Errorf("hp %d", c.HP)
	}
}

func TestAddTempHP(t *testing.T) {
	e := newEnc()
	id := add(t, e, "Mira", "9", 27)
	c, _ := e.Combatant(id)
	must(t)(e.SetTempHP(id, 5))
	evs := must(t)(e.AddTempHP(id, 3))
	if s := evs[0].(TempHPSet); s.From != 5 || s.To != 8 || !s.Added || c.TempHP != 8 {
		t.Errorf("%+v", s)
	}
	if s := must(t)(e.SetTempHP(id, 2))[0].(TempHPSet); s.Added || c.TempHP != 2 {
		t.Errorf("%+v", s)
	}
	_, err := e.AddTempHP(id, -1)
	wantErr(t, err, ErrInvalid)
}

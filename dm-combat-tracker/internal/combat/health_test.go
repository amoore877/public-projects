package combat

import (
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
				{"dmg", 25, 0, 0, []string{"Damaged", "Downed"}},
				{"dmg", 3, 0, 0, []string{"Damaged"}}, // already down: no second Downed
				{"heal", 4, 4, 0, []string{"Healed", "Revived"}},
				{"heal", 100, 10, 0, []string{"Healed"}},
			},
		},
		{
			name:  "set hp clamps and crosses zero",
			start: CombatantSpec{MaxHP: 10},
			steps: []step{
				{"set", 50, 10, 0, []string{"HPSet"}},
				{"set", -5, 0, 0, []string{"HPSet", "Downed"}},
				{"set", 3, 3, 0, []string{"HPSet", "Revived"}},
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

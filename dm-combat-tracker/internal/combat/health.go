package combat

import "fmt"

// Damage takes n from temp HP first, then HP, stopping at the ruleset's HP
// floor (RS2).
func (e *Encounter) Damage(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	absorbed := min(c.TempHP, n)
	c.TempHP -= absorbed
	before := e.HealthState(c)
	c.HP = max(c.HP-(n-absorbed), e.Rules().HPFloor)
	evs := []Event{Damaged{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: n, HP: c.HP, TempHP: c.TempHP, AbsorbedByTemp: absorbed}}
	return append(evs, e.stateChange(c, before)...), nil
}

// Heal restores up to n HP, capped at max HP, counting up from the real
// value (healing 5 at -7 gives -2). Temp HP is unchanged.
func (e *Encounter) Heal(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	before := e.HealthState(c)
	from := c.HP
	c.HP = min(c.HP+n, c.MaxHP)
	evs := []Event{Healed{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: c.HP - from, HP: c.HP}}
	return append(evs, e.stateChange(c, before)...), nil
}

// SetHP overrides HP directly, clamped to [HP floor, MaxHP].
func (e *Encounter) SetHP(id RowID, n int) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	before := e.HealthState(c)
	from := c.HP
	c.HP = min(max(n, e.Rules().HPFloor), c.MaxHP)
	evs := []Event{HPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: c.HP}}
	return append(evs, e.stateChange(c, before)...), nil
}

// SetTempHP replaces temp HP (it doesn't stack).
func (e *Encounter) SetTempHP(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	from := c.TempHP
	c.TempHP = n
	return []Event{TempHPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: n}}, nil
}

func (e *Encounter) hpTarget(id RowID, n int) (*Combatant, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("%w: amount can't be negative", ErrInvalid)
	}
	return e.mustCombatant(id)
}

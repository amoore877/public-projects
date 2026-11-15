package combat

import "fmt"

// Damage takes n from temp HP first, then HP, clamping at 0.
func (e *Encounter) Damage(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	absorbed := min(c.TempHP, n)
	c.TempHP -= absorbed
	wasUp := !c.Down()
	c.HP = max(c.HP-(n-absorbed), 0)
	evs := []Event{Damaged{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: n, HP: c.HP, TempHP: c.TempHP, AbsorbedByTemp: absorbed}}
	if wasUp && c.Down() {
		evs = append(evs, Downed{EventMeta: e.meta(), ID: c.ID, Name: c.Name})
	}
	return evs, nil
}

// Heal restores up to n HP, capped at max HP. Temp HP is unchanged.
func (e *Encounter) Heal(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	wasDown := c.Down()
	before := c.HP
	c.HP = min(c.HP+n, c.MaxHP)
	evs := []Event{Healed{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: c.HP - before, HP: c.HP}}
	if wasDown && !c.Down() {
		evs = append(evs, Revived{EventMeta: e.meta(), ID: c.ID, Name: c.Name})
	}
	return evs, nil
}

// SetHP overrides HP directly, clamped to [0, MaxHP].
func (e *Encounter) SetHP(id RowID, n int) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	wasDown := c.Down()
	from := c.HP
	c.HP = min(max(n, 0), c.MaxHP)
	evs := []Event{HPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: c.HP}}
	switch {
	case !wasDown && c.Down():
		evs = append(evs, Downed{EventMeta: e.meta(), ID: c.ID, Name: c.Name})
	case wasDown && !c.Down():
		evs = append(evs, Revived{EventMeta: e.meta(), ID: c.ID, Name: c.Name})
	}
	return evs, nil
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

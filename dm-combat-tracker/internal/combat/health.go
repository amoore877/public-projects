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
// value (healing 5 at -7 gives -2). In rulesets that track it, it also
// removes up to n nonlethal damage, even at full HP (RS4). Temp HP is
// unchanged.
func (e *Encounter) Heal(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	before := e.HealthState(c)
	from := c.HP
	c.HP = min(c.HP+n, c.MaxHP)
	nl := 0
	if e.Rules().Nonlethal {
		nl = min(n, c.Nonlethal)
		c.Nonlethal -= nl
	}
	evs := []Event{Healed{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: c.HP - from, HP: c.HP, NonlethalHealed: nl}}
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

// SetTempHP replaces temp HP.
func (e *Encounter) SetTempHP(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	from := c.TempHP
	c.TempHP = n
	return []Event{TempHPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: n}}, nil
}

// AddTempHP adds n to temp HP, for sources that stack. Which ones do is the
// DM's call (RS6).
func (e *Encounter) AddTempHP(id RowID, n int) ([]Event, error) {
	c, err := e.hpTarget(id, n)
	if err != nil {
		return nil, err
	}
	from := c.TempHP
	c.TempHP += n
	return []Event{TempHPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: c.TempHP, Added: true}}, nil
}

// SetMaxHP changes max HP, in every ruleset (RS5). A lower max brings
// current HP down to it; a higher one leaves current HP alone, since whether
// the creature also heals depends on the rule.
func (e *Encounter) SetMaxHP(id RowID, n int) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	if n < 1 {
		return nil, fmt.Errorf("%w: max HP must be at least 1", ErrInvalid)
	}
	if n == c.MaxHP {
		return nil, nil
	}
	before := e.HealthState(c)
	from := c.MaxHP
	c.MaxHP = n
	c.HP = min(c.HP, n)
	evs := []Event{MaxHPSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: n, HP: c.HP}}
	return append(evs, e.stateChange(c, before)...), nil
}

// NonlethalDamage adds n to the combatant's nonlethal total (RS4). Temp HP
// doesn't absorb it.
func (e *Encounter) NonlethalDamage(id RowID, n int) ([]Event, error) {
	c, err := e.nonlethalTarget(id, n)
	if err != nil {
		return nil, err
	}
	before := e.HealthState(c)
	c.Nonlethal += n
	evs := []Event{NonlethalDamaged{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Amount: n, Total: c.Nonlethal}}
	return append(evs, e.stateChange(c, before)...), nil
}

// SetNonlethal sets the nonlethal total directly, to fix a mistake.
func (e *Encounter) SetNonlethal(id RowID, n int) ([]Event, error) {
	c, err := e.nonlethalTarget(id, n)
	if err != nil {
		return nil, err
	}
	before := e.HealthState(c)
	from := c.Nonlethal
	c.Nonlethal = n
	evs := []Event{NonlethalSet{EventMeta: e.meta(), ID: c.ID, Name: c.Name, From: from, To: n}}
	return append(evs, e.stateChange(c, before)...), nil
}

func (e *Encounter) nonlethalTarget(id RowID, n int) (*Combatant, error) {
	if !e.Rules().Nonlethal {
		return nil, fmt.Errorf("%w (%s)", ErrNoNonlethal, e.Rules().Name)
	}
	return e.hpTarget(id, n)
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

package combat

import (
	"fmt"
	"strings"
)

// AddCondition puts a condition on a combatant. rounds is nil for an
// indefinite condition; otherwise it counts down at the start of the
// combatant's turns (D12).
func (e *Encounter) AddCondition(id RowID, name string, rounds *int, note string) (RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return 0, nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return 0, nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil, fmt.Errorf("%w: condition name is required", ErrInvalid)
	}
	if rounds != nil && *rounds < 1 {
		return 0, nil, fmt.Errorf("%w: duration must be at least 1 round", ErrInvalid)
	}
	cond := &Condition{ID: e.newID(), Name: name, Remaining: copyIntPtr(rounds), Note: strings.TrimSpace(note)}
	c.Conditions = append(c.Conditions, cond)
	return cond.ID, []Event{ConditionApplied{
		EventMeta: e.meta(), TargetID: c.ID, Target: c.Name,
		Condition: name, Remaining: copyIntPtr(rounds), Note: cond.Note,
	}}, nil
}

// RemoveCondition removes one of a combatant's own conditions by ID.
func (e *Encounter) RemoveCondition(id, condID RowID) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	for i, cond := range c.Conditions {
		if cond.ID == condID {
			c.Conditions = append(c.Conditions[:i], c.Conditions[i+1:]...)
			return []Event{ConditionRemoved{EventMeta: e.meta(), TargetID: c.ID, Target: c.Name, Condition: cond.Name}}, nil
		}
	}
	return nil, fmt.Errorf("%w: condition %d on %s", ErrNotFound, condID, c.Name)
}

// FindCondition returns the first of a combatant's own conditions with the
// given name (case-insensitive).
func (c *Combatant) FindCondition(name string) (*Condition, bool) {
	for _, cond := range c.Conditions {
		if strings.EqualFold(cond.Name, strings.TrimSpace(name)) {
			return cond, true
		}
	}
	return nil, false
}

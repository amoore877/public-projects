package combat

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseDuration parses a round count with an optional trailing "e" for
// end-of-turn (e.g. "2" → 2, false; "1e" → 1, true; ET1, D72).
func ParseDuration(s string) (rounds int, endOfTurn bool, err error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return 0, false, fmt.Errorf("%w: duration is required", ErrInvalid)
	}
	num := raw
	if last := raw[len(raw)-1]; last == 'e' || last == 'E' {
		endOfTurn = true
		num = raw[:len(raw)-1]
	}
	n, convErr := strconv.Atoi(num)
	if convErr != nil || n < 1 || strings.HasPrefix(num, "+") {
		return 0, false, fmt.Errorf("%w: %q isn't a number of rounds (e.g. 2 or 1e for end of turn)", ErrInvalid, raw)
	}
	return n, endOfTurn, nil
}

// AddCondition puts a condition on a combatant. rounds is nil for an
// indefinite condition; otherwise it counts down at the start of the
// combatant's turns (D12).
func (e *Encounter) AddCondition(id RowID, name string, rounds *int, note string) (RowID, []Event, error) {
	return e.AddConditionEnd(id, name, rounds, false, 0, note)
}

// AddConditionFrom is AddCondition with the creature that caused the
// condition (RS7, G8). A timed condition then counts down at the start of the
// source's turns instead of the bearer's. source 0, or the bearer itself,
// means no source.
func (e *Encounter) AddConditionFrom(id RowID, name string, rounds *int, source RowID, note string) (RowID, []Event, error) {
	return e.AddConditionEnd(id, name, rounds, false, source, note)
}

// AddConditionEnd is AddConditionFrom with an optional end-of-turn anchor
// (ET1–ET2, D72–D73). When rounds is nil, endOfTurn is ignored.
func (e *Encounter) AddConditionEnd(id RowID, name string, rounds *int, endOfTurn bool, source RowID, note string) (RowID, []Event, error) {
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
	if rounds == nil {
		endOfTurn = false
	}
	var sourceName string
	if source == id {
		source = 0
	}
	if source != 0 {
		src, ok := e.Combatant(source)
		if !ok {
			return 0, nil, fmt.Errorf("%w: source %d isn't a combatant", ErrNotFound, source)
		}
		sourceName = src.Name
	}
	cond := &Condition{ID: e.newID(), Name: name, Remaining: copyIntPtr(rounds), EndOfTurn: endOfTurn, Note: strings.TrimSpace(note), SourceID: source}
	c.Conditions = append(c.Conditions, cond)
	return cond.ID, []Event{ConditionApplied{
		EventMeta: e.meta(), TargetID: c.ID, Target: c.Name,
		Condition: name, Remaining: copyIntPtr(rounds), EndOfTurn: endOfTurn, Note: cond.Note, Source: sourceName,
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

package combat

import (
	"fmt"
	"strings"
)

// SetResource creates or updates a named counter on a combatant (RP1, D66).
// Matching by name is case-insensitive. Bounded counters require *maxVal >= 1
// and clamp cur to [0, *maxVal]; unbounded counters (maxVal == nil) require
// resetOnTurn == false and cur >= 0.
func (e *Encounter) SetResource(id RowID, name string, cur int, maxVal *int, resetOnTurn bool) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: resource name is required", ErrInvalid)
	}
	if cur < 0 {
		return nil, fmt.Errorf("%w: resource value can't be negative", ErrInvalid)
	}
	if maxVal != nil {
		if *maxVal < 1 {
			return nil, fmt.Errorf("%w: resource max must be at least 1", ErrInvalid)
		}
		if cur > *maxVal {
			cur = *maxVal
		}
	} else if resetOnTurn {
		return nil, fmt.Errorf("%w: turn reset requires a max (e.g. 3/3 reset)", ErrInvalid)
	}
	if existing := findResource(c, name); existing != nil {
		existing.Name = name
		existing.Current = cur
		existing.Max = copyIntPtr(maxVal)
		existing.ResetOnTurn = resetOnTurn
		return []Event{ResourceChanged{
			EventMeta: e.meta(), ID: c.ID, Target: c.Name,
			Name: existing.Name, Current: existing.Current, Max: copyIntPtr(existing.Max), ResetOnTurn: existing.ResetOnTurn,
		}}, nil
	}
	r := &Resource{Name: name, Current: cur, Max: copyIntPtr(maxVal), ResetOnTurn: resetOnTurn}
	c.Resources = append(c.Resources, r)
	return []Event{ResourceChanged{
		EventMeta: e.meta(), ID: c.ID, Target: c.Name,
		Name: r.Name, Current: r.Current, Max: copyIntPtr(r.Max), ResetOnTurn: r.ResetOnTurn,
	}}, nil
}

// SetResourceCurrent sets an existing counter's current value (preserving its
// Max and ResetOnTurn), or creates a new unbounded counter when no counter
// with that name exists yet.
func (e *Encounter) SetResourceCurrent(id RowID, name string, cur int) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: resource name is required", ErrInvalid)
	}
	if cur < 0 {
		return nil, fmt.Errorf("%w: resource value can't be negative", ErrInvalid)
	}
	if existing := findResource(c, name); existing != nil {
		if existing.Max != nil && cur > *existing.Max {
			cur = *existing.Max
		}
		if existing.Current == cur {
			return nil, nil
		}
		existing.Current = cur
		return []Event{ResourceChanged{
			EventMeta: e.meta(), ID: c.ID, Target: c.Name,
			Name: existing.Name, Current: existing.Current, Max: copyIntPtr(existing.Max), ResetOnTurn: existing.ResetOnTurn,
		}}, nil
	}
	return e.SetResource(id, name, cur, nil, false)
}

// AdjustResource adds delta (positive or negative) to an existing counter,
// clamping to [0, *Max] (or >= 0 when unbounded).
func (e *Encounter) AdjustResource(id RowID, name string, delta int) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	existing := findResource(c, name)
	if existing == nil {
		return nil, fmt.Errorf("%w: %s has no resource %q", ErrNotFound, c.Name, name)
	}
	next := existing.Current + delta
	if next < 0 {
		next = 0
	}
	if existing.Max != nil && next > *existing.Max {
		next = *existing.Max
	}
	if next == existing.Current {
		return nil, nil
	}
	existing.Current = next
	return []Event{ResourceChanged{
		EventMeta: e.meta(), ID: c.ID, Target: c.Name,
		Name: existing.Name, Current: existing.Current, Max: copyIntPtr(existing.Max), ResetOnTurn: existing.ResetOnTurn,
	}}, nil
}

// RemoveResource deletes a named counter from a combatant.
func (e *Encounter) RemoveResource(id RowID, name string) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	for i, r := range c.Resources {
		if strings.EqualFold(r.Name, name) {
			removed := r.Name
			c.Resources = append(c.Resources[:i], c.Resources[i+1:]...)
			return []Event{ResourceRemoved{EventMeta: e.meta(), ID: c.ID, Target: c.Name, Name: removed}}, nil
		}
	}
	return nil, fmt.Errorf("%w: %s has no resource %q", ErrNotFound, c.Name, name)
}

func findResource(c *Combatant, name string) *Resource {
	for _, r := range c.Resources {
		if strings.EqualFold(r.Name, name) {
			return r
		}
	}
	return nil
}

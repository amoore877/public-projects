package combat

import (
	"fmt"
	"strings"
)

// EffectSpec describes an area effect to start.
type EffectSpec struct {
	Name       string
	SourceID   RowID       // zero for environmental/lair effects
	Initiative *Initiative // defaults to the source's initiative; required without a source
	Rounds     *int        // nil for indefinite
	EndOfTurn  bool        // count down when Next leaves the anchor's turn (ET1, D72)
	Targets    []RowID
	Note       string
}

// AddEffect starts an effect. It gets its own row (by default right after its
// source) and shows as a linked condition on each target.
func (e *Encounter) AddEffect(spec EffectSpec) (RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return 0, nil, err
	}
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		return 0, nil, fmt.Errorf("%w: effect name is required", ErrInvalid)
	}
	if spec.Rounds != nil && *spec.Rounds < 1 {
		return 0, nil, fmt.Errorf("%w: duration must be at least 1 round", ErrInvalid)
	}
	var init Initiative
	sourceName := ""
	switch {
	case spec.Initiative != nil:
		init = *spec.Initiative
	case spec.SourceID != 0:
	default:
		return 0, nil, fmt.Errorf("%w: an effect without a source needs an initiative", ErrInvalid)
	}
	if spec.SourceID != 0 {
		src, err := e.mustCombatant(spec.SourceID)
		if err != nil {
			return 0, nil, fmt.Errorf("effect source: %w", err)
		}
		sourceName = src.Name
		if spec.Initiative == nil {
			init = src.Initiative
		}
	}
	targets, names, err := e.resolveTargets(nil, spec.Targets)
	if err != nil {
		return 0, nil, err
	}
	endOfTurn := spec.EndOfTurn && spec.Rounds != nil
	fx := &Effect{
		ID:         e.newID(),
		Name:       name,
		SourceID:   spec.SourceID,
		Initiative: init,
		Remaining:  copyIntPtr(spec.Rounds),
		EndOfTurn:  endOfTurn,
		Targets:    targets,
		Note:       strings.TrimSpace(spec.Note),
		AddedSeq:   e.newSeq(),
	}
	e.Effects = append(e.Effects, fx)
	return fx.ID, []Event{EffectStarted{
		EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, Source: sourceName,
		Remaining: copyIntPtr(fx.Remaining), EndOfTurn: endOfTurn, Targets: names,
	}}, nil
}

// AddTargets adds combatants to an effect (they moved into the area).
func (e *Encounter) AddTargets(id RowID, targets []RowID) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	fx, err := e.mustEffect(id)
	if err != nil {
		return nil, err
	}
	added, names, err := e.resolveTargets(fx.Targets, targets)
	if err != nil {
		return nil, err
	}
	if len(added) == 0 {
		return nil, nil
	}
	fx.Targets = append(fx.Targets, added...)
	return []Event{EffectTargetsChanged{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, Added: names}}, nil
}

// RemoveTargets removes combatants from an effect (they left the area).
func (e *Encounter) RemoveTargets(id RowID, targets []RowID) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	fx, err := e.mustEffect(id)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, t := range targets {
		if !containsID(fx.Targets, t) {
			continue
		}
		fx.Targets = removeID(fx.Targets, t)
		if c, ok := e.Combatant(t); ok {
			names = append(names, c.Name)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	return []Event{EffectTargetsChanged{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, Removed: names}}, nil
}

// EndEffect ends or dispels an effect early, removing its row and every
// linked condition at once.
func (e *Encounter) EndEffect(id RowID, reason string) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	fx, err := e.mustEffect(id)
	if err != nil {
		return nil, err
	}
	order := e.Order()
	oldIdx := indexOf(order, e.Current)
	evs := []Event{EffectEnded{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, Reason: strings.TrimSpace(reason)}}
	e.dropEffect(id)
	if e.Current == id && e.Status == Active {
		evs = append(evs, e.passTurn(order, oldIdx)...)
	}
	return evs, nil
}

func (e *Encounter) dropEffect(id RowID) {
	for i, fx := range e.Effects {
		if fx.ID == id {
			e.Effects = append(e.Effects[:i], e.Effects[i+1:]...)
			return
		}
	}
}

// resolveTargets validates new target IDs, skipping duplicates and any
// already in existing. It returns the new IDs and their names.
func (e *Encounter) resolveTargets(existing, ids []RowID) ([]RowID, []string, error) {
	var out []RowID
	var names []string
	for _, id := range ids {
		c, err := e.mustCombatant(id)
		if err != nil {
			return nil, nil, fmt.Errorf("effect target: %w", err)
		}
		if containsID(existing, id) || containsID(out, id) {
			continue
		}
		out = append(out, id)
		names = append(names, c.Name)
	}
	return out, names, nil
}

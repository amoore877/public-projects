package combat

import "fmt"

// TurnRecord is what Previous needs to undo one Next: the old pointer and
// round, plus every duration change made when the new turn started.
type TurnRecord struct {
	PrevCurrent       RowID              `json:"prevCurrent"`
	PrevRound         int                `json:"prevRound"`
	Ticked            []RowID            `json:"ticked,omitempty"` // conditions/effects decremented but still present
	ExpiredConditions []ExpiredCondition `json:"expiredConditions,omitempty"`
	ExpiredEffects    []Effect           `json:"expiredEffects,omitempty"`
}

// ExpiredCondition is a condition removed by a countdown, kept for rewind.
type ExpiredCondition struct {
	OwnerID   RowID     `json:"ownerId"`
	Index     int       `json:"index"`
	Condition Condition `json:"condition"` // as it was before the final tick
}

// Start begins combat: round 1, first row in order. Durations do not tick on
// the opening turn; anything applied during setup starts counting from the
// next turn of its anchor.
func (e *Encounter) Start() ([]Event, error) {
	if e.Status != Setup {
		return nil, ErrNotSetup
	}
	if len(e.Combatants) == 0 {
		return nil, ErrEmpty
	}
	order := e.Order()
	e.Status = Active
	e.Round = 1
	e.Current = order[0].ID()
	entries := make([]OrderEntry, len(order))
	for i, r := range order {
		entries[i] = OrderEntry{Name: r.Name(), Initiative: r.Initiative(), IsEffect: r.Effect != nil}
	}
	m := e.meta()
	return []Event{
		CombatStarted{EventMeta: m, Order: entries},
		RoundStarted{EventMeta: m},
		TurnStarted{EventMeta: m, ID: e.Current, Name: m.TurnHolder},
	}, nil
}

// End finishes combat. Allowed from Setup or Active.
func (e *Encounter) End() ([]Event, error) {
	if e.Status == Ended {
		return nil, ErrEnded
	}
	m := e.meta()
	e.Status = Ended
	return []Event{CombatEnded{EventMeta: m, Rounds: e.Round}}, nil
}

// Next advances to the next row, wrapping into a new round, and counts down
// the durations anchored to the row whose turn is starting (D12).
func (e *Encounter) Next() ([]Event, error) {
	if e.Status != Active {
		return nil, ErrNotActive
	}
	order := e.Order()
	idx := indexOf(order, e.Current)
	rec := TurnRecord{PrevCurrent: e.Current, PrevRound: e.Round}
	evs := e.advance(order, idx, &rec)
	e.History = append(e.History, rec)
	if len(e.History) > historyCap {
		e.History = append([]TurnRecord(nil), e.History[len(e.History)-historyCap:]...)
	}
	return evs, nil
}

// advance moves Current to the row after order[idx] and starts its turn.
// If that row is an unsourced effect that expires on its own turn, the turn
// moves on.
func (e *Encounter) advance(order []Row, idx int, rec *TurnRecord) []Event {
	var evs []Event
	for {
		if len(order) == 0 {
			e.Current = 0
			return evs
		}
		next := idx + 1
		if next >= len(order) {
			next = 0
			e.Round++
			e.Current = order[0].ID()
			evs = append(evs, RoundStarted{EventMeta: e.meta()})
		}
		e.Current = order[next].ID()
		evs = append(evs, e.tick(e.Current, rec)...)
		if r, ok := e.row(e.Current); ok {
			m := e.meta()
			return append(evs, TurnStarted{EventMeta: m, ID: r.ID(), Name: r.Name()})
		}
		// The row expired at the start of its own turn; continue from its slot.
		order = e.Order()
		idx = next - 1
	}
}

// tick counts down everything anchored to row id, recording into rec: a
// combatant's own conditions and the effects it is the source of, or, for an
// effect row, the effect itself if it has no source (D12).
func (e *Encounter) tick(id RowID, rec *TurnRecord) []Event {
	var evs []Event
	if c, ok := e.Combatant(id); ok {
		kept := c.Conditions[:0:0]
		for i, cond := range c.Conditions {
			if cond.Remaining == nil {
				kept = append(kept, cond)
				continue
			}
			if *cond.Remaining <= 1 {
				saved := *cond
				saved.Remaining = copyIntPtr(cond.Remaining)
				rec.ExpiredConditions = append(rec.ExpiredConditions, ExpiredCondition{OwnerID: c.ID, Index: i, Condition: saved})
				evs = append(evs, ConditionExpired{EventMeta: e.meta(), TargetID: c.ID, Target: c.Name, Condition: cond.Name})
				continue
			}
			*cond.Remaining--
			rec.Ticked = append(rec.Ticked, cond.ID)
			kept = append(kept, cond)
		}
		c.Conditions = kept
		for _, fx := range e.EffectsFrom(id) {
			evs = append(evs, e.tickEffect(fx, rec)...)
		}
		return evs
	}
	if fx, ok := e.Effect(id); ok && fx.SourceID == 0 {
		evs = e.tickEffect(fx, rec)
	}
	return evs
}

// tickEffect counts an effect down by one round, removing it at zero.
func (e *Encounter) tickEffect(fx *Effect, rec *TurnRecord) []Event {
	if fx.Remaining == nil {
		return nil
	}
	if *fx.Remaining <= 1 {
		saved := *fx
		saved.Remaining = copyIntPtr(fx.Remaining)
		saved.Targets = append([]RowID(nil), fx.Targets...)
		rec.ExpiredEffects = append(rec.ExpiredEffects, saved)
		ev := EffectExpired{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name}
		e.dropEffect(fx.ID)
		return []Event{ev}
	}
	*fx.Remaining--
	rec.Ticked = append(rec.Ticked, fx.ID)
	return nil
}

// Previous rewinds the latest Next: pointer, round, and its duration changes.
// HP and other edits are not undone (R2).
func (e *Encounter) Previous() ([]Event, error) {
	if e.Status != Active {
		return nil, ErrNotActive
	}
	if len(e.History) == 0 {
		return nil, ErrNothingToUndo
	}
	rec := e.History[len(e.History)-1]
	if _, ok := e.row(rec.PrevCurrent); !ok {
		e.History = nil
		return nil, fmt.Errorf("%w: that row has been removed", ErrNothingToUndo)
	}
	e.History = e.History[:len(e.History)-1]

	for _, id := range rec.Ticked {
		if cond := e.findCondition(id); cond != nil && cond.Remaining != nil {
			*cond.Remaining++
		} else if fx, ok := e.Effect(id); ok && fx.Remaining != nil {
			*fx.Remaining++
		}
	}
	for _, x := range rec.ExpiredEffects {
		fx := x
		fx.Remaining = copyIntPtr(x.Remaining)
		fx.Targets = nil
		for _, t := range x.Targets {
			if _, ok := e.Combatant(t); ok {
				fx.Targets = append(fx.Targets, t)
			}
		}
		if fx.SourceID != 0 {
			if _, ok := e.Combatant(fx.SourceID); !ok {
				fx.SourceID = 0
			}
		}
		e.Effects = append(e.Effects, &fx)
	}
	// Indices are the original positions, so reinsert in ascending order.
	for _, x := range rec.ExpiredConditions {
		c, ok := e.Combatant(x.OwnerID)
		if !ok {
			continue
		}
		cond := x.Condition
		cond.Remaining = copyIntPtr(x.Condition.Remaining)
		at := min(x.Index, len(c.Conditions))
		c.Conditions = append(c.Conditions[:at], append([]*Condition{&cond}, c.Conditions[at:]...)...)
	}

	e.Current = rec.PrevCurrent
	e.Round = rec.PrevRound
	m := e.meta()
	return []Event{TurnRewound{EventMeta: m, ID: e.Current, Name: m.TurnHolder}}, nil
}

func (e *Encounter) findCondition(id RowID) *Condition {
	for _, c := range e.Combatants {
		for _, cond := range c.Conditions {
			if cond.ID == id {
				return cond
			}
		}
	}
	return nil
}

func indexOf(order []Row, id RowID) int {
	for i, r := range order {
		if r.ID() == id {
			return i
		}
	}
	return -1
}

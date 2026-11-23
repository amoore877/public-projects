package combat

import "fmt"

// TurnRecord is what Previous needs to undo one Next: the old pointer and
// round, plus every duration change made when the new turn started.
type TurnRecord struct {
	PrevCurrent       RowID              `json:"prevCurrent"`
	PrevRound         int                `json:"prevRound"`
	PrevDelayed       []RowID            `json:"prevDelayed,omitempty"`
	Ticked            []RowID            `json:"ticked,omitempty"` // conditions/effects decremented but still present
	ExpiredConditions []ExpiredCondition `json:"expiredConditions,omitempty"`
	ExpiredEffects    []Effect           `json:"expiredEffects,omitempty"`
	ResetResources    []ResetResource    `json:"resetResources,omitempty"`
}

// ResetResource records a turn-start resource reset so Previous can restore it (D67).
type ResetResource struct {
	OwnerID RowID  `json:"ownerId"`
	Name    string `json:"name"`
	Prev    int    `json:"prev"`
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

// End finishes combat and starts the next fight empty. Allowed from Setup or Active.
func (e *Encounter) End() ([]Event, error) {
	return e.EndWith(false)
}

// EndWith finishes combat; when keepPCs is true, the session carries the PCs
// into the next fight's Setup (CP1–CP4). Allowed from Setup or Active.
func (e *Encounter) EndWith(keepPCs bool) ([]Event, error) {
	if e.Status == Ended {
		return nil, ErrEnded
	}
	m := e.meta()
	e.Status = Ended
	return []Event{CombatEnded{EventMeta: m, Rounds: e.Round, KeepPCs: keepPCs}}, nil
}

// Next advances to the next row, wrapping into a new round, and counts down
// the durations anchored to the row whose turn is starting (D12).
func (e *Encounter) Next() ([]Event, error) {
	if e.Status != Active {
		return nil, ErrNotActive
	}
	order := e.Order()
	idx := indexOf(order, e.Current)
	rec := e.newRecord()
	evs := e.advance(order, idx, &rec)
	e.pushHistory(rec)
	return evs, nil
}

func (e *Encounter) newRecord() TurnRecord {
	return TurnRecord{PrevCurrent: e.Current, PrevRound: e.Round, PrevDelayed: append([]RowID(nil), e.Delayed...)}
}

func (e *Encounter) pushHistory(rec TurnRecord) {
	e.History = append(e.History, rec)
	if len(e.History) > historyCap {
		e.History = append([]TurnRecord(nil), e.History[len(e.History)-historyCap:]...)
	}
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
			e.Delayed = nil
			e.Current = order[0].ID()
			evs = append(evs, RoundStarted{EventMeta: e.meta()})
		}
		e.Current = order[next].ID()
		if containsID(e.Delayed, e.Current) {
			// It already started a turn this round and stepped aside (RS10).
			e.Delayed = removeID(append([]RowID(nil), e.Delayed...), e.Current)
		} else {
			evs = append(evs, e.tick(e.Current, rec)...)
		}
		if r, ok := e.row(e.Current); ok {
			m := e.meta()
			return append(evs, TurnStarted{EventMeta: m, ID: r.ID(), Name: r.Name()})
		}
		// The row expired at the start of its own turn; continue from its slot.
		order = e.Order()
		idx = next - 1
	}
}

// tick counts down everything anchored to row id, recording into rec: for a
// combatant, its own conditions that have no source, the conditions it caused
// on others, and the effects it is the source of; for an effect row, the
// effect itself if it has no source (D12, RS7). Under a FixedAnchor ruleset
// every effect counts down on its own row instead (D32).
func (e *Encounter) tick(id RowID, rec *TurnRecord) []Event {
	var evs []Event
	fixed := e.Rules().FixedAnchor
	if cur, ok := e.Combatant(id); ok {
		for _, res := range cur.Resources {
			if res.ResetOnTurn && res.Max != nil && res.Current != *res.Max {
				rec.ResetResources = append(rec.ResetResources, ResetResource{OwnerID: cur.ID, Name: res.Name, Prev: res.Current})
				res.Current = *res.Max
			}
		}
		for _, c := range e.Combatants {
			evs = append(evs, e.tickConditions(c, id, rec)...)
		}
		if !fixed {
			for _, fx := range e.EffectsFrom(id) {
				evs = append(evs, e.tickEffect(fx, rec)...)
			}
		}
		return evs
	}
	if fx, ok := e.Effect(id); ok && (fx.SourceID == 0 || fixed) {
		evs = e.tickEffect(fx, rec)
	}
	return evs
}

// tickConditions counts down c's timed conditions anchored to row anchor: its
// unsourced ones when c is the anchor, otherwise those whose source it is.
func (e *Encounter) tickConditions(c *Combatant, anchor RowID, rec *TurnRecord) []Event {
	var evs []Event
	kept := c.Conditions[:0:0]
	for i, cond := range c.Conditions {
		owner := cond.SourceID
		if owner == 0 {
			owner = c.ID
		}
		if cond.Remaining == nil || owner != anchor {
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
		if cond.SourceID != 0 {
			if _, ok := e.Combatant(cond.SourceID); !ok {
				cond.SourceID = 0
			}
		}
		at := min(x.Index, len(c.Conditions))
		c.Conditions = append(c.Conditions[:at], append([]*Condition{&cond}, c.Conditions[at:]...)...)
	}
	for _, x := range rec.ResetResources {
		if c, ok := e.Combatant(x.OwnerID); ok {
			if res := findResource(c, x.Name); res != nil {
				res.Current = x.Prev
			}
		}
	}

	e.Current = rec.PrevCurrent
	e.Round = rec.PrevRound
	e.Delayed = rec.PrevDelayed
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

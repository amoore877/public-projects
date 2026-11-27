package combat

import (
	"fmt"
	"strings"
)

// AddTimer adds a standalone round countdown that fires after rounds new rounds
// (RC1, RC2, D69).
func (e *Encounter) AddTimer(name string, rounds int) (RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return 0, nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, nil, fmt.Errorf("%w: timer name is required", ErrInvalid)
	}
	if rounds < 1 {
		return 0, nil, fmt.Errorf("%w: countdown must be at least 1 round", ErrInvalid)
	}
	t := &Timer{ID: e.newID(), Name: name, Remaining: rounds}
	e.Timers = append(e.Timers, t)
	return t.ID, []Event{TimerAdded{EventMeta: e.meta(), ID: t.ID, Name: t.Name, Remaining: t.Remaining}}, nil
}

// AddTimerAtRound adds a countdown set to fire at the start of targetRound.
// During Setup (Round 0), Round 1 is the opening round and doesn't tick (D15),
// so targetRound must be 2 or higher (Remaining = targetRound - 1). During
// Active combat, targetRound must be greater than e.Round.
func (e *Encounter) AddTimerAtRound(name string, targetRound int) (RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return 0, nil, err
	}
	base := e.Round
	if base == 0 {
		base = 1
	}
	rem := targetRound - base
	if rem < 1 {
		return 0, nil, fmt.Errorf("%w: target round must be after round %d", ErrInvalid, base)
	}
	return e.AddTimer(name, rem)
}

// RemoveTimer cancels an active countdown early.
func (e *Encounter) RemoveTimer(id RowID) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	for i, t := range e.Timers {
		if t.ID == id {
			e.Timers = append(e.Timers[:i], e.Timers[i+1:]...)
			return []Event{TimerRemoved{EventMeta: e.meta(), ID: t.ID, Name: t.Name}}, nil
		}
	}
	return nil, fmt.Errorf("%w: no such timer %d", ErrNotFound, id)
}

// Timer looks up an active countdown by ID.
func (e *Encounter) Timer(id RowID) (*Timer, bool) {
	for _, t := range e.Timers {
		if t.ID == id {
			return t, true
		}
	}
	return nil, false
}

// tickTimers decrements every standalone countdown at the start of a new
// round, removing and emitting TimerExpired for those reaching 0.
func (e *Encounter) tickTimers(rec *TurnRecord) []Event {
	if len(e.Timers) == 0 {
		return nil
	}
	var evs []Event
	kept := e.Timers[:0:0]
	for _, t := range e.Timers {
		if t.Remaining <= 1 {
			rec.ExpiredTimers = append(rec.ExpiredTimers, ExpiredTimer{Index: len(kept), Timer: *t})
			evs = append(evs, TimerExpired{EventMeta: e.meta(), ID: t.ID, Name: t.Name})
			continue
		}
		t.Remaining--
		rec.TickedTimers = append(rec.TickedTimers, t.ID)
		kept = append(kept, t)
	}
	e.Timers = kept
	return evs
}

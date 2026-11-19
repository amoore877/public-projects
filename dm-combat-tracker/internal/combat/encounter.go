// Package combat is the pure domain model for dm-combat-tracker: turn order,
// HP, conditions, effects and durations. It performs no I/O; every mutating
// operation returns the []Event it produced.
package combat

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

// SchemaVersion is the current encounter serialization version.
// Version 2 added Ruleset; version 1 files load as the default ruleset.
const SchemaVersion = 2

// historyCap bounds how many turn advances Previous can rewind.
const historyCap = 50

// RowID identifies a combatant, effect or condition. Zero means "none".
type RowID int

// Status is the encounter lifecycle state.
type Status string

const (
	Setup  Status = "setup"
	Active Status = "active"
	Ended  Status = "ended"
)

// Kind distinguishes player characters from everything else.
type Kind string

const (
	PC  Kind = "pc"
	NPC Kind = "npc"
)

// Clock supplies the current time; injected so tests are deterministic.
type Clock func() time.Time

var (
	ErrNotFound      = errors.New("no such row")
	ErrNotCombatant  = errors.New("row is not a combatant")
	ErrNotEffect     = errors.New("row is not an effect")
	ErrNotActive     = errors.New("combat is not active")
	ErrNotSetup      = errors.New("combat has already started")
	ErrEnded         = errors.New("combat has ended")
	ErrEmpty         = errors.New("no combatants")
	ErrNothingToUndo = errors.New("no earlier turn to go back to")
	ErrInvalid       = errors.New("invalid input")
	ErrNoNonlethal   = errors.New("this ruleset doesn't track nonlethal damage")
)

// Encounter is one fight. All fields are exported for serialization by
// internal/store; mutate it only through its methods.
type Encounter struct {
	SchemaVersion int          `json:"schemaVersion"`
	Status        Status       `json:"status"`
	Ruleset       string       `json:"ruleset"`
	Round         int          `json:"round"`
	Current       RowID        `json:"current"`
	Combatants    []*Combatant `json:"combatants"`
	Effects       []*Effect    `json:"effects"`
	History       []TurnRecord `json:"history"`
	// Delayed holds rows that stepped aside during their turn this round
	// (RS10, D36). Their turn comes again at their new count, but their
	// durations don't count down a second time. Cleared each new round.
	Delayed []RowID `json:"delayed,omitempty"`
	LogPath string  `json:"logPath,omitempty"`
	NextID  int     `json:"nextId"`
	NextSeq int     `json:"nextSeq"`

	clock Clock
}

// Combatant is a creature in the fight.
type Combatant struct {
	ID         RowID        `json:"id"`
	Name       string       `json:"name"`
	Kind       Kind         `json:"kind"`
	Initiative Initiative   `json:"initiative"`
	HP         int          `json:"hp"`
	MaxHP      int          `json:"maxHp"`
	TempHP     int          `json:"tempHp"`
	Nonlethal  int          `json:"nonlethal,omitempty"` // running total, in rulesets that track it (RS4)
	Conditions []*Condition `json:"conditions"`          // own conditions only; effect-linked ones are derived
	Note       string       `json:"note,omitempty"`
	AddedSeq   int          `json:"addedSeq"`
}

// Bloodied reports whether HP is at or below half (display only).
func (c *Combatant) Bloodied() bool { return c.HP > 0 && c.HP*2 <= c.MaxHP }

// Condition is a status on a combatant. Remaining is nil for indefinite.
type Condition struct {
	ID        RowID  `json:"id"`
	Name      string `json:"name"`
	Remaining *int   `json:"remaining,omitempty"`
	Note      string `json:"note,omitempty"`
	// SourceID is the creature that caused the condition (RS7, G8). When set,
	// the duration counts down on the source's turn instead of the bearer's.
	SourceID RowID `json:"sourceId,omitempty"`
}

// Effect is an area spell or ability. It is its own row in the turn order and
// appears as a derived, linked condition on each target (D5).
type Effect struct {
	ID         RowID      `json:"id"`
	Name       string     `json:"name"`
	SourceID   RowID      `json:"sourceId,omitempty"`
	Initiative Initiative `json:"initiative"`
	Remaining  *int       `json:"remaining,omitempty"`
	Targets    []RowID    `json:"targets"`
	Note       string     `json:"note,omitempty"`
	AddedSeq   int        `json:"addedSeq"`
}

// Row is one entry of the turn order: exactly one of Combatant or Effect is set.
type Row struct {
	Combatant *Combatant
	Effect    *Effect
}

// ID returns the row's ID.
func (r Row) ID() RowID {
	if r.Combatant != nil {
		return r.Combatant.ID
	}
	return r.Effect.ID
}

// Name returns the row's display name.
func (r Row) Name() string {
	if r.Combatant != nil {
		return r.Combatant.Name
	}
	return r.Effect.Name
}

// Initiative returns the row's initiative.
func (r Row) Initiative() Initiative {
	if r.Combatant != nil {
		return r.Combatant.Initiative
	}
	return r.Effect.Initiative
}

func (r Row) seq() int {
	if r.Combatant != nil {
		return r.Combatant.AddedSeq
	}
	return r.Effect.AddedSeq
}

// New returns an empty encounter in Setup. A nil clock means time.Now.
func New(clock Clock) *Encounter {
	e := &Encounter{SchemaVersion: SchemaVersion, Status: Setup, Ruleset: DefaultRuleset}
	e.SetClock(clock)
	return e
}

// SetClock sets the clock (needed after deserializing). Nil means time.Now.
func (e *Encounter) SetClock(clock Clock) {
	if clock == nil {
		clock = time.Now
	}
	e.clock = clock
}

func (e *Encounter) now() time.Time {
	if e.clock == nil {
		return time.Now()
	}
	return e.clock()
}

func (e *Encounter) newID() RowID {
	e.NextID++
	return RowID(e.NextID)
}

func (e *Encounter) newSeq() int {
	e.NextSeq++
	return e.NextSeq
}

// meta stamps an event with the current round, turn holder and time.
// Meta is the round, turn holder and time as an event made now would carry
// them; the session stamps its own log lines (undo, redo) with it.
func (e *Encounter) Meta() EventMeta { return e.meta() }

func (e *Encounter) meta() EventMeta {
	m := EventMeta{Round: e.Round, At: e.now()}
	if r, ok := e.row(e.Current); ok {
		m.TurnHolder = r.Name()
	}
	return m
}

// Order returns every combatant and effect in turn order: initiative
// descending, combatants before effects, then insertion order. Under a
// FixedAnchor ruleset a sourced effect tied with its source sorts just
// before it instead (D32), so it ends as the source's turn comes up.
func (e *Encounter) Order() []Row {
	rows := make([]Row, 0, len(e.Combatants)+len(e.Effects))
	for _, c := range e.Combatants {
		rows = append(rows, Row{Combatant: c})
	}
	for _, fx := range e.Effects {
		rows = append(rows, Row{Effect: fx})
	}
	keys := make(map[RowID]orderKey, len(rows))
	fixed := e.Rules().FixedAnchor
	for _, r := range rows {
		keys[r.ID()] = e.orderKey(r, fixed)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return keys[rows[i].ID()].before(keys[rows[j].ID()])
	})
	return rows
}

// orderKey places a row among others at the same initiative: rows sort by
// group (combatants, then effects), then by slot within the group, then by
// lane, then by insertion. A pinned effect takes its source's slot in the
// combatant group, in the lane ahead of it.
type orderKey struct {
	init  Initiative
	group int // 0 combatants (and pinned effects), 1 other effects
	slot  int // AddedSeq of the combatant whose place this is
	lane  int // 0 a pinned effect, 1 the combatant itself
	seq   int
}

func (e *Encounter) orderKey(r Row, fixed bool) orderKey {
	k := orderKey{init: r.Initiative(), seq: r.seq()}
	if r.Combatant != nil {
		k.slot, k.lane = r.Combatant.AddedSeq, 1
		return k
	}
	if fixed && r.Effect.SourceID != 0 {
		if src, ok := e.Combatant(r.Effect.SourceID); ok && src.Initiative == r.Effect.Initiative {
			k.slot = src.AddedSeq
			return k
		}
	}
	k.group, k.slot = 1, r.Effect.AddedSeq
	return k
}

func (a orderKey) before(b orderKey) bool {
	switch {
	case a.init != b.init:
		return a.init > b.init
	case a.group != b.group:
		return a.group < b.group
	case a.slot != b.slot:
		return a.slot < b.slot
	case a.lane != b.lane:
		return a.lane < b.lane
	}
	return a.seq < b.seq
}

// Ties returns the IDs of combatants whose initiative exactly equals another
// combatant's. The UI asks the DM to break these with a decimal (R2).
func (e *Encounter) Ties() map[RowID]bool {
	byInit := map[Initiative][]RowID{}
	for _, c := range e.Combatants {
		byInit[c.Initiative] = append(byInit[c.Initiative], c.ID)
	}
	ties := map[RowID]bool{}
	for _, ids := range byInit {
		if len(ids) > 1 {
			for _, id := range ids {
				ties[id] = true
			}
		}
	}
	return ties
}

// Combatant looks up a combatant by ID.
func (e *Encounter) Combatant(id RowID) (*Combatant, bool) {
	for _, c := range e.Combatants {
		if c.ID == id {
			return c, true
		}
	}
	return nil, false
}

// Effect looks up an effect by ID.
func (e *Encounter) Effect(id RowID) (*Effect, bool) {
	for _, fx := range e.Effects {
		if fx.ID == id {
			return fx, true
		}
	}
	return nil, false
}

func (e *Encounter) row(id RowID) (Row, bool) {
	if id == 0 {
		return Row{}, false
	}
	if c, ok := e.Combatant(id); ok {
		return Row{Combatant: c}, true
	}
	if fx, ok := e.Effect(id); ok {
		return Row{Effect: fx}, true
	}
	return Row{}, false
}

// Row looks up any row by ID.
func (e *Encounter) Row(id RowID) (Row, bool) { return e.row(id) }

// LinkedEffects returns the effects targeting a combatant, in turn order.
// These are shown as linked conditions on that combatant.
func (e *Encounter) LinkedEffects(id RowID) []*Effect {
	var out []*Effect
	for _, r := range e.Order() {
		if r.Effect != nil && containsID(r.Effect.Targets, id) {
			out = append(out, r.Effect)
		}
	}
	return out
}

// EffectsFrom returns the effects whose source is the given combatant.
func (e *Encounter) EffectsFrom(id RowID) []*Effect {
	var out []*Effect
	for _, fx := range e.Effects {
		if fx.SourceID == id {
			out = append(out, fx)
		}
	}
	return out
}

func (e *Encounter) mustCombatant(id RowID) (*Combatant, error) {
	if c, ok := e.Combatant(id); ok {
		return c, nil
	}
	if _, ok := e.Effect(id); ok {
		return nil, ErrNotCombatant
	}
	return nil, fmt.Errorf("%w: %d", ErrNotFound, id)
}

func (e *Encounter) mustEffect(id RowID) (*Effect, error) {
	if fx, ok := e.Effect(id); ok {
		return fx, nil
	}
	if _, ok := e.Combatant(id); ok {
		return nil, ErrNotEffect
	}
	return nil, fmt.Errorf("%w: %d", ErrNotFound, id)
}

func (e *Encounter) checkEditable() error {
	if e.Status == Ended {
		return ErrEnded
	}
	return nil
}

func containsID(ids []RowID, id RowID) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func intPtr(n int) *int { return &n }

func copyIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	return intPtr(*p)
}

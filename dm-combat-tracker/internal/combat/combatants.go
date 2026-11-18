package combat

import (
	"fmt"
	"strconv"
	"strings"
)

// CombatantSpec describes a combatant to add.
type CombatantSpec struct {
	Name       string
	Kind       Kind // defaults to NPC
	Initiative Initiative
	MaxHP      int
	HP         *int // defaults to MaxHP
	TempHP     int
	Note       string
}

func (s *CombatantSpec) validate(hpFloor int) error {
	s.Name = strings.TrimSpace(s.Name)
	if s.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalid)
	}
	if s.Kind == "" {
		s.Kind = NPC
	}
	if s.Kind != PC && s.Kind != NPC {
		return fmt.Errorf("%w: kind must be pc or npc", ErrInvalid)
	}
	if s.MaxHP < 1 {
		return fmt.Errorf("%w: max HP must be at least 1", ErrInvalid)
	}
	if s.HP != nil && (*s.HP < hpFloor || *s.HP > s.MaxHP) {
		return fmt.Errorf("%w: HP must be between %d and max HP", ErrInvalid, hpFloor)
	}
	if s.TempHP < 0 {
		return fmt.Errorf("%w: temp HP can't be negative", ErrInvalid)
	}
	return nil
}

// Add adds one combatant. It is slotted into the order without changing
// whose turn it is (R3).
func (e *Encounter) Add(spec CombatantSpec) (RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return 0, nil, err
	}
	if err := spec.validate(e.Rules().HPFloor); err != nil {
		return 0, nil, err
	}
	c := e.insert(spec, spec.Name, spec.Initiative)
	return c.ID, []Event{e.addedEvent(c)}, nil
}

// GroupSpec describes a group add such as "Goblin x6 @ 12" (D8).
type GroupSpec struct {
	CombatantSpec // Name is the base name; Initiative is the shared value
	Count         int
	// Initiatives, if set, gives each member its own value (len == Count)
	// instead of the shared initiative with auto-filled decimals.
	Initiatives []Initiative
}

// AddGroup creates Count individually numbered combatants ("Goblin 1" …).
// Numbering continues after any existing "Goblin N". With a shared
// initiative, decimals are filled so the group runs in number order:
// 12 ×6 → 12.6 … 12.1; 12 ×12 → 12.12 … 12.01.
func (e *Encounter) AddGroup(g GroupSpec) ([]RowID, []Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, nil, err
	}
	if err := g.validate(e.Rules().HPFloor); err != nil {
		return nil, nil, err
	}
	if g.Count < 1 || g.Count > 999 {
		return nil, nil, fmt.Errorf("%w: group size must be 1–999", ErrInvalid)
	}
	inits := g.Initiatives
	if inits == nil {
		var err error
		if inits, err = GroupFill(g.Initiative, g.Count); err != nil {
			return nil, nil, err
		}
	} else if len(inits) != g.Count {
		return nil, nil, fmt.Errorf("%w: need %d initiatives, got %d", ErrInvalid, g.Count, len(inits))
	}
	start := e.highestNumber(g.Name) + 1
	ids := make([]RowID, 0, g.Count)
	evs := make([]Event, 0, g.Count)
	for i := range g.Count {
		c := e.insert(g.CombatantSpec, fmt.Sprintf("%s %d", g.Name, start+i), inits[i])
		ids = append(ids, c.ID)
		evs = append(evs, e.addedEvent(c))
	}
	return ids, evs, nil
}

// GroupFill returns n initiatives at base with auto-filled decimals, highest
// first, using the fewest decimal places that fit n. For a negative base the
// decimals extend downward (-1 ×3 → -1.1, -1.2, -1.3) so the values still
// read as "-1.x". The base must be a whole number.
func GroupFill(base Initiative, n int) ([]Initiative, error) {
	if !base.IsWhole() {
		return nil, fmt.Errorf("%w: group initiative must be a whole number to auto-fill decimals", ErrInvalid)
	}
	var step int64
	switch {
	case n <= 9:
		step = 100
	case n <= 99:
		step = 10
	case n <= 999:
		step = 1
	default:
		return nil, fmt.Errorf("%w: group too large", ErrInvalid)
	}
	out := make([]Initiative, n)
	for i := range n {
		if base < 0 {
			out[i] = base - Initiative(int64(i+1)*step)
		} else {
			out[i] = base + Initiative(int64(n-i)*step)
		}
	}
	return out, nil
}

// highestNumber finds the largest N among existing combatants named "base N".
func (e *Encounter) highestNumber(base string) int {
	best := 0
	prefix := strings.ToLower(base) + " "
	for _, c := range e.Combatants {
		name := strings.ToLower(c.Name)
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if n, err := strconv.Atoi(name[len(prefix):]); err == nil && n > best {
			best = n
		}
	}
	return best
}

func (e *Encounter) insert(spec CombatantSpec, name string, init Initiative) *Combatant {
	hp := spec.MaxHP
	if spec.HP != nil {
		hp = *spec.HP
	}
	c := &Combatant{
		ID:         e.newID(),
		Name:       name,
		Kind:       spec.Kind,
		Initiative: init,
		HP:         hp,
		MaxHP:      spec.MaxHP,
		TempHP:     spec.TempHP,
		Note:       spec.Note,
		AddedSeq:   e.newSeq(),
	}
	e.Combatants = append(e.Combatants, c)
	return c
}

func (e *Encounter) addedEvent(c *Combatant) Event {
	return CombatantAdded{EventMeta: e.meta(), ID: c.ID, Name: c.Name, Kind: c.Kind, Initiative: c.Initiative, HP: c.HP, MaxHP: c.MaxHP}
}

// SourcedEffects says what to do with effects whose source is being removed.
type SourcedEffects int

const (
	AskAboutEffects SourcedEffects = iota // fail with *NeedsDecisionError if there are any
	KeepEffects                           // keep them, unsourced, at their current initiative
	EndEffects                            // end them early
)

// NeedsDecisionError is returned by Remove when the combatant is the source of
// effects and the caller hasn't said whether to keep or end them (R5).
// Nothing has been changed.
type NeedsDecisionError struct {
	Combatant RowID
	Effects   []RowID
}

func (n *NeedsDecisionError) Error() string {
	return fmt.Sprintf("combatant is the source of %d effect(s): keep or end them?", len(n.Effects))
}

// Remove takes a combatant out of combat. It is removed from every effect's
// targets; if it held the turn, the turn passes to the next row.
func (e *Encounter) Remove(id RowID, sourced SourcedEffects) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	c, err := e.mustCombatant(id)
	if err != nil {
		return nil, err
	}
	owned := e.EffectsFrom(id)
	if len(owned) > 0 && sourced == AskAboutEffects {
		nd := &NeedsDecisionError{Combatant: id}
		for _, fx := range owned {
			nd.Effects = append(nd.Effects, fx.ID)
		}
		return nil, nd
	}

	order := e.Order()
	oldIdx := indexOf(order, e.Current)
	removingCurrent := false

	evs := []Event{CombatantRemoved{EventMeta: e.meta(), ID: c.ID, Name: c.Name}}
	for _, fx := range owned {
		if sourced == EndEffects {
			if fx.ID == e.Current {
				removingCurrent = true
			}
			evs = append(evs, EffectEnded{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, Reason: c.Name + " left combat"})
			e.dropEffect(fx.ID)
		} else {
			fx.SourceID = 0
		}
	}
	for _, fx := range e.Effects {
		fx.Targets = removeID(fx.Targets, id)
	}
	// Conditions it caused stay, and count down on their bearer's turn again.
	for _, x := range e.Combatants {
		for _, cond := range x.Conditions {
			if cond.SourceID == id {
				cond.SourceID = 0
			}
		}
	}
	if e.Current == id {
		removingCurrent = true
	}
	for i, x := range e.Combatants {
		if x.ID == id {
			e.Combatants = append(e.Combatants[:i], e.Combatants[i+1:]...)
			break
		}
	}
	if removingCurrent && e.Status == Active {
		evs = append(evs, e.passTurn(order, oldIdx)...)
	}
	return evs, nil
}

// passTurn moves the turn on after the current row was removed. It behaves
// like a normal advance (the new row's durations tick once) but can't be
// rewound, so turn history is cleared.
func (e *Encounter) passTurn(oldOrder []Row, oldIdx int) []Event {
	e.History = nil
	newOrder := e.Order()
	for j := oldIdx + 1; j < len(oldOrder); j++ {
		if k := indexOf(newOrder, oldOrder[j].ID()); k >= 0 {
			return e.advance(newOrder, k-1, &TurnRecord{})
		}
	}
	return e.advance(newOrder, len(newOrder)-1, &TurnRecord{})
}

// SourceMove says what happens to a creature's effects when its initiative
// changes under a FixedAnchor ruleset (D33).
type SourceMove int

const (
	AskToMoveEffects SourceMove = iota // fail with *MoveEffectsError if it matters
	MoveEffects                        // a corrected initiative: its effects shift with it
	LeaveEffects                       // Delay or Ready: its effects stay on their counts
)

// MoveEffectsError is returned by SetInitiative when the combatant is the
// source of effects pinned to their counts and the caller hasn't said whether
// they move too (D33). Nothing has been changed.
type MoveEffectsError struct {
	Combatant RowID
	To        Initiative
	Effects   []RowID
}

func (m *MoveEffectsError) Error() string {
	return fmt.Sprintf("combatant is the source of %d effect(s): move them too?", len(m.Effects))
}

// SetInitiative changes a combatant's or effect's initiative. The order
// re-sorts; the turn stays with the same row. Under a FixedAnchor ruleset,
// moving the source of effects asks whether they move by the same amount
// (D33); otherwise move is ignored, since effects follow their source's turn.
func (e *Encounter) SetInitiative(id RowID, v Initiative, move SourceMove) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	r, ok := e.row(id)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	from := r.Initiative()
	var owned []*Effect
	if r.Combatant != nil && v != from && e.Rules().FixedAnchor {
		owned = e.EffectsFrom(id)
	}
	if len(owned) > 0 && move == AskToMoveEffects {
		me := &MoveEffectsError{Combatant: id, To: v}
		for _, fx := range owned {
			me.Effects = append(me.Effects, fx.ID)
		}
		return nil, me
	}
	evs := []Event{InitiativeChanged{EventMeta: e.meta(), ID: id, Name: r.Name(), From: from, To: v}}
	if r.Combatant != nil {
		r.Combatant.Initiative = v
	} else {
		r.Effect.Initiative = v
	}
	if move == MoveEffects {
		for _, fx := range owned {
			old := fx.Initiative
			fx.Initiative += v - from
			evs = append(evs, InitiativeChanged{EventMeta: e.meta(), ID: fx.ID, Name: fx.Name, From: old, To: fx.Initiative})
		}
	}
	return evs, nil
}

// SetNote sets a row's side note.
func (e *Encounter) SetNote(id RowID, note string) ([]Event, error) {
	if err := e.checkEditable(); err != nil {
		return nil, err
	}
	r, ok := e.row(id)
	if !ok {
		return nil, fmt.Errorf("%w: %d", ErrNotFound, id)
	}
	note = strings.TrimSpace(note)
	if r.Combatant != nil {
		r.Combatant.Note = note
	} else {
		r.Effect.Note = note
	}
	return []Event{RowNoteSet{EventMeta: e.meta(), ID: id, Name: r.Name(), Note: note}}, nil
}

// LogNote records a manual one-line log entry. It doesn't change state, and
// is allowed in any status.
func (e *Encounter) LogNote(text string) ([]Event, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("%w: empty note", ErrInvalid)
	}
	return []Event{NoteLogged{EventMeta: e.meta(), Text: text}}, nil
}

func removeID(ids []RowID, id RowID) []RowID {
	out := ids[:0]
	for _, x := range ids {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

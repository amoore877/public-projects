package combat

import "time"

// Event is something that happened to an encounter (D11). Every mutating
// operation returns the events it produced; the app layer hands them to the
// journal and triggers autosave. The domain package itself does no I/O.
type Event interface {
	Meta() EventMeta
}

// EventMeta is common to every event.
type EventMeta struct {
	Round      int       // 0 during setup
	TurnHolder string    // name of the row whose turn it is; "" during setup
	At         time.Time // from the encounter's injected clock
}

// Meta implements Event.
func (m EventMeta) Meta() EventMeta { return m }

// OrderEntry is one row of the turn order at the moment combat started.
type OrderEntry struct {
	Name       string
	Initiative Initiative
	IsEffect   bool
}

type (
	CombatStarted struct {
		EventMeta
		Order []OrderEntry
	}
	CombatEnded struct {
		EventMeta
		Rounds  int
		KeepPCs bool
	}
	RoundStarted struct{ EventMeta }
	// RulesetChanged only happens during Setup (RS1).
	RulesetChanged struct {
		EventMeta
		From, To string
	}
	TurnStarted struct {
		EventMeta
		ID   RowID
		Name string
	}
	TurnRewound struct {
		EventMeta
		ID   RowID
		Name string
	}

	CombatantAdded struct {
		EventMeta
		ID         RowID
		Name       string
		Kind       Kind
		Initiative Initiative
		HP, MaxHP  int
	}
	CombatantRemoved struct {
		EventMeta
		ID   RowID
		Name string
	}
	InitiativeChanged struct {
		EventMeta
		ID       RowID
		Name     string
		From, To Initiative
	}
	RowNoteSet struct {
		EventMeta
		ID   RowID
		Name string
		Note string
	}
	RowRenamed struct {
		EventMeta
		ID       RowID
		From, To string
	}

	Damaged struct {
		EventMeta
		ID             RowID
		Name           string
		Amount         int
		HP, TempHP     int
		AbsorbedByTemp int
	}
	Healed struct {
		EventMeta
		ID     RowID
		Name   string
		Amount int // actually restored, after the max-HP cap
		HP     int
		// NonlethalHealed is how much nonlethal damage the healing also
		// removed (RS4); 0 in rulesets that don't track it.
		NonlethalHealed int
	}
	// NonlethalDamaged adds to the nonlethal total (RS4).
	NonlethalDamaged struct {
		EventMeta
		ID     RowID
		Name   string
		Amount int
		Total  int
	}
	NonlethalSet struct {
		EventMeta
		ID       RowID
		Name     string
		From, To int
	}
	// MaxHPSet changes max HP (RS5). HP is the current HP afterwards, which
	// comes down with a lower max.
	MaxHPSet struct {
		EventMeta
		ID       RowID
		Name     string
		From, To int
		HP       int
	}
	HPSet struct {
		EventMeta
		ID       RowID
		Name     string
		From, To int
	}
	TempHPSet struct {
		EventMeta
		ID       RowID
		Name     string
		From, To int
		Added    bool // To came from adding to From (+N), not replacing it (RS6)
	}
	// HealthStateChanged is a move between the ruleset's health states
	// (RS3). From or To is "" when the creature has no state (it is fine).
	HealthStateChanged struct {
		EventMeta
		ID        RowID
		Name      string
		From, To  string
		HP        int
		Nonlethal int
	}

	ConditionApplied struct {
		EventMeta
		TargetID  RowID
		Target    string
		Condition string
		Remaining *int
		Note      string
		Source    string // name of the creature that caused it; empty if none
	}
	ConditionRemoved struct {
		EventMeta
		TargetID  RowID
		Target    string
		Condition string
	}
	ConditionExpired struct {
		EventMeta
		TargetID  RowID
		Target    string
		Condition string
	}

	EffectStarted struct {
		EventMeta
		ID        RowID
		Name      string
		Source    string // "" for environmental effects
		Remaining *int
		Targets   []string
	}
	EffectTargetsChanged struct {
		EventMeta
		ID             RowID
		Name           string
		Added, Removed []string
	}
	EffectEnded struct { // ended or dispelled early
		EventMeta
		ID     RowID
		Name   string
		Reason string
	}
	EffectExpired struct {
		EventMeta
		ID   RowID
		Name string
	}

	NoteLogged struct {
		EventMeta
		Text string
	}

	ResourceChanged struct {
		EventMeta
		ID          RowID
		Target      string
		Name        string
		Current     int
		Max         *int
		ResetOnTurn bool
	}
	ResourceRemoved struct {
		EventMeta
		ID     RowID
		Target string
		Name   string
	}
)

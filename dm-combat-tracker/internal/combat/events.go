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
		Rounds int
	}
	RoundStarted struct{ EventMeta }
	TurnStarted  struct {
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
	}
	Downed struct {
		EventMeta
		ID   RowID
		Name string
	}
	Revived struct {
		EventMeta
		ID   RowID
		Name string
	}

	ConditionApplied struct {
		EventMeta
		TargetID  RowID
		Target    string
		Condition string
		Remaining *int
		Note      string
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
)

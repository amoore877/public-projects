package journal

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// Summary names a change in one line, for the status line after an undo
// or redo (UR3): "Orc took 7 damage", "Goblin 1 joined at initiative 12.3
// (and 3 more)", "Round 2 started: Mira's turn". It covers the changes the
// log leaves out too, such as HP arithmetic, and uses the log's wording for
// the rest.
func Summary(evs []combat.Event) string {
	if len(evs) == 0 {
		return ""
	}
	if s, ok := turnSummary(evs); ok {
		return s
	}
	first := evs[0]
	text := phrase(first)
	same := 0
	for _, ev := range evs[1:] {
		if reflect.TypeOf(ev) == reflect.TypeOf(first) {
			same++
		}
	}
	if same > 0 {
		text += fmt.Sprintf(" (and %d more)", same)
	}
	return text
}

// turnSummary describes a turn passing (n, or a removal or Delay that
// passed it). Such a change starts with the round, the countdown or the
// turn itself.
func turnSummary(evs []combat.Event) (string, bool) {
	switch evs[0].(type) {
	case combat.RoundStarted, combat.TurnStarted, combat.ConditionExpired, combat.EffectExpired:
	default:
		return "", false
	}
	var turn *combat.TurnStarted
	round := 0
	for _, ev := range evs {
		switch e := ev.(type) {
		case combat.RoundStarted:
			round = e.Round
		case combat.TurnStarted:
			turn = &e
		}
	}
	if turn == nil {
		return "", false // expiries alone don't happen; fall back to them
	}
	if round > 0 {
		return "Round " + strconv.Itoa(round) + " started: " + possessive(turn.Name) + " turn", true
	}
	return "Turn passed to " + turn.Name, true
}

func possessive(name string) string {
	if n := len(name); n > 0 && (name[n-1] == 's' || name[n-1] == 'S') {
		return name + "'"
	}
	return name + "'s"
}

// phrase is one event in words.
func phrase(ev combat.Event) string {
	switch e := ev.(type) {
	case combat.CombatStarted:
		return "Combat started" // the log's lineup is too long for the status line
	case combat.RulesetChanged:
		return fmt.Sprintf("Ruleset %s → %s", e.From, e.To)
	case combat.TurnStarted:
		return "Turn passed to " + e.Name
	case combat.RoundStarted:
		return "Round " + strconv.Itoa(e.Round) + " started"
	case combat.InitiativeChanged:
		return fmt.Sprintf("%s initiative %s → %s", e.Name, e.From, e.To)
	case combat.RowNoteSet:
		if e.Note == "" {
			return "Note on " + e.Name + " cleared"
		}
		return "Note on " + e.Name + ": " + oneLine(e.Note)
	case combat.Damaged:
		return fmt.Sprintf("%s took %d damage", e.Name, e.Amount)
	case combat.Healed:
		if e.Amount == 0 && e.NonlethalHealed > 0 {
			return fmt.Sprintf("%s healed %d nonlethal", e.Name, e.NonlethalHealed)
		}
		return fmt.Sprintf("%s healed %d", e.Name, e.Amount)
	case combat.NonlethalDamaged:
		return fmt.Sprintf("%s took %d nonlethal damage", e.Name, e.Amount)
	case combat.NonlethalSet:
		return fmt.Sprintf("%s nonlethal %d → %d", e.Name, e.From, e.To)
	case combat.HPSet:
		return fmt.Sprintf("%s HP %d → %d", e.Name, e.From, e.To)
	case combat.TempHPSet:
		return fmt.Sprintf("%s temp HP %d → %d", e.Name, e.From, e.To)
	case combat.ResourceChanged:
		if e.Max != nil {
			return fmt.Sprintf("%s %s %d/%d", e.Target, e.Name, e.Current, *e.Max)
		}
		return fmt.Sprintf("%s %s %d", e.Target, e.Name, e.Current)
	case combat.ResourceRemoved:
		return fmt.Sprintf("%s removed from %s", e.Name, e.Target)
	}
	if text, ok := logText(ev); ok {
		return text
	}
	return fmt.Sprintf("%T", ev) // a new event type without wording yet
}

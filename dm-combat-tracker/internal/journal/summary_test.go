package journal

import (
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func TestSummary(t *testing.T) {
	m := combat.EventMeta{Round: 2, TurnHolder: "Mira"}
	two := 2
	init := func(s string) combat.Initiative {
		v, err := combat.ParseInitiative(s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	for _, tc := range []struct {
		evs  []combat.Event
		want string
	}{
		{nil, ""},
		{[]combat.Event{combat.CombatStarted{EventMeta: m}, combat.TurnStarted{EventMeta: m, Name: "Mira"}}, "Combat started"},
		{[]combat.Event{combat.CombatEnded{EventMeta: m, Rounds: 3}}, "Combat ended after 3 rounds"},
		{[]combat.Event{combat.RulesetChanged{EventMeta: m, From: "5e", To: "3.5e"}}, "Ruleset 5e → 3.5e"},
		{[]combat.Event{combat.TurnStarted{EventMeta: m, Name: "Orc"}}, "Turn passed to Orc"},
		{[]combat.Event{combat.ConditionExpired{EventMeta: m, Target: "Orc", Condition: "Dazed"}, combat.TurnStarted{EventMeta: m, Name: "Orc"}}, "Turn passed to Orc"},
		{[]combat.Event{combat.RoundStarted{EventMeta: m}, combat.TurnStarted{EventMeta: m, Name: "Thomas"}}, "Round 2 started: Thomas' turn"},
		{[]combat.Event{combat.RoundStarted{EventMeta: m}}, "Round 2 started"},
		{[]combat.Event{combat.EffectExpired{EventMeta: m, Name: "Fog"}}, "Effect expired: Fog"},
		{[]combat.Event{combat.TurnRewound{EventMeta: m, Name: "Mira"}}, "Turn rewound to Mira"},
		{[]combat.Event{
			combat.CombatantAdded{EventMeta: m, Name: "Goblin 1", Initiative: init("12.2"), HP: 7, MaxHP: 7},
			combat.CombatantAdded{EventMeta: m, Name: "Goblin 2", Initiative: init("12.1"), HP: 7, MaxHP: 7},
		}, "Goblin 1 joined at initiative 12.2 (7/7 HP) (and 1 more)"},
		{[]combat.Event{combat.CombatantRemoved{EventMeta: m, Name: "Orc"}, combat.EffectEnded{EventMeta: m, Name: "Fog"}}, "Orc removed from combat"},
		{[]combat.Event{combat.InitiativeChanged{EventMeta: m, Name: "Mira", From: init("15"), To: init("4")}, combat.TurnStarted{EventMeta: m, Name: "Orc"}}, "Mira initiative 15 → 4"},
		{[]combat.Event{combat.RowNoteSet{EventMeta: m, Name: "Orc", Note: "fleeing"}}, "Note on Orc: fleeing"},
		{[]combat.Event{combat.RowNoteSet{EventMeta: m, Name: "Orc"}}, "Note on Orc cleared"},
		{[]combat.Event{combat.RowRenamed{EventMeta: m, From: "Hooded Figure", To: "Strahd"}}, "Hooded Figure renamed to Strahd"},
		{[]combat.Event{
			combat.Damaged{EventMeta: m, Name: "Orc", Amount: 7},
			combat.HealthStateChanged{EventMeta: m, Name: "Orc", To: "down"},
			combat.Damaged{EventMeta: m, Name: "Wolf", Amount: 7},
		}, "Orc took 7 damage (and 1 more)"},
		{[]combat.Event{combat.Healed{EventMeta: m, Name: "Orc", Amount: 5}}, "Orc healed 5"},
		{[]combat.Event{combat.Healed{EventMeta: m, Name: "Orc", NonlethalHealed: 3}}, "Orc healed 3 nonlethal"},
		{[]combat.Event{combat.NonlethalDamaged{EventMeta: m, Name: "Orc", Amount: 4}}, "Orc took 4 nonlethal damage"},
		{[]combat.Event{combat.NonlethalSet{EventMeta: m, Name: "Orc", From: 4, To: 0}}, "Orc nonlethal 4 → 0"},
		{[]combat.Event{combat.MaxHPSet{EventMeta: m, Name: "Orc", From: 10, To: 6}}, "Orc max HP 10 → 6"},
		{[]combat.Event{combat.HPSet{EventMeta: m, Name: "Orc", From: 10, To: 0}}, "Orc HP 10 → 0"},
		{[]combat.Event{combat.TempHPSet{EventMeta: m, Name: "Orc", From: 0, To: 5}}, "Orc temp HP 0 → 5"},
		{[]combat.Event{combat.ResourceChanged{EventMeta: m, Target: "Orc", Name: "LA", Current: 2, Max: &two}}, "Orc LA 2/2"},
		{[]combat.Event{combat.ResourceChanged{EventMeta: m, Target: "Orc", Name: "Arrows", Current: 14}}, "Orc Arrows 14"},
		{[]combat.Event{combat.ResourceRemoved{EventMeta: m, Target: "Orc", Name: "LA"}}, "LA removed from Orc"},
		{[]combat.Event{combat.HealthStateChanged{EventMeta: m, Name: "Orc", To: "dying", HP: -3}}, "Orc is dying (-3 HP)"},
		{[]combat.Event{combat.ConditionApplied{EventMeta: m, Target: "Orc", Condition: "Dazed", Remaining: &two, Source: "Mira"}}, "Dazed applied to Orc by Mira (2 rounds)"},
		{[]combat.Event{combat.ConditionApplied{EventMeta: m, Target: "Orc", Condition: "Stunned", Remaining: &two, EndOfTurn: true}}, "Stunned applied to Orc (2 rounds, end of turn)"},
		{[]combat.Event{combat.EffectStarted{EventMeta: m, Name: "Shield", Source: "Mira", Remaining: &two, EndOfTurn: true, Targets: []string{"Mira"}}}, "Effect started: Shield (Mira, 2 rounds, end of turn) on Mira"},
		{[]combat.Event{combat.ConditionRemoved{EventMeta: m, Target: "Orc", Condition: "Prone"}}, "Prone removed from Orc"},
		{[]combat.Event{combat.EffectStarted{EventMeta: m, Name: "Fog", Targets: []string{"Orc"}}}, "Effect started: Fog on Orc"},
		{[]combat.Event{combat.EffectTargetsChanged{EventMeta: m, Name: "Fog", Added: []string{"Wolf"}}}, "Fog now also on Wolf"},
		{[]combat.Event{combat.EffectEnded{EventMeta: m, Name: "Fog", Reason: "dispelled"}}, "Effect ended early: Fog (dispelled)"},
		{[]combat.Event{combat.NoteLogged{EventMeta: m, Text: "bridge out"}}, "📝 bridge out"},
		{[]combat.Event{combat.TimerAdded{EventMeta: m, Name: "Reinforcements", Remaining: 3}}, "Timer set: Reinforcements (3 rounds)"},
		{[]combat.Event{combat.TimerRemoved{EventMeta: m, Name: "Reinforcements"}}, "Timer cancelled: Reinforcements"},
		{[]combat.Event{combat.TimerExpired{EventMeta: m, Name: "Reinforcements"}}, "⏱ Reinforcements"},
		{[]combat.Event{combat.EffectTargetsChanged{EventMeta: m, Name: "Fog"}}, "combat.EffectTargetsChanged"}, // not emitted: no change
	} {
		if got := Summary(tc.evs); got != tc.want {
			t.Errorf("Summary(%T…) = %q, want %q", first(tc.evs), got, tc.want)
		}
	}
}

func first(evs []combat.Event) combat.Event {
	if len(evs) == 0 {
		return nil
	}
	return evs[0]
}

// Every event the domain emits in a real fight has words of its own, not
// the %T fallback.
func TestSummaryCoversRealEvents(t *testing.T) {
	path := t.TempDir() + "/log.md"
	f := newFight(t, path, newClock())
	f.do(f.enc.SetRuleset("3.5e"))
	f.add("Mira", combat.PC, "15", 27)
	f.add("Orc", combat.NPC, "12", 10)
	var all []combat.Event
	rec := func(evs []combat.Event, err error) {
		t.Helper()
		f.do(evs, err)
		all = append(all, evs...)
	}
	rec(f.enc.Start())
	rec(f.enc.Damage(f.id("Orc"), 12))
	rec(f.enc.Heal(f.id("Orc"), 4))
	rec(f.enc.NonlethalDamage(f.id("Orc"), 2))
	rec(f.enc.SetTempHP(f.id("Mira"), 3))
	rec(f.enc.SetHP(f.id("Mira"), 20))
	rec(f.enc.SetNote(f.id("Mira"), "x"))
	rec(f.enc.Next())
	rec(f.enc.Previous())
	for _, ev := range all {
		if s := Summary([]combat.Event{ev}); strings.HasPrefix(s, "combat.") {
			t.Errorf("%T has no wording", ev)
		}
	}
}

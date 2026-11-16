package command

import (
	"errors"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func TestAddWithTempHPAndNote(t *testing.T) {
	e := encounter(t)
	cmd := run(t, e, ":add Thorin Jr 17 31/44+5 pc -- cursed blade")
	c, _ := e.Combatant(*cmd.Created)
	if c.HP != 31 || c.MaxHP != 44 || c.TempHP != 5 || c.Note != "cursed blade" || c.Kind != combat.PC {
		t.Errorf("added: %+v", c)
	}
	cmd = run(t, e, ":add Wolf x2 15 11+3 npc -- pack tactics")
	if c, _ := e.Combatant(*cmd.Created); c.Name != "Wolf 1" || c.HP != 11 || c.TempHP != 3 || c.Note != "pack tactics" {
		t.Errorf("group member: %+v", c)
	}
	parseErr(t, e, ":add Orc 12 15+x", "temp HP")
	parseErr(t, e, ":add Orc 12 20/15", "current HP must be 0–15")
	parseErr(t, e, ":add Orc x0 12 15", "group size")
	parseErr(t, e, ":add Orc twelve 15", "not a number")
	parseErr(t, e, ":add Orc x2 1,zz 15", "not a number")
	parseErr(t, e, ":add", "usage")
}

func TestParseHP(t *testing.T) {
	for text, want := range map[string][3]int{
		"44":         {-1, 44, 0},
		"31/44":      {31, 44, 0},
		" 0 / 7 ":    {0, 7, 0},
		"44+5":       {-1, 44, 5},
		"31/44 + 12": {31, 44, 12},
	} {
		cur, maxHP, temp, err := ParseHP(text)
		got := [3]int{-1, maxHP, temp}
		if cur != nil {
			got[0] = *cur
		}
		if err != nil || got != want {
			t.Errorf("ParseHP(%q) = %v, %v; want %v", text, got, err, want)
		}
	}
	for _, text := range []string{"", "0", "x", "-1/7", "8/7", "7+-1", "7+"} {
		if _, _, _, err := ParseHP(text); err == nil {
			t.Errorf("ParseHP(%q) should fail", text)
		}
	}
}

func TestUsageAndArgumentErrors(t *testing.T) {
	e := encounter(t)
	for line, want := range map[string]string{
		"":                         "type a command",
		":":                        "type a command",
		":next now":                ":next takes no arguments",
		":frobnicate":              "unknown command :frobnicate",
		":rm":                      "missing combatant name",
		":rm spirit":               `no combatant matches "spirit"`,
		":dmg g1":                  "usage: :dmg TARGETS N",
		":dmg g1 -4":               "whole number of 0 or more",
		":dmg , 4":                 "missing combatant name",
		":hp g1,g2 3":              ":hp takes one target",
		":cond g1":                 "usage: :cond",
		":cond nobody prone":       `no combatant matches "nobody"`,
		":cond gob prone":          "be more specific",
		":cond g1 prone 0":         "at least 1 round",
		":uncond g1":               "usage: :uncond",
		":uncond nobody prone":     `no combatant matches "nobody"`,
		":fx":                      "usage: :fx",
		":fx Web 0 from mira":      "at least 1 round",
		":fx Web from nobody":      "source: no combatant",
		":fx Web at soon":          "not a number",
		":fx Web":                  "needs a source",
		":fx Web from mira on zed": `no combatant matches "zed"`,
		":fxadd spirit":            "usage: :fxadd FX TARGETS",
		":fxadd nothing g1":        `no effect matches "nothing"`,
		":fxrm spirit nobody":      `no combatant matches "nobody"`,
		":end":                     "missing effect name",
		":init g1":                 "usage: :init",
		":init g1 fast":            "not a number",
		":init nobody 3":           `no row matches "nobody"`,
		":note":                    "usage: :note",
		":note nobody hi":          `no row matches "nobody"`,
		":note g1,g2 hi":           ":note takes one target",
		":log":                     "usage: :log",
	} {
		parseErr(t, e, line, want)
	}
}

func TestAliasesAndKinds(t *testing.T) {
	e := encounter(t)
	for line, kind := range map[string]Kind{
		"n": Turn, ":p": Turn, ":previous": Turn, "remove g1": Remove,
		"damage g2 1": Edit, "condition g2 prone": Edit, "effect Fog at 3": Edit, ":endcombat": EndCombat,
	} {
		cmd, err := Parse(e, line)
		if err != nil || cmd.Kind != kind {
			t.Errorf("Parse(%q) = kind %v, %v; want kind %v", line, cmd.Kind, err, kind)
		}
	}
	// :start on a running fight parses, and the domain refuses it.
	cmd := mustParse(t, e, ":start")
	if _, err := cmd.Op(e); !errors.Is(err, combat.ErrNotSetup) {
		t.Errorf(":start mid-combat: %v", err)
	}
	// :prev with no history likewise.
	if _, err := mustParse(t, e, ":prev").Op(e); !errors.Is(err, combat.ErrNothingToUndo) {
		t.Errorf(":prev at the start: %v", err)
	}
	// :endcombat ends it; afterwards a list command stops at the first refusal.
	dmg := mustParse(t, e, ":dmg g1,g2 1")
	if _, err := mustParse(t, e, ":endcombat").Op(e); err != nil {
		t.Fatal(err)
	}
	if evs, err := dmg.Op(e); !errors.Is(err, combat.ErrEnded) || len(evs) != 0 {
		t.Errorf("damage after the end: %v, %v", evs, err)
	}
}

func TestNoteOnEffectAndClearing(t *testing.T) {
	e := encounter(t)
	run(t, e, ":note spirit 3d8 radiant")
	fx, _ := e.Effect(find(t, e, "Spirit Guardians"))
	if fx.Note != "3d8 radiant" {
		t.Errorf("effect note = %q", fx.Note)
	}
	run(t, e, ":note bandit captain")
	run(t, e, ":note spirit guardians")
	if fx.Note != "" {
		t.Errorf("a bare :note should clear it, got %q", fx.Note)
	}
}

func TestEffectTargetCommandsFocusTheEffect(t *testing.T) {
	e := encounter(t)
	fx := find(t, e, "Spirit Guardians")
	if cmd := run(t, e, ":fxadd spirit guardians g2, g3"); cmd.Focus != fx {
		t.Errorf("focus = %d, want the effect", cmd.Focus)
	}
	run(t, e, ":fxrm spirit g1,g2")
	got, _ := e.Effect(fx)
	if len(got.Targets) != 1 || got.Targets[0] != find(t, e, "Goblin 3") {
		t.Errorf("targets = %v", got.Targets)
	}
	// Repeating a name in a list counts once.
	cmd := run(t, e, ":dmg g3, goblin 3 2")
	if c, _ := e.Combatant(cmd.Focus); c.HP != 5 {
		t.Errorf("Goblin 3 HP = %d, want 5 (damaged once)", c.HP)
	}
}

func mustParse(t *testing.T, e *combat.Encounter, line string) Command {
	t.Helper()
	cmd, err := Parse(e, line)
	if err != nil {
		t.Fatalf("Parse(%q): %v", line, err)
	}
	return cmd
}

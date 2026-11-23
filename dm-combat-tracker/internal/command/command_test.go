package command

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func clock() time.Time { return time.Date(2026, 11, 14, 19, 42, 0, 0, time.UTC) }

func mustInit(s string) combat.Initiative {
	v, err := combat.ParseInitiative(s)
	if err != nil {
		panic(err)
	}
	return v
}

// encounter: Thorin 18, Goblin 1–3 and Goblin 10 at 12.x, Bandit Captain 9,
// Mira 9.5, and Spirit Guardians from Mira on Goblin 1.
func encounter(t *testing.T) *combat.Encounter {
	t.Helper()
	e := combat.New(clock)
	must := func(_ any, _ []combat.Event, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(e.Add(combat.CombatantSpec{Name: "Thorin", Kind: combat.PC, Initiative: mustInit("18"), MaxHP: 44}))
	must(e.AddGroup(combat.GroupSpec{CombatantSpec: combat.CombatantSpec{Name: "Goblin", Initiative: mustInit("12"), MaxHP: 7}, Count: 3}))
	must(e.Add(combat.CombatantSpec{Name: "Goblin 10", Initiative: mustInit("11"), MaxHP: 7}))
	must(e.Add(combat.CombatantSpec{Name: "Bandit Captain", Initiative: mustInit("9"), MaxHP: 65}))
	must(e.Add(combat.CombatantSpec{Name: "Mira", Kind: combat.PC, Initiative: mustInit("9.5"), MaxHP: 27}))
	mira := find(t, e, "Mira")
	must(e.AddEffect(combat.EffectSpec{Name: "Spirit Guardians", SourceID: mira, Rounds: ptr(10), Targets: []combat.RowID{find(t, e, "Goblin 1")}}))
	if _, err := e.Start(); err != nil {
		t.Fatal(err)
	}
	return e
}

func ptr(n int) *int { return &n }

func find(t *testing.T, e *combat.Encounter, name string) combat.RowID {
	t.Helper()
	for _, r := range e.Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	t.Fatalf("no row %q", name)
	return 0
}

func run(t *testing.T, e *combat.Encounter, line string) Command {
	t.Helper()
	cmd, err := Parse(e, line)
	if err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	if cmd.Op != nil {
		if _, err := cmd.Op(e); err != nil {
			t.Fatalf("%s: op: %v", line, err)
		}
	}
	return cmd
}

func parseErr(t *testing.T, e *combat.Encounter, line, want string) {
	t.Helper()
	_, err := Parse(e, line)
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("%s: error %v, want %q", line, err, want)
	}
}

func TestResolve(t *testing.T) {
	e := encounter(t)
	for q, want := range map[string]string{
		"goblin 2": "Goblin 2", "Goblin2": "Goblin 2", "gob2": "Goblin 2", "g2": "Goblin 2",
		"g1": "Goblin 1", "goblin 1": "Goblin 1", "g10": "Goblin 10", "GOBLIN 10": "Goblin 10",
		"band": "Bandit Captain", "bandit captain": "Bandit Captain", "th": "Thorin", "mira": "Mira",
	} {
		id, err := resolve(e, q, combatantRow)
		if err != nil {
			t.Errorf("%q: %v", q, err)
			continue
		}
		if r, _ := e.Row(id); r.Name() != want {
			t.Errorf("%q → %s, want %s", q, r.Name(), want)
		}
	}

	_, err := resolve(e, "goblin", combatantRow)
	amb, ok := err.(*AmbiguousError)
	if !ok || strings.Join(amb.Candidates, ",") != "Goblin 1,Goblin 2,Goblin 3,Goblin 10" {
		t.Errorf("goblin: %v", err)
	}
	if _, err := resolve(e, "zed", combatantRow); err == nil || !strings.Contains(err.Error(), `no combatant matches "zed"`) {
		t.Errorf("zed: %v", err)
	}
	if _, err := resolve(e, "spirit", combatantRow); err == nil {
		t.Error("an effect isn't a combatant")
	}
	if id, err := resolve(e, "spirit", effectRow); err != nil || id != find(t, e, "Spirit Guardians") {
		t.Errorf("spirit as effect: %v", err)
	}
}

func TestHPCommands(t *testing.T) {
	e := encounter(t)
	cmd := run(t, e, ":dmg g1,g3 5")
	g1, _ := e.Combatant(find(t, e, "Goblin 1"))
	g3, _ := e.Combatant(find(t, e, "Goblin 3"))
	if g1.HP != 2 || g3.HP != 2 || cmd.Focus != g1.ID {
		t.Errorf("dmg: %d %d focus %d", g1.HP, g3.HP, cmd.Focus)
	}
	run(t, e, "dmg goblin 1 9") // leading ":" optional; spaced name
	if e.HealthState(g1) == nil {
		t.Error("goblin 1 should be down")
	}
	run(t, e, ":heal g1, g3 10")
	if g1.HP != 7 || g3.HP != 7 {
		t.Errorf("heal: %d %d", g1.HP, g3.HP)
	}
	run(t, e, ":hp bandit captain 30")
	run(t, e, ":temp mira 5")
	bc, _ := e.Combatant(find(t, e, "Bandit Captain"))
	mira, _ := e.Combatant(find(t, e, "Mira"))
	if bc.HP != 30 || mira.TempHP != 5 {
		t.Errorf("hp %d temp %d", bc.HP, mira.TempHP)
	}
	parseErr(t, e, ":dmg g1 -3", "isn't a whole number")
	parseErr(t, e, ":dmg g1", "usage: :dmg")
	parseErr(t, e, ":dmg goblin 4", "matches Goblin 1, Goblin 2, Goblin 3, Goblin 10")
	parseErr(t, e, ":dmg g1,zed 4", `no combatant matches "zed"`)
	parseErr(t, e, ":hp g1,g2 4", "one target")
}

func TestConditionCommands(t *testing.T) {
	e := encounter(t)
	run(t, e, ":cond g3 prone")
	run(t, e, ":cond thorin, mira blessed 10")
	run(t, e, ":cond goblin 2 hold person 3 -- WIS save ends")
	g3, _ := e.Combatant(find(t, e, "Goblin 3"))
	if c, ok := g3.FindCondition("Prone"); !ok || c.Name != "Prone" || c.Remaining != nil {
		t.Errorf("prone: %+v", g3.Conditions)
	}
	for _, n := range []string{"Thorin", "Mira"} {
		c, _ := e.Combatant(find(t, e, n))
		if cond, ok := c.FindCondition("blessed"); !ok || *cond.Remaining != 10 {
			t.Errorf("%s blessed: %+v", n, c.Conditions)
		}
	}
	g2, _ := e.Combatant(find(t, e, "Goblin 2"))
	if cond, ok := g2.FindCondition("hold person"); !ok || *cond.Remaining != 3 || cond.Note != "WIS save ends" {
		t.Errorf("hold person: %+v", g2.Conditions)
	}

	run(t, e, ":uncond goblin 2 hold person")
	if len(g2.Conditions) != 0 {
		t.Errorf("uncond: %+v", g2.Conditions)
	}
	parseErr(t, e, ":uncond g3 stunned", `Goblin 3 has no condition "stunned"`)
	parseErr(t, e, ":cond goblin prone", "matches Goblin 1")
	parseErr(t, e, ":cond g3", "usage")

	// by SOURCE names who caused it (RS7, G8), with or without a duration or note.
	run(t, e, ":cond g3 dazed 1 by mira -- until her next turn")
	run(t, e, ":cond g1, g2 frightened by thorin")
	mira, thorin := find(t, e, "Mira"), find(t, e, "Thorin")
	if c, ok := g3.FindCondition("dazed"); !ok || c.SourceID != mira || *c.Remaining != 1 || c.Note != "until her next turn" {
		t.Errorf("dazed: %+v", c)
	}
	if c, ok := g2.FindCondition("frightened"); !ok || c.SourceID != thorin || c.Remaining != nil {
		t.Errorf("frightened: %+v", c)
	}
	parseErr(t, e, ":cond g3 dazed 1 by zed", `source: no combatant matches "zed"`)
}

func TestEffectCommands(t *testing.T) {
	e := encounter(t)
	cmd := run(t, e, ":fx Wall of Fire 10 from thorin on g2,g3")
	fx, ok := e.Effect(*cmd.Created)
	if !ok || fx.Name != "Wall of Fire" || *fx.Remaining != 10 || fx.SourceID != find(t, e, "Thorin") || len(fx.Targets) != 2 {
		t.Fatalf("wall: %+v", fx)
	}
	cmd = run(t, e, ":fx Lair rumble at 20")
	lair, _ := e.Effect(*cmd.Created)
	if lair.Initiative.String() != "20" || lair.Remaining != nil || len(lair.Targets) != 0 {
		t.Errorf("lair: %+v", lair)
	}
	parseErr(t, e, ":fx Fog Cloud 5", "needs a source")
	parseErr(t, e, ":fx Fog Cloud from zed", "source: no combatant")

	run(t, e, ":fxadd wall g1, mira")
	run(t, e, ":fxrm wall of fire g2")
	var names []string
	for _, id := range fx.Targets {
		c, _ := e.Combatant(id)
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "Goblin 3,Goblin 1,Mira" {
		t.Errorf("targets %v", names)
	}

	run(t, e, ":end spirit -- concentration broken")
	if _, ok := e.Effect(find(t, e, "Wall of Fire")); !ok || len(e.Effects) != 2 {
		t.Errorf("only Spirit Guardians should end: %d effects", len(e.Effects))
	}
	parseErr(t, e, ":end thorin", `no effect matches "thorin"`)
}

func TestAddCommand(t *testing.T) {
	e := encounter(t)
	cmd := run(t, e, ":add Ogre 14.5 40/59")
	ogre, _ := e.Combatant(*cmd.Created)
	if ogre.Name != "Ogre" || ogre.Kind != combat.NPC || ogre.HP != 40 || ogre.MaxHP != 59 || ogre.Initiative.String() != "14.5" {
		t.Errorf("ogre: %+v", ogre)
	}
	cmd = run(t, e, ":add Hill Giant Chief 5 105hp")
	if c, _ := e.Combatant(*cmd.Created); c.Name != "Hill Giant Chief" || c.MaxHP != 105 {
		t.Errorf("chief: %+v", c)
	}
	run(t, e, ":add Wolf x3 15 11")
	run(t, e, ":add Elf Scout ×2 16,14 16 pc")
	w2, _ := e.Combatant(find(t, e, "Wolf 2"))
	s2, _ := e.Combatant(find(t, e, "Elf Scout 2"))
	if w2.Initiative.String() != "15.2" || s2.Initiative.String() != "14" || s2.Kind != combat.PC {
		t.Errorf("groups: wolf 2 %s, scout 2 %s %s", w2.Initiative, s2.Initiative, s2.Kind)
	}
	cmd = run(t, e, ":add Goblin x2 12 7")
	if c, _ := e.Combatant(*cmd.Created); c.Name != "Goblin 11" {
		t.Errorf("numbering should continue after Goblin 10, got %s", c.Name)
	}
	parseErr(t, e, ":add Orc 12", "usage")
	parseErr(t, e, ":add Orc x2 12.5 15", "must be a whole number")
	parseErr(t, e, ":add Orc x2 1,2,3 15", "need 2 initiatives")
	parseErr(t, e, ":add Orc 12 0", "max HP")
}

func TestOtherCommands(t *testing.T) {
	e := encounter(t)
	run(t, e, ":init g2 12.35")
	if g2, _ := e.Combatant(find(t, e, "Goblin 2")); g2.Initiative.String() != "12.35" {
		t.Errorf("init %s", g2.Initiative)
	}
	run(t, e, ":init spirit 9.4") // effects too
	run(t, e, ":note goblin 1 fleeing to the bridge")
	if g1, _ := e.Combatant(find(t, e, "Goblin 1")); g1.Note != "fleeing to the bridge" {
		t.Errorf("note %q", g1.Note)
	}
	run(t, e, ":note goblin 1")
	if g1, _ := e.Combatant(find(t, e, "Goblin 1")); g1.Note != "" {
		t.Errorf("a bare :note should clear it, got %q", g1.Note)
	}

	cmd, err := Parse(e, ":log Thorin shoves the goblin off the bridge")
	if err != nil {
		t.Fatal(err)
	}
	evs, _ := cmd.Op(e)
	if n, ok := evs[0].(combat.NoteLogged); !ok || n.Text != "Thorin shoves the goblin off the bridge" {
		t.Errorf("log: %+v", evs)
	}

	if cmd := run(t, e, ":rm g3"); cmd.Kind != Remove || cmd.Target != find(t, e, "Goblin 3") || cmd.Op != nil {
		t.Errorf("rm: %+v", cmd)
	}
	for line, kind := range map[string]Kind{":next": Turn, ":n": Turn, ":prev": Turn, ":endcombat": EndCombat, ":undo": Undo, ":REDO": Redo, ":initall": InitAll} {
		if cmd, err := Parse(e, line); err != nil || cmd.Kind != kind {
			t.Errorf("%s: %+v %v", line, cmd, err)
		}
	}
	parseErr(t, e, ":initall 5", ":initall takes no arguments")
	if cmd, err := Parse(e, ":endcombat"); err != nil || cmd.Kind != EndCombat || cmd.Clear {
		t.Errorf(":endcombat: %+v %v", cmd, err)
	}
	if cmd, err := Parse(e, ":endcombat CLEAR"); err != nil || cmd.Kind != EndCombat || !cmd.Clear {
		t.Errorf(":endcombat clear: %+v %v", cmd, err)
	}
	parseErr(t, e, ":endcombat foo", "usage: :endcombat [clear]")
	parseErr(t, e, ":next 2", "takes no arguments")
	parseErr(t, e, ":undo 3", ":undo takes no arguments")

	if cmd, err := Parse(e, ":ROLL d20 + 5"); err != nil || cmd.Kind != Roll || cmd.Op != nil || cmd.Dice == nil || cmd.Dice.String() != "1d20+5" {
		t.Errorf("roll: %+v %v", cmd, err)
	}
	if cmd, err := Parse(e, ":roll"); err != nil || cmd.Kind != Roll || cmd.Dice != nil {
		t.Errorf("roll again: %+v %v", cmd, err)
	}
	parseErr(t, e, ":roll 4d", `the number of sides is missing after "4d"`)
	parseErr(t, e, ":fly g1", "unknown command :fly")
	parseErr(t, e, ":", "type a command")

	g2 := find(t, e, "Goblin 2")
	if cmd := run(t, e, ":rename g2 Goblin Boss"); cmd.Focus != g2 {
		t.Errorf("rename focus = %d, want %d", cmd.Focus, g2)
	}
	if c, _ := e.Combatant(g2); c.Name != "Goblin Boss" {
		t.Errorf("renamed name = %q", c.Name)
	}
	parseErr(t, e, ":rename g1", "usage: :rename TARGET NEWNAME")
	parseErr(t, e, ":rename g1,g3 Boss", ":rename takes one target")

	run(t, e, ":res band LA /3 reset")
	bandID := find(t, e, "Bandit Captain")
	bc, _ := e.Combatant(bandID)
	if len(bc.Resources) != 1 || bc.Resources[0].Current != 3 || *bc.Resources[0].Max != 3 || !bc.Resources[0].ResetOnTurn {
		t.Fatalf(":res /3 reset: %+v", bc.Resources)
	}
	run(t, e, ":res band LA -1")
	if bc.Resources[0].Current != 2 {
		t.Errorf(":res -1: cur=%d", bc.Resources[0].Current)
	}
	run(t, e, ":res band Spell Slots 2/4")
	if len(bc.Resources) != 2 || bc.Resources[1].Name != "Spell Slots" || bc.Resources[1].Current != 2 {
		t.Fatalf(":res multi-word name: %+v", bc.Resources)
	}
	run(t, e, ":res band Arrows 14")
	run(t, e, ":res band Arrows +2")
	if bc.Resources[2].Current != 16 {
		t.Errorf("Arrows = %d", bc.Resources[2].Current)
	}
	run(t, e, ":res band LA rm")
	if len(bc.Resources) != 2 {
		t.Errorf("after rm: %+v", bc.Resources)
	}
	parseErr(t, e, ":res band LA", "usage: :res")
	parseErr(t, e, ":res band Arrows 14 reset", "turn reset requires a max")
	parseErr(t, e, ":res band LA rm reset", "rm doesn't take reset")
	parseErr(t, e, ":res band LA 1/0", "valid CUR/MAX")
}

func TestEveryVerbIsDocumented(t *testing.T) {
	all := ""
	for _, s := range Specs {
		all += s.Usage + " "
	}
	for _, v := range Verbs {
		if !strings.Contains(all, ":"+v+" ") {
			t.Errorf(":%s is missing from Specs", v)
		}
	}
}

func TestRulesetCommand(t *testing.T) {
	e := combat.New(clock)
	parseErr(t, e, ":ruleset", "usage: :ruleset 5e|3.5e (now 5e)")
	parseErr(t, e, ":ruleset 4e", "usage: :ruleset 5e|3.5e (now 5e)")
	run(t, e, ":ruleset 3.5")
	if e.Ruleset != "3.5e" {
		t.Fatalf("ruleset %q", e.Ruleset)
	}

	// :cond uses the chosen ruleset's spelling.
	if _, _, err := e.Add(combat.CombatantSpec{Name: "Orc", Initiative: mustInit("12"), MaxHP: 20}); err != nil {
		t.Fatal(err)
	}
	run(t, e, ":cond orc flat-footed 1")
	orc := e.Combatants[0]
	if _, ok := orc.FindCondition("Flat-Footed"); !ok || orc.Conditions[0].Name != "Flat-Footed" {
		t.Errorf("conditions: %+v", orc.Conditions[0])
	}

	// Refused once combat has started.
	if _, err := e.Start(); err != nil {
		t.Fatal(err)
	}
	cmd, err := Parse(e, ":ruleset 5e")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cmd.Op(e); !errors.Is(err, combat.ErrNotSetup) || e.Ruleset != "3.5e" {
		t.Errorf("mid-fight: %v, ruleset %q", err, e.Ruleset)
	}
}

// In 3.5e HP can be set, and a combatant added, below zero down to -10.
func TestNegativeHP(t *testing.T) {
	e := encounter(t)
	parseErr(t, e, ":hp g1 -3", `"-3" isn't a whole number of 0 or more`)
	parseErr(t, e, ":add Orc 12 -4/22", "current HP must be 0–22")

	e = combat.New(clock)
	run(t, e, ":ruleset 3.5e")
	run(t, e, ":add Orc 12 -4/22+3")
	orc := e.Combatants[0]
	if orc.HP != -4 || orc.MaxHP != 22 || orc.TempHP != 3 {
		t.Fatalf("added %+v", orc)
	}
	parseErr(t, e, ":add Orc 12 -11/22", "current HP must be from -10 to 22")
	run(t, e, ":hp orc -9")
	if orc.HP != -9 {
		t.Errorf("hp %d", orc.HP)
	}
	parseErr(t, e, ":hp orc -11", `"-11" isn't a whole number of -10 or more`)
	// Amounts are still never negative.
	parseErr(t, e, ":dmg orc -2", `"-2" isn't a whole number of 0 or more`)
	parseErr(t, e, ":heal orc -2", `"-2" isn't a whole number of 0 or more`)
	run(t, e, ":heal orc 5")
	if orc.HP != -4 {
		t.Errorf("healing 5 at -9 should give -4, got %d", orc.HP)
	}
}

func TestHPExtraCommands(t *testing.T) {
	e := encounter(t)
	mira, _ := e.Combatant(find(t, e, "Mira"))
	run(t, e, ":temp mira 5")
	run(t, e, ":temp mira +3")
	if mira.TempHP != 8 {
		t.Errorf("temp %d", mira.TempHP)
	}
	parseErr(t, e, ":temp mira +-3", `"+-3" isn't a whole number`)
	parseErr(t, e, ":temp mira =3", `"=3" isn't a whole number`)
	run(t, e, ":max mira 20")
	if mira.MaxHP != 20 || mira.HP != 20 {
		t.Errorf("maxhp %d/%d", mira.HP, mira.MaxHP)
	}
	parseErr(t, e, ":maxhp mira 0", `"0" isn't a whole number of 1 or more`)
	parseErr(t, e, ":maxhp g1,g2 9", ":maxhp takes one target")
	parseErr(t, e, ":nl mira 3", "this ruleset doesn't track nonlethal damage (5e)")

	e = combat.New(clock)
	run(t, e, ":ruleset 3.5e")
	run(t, e, ":add Orc x2 12 10")
	run(t, e, ":nl o1,o2 4")
	run(t, e, ":nonlethal o1 =10")
	o1, o2 := e.Combatants[0], e.Combatants[1]
	if o1.Nonlethal != 10 || o2.Nonlethal != 4 || e.HealthState(o1).Name != "staggered" {
		t.Errorf("nonlethal %d %d", o1.Nonlethal, o2.Nonlethal)
	}
	parseErr(t, e, ":nl o1 +2", `"+2" isn't a whole number`)
	parseErr(t, e, ":nl o1", "usage: :nl TARGETS [=]N")
}

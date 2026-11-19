package journal

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

var update = flag.Bool("update", false, "rewrite golden files")

// fakeClock starts at 19:42 and only moves when tick is called.
type fakeClock struct{ now time.Time }

func newClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 11, 14, 19, 42, 0, 0, time.UTC)}
}
func (c *fakeClock) Now() time.Time       { return c.now }
func (c *fakeClock) tick(d time.Duration) { c.now = c.now.Add(d) }
func (c *fakeClock) minutes(n int)        { c.tick(time.Duration(n) * time.Minute) }

// fight wires an encounter to a journal writer the way the app layer does.
type fight struct {
	t   *testing.T
	clk *fakeClock
	enc *combat.Encounter
	w   *Writer
}

func newFight(t *testing.T, path string, clk *fakeClock) *fight {
	t.Helper()
	enc := combat.New(clk.Now)
	w, err := Open(path, Resume{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { w.Close() })
	return &fight{t: t, clk: clk, enc: enc, w: w}
}

func (f *fight) do(evs []combat.Event, err error) {
	f.t.Helper()
	if err != nil {
		f.t.Fatal(err)
	}
	if err := f.w.Write(evs); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fight) id(name string) combat.RowID {
	f.t.Helper()
	for _, r := range f.enc.Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	f.t.Fatalf("no row %q", name)
	return 0
}

func (f *fight) add(name string, kind combat.Kind, init string, hp int) {
	f.t.Helper()
	v, err := combat.ParseInitiative(init)
	if err != nil {
		f.t.Fatal(err)
	}
	_, evs, err := f.enc.Add(combat.CombatantSpec{Name: name, Kind: kind, Initiative: v, MaxHP: hp})
	f.do(evs, err)
}

func (f *fight) group(name string, n int, init string, hp int) {
	f.t.Helper()
	v, _ := combat.ParseInitiative(init)
	_, evs, err := f.enc.AddGroup(combat.GroupSpec{CombatantSpec: combat.CombatantSpec{Name: name, Initiative: v, MaxHP: hp}, Count: n})
	f.do(evs, err)
}

func (f *fight) nextTo(name string) {
	f.t.Helper()
	for range 50 {
		f.do(f.enc.Next())
		if r, ok := f.enc.Row(f.enc.Current); ok && r.Name() == name {
			return
		}
	}
	f.t.Fatalf("never reached %s", name)
}

func rounds(n int) *int { return &n }

func golden(t *testing.T, path, name string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	gp := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(gp, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(gp)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/journal -update to create it)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("log differs from %s\n--- got ---\n%s\n--- want ---\n%s", gp, got, want)
	}
}

// TestFullFight covers every logged event kind across two rounds.
func TestFullFight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	clk := newClock()
	f := newFight(t, path, clk)

	f.add("Thorin", combat.PC, "18", 44)
	f.group("Goblin", 4, "12", 7)
	f.add("Mira", combat.PC, "9", 27)
	_, evs, err := f.enc.AddCondition(f.id("Thorin"), "Blessed", rounds(2), "") // setup: not logged
	f.do(evs, err)
	f.do(f.enc.LogNote("Ambush at the bridge"))
	f.do(f.enc.Start())

	clk.minutes(1)
	f.do(f.enc.Damage(f.id("Goblin 2"), 9))
	f.do(f.enc.Damage(f.id("Thorin"), 3)) // HP arithmetic only: not logged
	f.nextTo("Goblin 1")
	clk.minutes(1)
	f.do(f.enc.LogNote("Goblin 1 tried to flee\ntoward the bridge"))
	f.nextTo("Mira")
	clk.minutes(1)
	fxID, evs, err := f.enc.AddEffect(combat.EffectSpec{
		Name: "Spirit Guardians", SourceID: f.id("Mira"), Rounds: rounds(10),
		Targets: []combat.RowID{f.id("Goblin 1"), f.id("Goblin 3")},
	})
	f.do(evs, err)
	_, evs, err = f.enc.AddCondition(f.id("Goblin 3"), "Prone", nil, "")
	f.do(evs, err)
	_, evs, err = f.enc.AddCondition(f.id("Goblin 4"), "Frightened", rounds(1), "DC 14 WIS save ends")
	f.do(evs, err)
	f.do(f.enc.AddTargets(fxID, []combat.RowID{f.id("Goblin 4")}))
	f.do(f.enc.RemoveTargets(fxID, []combat.RowID{f.id("Goblin 1")}))

	clk.minutes(2)
	f.nextTo("Thorin") // round 2; Blessed ticks 2 → 1
	f.do(f.enc.Previous())
	f.do(f.enc.Next())
	f.do(f.enc.Heal(f.id("Goblin 2"), 3))
	f.do(f.enc.RemoveCondition(f.id("Goblin 3"), f.condID("Goblin 3", "Prone")))
	f.add("Owlbear", combat.NPC, "11", 59)
	f.nextTo("Goblin 4") // Frightened expires
	clk.minutes(3)
	f.do(f.enc.Remove(f.id("Goblin 1"), combat.AskAboutEffects))
	f.do(f.enc.EndEffect(fxID, "concentration broken"))
	f.nextTo("Thorin") // round 3: Blessed expires
	clk.minutes(2)
	f.do(f.enc.End())

	golden(t, path, "full_fight.golden")
}

func (f *fight) condID(who, name string) combat.RowID {
	c, _ := f.enc.Combatant(f.id(who))
	cond, ok := c.FindCondition(name)
	if !ok {
		f.t.Fatalf("%s has no %s", who, name)
	}
	return cond.ID
}

// TestEffectExpiresAndSourceLeaves covers automatic expiry, unsourced
// effects, and ending effects because their source left.
func TestEffectExpiresAndSourceLeaves(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.add("Mira", combat.PC, "15", 27)
	f.add("Ogre", combat.NPC, "8", 59)
	f.do(f.enc.Start())
	_, evs, err := f.enc.AddEffect(combat.EffectSpec{Name: "Bless", SourceID: f.id("Mira"), Rounds: rounds(1), Targets: []combat.RowID{f.id("Mira")}})
	f.do(evs, err)
	lair := combat.InitiativeFromInt(20)
	_, evs, err = f.enc.AddEffect(combat.EffectSpec{Name: "Collapsing Ceiling", Initiative: &lair})
	f.do(evs, err)
	_, evs, err = f.enc.AddEffect(combat.EffectSpec{Name: "Hunger of Hadar", SourceID: f.id("Mira")})
	f.do(evs, err)
	f.nextTo("Mira") // round 2: Bless expires as its caster's turn begins
	f.do(f.enc.Remove(f.id("Mira"), combat.EndEffects))
	f.do(f.enc.End())

	golden(t, path, "effects.golden")
}

// TestResumeAndSecondCombat checks that a writer reopened on a resumed fight
// doesn't repeat headings, and a second fight appends a new section.
func TestResumeAndSecondCombat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	clk := newClock()
	f := newFight(t, path, clk)
	f.add("Thorin", combat.PC, "18", 44)
	f.add("Bandit", combat.NPC, "10", 11)
	f.do(f.enc.Start())
	f.do(f.enc.Next())
	f.do(f.enc.Next()) // round 2
	f.w.Close()

	// "Restart the app": same encounter, new writer positioned from state.
	clk.minutes(5)
	w, err := Open(path, ResumeFor(f.enc))
	if err != nil {
		t.Fatal(err)
	}
	f.w = w
	f.do(f.enc.LogNote("Back after a break"))
	f.do(f.enc.Next())
	f.do(f.enc.Next()) // round 3
	f.do(f.enc.End())
	f.w.Close()

	// A second combat later the same session.
	clk.minutes(30)
	g := newFight(t, path, clk)
	g.do(g.enc.LogNote("Wolves circle the camp")) // setup note writes the header
	g.group("Wolf", 2, "13", 11)
	g.do(g.enc.Start())
	g.do(g.enc.End())

	golden(t, path, "resume_and_second.golden")
}

func TestEndedDuringSetupWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.add("Thorin", combat.PC, "18", 44)
	f.do(f.enc.End())
	if b, _ := os.ReadFile(path); len(b) != 0 {
		t.Fatalf("log = %q, want empty", b)
	}
}

func TestLineup(t *testing.T) {
	v := func(s string) combat.Initiative { i, _ := combat.ParseInitiative(s); return i }
	got := lineup([]combat.OrderEntry{
		{Name: "Thorin", Initiative: v("18")},
		{Name: "Goblin 1", Initiative: v("12.4")},
		{Name: "Goblin 2", Initiative: v("12.3")},
		{Name: "Goblin 3", Initiative: v("12.2")},
		{Name: "Moonbeam", Initiative: v("12.2"), IsEffect: true},
		{Name: "Goblin 4", Initiative: v("12.1")},
		{Name: "Room 101", Initiative: v("11")},
		{Name: "Mira", Initiative: v("9")},
		{Name: "Wolf 3", Initiative: v("5")},
		{Name: "Wolf 1", Initiative: v("4")},
	})
	want := "Thorin (18), Goblin 1–3 (12.4–12.2), ◆ Moonbeam (12.2), Goblin 4 (12.1), Room 101 (11), Mira (9), Wolf 3 (5), Wolf 1 (4)"
	if got != want {
		t.Fatalf("lineup =\n%s\nwant\n%s", got, want)
	}
}

// TestHealthStates35 covers the 3.5e health-state entries (RS3).
func TestHealthStates35(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.do(f.enc.SetRuleset("3.5e"))
	f.add("Tordek", combat.PC, "15", 22)
	f.add("Orc", combat.NPC, "8", 9)
	f.do(f.enc.Start())
	f.do(f.enc.Damage(f.id("Tordek"), 22))
	f.do(f.enc.Damage(f.id("Tordek"), 4))
	f.do(f.enc.Heal(f.id("Tordek"), 3))
	f.do(f.enc.Heal(f.id("Tordek"), 8))
	f.do(f.enc.Damage(f.id("Orc"), 30))
	f.do(f.enc.End())

	golden(t, path, "states_35.golden")
}

// TestHPExtras35 covers the nonlethal and max HP entries (RS4, RS5).
func TestHPExtras35(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.do(f.enc.SetRuleset("3.5e"))
	f.add("Orc", combat.NPC, "8", 10)
	f.do(f.enc.Start())
	f.do(f.enc.NonlethalDamage(f.id("Orc"), 10)) // staggered
	f.do(f.enc.NonlethalDamage(f.id("Orc"), 1))  // unconscious
	f.do(f.enc.Heal(f.id("Orc"), 4))             // nonlethal 7 < 10 HP
	f.do(f.enc.SetMaxHP(f.id("Orc"), 6))
	f.do(f.enc.AddTempHP(f.id("Orc"), 2)) // not logged
	f.do(f.enc.End())

	golden(t, path, "hp_extras_35.golden")
}

// A condition with a source names it in the log (RS7, G8).
func TestConditionSourceLogged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.add("Mira", combat.PC, "15", 27)
	f.add("Goblin", combat.NPC, "12", 7)
	f.do(f.enc.Start())
	_, evs, err := f.enc.AddConditionFrom(f.id("Goblin"), "Dazed", rounds(1), f.id("Mira"), "")
	f.do(evs, err)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Dazed applied to Goblin by Mira (1 round)") {
		t.Errorf("log:\n%s", data)
	}
}

// A ruleset file can put a state above 0 HP, or a state called "down" below
// it; the log then names the state instead of assuming 0 HP (RS9).
func TestHealthStatesFromAFile(t *testing.T) {
	r, err := combat.ParseRuleset([]byte(`{"name":"grim","hpFloor":-5,"states":[
		{"name":"hurt","min":1,"max":3},
		{"name":"down","min":-4,"max":0},
		{"name":"dead","max":-5}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := combat.UseRulesets([]combat.Ruleset{r}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = combat.UseRulesets(nil) })

	path := filepath.Join(t.TempDir(), "log.md")
	f := newFight(t, path, newClock())
	f.do(f.enc.SetRuleset("grim"))
	f.add("Orc", combat.NPC, "8", 10)
	f.do(f.enc.Start())
	f.do(f.enc.Damage(f.id("Orc"), 8)) // 2: hurt
	f.do(f.enc.Damage(f.id("Orc"), 2)) // 0: down
	f.do(f.enc.Damage(f.id("Orc"), 3)) // -3: still down
	f.do(f.enc.Heal(f.id("Orc"), 5))   // 2: hurt
	f.do(f.enc.Heal(f.id("Orc"), 5))   // 7: fine
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Orc is hurt (2 HP)", "Orc dropped to 0 HP", "Orc is hurt (2 HP)", "Orc is back above 0 HP"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log is missing %q:\n%s", want, data)
		}
	}

	// Down below 0, and back out of a state to none at or below 0.
	r2, err := combat.ParseRuleset([]byte(`{"name":"odd","hpFloor":-5,"states":[{"name":"down","min":-5,"max":-1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := combat.UseRulesets([]combat.Ruleset{r2}); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(t.TempDir(), "log.md")
	f = newFight(t, path, newClock())
	f.do(f.enc.SetRuleset("odd"))
	f.add("Orc", combat.NPC, "8", 10)
	f.do(f.enc.Start())
	f.do(f.enc.Damage(f.id("Orc"), 12)) // -2: down
	f.do(f.enc.Heal(f.id("Orc"), 2))    // 0: no state
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Orc is down (-2 HP)", "Orc is no longer down"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("log is missing %q:\n%s", want, data)
		}
	}
}

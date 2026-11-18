package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
)

// typeText sends each rune of s as a key press.
func typeText(m *Model, s string) {
	for _, r := range s {
		m.Update(key(string(r)))
	}
}

// selectRow moves the selection to the named row.
func selectRow(t *testing.T, m *Model, name string) {
	t.Helper()
	m.selected = idOf(t, m.sess, name)
	m.syncSelection()
}

func combatant(t *testing.T, s *session.Session, name string) *combat.Combatant {
	t.Helper()
	c, ok := s.Encounter().Combatant(idOf(t, s, name))
	if !ok {
		t.Fatalf("%s is not a combatant", name)
	}
	return c
}

// saved loads the state file, to check a change was autosaved.
func saved(t *testing.T) *combat.Encounter {
	t.Helper()
	enc, err := store.Load(session.DefaultStatePath, fixedClock)
	if err != nil || enc == nil {
		t.Fatalf("load state: %v, %v", enc, err)
	}
	return enc
}

func TestDamageHealTemp(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Goblin 1")

	press(m, "d")
	if m.overlay == nil || !strings.Contains(screen(m), "Damage Goblin 1:") {
		t.Fatalf("d should open the damage prompt:\n%s", screen(m))
	}
	typeText(m, "abc")
	press(m, "enter")
	if m.overlay == nil || !strings.Contains(screen(m), `"abc" isn't a whole number`) {
		t.Fatalf("bad input should keep the prompt open with an error:\n%s", screen(m))
	}
	press(m, "ctrl+u")
	typeText(m, "5")
	press(m, "enter")
	if m.overlay != nil || combatant(t, s, "Goblin 1").HP != 2 {
		t.Fatalf("damage 5: overlay %v HP %d", m.overlay, combatant(t, s, "Goblin 1").HP)
	}
	if saved(t).Combatants[1].HP != 2 {
		t.Error("damage should be autosaved")
	}

	press(m, "h")
	typeText(m, "99")
	press(m, "enter")
	if hp := combatant(t, s, "Goblin 1").HP; hp != 7 {
		t.Errorf("heal is capped at max HP, got %d", hp)
	}

	press(m, "d")
	typeText(m, "=0")
	press(m, "enter")
	if c := combatant(t, s, "Goblin 1"); s.Encounter().HealthState(c) == nil {
		t.Errorf("=0 should set HP to 0, got %d", c.HP)
	}

	selectRow(t, m, "Mira")
	press(m, "t")
	if !strings.Contains(screen(m), "Temp HP for Mira (has 5):") {
		t.Errorf("temp prompt should show the current value:\n%s", screen(m))
	}
	typeText(m, "8")
	press(m, "enter")
	if c := combatant(t, s, "Mira"); c.TempHP != 8 {
		t.Errorf("temp HP %d, want 8", c.TempHP)
	}

	press(m, "d")
	typeText(m, "30")
	press(m, "esc")
	if m.overlay != nil || combatant(t, s, "Mira").TempHP != 8 {
		t.Error("esc should cancel without changing anything")
	}
}

func TestEditingNeedsACombatant(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Spirit Guardians")
	for _, k := range []string{"d", "h", "t", "c", "C"} {
		press(m, k)
		if m.overlay != nil || m.level != levelWarn || !strings.Contains(m.msg, "Select a combatant") {
			t.Errorf("%s on an effect row: overlay %v msg %q", k, m.overlay, m.msg)
		}
	}
}

func TestAddCombatant(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), start)
	m := New(s, "")

	press(m, "a")
	typeText(m, "Ogre")
	press(m, "enter")
	typeText(m, "14.5")
	press(m, "enter")
	if !strings.Contains(screen(m), "Add Ogre · HP:") {
		t.Fatalf("expected the HP step:\n%s", screen(m))
	}
	typeText(m, "40/59")
	press(m, "enter")
	if !strings.Contains(screen(m), "Add Ogre · kind?") {
		t.Fatalf("expected the kind step:\n%s", screen(m))
	}
	press(m, "enter") // NPC by default
	ogre := combatant(t, s, "Ogre")
	if ogre.Kind != combat.NPC || ogre.HP != 40 || ogre.MaxHP != 59 || ogre.Initiative.String() != "14.5" {
		t.Errorf("ogre: %+v", ogre)
	}
	if m.Selected() != ogre.ID {
		t.Error("the new combatant should be selected")
	}
	if s.Encounter().Current != idOf(t, s, "Thorin") {
		t.Error("adding shouldn't change whose turn it is")
	}

	press(m, "a")
	typeText(m, "Mira")
	press(m, "enter")
	typeText(m, "9")
	press(m, "enter")
	typeText(m, "27")
	press(m, "enter", "p")
	if c := combatant(t, s, "Mira"); c.Kind != combat.PC || c.HP != 27 {
		t.Errorf("mira: %+v", c)
	}
}

func TestAddGroup(t *testing.T) {
	s := newSession(t)
	m := New(s, "")

	press(m, "a")
	typeText(m, "Goblin x3")
	press(m, "enter")
	typeText(m, "12.5")
	press(m, "enter")
	if m.overlay == nil || !strings.Contains(screen(m), "must be a whole number") {
		t.Fatalf("a non-whole shared initiative should be refused in place:\n%s", screen(m))
	}
	press(m, "ctrl+u")
	typeText(m, "12")
	press(m, "enter")
	typeText(m, "7")
	press(m, "enter", "n")
	for i, want := range []string{"12.3", "12.2", "12.1"} {
		c := combatant(t, s, "Goblin "+string(rune('1'+i)))
		if c.Initiative.String() != want || c.MaxHP != 7 {
			t.Errorf("goblin %d: init %s hp %d", i+1, c.Initiative, c.MaxHP)
		}
	}
	if m.Selected() != idOf(t, s, "Goblin 1") {
		t.Error("the first new member should be selected")
	}

	press(m, "a")
	typeText(m, "Wolf ×2")
	press(m, "enter")
	typeText(m, "15,11")
	press(m, "enter")
	typeText(m, "11")
	press(m, "enter", "enter")
	if combatant(t, s, "Wolf 1").Initiative.String() != "15" || combatant(t, s, "Wolf 2").Initiative.String() != "11" {
		t.Error("separate initiatives should be used as given")
	}

	press(m, "a")
	typeText(m, "Goblin x2")
	press(m, "enter")
	typeText(m, "1,2,3")
	press(m, "enter")
	if !strings.Contains(screen(m), "Need 2 initiatives, got 3") {
		t.Errorf("wrong count of initiatives:\n%s", screen(m))
	}
}

func TestConditions(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Goblin 1")

	press(m, "c")
	typeText(m, "p")
	got := screen(m)
	if !strings.Contains(got, "tab: Paralyzed · Petrified · Poisoned · Prone") {
		t.Errorf("presets starting with p should be suggested:\n%s", got)
	}
	typeText(m, "r")
	press(m, "tab")
	if !strings.Contains(screen(m), "Condition on Goblin 1: Prone") {
		t.Errorf("tab should complete to Prone:\n%s", screen(m))
	}
	press(m, "enter")
	typeText(m, "x")
	press(m, "enter")
	if !strings.Contains(screen(m), `"x" isn't a number of rounds`) {
		t.Errorf("bad duration:\n%s", screen(m))
	}
	press(m, "backspace")
	typeText(m, "2")
	press(m, "enter")
	if got := screen(m); !strings.Contains(got, "counts down on whose turn?") || !strings.Contains(got, "caused it") {
		t.Errorf("a timed condition off the turn holder should ask for a source:\n%s", got)
	}
	press(m, "n")
	typeText(m, "DC 12 STR to stand")
	press(m, "enter")
	c := combatant(t, s, "Goblin 1")
	cond, ok := c.FindCondition("Prone")
	if !ok || cond.Remaining == nil || *cond.Remaining != 2 || cond.Note != "DC 12 STR to stand" || cond.SourceID != 0 {
		t.Fatalf("condition: %+v", cond)
	}

	// A custom name typed in lower case of a preset uses the preset spelling;
	// any other name is kept as typed.
	press(m, "c")
	typeText(m, "hexed")
	press(m, "enter", "enter", "enter")
	if _, ok := c.FindCondition("hexed"); !ok || c.Conditions[1].Name != "hexed" {
		t.Errorf("custom condition: %+v", c.Conditions)
	}

	press(m, "C")
	if !strings.Contains(screen(m), "1 Prone, 2 hexed") {
		t.Errorf("remove prompt should list the conditions:\n%s", screen(m))
	}
	typeText(m, "2")
	press(m, "enter")
	if len(c.Conditions) != 1 || c.Conditions[0].Name != "Prone" {
		t.Errorf("after removing #2: %+v", c.Conditions)
	}
	press(m, "C", "enter") // a lone condition is filled in
	if len(c.Conditions) != 0 {
		t.Errorf("after removing the last: %+v", c.Conditions)
	}
	press(m, "C")
	if m.overlay != nil || !strings.Contains(m.msg, "only effect conditions") {
		t.Errorf("no own conditions: overlay %v msg %q", m.overlay, m.msg)
	}
}

func TestRemoveCombatant(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Goblin 2")

	press(m, "x", "n")
	if _, ok := s.Encounter().Row(idOf(t, s, "Goblin 2")); !ok || m.overlay != nil {
		t.Fatal("n should cancel the removal")
	}
	press(m, "x", "y")
	for _, r := range s.Encounter().Order() {
		if r.Name() == "Goblin 2" {
			t.Fatal("Goblin 2 should be gone")
		}
	}
	if m.Selected() != idOf(t, s, "Goblin 3") {
		t.Errorf("selection should land on the row in its place, got %d", m.Selected())
	}

	// Removing an effect's source asks what to do with the effect.
	selectRow(t, m, "Mira")
	press(m, "x", "y")
	if !strings.Contains(screen(m), "Mira is the source of Spirit Guardians: keep or end?") {
		t.Fatalf("expected the keep/end question:\n%s", screen(m))
	}
	if _, ok := s.Encounter().Row(idOf(t, s, "Mira")); !ok {
		t.Fatal("nothing should change before the DM decides")
	}
	press(m, "e")
	if len(s.Encounter().Effects) != 0 || m.overlay != nil {
		t.Errorf("e should end the effect: effects %d overlay %v", len(s.Encounter().Effects), m.overlay)
	}

	selectRow(t, m, "Thorin") // the turn holder
	press(m, "x", "y")
	if s.Encounter().Current != idOf(t, s, "Goblin 1") {
		t.Errorf("removing the turn holder should pass the turn on")
	}
}

func TestRemoveSourceKeepsEffects(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Mira")
	press(m, "x", "y", "k")
	if len(s.Encounter().Effects) != 1 || s.Encounter().Effects[0].SourceID != 0 {
		t.Errorf("k should keep the effect unsourced: %+v", s.Encounter().Effects)
	}
}

func TestInitiativeAndNote(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Bandit Captain")

	press(m, "i")
	if !strings.Contains(screen(m), "Initiative for Bandit Captain: 9") {
		t.Fatalf("initiative prompt should show the current value:\n%s", screen(m))
	}
	typeText(m, ".1")
	press(m, "enter")
	if v := combatant(t, s, "Bandit Captain").Initiative.String(); v != "9.1" {
		t.Errorf("initiative %s, want 9.1", v)
	}
	if len(s.Encounter().Ties()) != 0 {
		t.Error("the tie should be settled")
	}
	if m.Selected() != idOf(t, s, "Bandit Captain") {
		t.Error("selection should follow the row as it re-sorts")
	}

	press(m, "i", "ctrl+u")
	typeText(m, "12.1234")
	press(m, "enter")
	if m.overlay == nil || !strings.Contains(screen(m), "more than 3 decimal places") {
		t.Errorf("bad initiative should be refused in place:\n%s", screen(m))
	}
	press(m, "esc")

	selectRow(t, m, "Goblin 1")
	press(m, "N")
	if !strings.Contains(screen(m), "Note for Goblin 1: fleeing") {
		t.Fatalf("note prompt should show the note:\n%s", screen(m))
	}
	typeText(m, " to the bridge")
	press(m, "enter")
	if n := combatant(t, s, "Goblin 1").Note; n != "fleeing to the bridge" {
		t.Errorf("note %q", n)
	}

	selectRow(t, m, "Spirit Guardians") // effects have notes too
	press(m, "N", "ctrl+u", "enter")
	if s.Encounter().Effects[0].Note != "" {
		t.Error("a blank note should clear it")
	}
}

func TestLogEntry(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	press(m, "l", "enter")
	if m.overlay == nil || !strings.Contains(screen(m), "Type something to log") {
		t.Fatalf("an empty entry should be refused:\n%s", screen(m))
	}
	typeText(m, "Thorin shoves the goblin off the bridge")
	press(m, "enter")
	if !strings.Contains(m.msg, "Logged to dmct-log-2026-11-14.md") {
		t.Errorf("msg %q", m.msg)
	}
	b, err := os.ReadFile(s.Encounter().LogPath)
	if err != nil || !strings.Contains(string(b), "📝 Thorin shoves the goblin off the bridge") {
		t.Errorf("log: %v\n%s", err, b)
	}
}

func TestEndCombat(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	press(m, "E")
	if !strings.Contains(screen(m), "End combat after 1 round?") {
		t.Fatalf("expected confirmation:\n%s", screen(m))
	}
	press(m, "y")
	if s.Encounter().Status != combat.Setup || len(s.Encounter().Combatants) != 0 {
		t.Errorf("a new combat should begin: %+v", s.Encounter())
	}
	if !strings.Contains(m.msg, "archived as dmct-state-20261114-194200.json") {
		t.Errorf("msg %q", m.msg)
	}
	if _, err := os.Stat(filepath.Join(".", "dmct-state-20261114-194200.json")); err != nil {
		t.Error(err)
	}
	if m.Selected() != 0 {
		t.Error("selection should be empty")
	}
}

func TestLineEditing(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44))
	m := New(s, "")
	press(m, "l")
	typeText(m, "helo")
	press(m, "left")
	typeText(m, "l")
	if got := m.overlay.(*inputPrompt).ed.String(); got != "hello" {
		t.Errorf("insert at cursor: %q", got)
	}
	m.Update(pasteMsg("  world\nagain "))
	if got := m.overlay.(*inputPrompt).ed.String(); got != "hellworld againo" {
		t.Errorf("paste: %q", got)
	}
	// Hotkeys are plain text while a prompt is open.
	typeText(m, "qn")
	if m.overlay == nil || s.Encounter().Status != combat.Setup {
		t.Error("q and n should be typed into the prompt")
	}
}

func TestRenderPrompt(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	selectRow(t, m, "Goblin 3")
	press(m, "d")
	typeText(m, "4")
	got := screen(m)
	for i, line := range strings.Split(got, "\n") {
		if w := len([]rune(line)); w != 80 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
	golden(t, "prompt_damage.golden", got)
}

func pasteMsg(s string) tea.PasteMsg { return tea.PasteMsg{Content: s} }

func TestNonlethalMaxHPAndAddedTemp(t *testing.T) {
	s := newSession(t, fight35()...)
	m := New(s, "")
	selectRow(t, m, "Orc 4")
	press(m, "D")
	typeText(m, "6")
	press(m, "enter")
	if c := combatant(t, s, "Orc 4"); c.Nonlethal != 6 {
		t.Fatalf("nonlethal %d", c.Nonlethal)
	}
	press(m, "D")
	if !strings.Contains(screen(m), "Nonlethal damage to Orc 4 (has 6):") {
		t.Errorf("prompt:\n%s", screen(m))
	}
	typeText(m, "=9")
	press(m, "enter")
	if got := screen(m); !strings.Contains(got, "STAGR 9/9 nl9") && !strings.Contains(got, "STAGR 9/9") {
		t.Errorf("staggered:\n%s", got)
	}

	press(m, "M")
	if !strings.Contains(screen(m), "Max HP for Orc 4: 9") {
		t.Errorf("max HP prompt starts with the current max:\n%s", screen(m))
	}
	press(m, "backspace")
	typeText(m, "0")
	press(m, "enter")
	if got := screen(m); m.overlay == nil || !strings.Contains(got, "isn't a whole number of 1 or more") {
		t.Errorf("max HP 0:\n%s", got)
	}
	press(m, "ctrl+u")
	typeText(m, "5")
	press(m, "enter")
	if c := combatant(t, s, "Orc 4"); c.MaxHP != 5 || c.HP != 5 {
		t.Errorf("max %d hp %d", c.MaxHP, c.HP)
	}

	selectRow(t, m, "Tordek")
	press(m, "t")
	typeText(m, "4")
	press(m, "enter")
	press(m, "t")
	typeText(m, "+3")
	press(m, "enter")
	if c := combatant(t, s, "Tordek"); c.TempHP != 7 {
		t.Errorf("temp %d", c.TempHP)
	}
	press(m, "t")
	typeText(m, "=2")
	press(m, "enter")
	if got := screen(m); m.overlay == nil || !strings.Contains(got, `"=2" isn't a whole number`) {
		t.Errorf("=N isn't a temp HP form:\n%s", got)
	}
	press(m, "esc")

	// 5e has no nonlethal: D says so instead of opening a prompt.
	m5 := New(newSession(t, fight()...), "")
	press(m5, "D")
	if got := screen(m5); m5.overlay != nil || !strings.Contains(got, "The 5e ruleset doesn't track nonlethal damage") {
		t.Errorf("5e D:\n%s", got)
	}
}

// A cut-off HP cell is spelled out in the status line for the selection.
func TestHPCellDetail(t *testing.T) {
	s := newSession(t, append(fight35(),
		on("Orc 1", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.SetTempHP(id, 5) }),
		on("Orc 1", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.NonlethalDamage(id, 2) }),
	)...)
	m := New(s, "")
	selectRow(t, m, "Orc 1")
	if got := screen(m); !strings.Contains(got, "Orc 1 · HP DSBLD 0/9 +5 nl2") {
		t.Errorf("detail:\n%s", got)
	}
}

// Answering s to the source question makes the turn holder the source, and
// the table names it (RS7, G8).
func TestConditionSourceFromTurnHolder(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	holder := s.Encounter().Current
	selectRow(t, m, "Goblin 1")
	press(m, "c")
	typeText(m, "Dazed")
	press(m, "enter")
	typeText(m, "1")
	press(m, "enter", "s", "enter")
	c := combatant(t, s, "Goblin 1")
	cond, ok := c.FindCondition("Dazed")
	if !ok || cond.SourceID != holder {
		t.Fatalf("condition: %+v, want source %d", cond, holder)
	}
	src, _ := s.Encounter().Combatant(holder)
	if !strings.Contains(screen(m), "Dazed (1 by "+src.Name) {
		t.Errorf("table should name the source:\n%s", screen(m))
	}
}

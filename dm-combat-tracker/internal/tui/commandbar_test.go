package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func runCmd(m *Model, line string) {
	press(m, ":")
	typeText(m, line)
	press(m, "enter")
}

func TestCommandBar(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")

	runCmd(m, "dmg g1,g3 2")
	if c := combatant(t, s, "Goblin 1"); c.HP != 5 || combatant(t, s, "Goblin 3").HP != 1 {
		t.Errorf("dmg: %d %d", c.HP, combatant(t, s, "Goblin 3").HP)
	}
	if m.Selected() != idOf(t, s, "Goblin 1") || m.overlay != nil {
		t.Error("the first target should be selected and the bar closed")
	}
	if saved(t).Combatants[1].HP != 5 {
		t.Error("commands should autosave")
	}

	// A parse error keeps the bar open with the reason, and the text.
	runCmd(m, "dmg goblin 4")
	got := screen(m)
	if m.overlay == nil || !strings.Contains(got, "matches Goblin 1, Goblin 2, Goblin 3") || !strings.Contains(got, " : dmg goblin 4") {
		t.Fatalf("ambiguous target:\n%s", got)
	}
	press(m, "esc")

	// So does an operation the domain refuses.
	runCmd(m, "prev")
	if m.overlay == nil || !strings.Contains(screen(m), "No earlier turn") {
		t.Errorf("refused op should stay open:\n%s", screen(m))
	}
	press(m, "esc")

	runCmd(m, "fx Web 3 at 20 on band")
	fx := effect(t, m, "Web")
	if m.Selected() != fx.ID {
		t.Error("a new effect should be selected")
	}
	runCmd(m, "cond band restrained -- from the web")
	if _, ok := combatant(t, s, "Bandit Captain").FindCondition("Restrained"); !ok {
		t.Error("condition not added")
	}

	runCmd(m, "next")
	if s.Encounter().Current != idOf(t, s, "Goblin 1") || m.Selected() != s.Encounter().Current {
		t.Errorf(":next should move the turn and the selection")
	}
}

func TestLogCommandConfirms(t *testing.T) {
	m := New(newSession(t, fight()...), "")
	press(m, ":")
	typeText(m, "log Thorin shoves the goblin")
	press(m, "enter")
	if m.overlay != nil || m.msg != "Logged to dmct-log-2026-11-14.md" {
		t.Errorf(":log should confirm like the l key: overlay %T, msg %q", m.overlay, m.msg)
	}
}

func TestCommandBarRemoveAndEnd(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	runCmd(m, "rm mira")
	if !strings.Contains(screen(m), "Mira is the source of Spirit Guardians: keep or end?") {
		t.Fatalf(":rm of a source should ask:\n%s", screen(m))
	}
	press(m, "k")
	if len(s.Encounter().Effects) != 1 {
		t.Error("k keeps the effect")
	}
	runCmd(m, "rm g2")
	for _, r := range s.Encounter().Order() {
		if r.Name() == "Goblin 2" || r.Name() == "Mira" {
			t.Errorf("%s should be gone", r.Name())
		}
	}
	runCmd(m, "endcombat")
	if s.Encounter().Status != combat.Setup || !strings.Contains(m.msg, "archived as") {
		t.Errorf(":endcombat: status %v msg %q", s.Encounter().Status, m.msg)
	}
}

func TestCommandHistoryAndCompletion(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	runCmd(m, "dmg g1 1")
	runCmd(m, "heal g1 1")
	press(m, ":", "up")
	if got := m.overlay.(*inputPrompt).ed.String(); got != "heal g1 1" {
		t.Errorf("up: %q", got)
	}
	press(m, "up", "up")
	if got := m.overlay.(*inputPrompt).ed.String(); got != "dmg g1 1" {
		t.Errorf("up up: %q", got)
	}
	press(m, "down", "down")
	if got := m.overlay.(*inputPrompt).ed.String(); got != "" {
		t.Errorf("down past the newest should clear: %q", got)
	}
	typeText(m, "fx")
	if !strings.Contains(screen(m), "tab: fx · fxadd · fxrm") {
		t.Errorf("verbs should be suggested:\n%s", screen(m))
	}
	press(m, "tab", "tab")
	if got := m.overlay.(*inputPrompt).ed.String(); got != "fxadd" {
		t.Errorf("tab tab: %q", got)
	}
	press(m, "esc")
	press(m, ":", "enter")
	if m.overlay != nil {
		t.Error("an empty command just closes the bar")
	}
}

func TestHelp(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	press(m, "?")
	got := screen(m)
	if !strings.Contains(got, " Help") || !strings.Contains(got, "n / space") || !strings.Contains(got, "scroll · lines 1–22 of") {
		t.Fatalf("help screen:\n%s", got)
	}
	for i, line := range strings.Split(got, "\n") {
		if w := len([]rune(line)); w != 80 {
			t.Errorf("line %d is %d wide: %q", i, w, line)
		}
	}
	golden(t, "help_80.golden", got)

	press(m, "G")
	got = screen(m)
	if !strings.Contains(got, ":next  :prev  :start  :endcombat") {
		t.Errorf("G should scroll to the end:\n%s", got)
	}
	press(m, "n") // any other key closes, without acting
	if m.help || m.sess.Encounter().Current != idOf(t, m.sess, "Thorin") {
		t.Error("a key that closes help shouldn't also run")
	}
	// Every hotkey handled by the model is in the help.
	for _, k := range []string{"a", "x", "d", "h", "t", "c", "C", "e", "T", "i", "N", "l", ":", "S", "E", "?", "q"} {
		found := false
		for _, h := range keyHelp {
			for _, part := range strings.Split(h[0], " / ") {
				found = found || part == k
			}
		}
		if !found {
			t.Errorf("key %q is missing from the help", k)
		}
	}
}

// The ruleset is chosen in Setup, shown in the title, and sets which
// conditions the picker offers (RS1, RS8).
func TestRuleset(t *testing.T) {
	s := newSession(t, add("Orc", combat.NPC, "12", 20))
	m := New(s, "")
	if !strings.Contains(screen(m), "5e \n") {
		t.Fatalf("the title should show the default ruleset:\n%s", screen(m))
	}
	runCmd(m, "ruleset 3.5e")
	if got := strings.SplitN(screen(m), "\n", 2)[0]; m.overlay != nil || !strings.HasSuffix(got, " 3.5e ") {
		t.Fatalf("title %q", got)
	}
	if saved(t).Ruleset != "3.5e" {
		t.Error("the choice should be saved")
	}

	press(m, "c")
	typeText(m, "sh")
	if got := screen(m); !strings.Contains(got, "Shaken") {
		t.Errorf("3.5e presets should be suggested:\n%s", got)
	}
	press(m, "ctrl+u")
	typeText(m, "pois")
	if got := screen(m); strings.Contains(got, "Poisoned") {
		t.Errorf("5e-only presets shouldn't be suggested:\n%s", got)
	}
	press(m, "ctrl+u")
	typeText(m, "SICKENED")
	press(m, "enter", "enter", "enter")
	if c := combatant(t, s, "Orc"); len(c.Conditions) != 1 || c.Conditions[0].Name != "Sickened" {
		t.Errorf("conditions %+v", c.Conditions)
	}

	// Fixed once combat starts; the bar stays open with the reason.
	runCmd(m, "start")
	runCmd(m, "ruleset 5e")
	if got := screen(m); m.overlay == nil || !strings.Contains(got, "can't change mid-fight") {
		t.Errorf("mid-fight change:\n%s", got)
	}
	press(m, "esc")
	if got := strings.SplitN(screen(m), "\n", 2)[0]; !strings.HasSuffix(got, " 3.5e ") {
		t.Errorf("title %q", got)
	}
}

// A title too long for the terminal keeps the turn holder and drops the
// ruleset label.
func TestTitleDropsRulesetWhenNarrow(t *testing.T) {
	s := newSession(t, add("Archmage Vellindra the Unbound", combat.NPC, "20", 99), start)
	m := New(s, "")
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 24})
	got := strings.SplitN(screen(m), "\n", 2)[0]
	if !strings.Contains(got, "Vellindra the Unbound") || strings.Contains(got, "5e") || len([]rune(got)) != 48 {
		t.Errorf("title %q", got)
	}
}

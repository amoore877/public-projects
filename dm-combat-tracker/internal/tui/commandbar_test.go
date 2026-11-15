package tui

import (
	"strings"
	"testing"

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
	t.Chdir(goldenDir)
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

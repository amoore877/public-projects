package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func effect(t *testing.T, m *Model, name string) *combat.Effect {
	t.Helper()
	fx, ok := m.sess.Encounter().Effect(idOf(t, m.sess, name))
	if !ok {
		t.Fatalf("%s is not an effect", name)
	}
	return fx
}

func targetNames(m *Model, fx *combat.Effect) []string {
	var out []string
	for _, id := range fx.Targets {
		if c, ok := m.sess.Encounter().Combatant(id); ok {
			out = append(out, c.Name)
		}
	}
	return out
}

func TestNewEffectFromSource(t *testing.T) {
	s := newSession(t, add("Mira", combat.PC, "15", 27), group("Goblin", 3, 12, 7), start)
	m := sized(New(s, ""), 80, 24)
	selectRow(t, m, "Mira")

	press(m, "e")
	typeText(m, "Spirit Guardians")
	press(m, "enter")
	if !strings.Contains(screen(m), "Spirit Guardians · source?") || !strings.Contains(screen(m), "s Mira, right after their turn (enter)") {
		t.Fatalf("expected the source question:\n%s", screen(m))
	}
	press(m, "enter")
	typeText(m, "10")
	press(m, "enter")

	// The checklist is drawn on the table: Mira is selected, unticked.
	got := screen(m)
	if !strings.Contains(got, "Spirit Guardians (Mira) · pick targets: none") || !strings.Contains(got, "│ · │ 15") {
		t.Fatalf("expected the target checklist:\n%s", got)
	}
	press(m, "j", "space", "space") // Goblin 1, Goblin 2 (space ticks and moves on)
	press(m, "k", "space")          // untick Goblin 2
	press(m, "j", "space")          // Goblin 3
	got = screen(m)
	if !strings.Contains(got, "pick targets: Goblin 1, Goblin 3") || !strings.Contains(got, "│ ✓ │ 12.3") {
		t.Fatalf("ticked rows should show:\n%s", got)
	}
	golden(t, "fx_picker.golden", got)
	press(m, "enter")

	fx := effect(t, m, "Spirit Guardians")
	if fx.SourceID != idOf(t, s, "Mira") || fx.Initiative.String() != "15" || fx.Remaining == nil || *fx.Remaining != 10 {
		t.Errorf("effect: %+v", fx)
	}
	if got := strings.Join(targetNames(m, fx), ","); got != "Goblin 1,Goblin 3" {
		t.Errorf("targets %s", got)
	}
	if m.Selected() != fx.ID || m.overlay != nil {
		t.Error("the new effect row should be selected")
	}
	order := s.Encounter().Order()
	if order[1].ID() != fx.ID {
		t.Errorf("the effect should come right after its source, got %s", order[1].Name())
	}
	if !strings.Contains(screen(m), "Goblin 1     │ NPC  │ 7/7         │ ◆ Spirit") {
		t.Errorf("targets should show the linked condition:\n%s", screen(m))
	}
}

func TestNewEffectWithoutSource(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), add("Ogre", combat.NPC, "8", 59))
	m := New(s, "")
	selectRow(t, m, "Thorin")
	press(m, "e")
	typeText(m, "Lair: falling rocks")
	press(m, "enter", "n")
	if !strings.Contains(screen(m), "Lair: falling rocks · initiative:") {
		t.Fatalf("no source should ask for an initiative:\n%s", screen(m))
	}
	press(m, "enter")
	if m.overlay == nil {
		t.Fatal("an empty initiative should be refused")
	}
	typeText(m, "20")
	press(m, "enter", "enter") // indefinite
	press(m, "enter")          // no targets is fine
	fx := effect(t, m, "Lair: falling rocks")
	if fx.SourceID != 0 || fx.Initiative.String() != "20" || fx.Remaining != nil || len(fx.Targets) != 0 {
		t.Errorf("effect: %+v", fx)
	}
	if s.Encounter().Order()[0].ID() != fx.ID {
		t.Error("initiative 20 should sort first")
	}

	// With an effect row selected there is no source to suggest.
	press(m, "e")
	typeText(m, "Fog Cloud")
	press(m, "enter")
	if !strings.Contains(screen(m), "Fog Cloud · initiative:") {
		t.Errorf("from an effect row the source should be none:\n%s", screen(m))
	}
	press(m, "esc")
	if m.overlay != nil || len(s.Encounter().Effects) != 1 {
		t.Error("esc should cancel without adding")
	}
}

func TestEditTargets(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Goblin 1")
	press(m, "T")
	if !strings.Contains(m.msg, "Select an effect row") {
		t.Errorf("T on a combatant: %q", m.msg)
	}

	selectRow(t, m, "Spirit Guardians")
	press(m, "T")
	if !strings.Contains(screen(m), "Spirit Guardians · targets: Goblin 1, Goblin 3") {
		t.Fatalf("the picker should start with the current targets:\n%s", screen(m))
	}
	selectRow(t, m, "Goblin 1")
	press(m, "space") // out of the area
	selectRow(t, m, "Bandit Captain")
	press(m, "space") // into it
	press(m, "enter")
	fx := effect(t, m, "Spirit Guardians")
	if got := strings.Join(targetNames(m, fx), ","); got != "Goblin 3,Bandit Captain" {
		t.Errorf("targets %s", got)
	}
	if m.Selected() != fx.ID {
		t.Error("selection should go back to the effect row")
	}

	press(m, "x", "esc") // x on an effect row ends it, not removes; esc cancels
	if len(s.Encounter().Effects) != 1 {
		t.Fatal("esc should keep the effect")
	}
	b, _ := os.ReadFile(s.Encounter().LogPath)
	if !strings.Contains(string(b), "Spirit Guardians") {
		t.Errorf("target changes should be logged:\n%s", b)
	}
}

func TestEndEffectEarly(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	selectRow(t, m, "Spirit Guardians")
	press(m, "x")
	if !strings.Contains(screen(m), "End Spirit Guardians early · reason:") {
		t.Fatalf("expected the reason prompt:\n%s", screen(m))
	}
	typeText(m, "concentration broken")
	press(m, "enter")
	if len(s.Encounter().Effects) != 0 {
		t.Fatal("the effect should be gone")
	}
	if strings.Contains(screen(m), "◆") {
		t.Errorf("the row and every linked condition should be gone:\n%s", screen(m))
	}
	b, _ := os.ReadFile(s.Encounter().LogPath)
	if !strings.Contains(string(b), "concentration broken") {
		t.Errorf("the reason should be logged:\n%s", b)
	}
}

func TestEndEffectHoldingTheTurn(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	press(m, "G")
	for s.Encounter().Current != idOf(t, s, "Spirit Guardians") {
		press(m, "n")
	}
	press(m, "x", "enter") // no reason
	if s.Encounter().Current != idOf(t, s, "Thorin") || m.Selected() != s.Encounter().Current {
		t.Errorf("the turn should pass on and the selection follow it: current %d selected %d", s.Encounter().Current, m.Selected())
	}
}

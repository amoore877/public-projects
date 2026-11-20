package tui

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/dice"
)

func seededRand() *rand.Rand { return rand.New(rand.NewPCG(7, 11)) }

// want rolls the expressions in order with the test seed and returns the
// status line each roll should show.
func wantRolls(t *testing.T, exprs ...string) []string {
	t.Helper()
	r := seededRand()
	var out []string
	for _, s := range exprs {
		e, err := dice.Parse(s)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rollText(e.Roll(r)))
	}
	return out
}

func hint(m *Model) string {
	text, _ := m.overlay.hint()
	return text
}

// r opens a roll prompt; enter rolls and shows the total, a bad expression
// keeps the prompt open, and enter alone repeats the last roll (DR1–DR3, D47).
func TestRollKey(t *testing.T) {
	s := newSession(t, fight()...)
	m := sized(New(s, ""), 80, 24)
	m.rng = seededRand()
	want := wantRolls(t, "2d6+2+1d4", "4d6", "4d6")
	before, _ := s.UndoDepth()
	hpBefore := hpOf(t, s, "Goblin 1")

	press(m, "r")
	if !strings.Contains(hint(m), "e.g. 2d6+2+1d4") {
		t.Errorf("hint %q", hint(m))
	}
	press(m, "enter")
	if m.overlay == nil || !strings.Contains(hint(m), "Nothing to roll yet") {
		t.Fatalf("an empty first roll should keep the prompt open: %v %q", m.overlay, m.msg)
	}
	typeText(m, "2D6 + 2 + d4")
	press(m, "enter")
	if m.overlay != nil || m.msg != want[0] || m.level != levelInfo {
		t.Fatalf("after rolling: overlay %v, msg %q, want %q", m.overlay, m.msg, want[0])
	}
	if !strings.HasPrefix(want[0], "2d6+2+1d4 = ") || !strings.Contains(screen(m), want[0]) {
		t.Errorf("status line should show %q:\n%s", want[0], screen(m))
	}

	// A typo is refused in place and fixed there.
	press(m, "r")
	typeText(m, "4d")
	press(m, "enter")
	if m.overlay == nil || !strings.Contains(hint(m), `The number of sides is missing after "4d"`) {
		t.Fatalf("refused roll: overlay %v, hint %q", m.overlay, hint(m))
	}
	typeText(m, "6")
	press(m, "enter")
	if m.msg != want[1] {
		t.Errorf("msg %q, want %q", m.msg, want[1])
	}

	// Enter alone rolls the last expression again, and the prompt says so.
	press(m, "r")
	if !strings.Contains(hint(m), "enter alone rolls 4d6 again") {
		t.Errorf("hint %q", hint(m))
	}
	typeText(m, "  ")
	press(m, "enter")
	if m.msg != want[2] {
		t.Errorf("repeat: msg %q, want %q", m.msg, want[2])
	}

	// esc rolls nothing.
	press(m, "r")
	typeText(m, "1d20")
	press(m, "esc")
	if m.overlay != nil || m.msg != "" || m.lastRoll.String() != "4d6" {
		t.Errorf("esc: overlay %v, msg %q, last %v", m.overlay, m.msg, m.lastRoll)
	}

	// Nothing in the fight changed: no undo step, same HP (D45).
	if after, _ := s.UndoDepth(); after != before || hpOf(t, s, "Goblin 1") != hpBefore {
		t.Errorf("undo depth %d → %d, HP %d → %d", before, after, hpBefore, hpOf(t, s, "Goblin 1"))
	}
}

// :roll rolls too, in Setup as in combat; :roll alone repeats the last roll.
func TestRollCommand(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44))
	m := New(s, "")
	m.rng = seededRand()
	want := wantRolls(t, "d20+5", "d20+5")
	undoBefore, _ := s.UndoDepth()

	runCmd(m, ":roll")
	if m.overlay == nil || !strings.Contains(hint(m), "Nothing to roll yet") {
		t.Fatalf(":roll with nothing rolled should keep the bar open: %v", m.overlay)
	}
	press(m, "esc")
	runCmd(m, ":roll d 20+5")
	if m.overlay == nil || !strings.Contains(hint(m), "Put + or - between") {
		t.Fatalf("bad :roll should keep the bar open: %v", m.overlay)
	}
	press(m, "esc")
	runCmd(m, ":roll d20 + 5")
	if m.overlay != nil || m.msg != want[0] || !strings.HasPrefix(m.msg, "1d20+5 = ") {
		t.Fatalf(":roll: overlay %v, msg %q, want %q", m.overlay, m.msg, want[0])
	}
	runCmd(m, ":roll")
	if m.msg != want[1] {
		t.Errorf(":roll again: %q, want %q", m.msg, want[1])
	}
	// The r prompt and :roll share the last roll.
	press(m, "r")
	if !strings.Contains(hint(m), "rolls 1d20+5 again") {
		t.Errorf("hint %q", hint(m))
	}
	if s.Encounter().Status != combat.Setup {
		t.Error("rolling shouldn't start combat")
	}
	if u, r := s.UndoDepth(); u != undoBefore || r != 0 {
		t.Errorf("undo depth %d/%d after rolls, was %d/0", u, r, undoBefore)
	}
}

func TestRenderRoll(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	m.rng = seededRand()
	press(m, "r")
	typeText(m, "2d6+2+1d4")
	golden(t, "prompt_roll.golden", screen(m))
	press(m, "enter")
	golden(t, "roll_80.golden", screen(m))
}

// The default random source rolls within the dice.
func TestRollDefaultSource(t *testing.T) {
	m := New(newSession(t), "")
	for range 50 {
		runCmd(m, ":roll 1d4")
		if n := m.msg[len("1d4 = ")]; !strings.HasPrefix(m.msg, "1d4 = ") || n < '1' || n > '4' ||
			m.msg != fmt.Sprintf("1d4 = %c · [%c]", n, n) {
			t.Fatalf("msg %q", m.msg)
		}
	}
}

// The status line shows what each die rolled, in set order, and cuts the
// breakdown before the total on a narrow terminal (DR4, D48).
func TestRollBreakdown(t *testing.T) {
	e, _ := dice.Parse("2d6+2-1d4")
	r := e.Roll(seededRand())
	a, b, c := r.Terms[0].Faces[0], r.Terms[0].Faces[1], r.Terms[2].Faces[0]
	full := fmt.Sprintf("2d6+2-1d4 = %d · [%d %d] +2 -[%d]", a+b+2-c, a, b, c)
	if got := rollText(r); got != full {
		t.Fatalf("rollText %q, want %q", got, full)
	}
	head := fmt.Sprintf("2d6+2-1d4 = %d", a+b+2-c)
	total := fmt.Sprintf(" = %d", a+b+2-c)
	for _, c := range []struct {
		width int
		want  string
	}{
		{80, full},
		{len([]rune(full)), full},
		{len([]rune(full)) - 1, string([]rune(full)[:len([]rune(full))-2]) + "…"},
		{len(head) + 4, head + " · …"},
		{len(head) + 3, head},
		{len(head), head},
		{len(head) - 1, "2d6+2-1…" + total},
		{2, "…" + total},
	} {
		if got := rollLine(r, c.width); got != c.want {
			t.Errorf("width %d: %q, want %q", c.width, got, c.want)
		}
	}
}

func TestRenderLongRoll(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	m.rng = seededRand()
	runCmd(m, ":roll 20d6+3")
	golden(t, "roll_20d6_80.golden", screen(m))
	// Narrow: the breakdown goes, the total stays.
	m.Update(tea.WindowSizeMsg{Width: 43, Height: 24})
	got := screen(m)
	golden(t, "roll_20d6_43.golden", got)
	lines := strings.Split(got, "\n")
	if status := lines[len(lines)-2]; !strings.HasPrefix(status, " 20d6+3 = ") || !strings.HasSuffix(strings.TrimSpace(status), "…") {
		t.Errorf("status line %q", status)
	}
	// Another message replaces the roll as usual.
	press(m, "u")
	if strings.Contains(screen(m), "20d6") {
		t.Errorf("roll still shown:\n%s", screen(m))
	}
}

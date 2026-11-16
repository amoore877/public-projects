package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// special sends keys that key() doesn't build: ctrl chords and navigation.
func special(m *Model, names ...string) {
	for _, name := range names {
		var k tea.KeyPressMsg
		switch {
		case strings.HasPrefix(name, "ctrl+"):
			k = tea.KeyPressMsg{Code: []rune(name)[len("ctrl+")], Mod: tea.ModCtrl}
		case name == "shift+tab":
			k = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
		default:
			k = tea.KeyPressMsg{Code: map[string]rune{
				"right": tea.KeyRight, "home": tea.KeyHome, "end": tea.KeyEnd, "delete": tea.KeyDelete,
				"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "f5": tea.KeyF5,
			}[name]}
		}
		m.Update(k)
	}
}

func promptText(t *testing.T, m *Model) string {
	t.Helper()
	p, ok := m.overlay.(*inputPrompt)
	if !ok {
		t.Fatalf("no text prompt is open (overlay %T)", m.overlay)
	}
	return p.ed.String()
}

// The prompt's line editor: the keys the README promises (ctrl+a/e/u/k/w,
// arrows, delete).
func TestLineEditorKeys(t *testing.T) {
	m := New(newSession(t, add("Thorin", combat.PC, "18", 44)), "")
	press(m, "l")
	steps := []struct {
		do   func()
		want string
	}{
		{func() { typeText(m, "the goblin flees") }, "the goblin flees"},
		{func() { special(m, "ctrl+w") }, "the goblin "},
		{func() { special(m, "ctrl+w") }, "the "},
		{func() { typeText(m, "orc runs"); special(m, "ctrl+a"); typeText(m, "> ") }, "> the orc runs"},
		{func() { special(m, "delete") }, "> he orc runs"},
		{func() { special(m, "ctrl+e"); special(m, "delete") }, "> he orc runs"}, // nothing after the cursor
		{func() { press(m, "backspace") }, "> he orc run"},
		{func() { special(m, "home"); press(m, "backspace") }, "> he orc run"}, // nothing before it
		{func() { special(m, "right", "right", "ctrl+f", "ctrl+b", "ctrl+d") }, "> e orc run"},
		{func() { special(m, "end", "ctrl+h") }, "> e orc ru"},
		{func() { press(m, "left", "left"); special(m, "ctrl+k") }, "> e orc "},
		{func() { special(m, "ctrl+b"); press(m, "ctrl+u") }, " "},
		{func() { special(m, "f5", "ctrl+x") }, " "}, // keys with no text are ignored
	}
	for i, s := range steps {
		s.do()
		if got := promptText(t, m); got != s.want {
			t.Fatalf("step %d: text = %q, want %q", i, got, s.want)
		}
	}
	// Tab does nothing in a prompt with no completions; ctrl+c cancels like esc.
	press(m, "tab")
	special(m, "ctrl+c")
	if m.overlay != nil {
		t.Error("ctrl+c should close the prompt")
	}
}

// A line longer than the screen scrolls so the cursor stays visible.
func TestLongInputScrolls(t *testing.T) {
	m := sized(New(newSession(t, add("Thorin", combat.PC, "18", 44)), ""), 50, 12)
	press(m, "l")
	long := "the dragon lands on the tower and the whole bridge starts to give way"
	typeText(m, long)
	lines := strings.Split(screen(m), "\n")
	prompt := lines[len(lines)-2]
	if len([]rune(prompt)) != 50 || !strings.Contains(prompt, "…") || !strings.Contains(prompt, "give way") {
		t.Errorf("cursor end should be visible in 50 columns: %q", prompt)
	}
	special(m, "home")
	prompt = strings.Split(screen(m), "\n")[len(lines)-2]
	if len([]rune(prompt)) != 50 || !strings.Contains(prompt, "Log: the dragon") || !strings.HasSuffix(strings.TrimRight(prompt, " "), "…") {
		t.Errorf("start should be visible with the end cut: %q", prompt)
	}
	press(m, "enter")
	if !strings.Contains(m.msg, "Logged to dmct-log-2026-11-14.md") {
		t.Errorf("msg = %q", m.msg)
	}
}

func TestCompletionCyclesBothWays(t *testing.T) {
	m := New(newSession(t, fight()...), "")
	press(m, "c")
	typeText(m, "p")
	var seen []string
	for range 4 {
		press(m, "tab")
		seen = append(seen, promptText(t, m))
	}
	if got := strings.Join(seen, ","); got != "Paralyzed,Petrified,Poisoned,Prone" {
		t.Errorf("tab order: %s", got)
	}
	special(m, "shift+tab")
	if got := promptText(t, m); got != "Poisoned" {
		t.Errorf("shift+tab should step back, got %q", got)
	}
	// Typing after a completion starts a new cycle from the new text.
	press(m, "ctrl+u")
	typeText(m, "zz")
	press(m, "tab")
	if got := promptText(t, m); got != "zz" {
		t.Errorf("tab with no match should leave the text, got %q", got)
	}
}

func TestHelpScrolling(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	press(m, "?")
	at := func() int { return m.helpOff }
	press(m, "j", "down")
	if at() != 2 {
		t.Fatalf("j/down: offset %d", at())
	}
	press(m, "k", "up", "up")
	if at() != 0 {
		t.Fatalf("k/up stop at the top: offset %d", at())
	}
	press(m, "space")
	if at() != m.helpBody() {
		t.Fatalf("space pages down: offset %d", at())
	}
	special(m, "pgdown", "pgdown", "pgdown")
	limit := len(helpLines()) - m.helpBody()
	if at() != limit {
		t.Fatalf("paging stops at the end: offset %d, want %d", at(), limit)
	}
	press(m, "b")
	special(m, "pgup")
	press(m, "f")
	special(m, "home")
	if at() != 0 {
		t.Fatalf("home: offset %d", at())
	}
	special(m, "end")
	if at() != limit || !m.help {
		t.Fatalf("end: offset %d, help %v", at(), m.help)
	}
	press(m, "esc")
	if m.help {
		t.Error("esc should close help")
	}
	// In a tall window everything fits and the hint says so.
	sized(m, 100, 80)
	press(m, "?")
	if got := screen(m); !strings.Contains(got, "any key closes") || strings.Contains(got, "scroll") {
		t.Errorf("tall help screen:\n%s", got)
	}
}

func TestTableNavigationKeys(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	name := func() string {
		r, _ := m.sess.Encounter().Row(m.Selected())
		return r.Name()
	}
	press(m, "G")
	if name() != "Spirit Guardians" {
		t.Errorf("G: %s", name())
	}
	press(m, "j") // already at the bottom
	press(m, "g")
	if name() != "Thorin" {
		t.Errorf("g: %s", name())
	}
	special(m, "end")
	press(m, "k")
	if name() != "Bandit Captain" {
		t.Errorf("end, k: %s", name())
	}
	special(m, "home")
	if name() != "Thorin" {
		t.Errorf("home: %s", name())
	}
	// On an effect row the hints change, and combatant-only keys explain why.
	press(m, "G")
	if got := screen(m); !strings.Contains(got, "x end early") {
		t.Errorf("effect-row hints:\n%s", got)
	}
	for _, k := range []string{"d", "c", "C"} {
		press(m, k)
		if m.overlay != nil || m.msg != "Select a combatant row" {
			t.Errorf("%s on an effect row: overlay %T, msg %q", k, m.overlay, m.msg)
		}
	}
	// With nothing in the table, row keys say so instead of opening a prompt.
	empty := New(newSession(t), "")
	for k, want := range map[string]string{
		"d": "No combatant selected", "i": "Nothing selected", "N": "Nothing selected",
		"x": "Nothing selected", "e": "Add combatants first", "T": "Select an effect row to change its targets",
	} {
		press(empty, k)
		if empty.overlay != nil || empty.msg != want {
			t.Errorf("%s on an empty table: overlay %T, msg %q", k, empty.overlay, empty.msg)
		}
	}
	press(empty, "g", "G", "j") // no rows to move to, and no panic
	if empty.Selected() != 0 {
		t.Errorf("selection on an empty table: %d", empty.Selected())
	}
}

func TestTargetPickerNavigation(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	selectRow(t, m, "Spirit Guardians")
	press(m, "T")
	press(m, "g", "space")      // Thorin on
	special(m, "end")           // the effect's own row can't be ticked
	press(m, "space", "k", "k") // up to Mira
	press(m, "space")           // Mira on
	special(m, "home")          // back to the top
	press(m, "j", "down", "up") // Goblin 1
	press(m, "space")           // Goblin 1 off
	if got := screen(m); !strings.Contains(got, "targets: Thorin, Goblin 3, Mira") {
		t.Fatalf("picker line:\n%s", got)
	}
	press(m, "G", "enter")
	if got := strings.Join(targetNames(m, effect(t, m, "Spirit Guardians")), ","); got != "Goblin 3,Thorin,Mira" {
		t.Errorf("targets = %s", got)
	}
	// Cancelling leaves the targets alone and puts the selection back on
	// the effect, so T can be pressed again straight away.
	press(m, "T", "g", "space")
	special(m, "ctrl+c")
	if n := len(effect(t, m, "Spirit Guardians").Targets); n != 3 || m.overlay != nil {
		t.Errorf("after cancel: %d targets, overlay %T", n, m.overlay)
	}
	if m.Selected() != effect(t, m, "Spirit Guardians").ID {
		t.Error("cancelling the picker should restore the selection")
	}
	// Unticking everything shows "none".
	press(m, "T", "g", "space", "j", "j", "space", "space") // Thorin, Goblin 3 and Mira off
	if !strings.Contains(screen(m), "targets: none") {
		t.Errorf("empty picker:\n%s", screen(m))
	}
}

func TestChoicePromptKeys(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 80, 24)
	selectRow(t, m, "Goblin 2")
	press(m, "x")
	press(m, "z", "enter") // not an answer: the question stays (no default here)
	if m.overlay == nil || !strings.Contains(screen(m), "Remove Goblin 2 from combat?") {
		t.Fatalf("question should still be open:\n%s", screen(m))
	}
	press(m, "Y") // answers are case-insensitive
	if _, ok := m.sess.Encounter().Combatant(idOf2(m, "Goblin 2")); ok {
		t.Error("Goblin 2 should be gone")
	}
	selectRow(t, m, "Goblin 1")
	press(m, "x")
	special(m, "ctrl+c")
	press(m, "E", "n")
	if m.overlay != nil || m.sess.Encounter().Status != combat.Active {
		t.Error("ctrl+c and n should both back out")
	}
}

// idOf2 is idOf without failing the test when the row is gone.
func idOf2(m *Model, name string) combat.RowID {
	for _, r := range m.sess.Encounter().Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	return 0
}

func TestAddWithTempHP(t *testing.T) {
	s := newSession(t)
	m := New(s, "")
	press(m, "a")
	typeText(m, "Thorin")
	press(m, "enter")
	typeText(m, "18")
	press(m, "enter")
	typeText(m, "31/44+x")
	press(m, "enter")
	if hint, warn := m.overlay.hint(); !warn || !strings.Contains(hint, "Temp HP") {
		t.Fatalf("bad temp HP should be refused in the prompt: %q", hint)
	}
	press(m, "backspace")
	typeText(m, "5")
	press(m, "enter", "p")
	if c := combatant(t, s, "Thorin"); c.HP != 31 || c.MaxHP != 44 || c.TempHP != 5 || c.Kind != combat.PC {
		t.Errorf("thorin: %+v", c)
	}
	if !strings.Contains(screen(m), "31/44 +5") {
		t.Errorf("table should show the temp HP:\n%s", screen(m))
	}
}

func TestRefusedInputStaysOpen(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	check := func(open string, typed, want string) {
		t.Helper()
		press(m, open)
		press(m, "ctrl+u")
		typeText(m, typed)
		press(m, "enter")
		if hint, warn := "", false; m.overlay != nil {
			if hint, warn = m.overlay.hint(); warn && strings.Contains(hint, want) {
				press(m, "esc")
				return
			}
			t.Errorf("%s %q: hint %q (warn %v), want %q", open, typed, hint, warn, want)
			press(m, "esc")
			return
		}
		t.Errorf("%s %q: the prompt closed", open, typed)
	}
	check("d", "", "Enter a number")
	check("d", "lots", "isn't a whole number")
	check("i", "fast", "not a number")
	check("l", "  ", "Type something to log")
	check("a", "", "Enter a name")
	check("a", "Orc x0", "Group size must be 1–999")
	check("e", " ", "Enter a name")
	check("c", "", "Type a condition name")
	selectRow(t, m, "Goblin 3")
	check("C", "stunned", `Goblin 3 has no condition "stunned"`)

	// Removing by number, and the two "nothing to remove" messages.
	press(m, "C", "ctrl+u", "1", "enter")
	if c := combatant(t, s, "Goblin 3"); len(c.Conditions) != 0 {
		t.Errorf("Prone should be removed by number: %+v", c.Conditions)
	}
	press(m, "C")
	if !strings.Contains(m.msg, "only effect conditions") {
		t.Errorf("msg = %q", m.msg)
	}
	selectRow(t, m, "Goblin 2")
	press(m, "C")
	if m.msg != "Goblin 2 has no conditions" {
		t.Errorf("msg = %q", m.msg)
	}
}

func TestErrText(t *testing.T) {
	for err, want := range map[error]string{
		combat.ErrEmpty:         "No combatants to start with",
		combat.ErrNotActive:     "Combat hasn't started; press S to start",
		combat.ErrNotSetup:      "Combat has already started",
		combat.ErrNothingToUndo: "No earlier turn to go back to",
		errors.New(""):          "",
	} {
		if got := errText(err); got != want {
			t.Errorf("errText(%v) = %q, want %q", err, got, want)
		}
	}
	// The same messages reach the status line.
	m := New(newSession(t), "")
	press(m, "S")
	if m.msg != "No combatants to start with" {
		t.Errorf("S on an empty table: %q", m.msg)
	}
	press(m, "n")
	if m.msg != "Combat hasn't started; press S to start" {
		t.Errorf("n in Setup: %q", m.msg)
	}
}

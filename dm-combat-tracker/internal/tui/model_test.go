package tui

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
)

// updating reports whether -update was passed. The flag itself is defined by
// teatest's golden package, which this test binary already links.
func updating() bool {
	f := flag.Lookup("update")
	return f != nil && f.Value.String() == "true"
}

func fixedClock() time.Time { return time.Date(2026, 11, 14, 19, 42, 0, 0, time.UTC) }

// newSession opens a session in a temp dir and runs ops on it.
func newSession(t *testing.T, ops ...session.Op) *session.Session {
	t.Helper()
	t.Chdir(t.TempDir())
	s, _, err := session.Open(session.Config{Clock: fixedClock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, op := range ops {
		out, err := s.Apply(op)
		if err != nil {
			t.Fatal(err)
		}
		if out.SaveErr != nil || out.LogErr != nil {
			t.Fatalf("save %v, log %v", out.SaveErr, out.LogErr)
		}
	}
	return s
}

func add(name string, kind combat.Kind, init string, hp int) session.Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		v, err := combat.ParseInitiative(init)
		if err != nil {
			return nil, err
		}
		_, evs, err := e.Add(combat.CombatantSpec{Name: name, Kind: kind, Initiative: v, MaxHP: hp})
		return evs, err
	}
}

func group(name string, n int, init int, hp int) session.Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		_, evs, err := e.AddGroup(combat.GroupSpec{
			CombatantSpec: combat.CombatantSpec{Name: name, Initiative: combat.InitiativeFromInt(init), MaxHP: hp},
			Count:         n,
		})
		return evs, err
	}
}

// on runs f against the named combatant.
func on(name string, f func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error)) session.Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		for _, c := range e.Combatants {
			if c.Name == name {
				return f(e, c.ID)
			}
		}
		return nil, combat.ErrNotFound
	}
}

func idOf(t *testing.T, s *session.Session, name string) combat.RowID {
	t.Helper()
	for _, r := range s.Encounter().Order() {
		if r.Name() == name {
			return r.ID()
		}
	}
	t.Fatalf("no row %q", name)
	return 0
}

func start(e *combat.Encounter) ([]combat.Event, error) { return e.Start() }

func ptr(n int) *int { return &n }

// fight is the screen from DESIGN.md: a PC, goblins in various states, a
// caster with temp HP and an effect on two goblins.
func fight() []session.Op {
	return []session.Op{
		add("Thorin", combat.PC, "18", 44),
		group("Goblin", 3, 12, 7),
		add("Mira", combat.PC, "9", 27),
		add("Bandit Captain", combat.NPC, "9", 65),
		on("Thorin", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) {
			_, evs, err := e.AddCondition(id, "Blessed", ptr(2), "")
			return evs, err
		}),
		on("Thorin", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.Damage(id, 13) }),
		on("Goblin 2", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.Damage(id, 7) }),
		on("Goblin 3", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.Damage(id, 4) }),
		on("Goblin 3", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) {
			_, evs, err := e.AddCondition(id, "Prone", nil, "")
			return evs, err
		}),
		on("Goblin 1", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.SetNote(id, "fleeing") }),
		on("Mira", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.Damage(id, 5) }),
		on("Mira", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) { return e.SetTempHP(id, 5) }),
		on("Mira", func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) {
			_, evs, err := e.AddCondition(id, "Concentrating", nil, "")
			return evs, err
		}),
		on("Mira", func(e *combat.Encounter, mira combat.RowID) ([]combat.Event, error) {
			var targets []combat.RowID
			for _, c := range e.Combatants {
				if c.Name == "Goblin 1" || c.Name == "Goblin 3" {
					targets = append(targets, c.ID)
				}
			}
			_, evs, err := e.AddEffect(combat.EffectSpec{Name: "Spirit Guardians", SourceID: mira, Rounds: ptr(10), Targets: targets, Note: "3d8 radiant"})
			return evs, err
		}),
		start,
	}
}

func key(k string) tea.KeyPressMsg {
	switch k {
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	r := []rune(k)[0]
	return tea.KeyPressMsg{Code: r, Text: k}
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		m.Update(key(k))
	}
}

func screen(m *Model) string { return ansi.Strip(m.render()) }

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if updating() {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/tui -update to create it)", err)
	}
	if got != string(want) {
		t.Errorf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// goldenDir remembers the package dir, since newSession changes directory.
var goldenDir, _ = os.Getwd()

func sized(m *Model, w, h int) *Model {
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m
}

func TestRenderFight(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    int
	}{
		{"fight_80.golden", 80},
		{"fight_120.golden", 120},
		{"fight_64.golden", 64},
		{"fight_56.golden", 56},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := sized(New(newSession(t, fight()...), ""), tc.w, 24)
			got := screen(m)
			for i, line := range strings.Split(got, "\n") {
				if w := ansi.StringWidth(line); w != tc.w {
					t.Errorf("line %d is %d wide, want %d: %q", i, w, tc.w, line)
				}
			}
			t.Chdir(goldenDir)
			golden(t, tc.name, got)
		})
	}
}

func TestRenderSetupEmpty(t *testing.T) {
	m := New(newSession(t), "")
	got := screen(m)
	t.Chdir(goldenDir)
	golden(t, "setup_empty.golden", got)
}

func TestNarrowColumns(t *testing.T) {
	has := func(w int, col column) bool {
		for _, c := range computeLayout(w).cols {
			if c == col {
				return true
			}
		}
		return false
	}
	if !has(80, colNote) || !has(80, colKind) {
		t.Error("80 columns should fit every column")
	}
	if has(64, colNote) || !has(64, colKind) {
		t.Error("64 columns should drop Note only")
	}
	if has(56, colNote) || has(56, colKind) {
		t.Error("56 columns should drop Note and Kind")
	}
}

func TestStartNextPrevious(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), add("Goblin", combat.NPC, "12", 7))
	m := New(s, "")
	enc := s.Encounter()

	press(m, "n")
	if !strings.Contains(m.msg, "press S") {
		t.Errorf("next during setup: msg %q", m.msg)
	}

	press(m, "S")
	if enc.Status != combat.Active || enc.Round != 1 || m.Selected() != idOf(t, s, "Thorin") {
		t.Fatalf("after start: status %v round %d selected %d", enc.Status, enc.Round, m.Selected())
	}
	if !strings.Contains(screen(m), "Round 1 ─ Turn: Thorin") {
		t.Errorf("header missing:\n%s", screen(m))
	}

	press(m, "n")
	if enc.Current != idOf(t, s, "Goblin") || m.Selected() != enc.Current {
		t.Errorf("next: current %d selected %d", enc.Current, m.Selected())
	}
	press(m, "space")
	if enc.Round != 2 || enc.Current != idOf(t, s, "Thorin") {
		t.Errorf("space should wrap to round 2, got round %d current %d", enc.Round, enc.Current)
	}

	press(m, "p", "p")
	if enc.Round != 1 || enc.Current != idOf(t, s, "Thorin") || m.Selected() != enc.Current {
		t.Errorf("previous twice: round %d current %d selected %d", enc.Round, enc.Current, m.Selected())
	}
	press(m, "p")
	if m.level != levelWarn || !strings.Contains(m.msg, "No earlier turn") {
		t.Errorf("rewind past start: level %v msg %q", m.level, m.msg)
	}
	press(m, "n")
	if m.msg != "" {
		t.Errorf("a successful action should clear the message, got %q", m.msg)
	}
}

func TestSelectionIsIndependentOfTurn(t *testing.T) {
	s := newSession(t, fight()...)
	m := New(s, "")
	enc := s.Encounter()

	press(m, "j", "j")
	if m.Selected() != idOf(t, s, "Goblin 2") || enc.Current != idOf(t, s, "Thorin") {
		t.Fatalf("j j: selected %d current %d", m.Selected(), enc.Current)
	}
	press(m, "k", "up", "up", "up")
	if m.Selected() != idOf(t, s, "Thorin") {
		t.Errorf("selection should stop at the top, got %d", m.Selected())
	}
	press(m, "G")
	if m.Selected() != idOf(t, s, "Spirit Guardians") {
		t.Errorf("G should select the last row, got %d", m.Selected())
	}
	press(m, "down")
	if m.Selected() != idOf(t, s, "Spirit Guardians") {
		t.Errorf("selection should stop at the bottom, got %d", m.Selected())
	}
	press(m, "g")
	if m.Selected() != idOf(t, s, "Thorin") {
		t.Errorf("g should select the first row, got %d", m.Selected())
	}
}

func TestSelectionSurvivesRemovedRow(t *testing.T) {
	s := newSession(t, group("Goblin", 3, 12, 7), start)
	m := New(s, "")
	press(m, "j")
	g2 := idOf(t, s, "Goblin 2")
	if m.Selected() != g2 {
		t.Fatalf("selected %d, want Goblin 2", m.Selected())
	}
	if _, err := s.Apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.Remove(g2, combat.KeepEffects) }); err != nil {
		t.Fatal(err)
	}
	m.syncSelection()
	if m.Selected() != idOf(t, s, "Goblin 3") {
		t.Errorf("after removing the selected row, selection should land on the row in its place; got %d", m.Selected())
	}
}

func TestScrollKeepsSelectionVisible(t *testing.T) {
	s := newSession(t, group("Orc", 20, 10, 15), start)
	m := sized(New(s, ""), 80, 12) // 5 visible rows
	press(m, "G")
	got := screen(m)
	if !strings.Contains(got, "Orc 20") || strings.Contains(got, "│ Orc 1 ") {
		t.Errorf("bottom row should be visible and top scrolled away:\n%s", got)
	}
	if !strings.Contains(got, "Rows 16–20 of 20") {
		t.Errorf("missing scroll position:\n%s", got)
	}
	press(m, "n") // turn moves to Orc 2; selection follows it back up
	got = screen(m)
	if !strings.Contains(got, "▶") || !strings.Contains(got, "│ Orc 2 ") {
		t.Errorf("selection should scroll back to the turn holder:\n%s", got)
	}
	if lines := strings.Count(got, "\n") + 1; lines != 12 {
		t.Errorf("screen is %d lines, want 12", lines)
	}
}

func TestResumeNotice(t *testing.T) {
	m := New(newSession(t, fight()...), "dmct-state.json")
	if !strings.Contains(screen(m), "Resumed from dmct-state.json") {
		t.Errorf("missing resume notice:\n%s", screen(m))
	}
}

func TestSaveFailureBanner(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), add("Goblin", combat.NPC, "12", 7), start)
	// Make the state file's directory unwritable so autosave fails.
	dir, _ := os.Getwd()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	m := New(s, "")
	press(m, "n")
	if s.Encounter().Current != idOf(t, s, "Goblin") {
		t.Error("the turn should still advance when saving fails")
	}
	if !strings.Contains(screen(m), "✖ Autosave failing") {
		t.Fatalf("no banner:\n%s", screen(m))
	}
	// The banner stays up while prompts open and other messages come and go.
	press(m, "d")
	if !strings.Contains(screen(m), "✖ Autosave failing") {
		t.Errorf("banner gone while a prompt is open:\n%s", screen(m))
	}
	press(m, "esc", "x", "y") // refused: x on a combatant asks first; y removes
	if !strings.Contains(screen(m), "✖ Autosave failing") {
		t.Errorf("banner gone after another change:\n%s", screen(m))
	}

	// Quitting asks first; n stays.
	if cmd := m.handleKey("q"); cmd != nil {
		t.Fatal("q quit without asking while autosave is failing")
	}
	if _, ok := m.overlay.(*choicePrompt); !ok {
		t.Fatalf("q opened %T, want a confirmation", m.overlay)
	}
	if _, cmd := m.Update(key("n")); cmd != nil {
		t.Fatal("n at the quit confirmation quit")
	}

	// Once the directory is writable again the next change saves and the
	// banner clears.
	os.Chmod(dir, 0o755)
	press(m, "n")
	if strings.Contains(screen(m), "Autosave failing") || !strings.Contains(m.msg, "working again") {
		t.Errorf("after recovery, msg %q:\n%s", m.msg, screen(m))
	}
	if cmd := m.handleKey("q"); cmd == nil {
		t.Error("q should quit at once when saving works")
	}
}

func TestQuitConfirmationQuits(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), start)
	m := New(s, "")
	m.saveErr = os.ErrPermission
	press(m, "q")
	if _, cmd := m.Update(key("y")); cmd == nil {
		t.Error("y at the quit confirmation should quit")
	}
}

func TestLogFailureBanner(t *testing.T) {
	t.Chdir(t.TempDir())
	s, _, err := session.Open(session.Config{Clock: fixedClock, LogPath: filepath.Join("logs", "fight.md")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	m := New(s, "")
	for _, op := range []session.Op{add("Thorin", combat.PC, "18", 44), add("Goblin", combat.NPC, "12", 7)} {
		m.apply(op)
	}
	if m.banner() != "" {
		t.Fatalf("banner in Setup: %q", m.banner())
	}
	press(m, "S", "n") // starting binds the log; logs/ doesn't exist
	if b := m.banner(); !strings.Contains(b, "✖ Log failing") || !strings.Contains(b, "2 changes not logged") {
		t.Fatalf("banner %q", b)
	}
	if strings.Contains(m.banner(), "Autosave") {
		t.Error("autosave worked; only the log should be in the banner")
	}
	wd, _ := os.Getwd()
	t.Chdir(goldenDir)
	golden(t, "banner_80.golden", screen(m))
	t.Chdir(wd)
	if err := os.Mkdir("logs", 0o755); err != nil {
		t.Fatal(err)
	}
	press(m, "n")
	if m.banner() != "" || !strings.Contains(m.msg, "2 changes before this weren't logged") {
		t.Errorf("after recovery: banner %q, msg %q", m.banner(), m.msg)
	}
}

func TestTooSmall(t *testing.T) {
	for _, size := range [][2]int{{minWidth - 1, 24}, {80, minHeight - 1}} {
		m := sized(New(newSession(t, fight()...), ""), size[0], size[1])
		got := screen(m)
		if !strings.Contains(got, "Terminal too small") {
			t.Errorf("%v: no notice:\n%s", size, got)
		}
		lines := strings.Split(got, "\n")
		if len(lines) != size[1] {
			t.Errorf("%v: %d lines", size, len(lines))
		}
		for i, line := range lines {
			if w := ansi.StringWidth(line); w != size[0] {
				t.Errorf("%v: line %d is %d wide", size, i, w)
			}
		}
		before := m.sess.Encounter().Current
		press(m, "n", "d")
		if m.overlay != nil || m.sess.Encounter().Current != before {
			t.Errorf("%v: keys acted on a screen that isn't showing", size)
		}
		if _, cmd := m.Update(key("q")); cmd == nil {
			t.Errorf("%v: q should still quit", size)
		}
	}
}

func TestNarrowestLayout(t *testing.T) {
	for w := minWidth; w <= 56; w++ {
		m := sized(New(newSession(t, fight()...), ""), w, minHeight)
		for i, line := range strings.Split(screen(m), "\n") {
			if lw := ansi.StringWidth(line); lw != w {
				t.Errorf("width %d: line %d is %d wide: %q", w, i, lw, line)
			}
		}
	}
	m := sized(New(newSession(t, fight()...), ""), minWidth, 24)
	t.Chdir(goldenDir)
	golden(t, "fight_43.golden", screen(m))
}

func TestFitName(t *testing.T) {
	for _, tc := range []struct {
		name string
		w    int
		want string
	}{
		{"Goblin 1", 10, "Goblin 1"},
		{"Goblin 1", 6, "Gob… 1"},
		{"Goblin 12", 6, "Go… 12"},
		{"Bandit Captain", 10, "Bandit Ca…"},
		{"◆ Spirit Guardians", 6, "◆ Spi…"},
		{"Orc 123456", 6, "Orc 1…"}, // the number alone wouldn't leave room for a name
	} {
		if got := strings.TrimRight(fitName(tc.name, tc.w), " "); got != tc.want {
			t.Errorf("fitName(%q, %d) = %q, want %q", tc.name, tc.w, got, tc.want)
		}
	}
}

func TestSelectedDetail(t *testing.T) {
	m := sized(New(newSession(t, fight()...), ""), 64, 24)
	selectRow(t, m, "Goblin 1") // its note is in the dropped Note column
	if got := m.selectedDetail(computeLayout(64), buildRows(m.sess.Encounter())); got != "Goblin 1 · ◆ Spirit Guardians · Note: fleeing" {
		t.Errorf("detail %q", got)
	}
	if !strings.Contains(screen(m), "Note: fleeing") {
		t.Errorf("detail not on screen:\n%s", screen(m))
	}
	selectRow(t, m, "Thorin") // everything fits
	if got := m.selectedDetail(computeLayout(64), buildRows(m.sess.Encounter())); got != "" {
		t.Errorf("detail %q for a row that fits", got)
	}
}

// TestProgram drives the real Bubble Tea program: start, advance, quit, and
// check the change reached the state file.
func TestProgram(t *testing.T) {
	s := newSession(t, add("Thorin", combat.PC, "18", 44), add("Goblin", combat.NPC, "12", 7))
	tm := teatest.NewTestModel(t, New(s, ""), teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool { return bytes.Contains(b, []byte("Setup")) })
	// Keys are handled in order; the renderer only sends what changed, so
	// check the outcome on the final model and the state file, not the bytes.
	tm.Send(key("S"))
	tm.Send(key("n"))
	for _, k := range []string{"d", "3", "enter"} { // Goblin now holds the turn and the selection
		tm.Send(key(k))
	}
	tm.Send(key("q"))
	fm := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(*Model)
	if fm.Selected() != idOf(t, s, "Goblin") {
		t.Errorf("final selection %d, want Goblin", fm.Selected())
	}

	enc, err := store.Load(session.DefaultStatePath, fixedClock)
	if err != nil || enc == nil {
		t.Fatalf("load state: %v, %v", enc, err)
	}
	if enc.Status != combat.Active || enc.Round != 1 || enc.Current != idOf(t, s, "Goblin") {
		t.Errorf("saved state: status %v round %d current %d", enc.Status, enc.Round, enc.Current)
	}
	if g, _ := enc.Combatant(idOf(t, s, "Goblin")); g == nil || g.HP != 4 {
		t.Errorf("saved goblin: %+v", g)
	}
}

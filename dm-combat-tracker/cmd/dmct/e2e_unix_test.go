//go:build unix

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
)

// End-to-end tests: the real dmct binary runs in a pseudo-terminal, keys are
// typed into it, and a terminal emulator turns its output back into the
// screen the DM would see. Each test is one story from REQUIREMENTS.md,
// checked on the screen and in the state and log files it leaves behind.
//
// Set GOCOVERDIR to collect coverage from the binary itself:
//
//	mkdir -p /tmp/dmct-cov && GOCOVERDIR=/tmp/dmct-cov go test ./cmd/dmct
//	go tool covdata percent -i /tmp/dmct-cov

var bin string // the built dmct binary

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "dmct-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	bin = filepath.Join(dir, "dmct")
	args := []string{"build", "-o", bin}
	if os.Getenv("GOCOVERDIR") != "" {
		args = append(args, "-cover", "-coverpkg=./...")
	}
	if out, err := exec.Command("go", append(args, ".")...).CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build dmct: %v\n%s", err, out)
		os.Exit(1)
	}
	// No test should read ruleset files from the real config folder (D38).
	// Set after the build, which needs the real HOME for its cache.
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Setenv("HOME", home)
	os.Setenv("XDG_CONFIG_HOME", "")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const (
	enter = "\r"
	esc   = "\x1b"
	up    = "\x1b[A"
	down  = "\x1b[B"
	tab   = "\t"
)

// app is one running dmct in a pseudo-terminal.
type app struct {
	t    *testing.T
	dir  string
	cmd  *exec.Cmd
	tty  *os.File
	done chan error

	mu   sync.Mutex // guards term: output arrives while the test reads the screen
	term *vt.Emulator
}

// Write feeds program output to the emulated terminal.
func (a *app) Write(p []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.Write(p)
}

// launch starts dmct in dir at 80×24 and waits for its first screen.
func launch(t *testing.T, dir string, args ...string) *app {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	tty, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("start dmct: %v", err)
	}
	a := &app{t: t, dir: dir, cmd: cmd, tty: tty, term: vt.NewEmulator(80, 24), done: make(chan error, 1)}
	go func() { _, _ = io.Copy(a, tty) }()      // program output → emulated screen
	go func() { _, _ = io.Copy(tty, a.term) }() // the terminal's replies to queries → program
	go func() { a.done <- cmd.Wait() }()
	t.Cleanup(func() {
		// The emulator is left open: its Close races with its own Read, and
		// the reply goroutine ends with the test binary anyway.
		_ = cmd.Process.Kill()
		_ = tty.Close()
	})
	a.see("? help")
	return a
}

// screen is what the terminal shows right now, as plain text.
func (a *app) screen() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.term.String()
}

// keys types into the terminal.
func (a *app) keys(keys ...string) {
	a.t.Helper()
	for _, k := range keys {
		if _, err := a.tty.WriteString(k); err != nil {
			a.t.Fatalf("type %q: %v", k, err)
		}
	}
}

// see waits until every one of want is on screen.
func (a *app) see(want ...string) {
	a.t.Helper()
	a.until(func(s string) bool {
		for _, w := range want {
			if !strings.Contains(s, w) {
				return false
			}
		}
		return true
	}, fmt.Sprintf("screen to show %q", want))
}

// gone waits until none of unwanted is on screen.
func (a *app) gone(unwanted ...string) {
	a.t.Helper()
	a.until(func(s string) bool {
		for _, w := range unwanted {
			if strings.Contains(s, w) {
				return false
			}
		}
		return true
	}, fmt.Sprintf("%q to leave the screen", unwanted))
}

func (a *app) until(ok func(screen string) bool, what string) {
	a.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !ok(a.screen()) {
		if time.Now().After(deadline) {
			a.t.Fatalf("timed out waiting for %s; screen:\n%s", what, a.screen())
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// do types keys, then waits for want to show: one step of a story.
func (a *app) do(keys string, want ...string) {
	a.t.Helper()
	a.keys(keys)
	a.see(want...)
}

// row returns the table line for the named row.
func (a *app) row(name string) string {
	a.t.Helper()
	for _, line := range strings.Split(a.screen(), "\n") {
		if strings.Contains(line, "│") && strings.Contains(line, name) {
			return line
		}
	}
	a.t.Fatalf("no table row for %q; screen:\n%s", name, a.screen())
	return ""
}

// rowShows waits for the named row to contain every one of want.
func (a *app) rowShows(name string, want ...string) {
	a.t.Helper()
	a.until(func(s string) bool {
		for _, line := range strings.Split(s, "\n") {
			if !strings.Contains(line, "│") || !strings.Contains(line, name) {
				continue
			}
			all := true
			for _, w := range want {
				all = all && strings.Contains(line, w)
			}
			if all {
				return true
			}
		}
		return false
	}, fmt.Sprintf("row %q to show %q", name, want))
}

// names lists the Name column top to bottom: the turn order on screen.
func (a *app) names() string {
	var out []string
	for _, line := range strings.Split(a.screen(), "\n") {
		cells := strings.Split(line, "│")
		if len(cells) < 5 || strings.TrimSpace(cells[3]) == "Name" {
			continue
		}
		out = append(out, strings.TrimSpace(cells[3]))
	}
	return strings.Join(out, ", ")
}

func (a *app) resize(cols, rows int) {
	a.t.Helper()
	a.mu.Lock()
	a.term.Resize(cols, rows)
	a.mu.Unlock()
	if err := pty.Setsize(a.tty, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}); err != nil {
		a.t.Fatal(err)
	}
}

// exit waits for dmct to exit and returns its exit code.
func (a *app) exit() int {
	a.t.Helper()
	select {
	case err := <-a.done:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		if err != nil {
			a.t.Fatalf("wait: %v", err)
		}
		return 0
	case <-time.After(5 * time.Second):
		a.t.Fatalf("dmct didn't exit; screen:\n%s", a.screen())
		return -1
	}
}

// quit presses q and expects a clean exit.
func (a *app) quit() {
	a.t.Helper()
	a.keys("q")
	if code := a.exit(); code != 0 {
		a.t.Fatalf("exit code %d", code)
	}
}

// state loads a state file from the app's folder.
func (a *app) state(name string) *combat.Encounter {
	a.t.Helper()
	enc, err := store.Load(filepath.Join(a.dir, name), nil)
	if err != nil || enc == nil {
		a.t.Fatalf("load %s: %v, %v", name, enc, err)
	}
	return enc
}

func combatantNamed(t *testing.T, enc *combat.Encounter, name string) *combat.Combatant {
	t.Helper()
	for _, c := range enc.Combatants {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no combatant %q in the state file", name)
	return nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// logLines returns the log's entries with the clock time cut off, so they
// can be compared exactly: "[R1 · Thorin] Goblin 2 dropped to 0 HP".
func logLines(t *testing.T, path string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(read(t, path), "\n") {
		switch {
		case strings.HasPrefix(line, "- `"):
			_, rest, _ := strings.Cut(line, "` ")
			out = append(out, strings.ReplaceAll(rest, "**", ""))
		case strings.HasPrefix(line, "# Combat — "):
			out = append(out, "# Combat")
		case line != "":
			out = append(out, line)
		}
	}
	return out
}

func wantLines(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("log:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func oneFile(t *testing.T, dir, pattern string) string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, pattern))
	if len(m) != 1 {
		entries, _ := os.ReadDir(dir)
		t.Fatalf("want one %s, have %v (folder: %v)", pattern, m, entries)
	}
	return m[0]
}

// The main story: set up a fight with hotkeys and the command bar, run two
// rounds of it, end it, and read the recap log (R1–R7).
func TestFullFight(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir)
	a.see("Setup ─ press S to start combat", "No combatants yet")

	// Setup: a PC through the add prompts, the rest through the command bar.
	a.do("a", "Add — name:")
	a.do("Thorin"+enter, "Add Thorin · initiative:")
	a.do("18"+enter, "Add Thorin · HP:")
	a.do("31/44"+enter, "Add Thorin · kind?")
	a.do("p", "31/44")
	a.do(":add Goblin x3 12 7"+enter, "Goblin 3")
	a.do(":add Mira 9 27+5 pc -- concentrating on bless"+enter, "27/27 +5")
	a.rowShows("Mira", "PC", "27/27 +5", "concentr")
	if got := a.names(); got != "Thorin, Goblin 1, Goblin 2, Goblin 3, Mira" {
		t.Fatalf("turn order on screen: %s", got)
	}

	a.do("S", "Round 1 ─ Turn: Thorin")
	a.rowShows("Thorin", "▶")

	// Thorin's turn: hit Goblin 1 for more than it has, with the hotkey.
	a.do("jd", "Damage Goblin 1:") // j selects Goblin 1; the prompt names who it acts on
	a.keys("9" + enter)
	a.rowShows("Goblin 1", "DOWN 0/7")

	// Goblin 1 is down but keeps its turn; it gets healed and knocked prone.
	a.do("n", "Round 1 ─ Turn: Goblin 1")
	a.do("h", "Heal Goblin 1:")
	a.keys("3" + enter)
	a.rowShows("Goblin 1", "3/7 ▼")
	a.do("c", "Condition on Goblin 1:")
	a.do("pro"+tab, "Prone")
	a.do(enter, "Prone on Goblin 1 · duration:")
	a.do("1"+enter, "Prone on Goblin 1 · note:")
	a.keys("DC 12 to stand" + enter)
	a.rowShows("Goblin 1", "Prone (1)")

	a.do("n", "Turn: Goblin 2")
	a.do("n", "Turn: Goblin 3")
	a.do("n", "Turn: Mira")

	// Mira casts Spirit Guardians on two goblins, picked on the table.
	a.do("e", "New effect · name:")
	a.do("Spirit Guardians"+enter, "Spirit Guardians · source?")
	a.do(enter, "Spirit Guardians (Mira) · duration:")
	a.do("10"+enter, "pick targets: none")
	a.do("gj", "space toggle") // top, then down to Goblin 1
	a.do(" ", "pick targets: Goblin 1")
	a.do(" ", "pick targets: Goblin 1, Goblin 2")
	a.do(enter, "10 rds · 2 ta")
	a.rowShows("Goblin 2", "◆ Spirit Guar")
	if got := a.names(); got != "Thorin, Goblin 1, Goblin 2, Goblin 3, Mira, ◆ Spirit Gu…" {
		t.Fatalf("the effect should sit right after Mira: %s", got)
	}
	// Mira takes a hit: temp HP soaks it first.
	a.keys(":dmg mira 8" + enter)
	a.rowShows("Mira", "24/27")
	a.do("lMira's guardians fill the hall"+enter, "Logged to dmct-log-")

	// The effect's own row comes up as a reminder (it counts down on Mira's
	// turns, not here), and round 2 begins.
	a.do("n", "Turn: Spirit Guardians", "10 rds")
	a.do("n", "Round 2 ─ Turn: Thorin")
	// Goblin 3 runs into the area; Goblin 1 crawls out of it.
	a.keys(":fxadd spirit g3" + enter)
	a.rowShows("Goblin 3", "◆ Spirit Guar")
	a.keys(":fxrm spirit g1" + enter)
	a.until(func(string) bool { return !strings.Contains(a.row("Goblin 1"), "◆") }, "Goblin 1 to leave the effect")

	// Goblin 1's turn starts: Prone runs out. A mis-pressed n is taken back.
	a.do("n", "Turn: Goblin 1")
	a.until(func(string) bool { return !strings.Contains(a.row("Goblin 1"), "Prone") }, "Prone to expire")
	a.do("n", "Turn: Goblin 2")
	a.do("p", "Turn: Goblin 1")

	// Mira drops; her concentration breaks and the effect is dispelled.
	a.keys(":hp mira 0" + enter)
	a.rowShows("Mira", "DOWN 0/27")
	a.do("G", "x end early") // the effect row
	a.do("x", "End Spirit Guardians early · reason:")
	a.keys("concentration broken" + enter)
	a.gone("Spirit Gu")

	// Everything so far is on disk.
	enc := a.state("dmct-state.json")
	if enc.Status != combat.Active || enc.Round != 2 || len(enc.Effects) != 0 {
		t.Errorf("saved: status %s, round %d, %d effects", enc.Status, enc.Round, len(enc.Effects))
	}
	if g := combatantNamed(t, enc, "Goblin 1"); g.HP != 3 || len(g.Conditions) != 0 || enc.Current != g.ID {
		t.Errorf("saved Goblin 1: %+v (current %d)", g, enc.Current)
	}
	if m := combatantNamed(t, enc, "Mira"); m.HP != 0 || m.TempHP != 0 || m.Kind != combat.PC || m.Note != "concentrating on bless" {
		t.Errorf("saved Mira: %+v", m)
	}

	// End the fight: the state is archived and a new Setup begins.
	a.do("E", "End combat after 2 rounds?")
	a.do("y", "Setup ─ press S to start combat", "Combat ended · archived as dmct-state-")
	a.quit()

	if _, err := os.Stat(filepath.Join(dir, "dmct-state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the live state file should be gone after the end (stat: %v)", err)
	}
	archived, err := store.Load(oneFile(t, dir, "dmct-state-*.json"), nil)
	if err != nil || archived.Status != combat.Ended || archived.Round != 2 || len(archived.Combatants) != 5 {
		t.Errorf("archive: %+v, %v", archived, err)
	}
	wantLines(t, logLines(t, oneFile(t, dir, "dmct-log-*.md")),
		"# Combat",
		"## Round 1",
		"[R1 · Thorin] Combat started: Thorin (18), Goblin 1–3 (12.3–12.1), Mira (9)",
		"[R1 · Thorin] Goblin 1 dropped to 0 HP",
		"[R1 · Goblin 1] Goblin 1 is back above 0 HP",
		"[R1 · Goblin 1] Prone applied to Goblin 1 (1 round; DC 12 to stand)",
		"[R1 · Mira] Effect started: Spirit Guardians (Mira, 10 rounds) on Goblin 1, Goblin 2",
		"[R1 · Mira] 📝 Mira's guardians fill the hall",
		"## Round 2",
		"[R2 · Thorin] Spirit Guardians now also on Goblin 3",
		"[R2 · Thorin] Spirit Guardians no longer on Goblin 1",
		"[R2 · Goblin 1] Prone expired on Goblin 1",
		"[R2 · Goblin 1] Turn rewound to Goblin 1",
		"[R2 · Goblin 1] Mira dropped to 0 HP",
		"[R2 · Goblin 1] Effect ended early: Spirit Guardians (concentration broken)",
		"[R2 · Goblin 1] Combat ended after 2 rounds",
	)
}

// A closed laptop costs nothing: quit mid-fight, start again, and the round,
// turn, HP, conditions and effects are all where they were (R7). The log
// carries on in the same file without repeating its headings.
func TestQuitAndResume(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir, "--state", "fight.json", "--log", "recap.md")
	a.do(":add Thorin 18 44 pc"+enter, "44/44")
	a.do(":add Goblin x2 12 7"+enter, "Goblin 2")
	a.do(":log the party kicks the door in"+enter, "Logged to recap.md")
	a.do(":start"+enter, "Round 1 ─ Turn: Thorin")
	a.keys(":cond g2 poisoned 3 -- DC 11" + enter)
	a.rowShows("Goblin 2", "Poisoned (3)")
	a.keys(":fx Bless 10 from thorin on thorin" + enter)
	a.rowShows("Thorin", "◆ Bless")
	a.do(":next"+enter, "Turn: Bless", "10 rds")
	a.do("n", "Turn: Goblin 1")
	a.keys(":dmg g1 4" + enter)
	a.rowShows("Goblin 1", "3/7 ▼")
	before := a.screen()
	a.quit()

	// A different --log is ignored: the fight keeps the log it started with.
	b := launch(t, dir, "--state", "fight.json", "--log", "elsewhere.md")
	b.see("Resumed from fight.json", "Round 1 ─ Turn: Goblin 1")
	for _, name := range []string{"Thorin", "Goblin 1", "Goblin 2", "Bless"} {
		was := ""
		for _, line := range strings.Split(before, "\n") {
			if strings.Contains(line, "│") && strings.Contains(line, name) {
				was = line
				break
			}
		}
		if got := b.row(name); got != was {
			t.Errorf("%s after resume:\n%s\nbefore:\n%s", name, got, was)
		}
	}
	b.do("n", "Turn: Goblin 2")
	b.rowShows("Goblin 2", "Poisoned (2)")
	b.do("n", "Round 2 ─ Turn: Thorin")
	b.rowShows("Bless", "9 rds") // an effect counts down as its caster's turn begins
	b.keys(":rm goblin 1" + enter)
	b.gone("Goblin 1")
	// Rewinding still works across the restart, back into round 1.
	b.do("p", "Round 1 ─ Turn: Goblin 2")
	b.rowShows("Goblin 2", "Poisoned (2)") // only the undone step's countdowns come back
	b.rowShows("Bless", "10 rds")
	// One more step would land on Goblin 1, who has left: refused.
	b.do("p", "No earlier turn to go back to: that row has been removed", "Round 1 ─ Turn: Goblin 2")
	b.quit()

	if _, err := os.Stat(filepath.Join(dir, "elsewhere.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the second --log should not be used (stat: %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dmct-state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("--state should replace the default file (stat: %v)", err)
	}
	wantLines(t, logLines(t, filepath.Join(dir, "recap.md")),
		"# Combat",
		"[Setup] 📝 the party kicks the door in",
		"## Round 1",
		"[R1 · Thorin] Combat started: Thorin (18), Goblin 1–2 (12.2–12.1)",
		"[R1 · Thorin] Poisoned applied to Goblin 2 (3 rounds; DC 11)",
		"[R1 · Thorin] Effect started: Bless (Thorin, 10 rounds) on Thorin",
		"## Round 2",
		"[R2 · Thorin] Goblin 1 removed from combat",
		"[R1 · Goblin 2] Turn rewound to Goblin 2",
	)
}

// Removing a combatant who is the source of effects asks whether to keep or
// end them, and removing the turn holder passes the turn (R3, R5).
func TestRemoveSourceAndTurnHolder(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir)
	a.do(":add Mira 15 27 pc"+enter, "27/27")
	a.do(":add Orc x2 10 15"+enter, "Orc 2")
	a.do(":start"+enter, "Round 1 ─ Turn: Mira")
	a.do(":fx Web from mira on o1,o2"+enter, "2 targets")
	a.do(":fx Fog from mira"+enter, "0 targets")

	// x on Mira: confirm, then the keep-or-end question. Esc backs out of it
	// with nothing changed.
	a.do("gx", "Remove Mira from combat?")
	a.do("y", "Mira is the source of Web, Fog: keep or end?")
	a.keys(esc)
	a.see("n next")
	if got := a.names(); got != "Mira, ◆ Web, ◆ Fog, Orc 1, Orc 2" {
		t.Fatalf("nothing should change on esc: %s", got)
	}

	// Again, keeping them: Mira goes, the effects stay where they were in
	// the order, and the turn passes to the next row.
	a.do("x", "Remove Mira from combat?")
	a.do("y", "keep or end?")
	a.do("k", "Round 1 ─ Turn: Web")
	if got := a.names(); got != "◆ Web, ◆ Fog, Orc 1, Orc 2" {
		t.Fatalf("after keeping: %s", got)
	}
	a.rowShows("Orc 1", "◆ Web")

	// The command bar asks the same question; this time end them. Orc 1 is
	// made the source of a new effect first.
	a.do(":fx Rage from orc 1"+enter, "◆ Rage")
	a.do(":rm o1"+enter, "Orc 1 is the source of Rage: keep or end?")
	a.keys("e")
	a.gone("Orc 1", "Rage")
	if got := a.names(); got != "◆ Web, ◆ Fog, Orc 2" {
		t.Fatalf("after ending: %s", got)
	}
	a.rowShows("Web", "1 target")
	a.quit()

	enc := a.state("dmct-state.json")
	if len(enc.Effects) != 2 || enc.Effects[0].SourceID != 0 || len(enc.Effects[0].Targets) != 1 {
		t.Errorf("saved effects: %+v", enc.Effects)
	}
	lines := strings.Join(logLines(t, oneFile(t, dir, "dmct-log-*.md")), "\n")
	for _, want := range []string{
		"[R1 · Mira] Mira removed from combat",
		"[R1 · Web] Orc 1 removed from combat",
		"[R1 · Web] Effect ended early: Rage (Orc 1 left combat)",
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("log is missing %q:\n%s", want, lines)
		}
	}
}

// Tied initiatives are flagged until the DM settles them with a decimal;
// re-sorting never moves the turn, and timed effects end on their own (R2, R5).
func TestInitiativeTiesAndExpiry(t *testing.T) {
	a := launch(t, t.TempDir())
	a.do(":add Thorin 15 44 pc"+enter, "44/44")
	a.do(":add Mira 15 27 pc"+enter, "! Tied initiative: enter a decimal")
	a.rowShows("Thorin", "15 !")
	a.rowShows("Mira", "15 !")
	a.do(":add Wolf x2 9,20 11"+enter, "Wolf 2")
	if got := a.names(); got != "Wolf 2, Thorin, Mira, Wolf 1" {
		t.Fatalf("order with separate group initiatives: %s", got)
	}
	a.do(":start"+enter, "Round 1 ─ Turn: Wolf 2")

	// Mira's initiative is fixed with the hotkey; the bad value is refused
	// inside the prompt.
	a.do("jji", "Initiative for Mira:")
	a.do("\x15fast"+enter, "not a number") // ctrl+u clears the old value
	a.keys("\x1515.5" + enter)
	a.gone("Tied initiative", "15 !")
	if got := a.names(); got != "Wolf 2, Mira, Thorin, Wolf 1" {
		t.Fatalf("order after the edit: %s", got)
	}
	a.see("Round 1 ─ Turn: Wolf 2")

	// A 2-round lair effect with its own place in the order.
	a.do(":fx Tremor 2 at 1"+enter, "2 rds")
	a.do("nnnn", "Turn: Tremor", "1 rd ")
	// Next time round it expires on its own turn, and the turn moves straight on.
	a.do("nnnn", "Round 2 ─ Turn: Wolf 1")
	a.do("n", "Round 3 ─ Turn: Wolf 2")
	a.gone("Tremor")
	// One step back restores it.
	a.do("p", "Round 2 ─ Turn: Wolf 1", "Tremor", "1 rd ")

	// Commands that can't work stay open with the reason.
	a.do(":dmg wolf 3"+enter, `"wolf" matches Wolf 2, Wolf 1; be more specific`)
	a.do(esc, "n next")
	a.do(":heal nobody 3"+enter, `No combatant matches "nobody"`)
	a.do(esc, "n next")
	a.quit()
}

// Previous turn, pressed repeatedly, walks back step by step: each step puts
// back the countdowns and expiries of the advance it undoes (and nothing
// else), down to the start of combat. Where a rewind can't work it says so
// and changes nothing: before the first turn, and after the turn holder was
// removed (D17).
func TestRewindStepByStep(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir)
	a.do(":add Thorin 18 44 pc"+enter, "44/44")
	a.do(":add Goblin 12 7"+enter, "7/7")
	a.do(":add Orc 10 15"+enter, "15/15")
	a.do("S", "Round 1 ─ Turn: Thorin")
	a.keys(":cond goblin stunned 1" + enter)
	a.rowShows("Goblin", "Stunned (1)")
	a.keys(":fx Bless 1 from thorin on thorin" + enter)
	a.rowShows("Bless", "1 rd ")

	// Nothing to rewind yet.
	a.do("p", "No earlier turn to go back to", "Round 1 ─ Turn: Thorin")

	// Forward through the round: Stunned runs out on Goblin's turn, and
	// Bless as Thorin's second turn begins.
	a.do("n", "Turn: Bless")
	a.do("n", "Turn: Goblin")
	a.until(func(string) bool { return !strings.Contains(a.row("Goblin"), "Stunned") }, "Stunned to expire")
	a.do("n", "Turn: Orc")
	a.keys(":dmg orc 5" + enter)
	a.rowShows("Orc", "10/15")
	a.do("n", "Round 2 ─ Turn: Thorin")
	a.gone("Bless")
	forward := a.screen()

	// Back one: round 1 again, Bless restored on Thorin; the damage stays.
	a.do("p", "Round 1 ─ Turn: Orc")
	a.rowShows("Bless", "1 rd ", "1 targ")
	a.rowShows("Thorin", "◆ Bless")
	a.rowShows("Orc", "10/15")
	// Back two: Goblin's turn. Stunned expired when this turn began, so it
	// is still gone.
	a.do("p", "Round 1 ─ Turn: Goblin")
	if strings.Contains(a.row("Goblin"), "Stunned") {
		t.Fatalf("Stunned should only come back with the step that expired it:\n%s", a.screen())
	}
	// Back three: before Goblin's turn began, so Stunned is back.
	a.do("p", "Round 1 ─ Turn: Bless")
	a.rowShows("Goblin", "Stunned (1)")
	// Back four: the opening turn, and no further.
	a.do("p", "Round 1 ─ Turn: Thorin")
	a.do("p", "No earlier turn to go back to", "Round 1 ─ Turn: Thorin")

	// Replaying forward lands on the same screen as the first time.
	a.do("nn", "Turn: Goblin")
	a.do("nn", "Round 2 ─ Turn: Thorin")
	a.gone("Bless", "Stunned")
	if got := a.screen(); got != forward {
		t.Errorf("replayed screen differs:\n%s\nfirst time:\n%s", got, forward)
	}

	// Removing the turn holder passes the turn and clears the history: a
	// rewind is refused rather than pointing at a row that is gone.
	a.do("x", "Remove Thorin from combat?")
	a.do("y", "Round 2 ─ Turn: Goblin")
	a.gone("Thorin")
	a.do("p", "No earlier turn to go back to", "Round 2 ─ Turn: Goblin")
	// From here on, rewinding works again.
	a.do("n", "Round 2 ─ Turn: Orc")
	a.do("p", "Round 2 ─ Turn: Goblin")
	a.quit()

	enc := a.state("dmct-state.json")
	if enc.Round != 2 || enc.Current != combatantNamed(t, enc, "Goblin").ID || len(enc.History) != 0 || len(enc.Effects) != 0 {
		t.Errorf("saved: round %d, current %d, %d history, %d effects", enc.Round, enc.Current, len(enc.History), len(enc.Effects))
	}
	wantLines(t, logLines(t, oneFile(t, dir, "dmct-log-*.md")),
		"# Combat",
		"## Round 1",
		"[R1 · Thorin] Combat started: Thorin (18), Goblin (12), Orc (10)",
		"[R1 · Thorin] Stunned applied to Goblin (1 round)",
		"[R1 · Thorin] Effect started: Bless (Thorin, 1 round) on Thorin",
		"[R1 · Goblin] Stunned expired on Goblin",
		"## Round 2",
		"[R2 · Thorin] Effect expired: Bless",
		"[R1 · Orc] Turn rewound to Orc",
		"[R1 · Goblin] Turn rewound to Goblin",
		"[R1 · Bless] Turn rewound to Bless",
		"[R1 · Thorin] Turn rewound to Thorin",
		"[R1 · Goblin] Stunned expired on Goblin",
		"[R2 · Thorin] Effect expired: Bless",
		"[R2 · Thorin] Thorin removed from combat",
		"[R2 · Goblin] Turn rewound to Goblin",
	)
}

// The screen adapts: help on ?, fewer columns when narrow, and a plain
// message when the window is too small to show the table.
func TestHelpAndTerminalSizes(t *testing.T) {
	a := launch(t, t.TempDir())
	a.do(":add Bandit Captain 9 65 -- has the key"+enter, "has the key")
	a.do(":cond band restrained 2"+enter, "Restrained (2)")

	a.do("?", " Help", "n / space", "q          quit", "lines 1–22 of")
	a.do("jj", ":add NAME [xN] INIT HP [pc|npc] [-- NOTE]")
	a.do("G", ":next  :prev  :start  :endcombat")
	a.do("x", "Setup ─ press S to start combat") // any other key closes help without acting
	a.see("Bandit Captain")

	a.resize(64, 24) // the Note column goes first; the status line spells it out
	a.gone("│ Note")
	a.see("│ Kind", "Bandit Captain · Restrained (2) · Note: has the key")
	a.resize(56, 24) // then Kind
	a.gone("│ Kind")
	a.see("│ HP", "Bandit Captain · Restrained (2) · Note: has the key")
	a.resize(40, 8)
	a.see("Terminal too small", "Need 43×9, have 40×8.")
	a.keys("x") // ignored: nothing can be removed unseen
	a.resize(120, 30)
	a.see("Bandit Captain", "has the key", "│ Note")
	a.gone("Remove")
	a.quit()
}

// When the folder can't be written, the fight carries on behind a banner, q
// asks before quitting, and saving recovers by itself.
func TestSaveFailureAndRecovery(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to a read-only folder")
	}
	dir := t.TempDir()
	a := launch(t, dir)
	a.do(":add Thorin 18 44 pc"+enter, "44/44")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	a.keys(":dmg thorin 4" + enter)
	a.see("✖ Autosave failing: changes aren't on disk", "40/44")
	if c := combatantNamed(t, a.state("dmct-state.json"), "Thorin"); c.HP != 44 {
		t.Fatalf("the state file should still hold the last good save, has HP %d", c.HP)
	}
	a.do("q", "Autosave is failing: quit anyway?")
	a.do("n", "a add · S start")

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a.keys(":dmg thorin 1" + enter)
	a.see("Autosave is working again", "39/44")
	a.gone("✖")
	a.quit()
	if c := combatantNamed(t, a.state("dmct-state.json"), "Thorin"); c.HP != 39 {
		t.Errorf("saved HP %d, want 39", c.HP)
	}
}

// A state file dmct can't read stops it at the door, with the file untouched
// and a way forward; so do bad arguments (no terminal needed for these).
func TestRefusalsAtStartup(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "dmct-state.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args     []string
		code     int
		wantText string
	}{
		{nil, 1, "The state file was left as it is. Fix or move it, or run with --state OTHER.json"},
		{[]string{"extra"}, 1, `dmct: unexpected argument "extra"`},
		{[]string{"--bogus"}, 1, "usage: dmct [--state FILE] [--log FILE]"},
		{[]string{"-h"}, 0, "usage: dmct [--state FILE] [--log FILE]"},
	} {
		cmd := exec.Command(bin, tc.args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		code := 0
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		}
		if code != tc.code || !strings.Contains(string(out), tc.wantText) {
			t.Errorf("dmct %v: exit %d, output:\n%s", tc.args, code, out)
		}
	}
	if got := read(t, filepath.Join(dir, "dmct-state.json")); got != "{" {
		t.Errorf("the corrupt state file was changed to %q", got)
	}
}

// The ruleset is chosen with --ruleset or :ruleset before combat starts, is
// shown in the title, survives a restart, and can't change mid-fight (RS1).
func TestRuleset(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir, "--ruleset", "3.5")
	a.see("Setup", "3.5e")
	a.do(":ruleset 5e"+enter, "5e")
	a.gone("3.5e")
	a.do(":ruleset 3.5e"+enter, "3.5e")
	a.do(":add Orc 12 20"+enter, "20/20")
	a.keys(":cond orc SHAKEN 2" + enter)
	a.rowShows("Orc", "Shaken (2)")
	a.do(":start"+enter, "Round 1 ─ Turn: Orc", "3.5e")
	a.do(":ruleset 5e"+enter, "the ruleset can't change mid-fight")
	a.keys(esc)
	a.gone(": ruleset 5e") // wait, so the q isn't read as part of an escape sequence
	a.quit()

	if got := a.state("dmct-state.json").Ruleset; got != "3.5e" {
		t.Errorf("saved ruleset %q", got)
	}

	cmd := exec.Command(bin, "--ruleset", "5e")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "holds a 3.5e fight in progress") || !strings.Contains(string(out), "Run without --ruleset") {
		t.Errorf("--ruleset 5e on a 3.5e fight: %v\n%s", err, out)
	}

	b := launch(t, dir)
	b.see("Resumed from dmct-state.json", "Round 1 ─ Turn: Orc", "3.5e")
	b.quit()
}

// A 3.5e fight: HP runs below zero to -10, healing counts up from the real
// value, and disabled, dying and dead each show in the table, the status
// line and the log (RS2, RS3).
func TestNegativeHP35(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir, "--ruleset", "3.5e", "--log", "recap.md")
	a.do(":add Tordek 18 44 pc"+enter, "44/44")
	a.do(":add Orc 12 22"+enter, "22/22")
	a.do(":add Wolf 5 -3/13"+enter, "DYING -3/13") // joins already dying
	a.do(":start"+enter, "Round 1 ─ Turn: Tordek")

	a.keys(":dmg orc 22" + enter)
	a.rowShows("Orc", "DSBLD 0/22")
	a.keys(":dmg orc 7" + enter)
	a.rowShows("Orc", "DYING -7/22")
	a.do("n", "Turn: Orc", "Orc is dying (-7 HP)")
	a.keys(":heal orc 5" + enter)
	a.rowShows("Orc", "DYING -2/22") // not 5
	a.keys(":dmg orc 100" + enter)
	a.rowShows("Orc", "DEAD -10/22")
	a.keys(":hp orc -1" + enter)
	a.rowShows("Orc", "DYING -1/22")
	a.keys(":heal orc 4" + enter)
	a.rowShows("Orc", "3/22 ▼")
	a.quit()

	if c := combatantNamed(t, a.state("dmct-state.json"), "Wolf"); c.HP != -3 {
		t.Errorf("Wolf saved at %d HP", c.HP)
	}
	b := launch(t, dir)
	b.see("Resumed from dmct-state.json", "DYING -3/13")
	b.quit()

	wantLines(t, logLines(t, filepath.Join(dir, "recap.md")),
		"# Combat",
		"## Round 1",
		"[R1 · Tordek] Combat started: Tordek (18), Orc (12), Wolf (5)",
		"[R1 · Tordek] Orc is disabled (0 HP)",
		"[R1 · Tordek] Orc is dying (-7 HP)",
		"[R1 · Orc] Orc is dead (-10 HP)",
		"[R1 · Orc] Orc is dying (-1 HP)",
		"[R1 · Orc] Orc is back above 0 HP",
	)
}

// A ruleset from a file is chosen, shown and used like a built-in, and a
// fight in it resumes only while its file is still there (RS9, D35, D38).
func TestRulesetFile(t *testing.T) {
	dir := t.TempDir()
	rules := filepath.Join(dir, "rules")
	if err := os.Mkdir(rules, 0o755); err != nil {
		t.Fatal(err)
	}
	pf := `{"name":"pf1","hpFloor":-12,"states":[
		{"name":"disabled","tag":"dsbld","min":0,"max":0,"acts":true},
		{"name":"dying","min":-11,"max":-1,"remind":true},
		{"name":"dead","max":-12}],
		"conditions":["Shaken","Sickened","Staggered"]}`
	if err := os.WriteFile(filepath.Join(rules, "pf1.json"), []byte(pf), 0o644); err != nil {
		t.Fatal(err)
	}

	a := launch(t, dir, "--rulesets", rules, "--ruleset", "pf1", "--log", "recap.md")
	a.see("Setup", "pf1")
	a.do(":add Orc 12 10"+enter, "10/10")
	a.keys(":cond orc sickened" + enter)
	a.rowShows("Orc", "Sickened")
	a.do(":start"+enter, "Round 1 ─ Turn: Orc", "pf1")
	a.keys(":dmg orc 15" + enter)
	a.rowShows("Orc", "DYING -5/10")
	a.keys(":dmg orc 20" + enter)
	a.rowShows("Orc", "DEAD -12/10") // the file's floor
	a.quit()

	wantLines(t, logLines(t, filepath.Join(dir, "recap.md")),
		"# Combat",
		"## Round 1",
		"[R1 · Orc] Combat started: Orc (12)",
		"[R1 · Orc] Orc is dying (-5 HP)",
		"[R1 · Orc] Orc is dead (-12 HP)",
	)

	// Without the folder the fight can't be read, and the state file is
	// left alone.
	before := read(t, filepath.Join(dir, "dmct-state.json"))
	cmd := exec.Command(bin)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), `unknown ruleset "pf1"`) || !strings.Contains(string(out), "Ruleset files are read from") {
		t.Errorf("resume without the folder: %v\n%s", err, out)
	}
	if read(t, filepath.Join(dir, "dmct-state.json")) != before {
		t.Error("the state file was changed")
	}

	b := launch(t, dir, "--rulesets", rules)
	b.see("Resumed from dmct-state.json", "DEAD -12/10", "pf1")
	b.quit()

	// A bad file is refused at start-up with the reason.
	if err := os.WriteFile(filepath.Join(rules, "pf1.json"), []byte(`{"name":"pf1","hpFloor":4}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, "--rulesets", rules)
	cmd.Dir = dir
	out, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), `pf1.json: "hpFloor" is 4; it can't be above 0`) {
		t.Errorf("bad file: %v\n%s", err, out)
	}
}

// Nonlethal damage, max HP edits and stacking temp HP, by key and by
// command, in a 3.5e fight (RS4–RS6).
func TestHPExtras(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir, "--ruleset", "3.5e", "--log", "recap.md")
	a.do(":add Tordek 18 30 pc"+enter, "30/30")
	a.do(":add Orc 12 10"+enter, "10/10")
	a.do(":start"+enter, "Round 1 ─ Turn: Tordek")

	// The HP cell is too narrow for these; the status line spells out the
	// selected row's (the command selects its target).
	a.do(":nl orc 10"+enter, "Orc · HP STAGR 10/10 nl10")
	a.rowShows("Orc", "STAGR 10/1…")
	a.do("D1"+enter, "Orc · HP UNCON 10/10 nl11")
	a.keys(":heal orc 4" + enter)
	a.rowShows("Orc", "10/10 nl7")
	a.do("M\x7f\x7f6"+enter, "Orc · HP UNCON 6/6 nl7") // backspace twice, then 6

	a.keys(":temp tordek 5" + enter)
	a.keys(":temp tordek +3" + enter)
	a.rowShows("Tordek", "30/30 +8")
	a.keys(":maxhp tordek 24" + enter)
	a.rowShows("Tordek", "24/24 +8")
	a.quit()

	if c := combatantNamed(t, a.state("dmct-state.json"), "Orc"); c.Nonlethal != 7 || c.MaxHP != 6 {
		t.Errorf("Orc saved with %d nonlethal, max %d", c.Nonlethal, c.MaxHP)
	}
	wantLines(t, logLines(t, filepath.Join(dir, "recap.md")),
		"# Combat",
		"## Round 1",
		"[R1 · Tordek] Combat started: Tordek (18), Orc (12)",
		"[R1 · Tordek] Orc is staggered (10 HP, 10 nonlethal)",
		"[R1 · Tordek] Orc is unconscious (10 HP, 11 nonlethal)",
		"[R1 · Tordek] Orc is no longer unconscious",
		"[R1 · Tordek] Orc max HP 10 → 6",
		"[R1 · Tordek] Orc is unconscious (6 HP, 7 nonlethal)",
		"[R1 · Tordek] Tordek max HP 30 → 24",
	)
}

// In a 3.5e fight a sourced effect stays on the count it began on: its row
// sorts just before its caster, Delay leaves it there, and it ends just
// before that count comes up again. A corrected initiative moves it along
// (RS7, D32, D33).
func TestFixedCountEffects(t *testing.T) {
	dir := t.TempDir()
	a := launch(t, dir, "--ruleset", "3.5e", "--log", "recap.md")
	a.do(":add Mira 15 27 pc"+enter, "27/27")
	a.do(":add Orc 10 15"+enter, "15/15")
	a.do(":start"+enter, "Round 1 ─ Turn: Mira")
	a.do(":fx Bless 1 from mira"+enter, "◆ Bless")
	if got := a.names(); got != "◆ Bless, Mira, Orc" {
		t.Fatalf("the effect row should sit just before its caster: %s", got)
	}

	// Mira delays to 5 on her turn: her effect stays on 15, the turn passes
	// to the orc, and she still acts this round (RS10).
	a.do(":init mira 5"+enter, "Mira is the source of Bless: move them too?")
	a.do("n", "Mira steps aside · turn passes to Orc", "Round 1 ─ Turn: Orc")
	if got := a.names(); got != "◆ Bless, Orc, Mira" {
		t.Fatalf("after the delay: %s", got)
	}
	a.do("n", "Round 1 ─ Turn: Mira")
	a.rowShows("Bless", "1 rd ")
	// Round 2: it expires on count 15, and the turn moves straight on.
	a.do("n", "Round 2 ─ Turn: Orc")
	a.gone("Bless")

	// A mistyped initiative moves the caster's effects with her.
	a.do(":fx Fog 3 from mira"+enter, "◆ Fog")
	a.do(":init mira 8"+enter, "move them too?")
	a.do("y", "n next")
	if got := a.names(); got != "Orc, ◆ Fog, Mira" {
		t.Fatalf("after the correction: %s", got)
	}
	a.quit()

	enc := a.state("dmct-state.json")
	if len(enc.Effects) != 1 || enc.Effects[0].Initiative.String() != "8" {
		t.Errorf("saved effects: %+v", enc.Effects)
	}
	lines := strings.Join(logLines(t, filepath.Join(dir, "recap.md")), "\n")
	if !strings.Contains(lines, "Effect expired: Bless") {
		t.Errorf("log is missing the expiry:\n%s", lines)
	}
}

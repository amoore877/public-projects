package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newClock() *clock {
	return &clock{now: time.Date(2026, 11, 14, 19, 42, 0, 0, time.UTC)}
}

// setup runs each test in its own directory so default paths land there.
func setup(t *testing.T) (*clock, Config) {
	t.Helper()
	t.Chdir(t.TempDir())
	c := newClock()
	return c, Config{Clock: c.Now}
}

func open(t *testing.T, cfg Config) (*Session, bool) {
	t.Helper()
	s, resumed, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, resumed
}

func apply(t *testing.T, s *Session, op Op) Outcome {
	t.Helper()
	out, err := s.Apply(op)
	if err != nil {
		t.Fatal(err)
	}
	if out.SaveErr != nil || out.LogErr != nil {
		t.Fatalf("save err %v, log err %v", out.SaveErr, out.LogErr)
	}
	return out
}

func addOp(name string, init, hp int) Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		_, evs, err := e.Add(combat.CombatantSpec{Name: name, Initiative: combat.InitiativeFromInt(init), MaxHP: hp})
		return evs, err
	}
}

func start(e *combat.Encounter) ([]combat.Event, error) { return e.Start() }
func next(e *combat.Encounter) ([]combat.Event, error)  { return e.Next() }
func end(e *combat.Encounter) ([]combat.Event, error)   { return e.End() }
func note(text string) Op {
	return func(e *combat.Encounter) ([]combat.Event, error) { return e.LogNote(text) }
}
func damage(name string, n int) Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		for _, c := range e.Combatants {
			if c.Name == name {
				return e.Damage(c.ID, n)
			}
		}
		return nil, combat.ErrNotFound
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestResumeAfterRestart(t *testing.T) {
	clk, cfg := setup(t)
	s, resumed := open(t, cfg)
	if resumed {
		t.Fatal("fresh directory reported as resumed")
	}
	apply(t, s, addOp("Thorin", 18, 44))
	apply(t, s, addOp("Ogre", 8, 59))
	apply(t, s, start)
	apply(t, s, damage("Ogre", 12))
	apply(t, s, next)
	s.Close()

	// Restart the next day: same encounter, same log file.
	clk.now = clk.now.Add(24 * time.Hour)
	s2, resumed := open(t, cfg)
	if !resumed {
		t.Fatal("expected to resume")
	}
	e := s2.Encounter()
	if e.Status != combat.Active || e.Round != 1 || e.Combatants[1].HP != 47 {
		t.Fatalf("resumed state wrong: status %s round %d ogre HP %d", e.Status, e.Round, e.Combatants[1].HP)
	}
	if r, _ := e.Row(e.Current); r.Name() != "Ogre" {
		t.Fatalf("turn = %s, want Ogre", r.Name())
	}
	apply(t, s2, next) // round 2
	apply(t, s2, damage("Ogre", 47))

	if _, err := os.Stat("dmct-log-2026-11-15.md"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("resumed fight started a new dated log")
	}
	log := readFile(t, "dmct-log-2026-11-14.md")
	want := "# Combat — 2026-11-14 19:42\n\n" +
		"## Round 1\n\n" +
		"- `19:42` **[R1 · Thorin]** Combat started: Thorin (18), Ogre (8)\n\n" +
		"## Round 2\n\n" +
		"- `19:42` **[R2 · Thorin]** Ogre dropped to 0 HP\n"
	if log != want {
		t.Fatalf("log =\n%s\nwant\n%s", log, want)
	}
}

func TestEndArchivesAndStartsFresh(t *testing.T) {
	clk, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, addOp("Thorin", 18, 44))
	apply(t, s, start)
	clk.now = clk.now.Add(10 * time.Minute)
	out := apply(t, s, end)

	if out.ArchivedTo != "dmct-state-20261114-195200.json" {
		t.Fatalf("archived to %q", out.ArchivedTo)
	}
	archived := readFile(t, out.ArchivedTo)
	if !strings.Contains(archived, `"status": "ended"`) {
		t.Fatalf("archive doesn't hold the final state:\n%s", archived)
	}
	if _, err := os.Stat(DefaultStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("state file still present after ending combat")
	}
	if e := s.Encounter(); e.Status != combat.Setup || len(e.Combatants) != 0 || e.LogPath != "" {
		t.Fatalf("next encounter isn't fresh: %+v", e)
	}

	// The next fight the same day appends a new section to the same log.
	apply(t, s, addOp("Wolf", 13, 11))
	apply(t, s, start)
	log := readFile(t, "dmct-log-2026-11-14.md")
	if n := strings.Count(log, "# Combat — "); n != 2 {
		t.Fatalf("log has %d combat sections, want 2:\n%s", n, log)
	}
	if !strings.Contains(log, "Combat ended after 1 round\n\n# Combat — 2026-11-14 19:52\n\n## Round 1\n") {
		t.Fatalf("second combat not appended as a new section:\n%s", log)
	}
}

func TestRejectedOpChangesNothing(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	if _, err := s.Apply(start); !errors.Is(err, combat.ErrEmpty) {
		t.Fatalf("err = %v, want ErrEmpty", err)
	}
	if _, err := os.Stat(DefaultStatePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a rejected operation wrote the state file")
	}
}

func TestSetupNoteThenRestartWritesOneHeader(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, note("Ambush at the bridge"))
	apply(t, s, addOp("Thorin", 18, 44))
	s.Close()

	s2, resumed := open(t, cfg)
	if !resumed {
		t.Fatal("expected to resume a Setup encounter")
	}
	apply(t, s2, start)
	log := readFile(t, "dmct-log-2026-11-14.md")
	if n := strings.Count(log, "# Combat — "); n != 1 {
		t.Fatalf("log has %d headers, want 1:\n%s", n, log)
	}
}

func TestOpenArchivesLeftoverEndedState(t *testing.T) {
	_, cfg := setup(t)
	// Simulate a crash between saving the ended state and archiving it.
	if err := os.WriteFile(DefaultStatePath, []byte(`{"schemaVersion":1,"status":"ended","round":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, resumed := open(t, cfg)
	if resumed || s.Encounter().Status != combat.Setup {
		t.Fatal("an ended encounter was resumed")
	}
	if _, err := os.Stat("dmct-state-20261114-194200.json"); err != nil {
		t.Fatalf("ended state not archived: %v", err)
	}
}

func TestLogFailureDoesNotBlockState(t *testing.T) {
	_, cfg := setup(t)
	cfg.LogPath = filepath.Join("no-such-dir", "log.md")
	s, _ := open(t, cfg)
	apply(t, s, addOp("Thorin", 18, 44))
	out, err := s.Apply(start)
	if err != nil {
		t.Fatal(err)
	}
	if out.LogErr == nil || out.SaveErr != nil {
		t.Fatalf("log err %v, save err %v; want only a log error", out.LogErr, out.SaveErr)
	}
	if !strings.Contains(readFile(t, DefaultStatePath), `"status": "active"`) {
		t.Fatal("state wasn't saved after a log failure")
	}
	if s.Encounter().LogPath != cfg.LogPath {
		t.Fatalf("LogPath = %q, want the --log path", s.Encounter().LogPath)
	}
	if out.Logged {
		t.Error("Logged should be false when the write failed")
	}

	// Once the directory exists, a quiet op opens the file without claiming
	// entries were logged, and the next logged change writes the missing
	// combat and round headings along with its entry.
	if err := os.Mkdir("no-such-dir", 0o755); err != nil {
		t.Fatal(err)
	}
	quietOut, err := s.Apply(damage("Thorin", 4))
	if err != nil || quietOut.LogErr != nil || quietOut.Logged {
		t.Fatalf("quiet op after recovery: err %v, log err %v, logged %v (want false)", err, quietOut.LogErr, quietOut.Logged)
	}
	out, err = s.Apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.LogNote("back") })
	if err != nil || out.LogErr != nil || !out.Logged {
		t.Fatalf("after recovery: err %v, log err %v, logged %v", err, out.LogErr, out.Logged)
	}
	recovered := readFile(t, cfg.LogPath)
	for _, want := range []string{"# Combat — ", "## Round 1", "back"} {
		if !strings.Contains(recovered, want) {
			t.Errorf("log after recovery missing %q:\n%s", want, recovered)
		}
	}
}

func TestOpenRejectsCorruptState(t *testing.T) {
	_, cfg := setup(t)
	if err := os.WriteFile(DefaultStatePath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Open(cfg); err == nil {
		t.Fatal("Open accepted a corrupt state file")
	}
	// The corrupt file is left alone for the DM to inspect.
	if readFile(t, DefaultStatePath) != "{" {
		t.Fatal("corrupt state file was modified")
	}
}

func TestRulesetConfig(t *testing.T) {
	_, cfg := setup(t)
	cfg.Ruleset = "3.5"
	s, _ := open(t, cfg)
	if got := s.Encounter().Ruleset; got != "3.5e" {
		t.Fatalf("new fight: ruleset %q", got)
	}
	apply(t, s, addOp("Orc", 12, 20))
	s.Close()

	// Still in Setup, so a restart may change it, or leave it alone.
	cfg.Ruleset = ""
	s, resumed := open(t, cfg)
	if !resumed || s.Encounter().Ruleset != "3.5e" {
		t.Fatalf("no flag should keep the saved ruleset, got %q", s.Encounter().Ruleset)
	}
	s.Close()
	cfg.Ruleset = "5e"
	s, _ = open(t, cfg)
	if s.Encounter().Ruleset != "5e" {
		t.Fatalf("a fight in Setup should take the flag, got %q", s.Encounter().Ruleset)
	}
	apply(t, s, start)
	s.Close()

	// In progress: a different ruleset is refused, the same one is fine.
	cfg.Ruleset = "3.5e"
	if _, _, err := Open(cfg); !errors.Is(err, ErrRulesetInUse) || !strings.Contains(err.Error(), "holds a 5e fight in progress") {
		t.Fatalf("mid-fight change: %v", err)
	}
	cfg.Ruleset = "5e"
	s, _ = open(t, cfg)
	s.Close()

	cfg.Ruleset = "4e"
	if _, _, err := Open(cfg); err == nil || !strings.Contains(err.Error(), `unknown ruleset "4e"`) {
		t.Fatalf("unknown ruleset: %v", err)
	}
}

// The fight after an ended one keeps its ruleset, whether combat ended in
// this run or the ended state file was found at start-up.
func TestRulesetCarriesToNextFight(t *testing.T) {
	_, cfg := setup(t)
	cfg.Ruleset = "3.5e"
	s, _ := open(t, cfg)
	apply(t, s, addOp("Orc", 12, 20))
	apply(t, s, start)
	out := apply(t, s, end)
	if !strings.Contains(readFile(t, out.ArchivedTo), `"ruleset": "3.5e"`) {
		t.Error("the archive should record the ruleset")
	}
	if e := s.Encounter(); e.Status != combat.Setup || e.Ruleset != "3.5e" {
		t.Fatalf("next fight: %s, ruleset %q", e.Status, e.Ruleset)
	}
	s.Close()

	if err := os.WriteFile(DefaultStatePath, []byte(`{"schemaVersion":2,"status":"ended","ruleset":"3.5e"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Ruleset = ""
	s, resumed := open(t, cfg)
	if resumed || s.Encounter().Ruleset != "3.5e" {
		t.Fatalf("after a leftover ended file: resumed %v, ruleset %q", resumed, s.Encounter().Ruleset)
	}
}

func TestCarryOverPCsPersistsAndClearsUndo(t *testing.T) {
	_, cfg := setup(t)
	s, _ := open(t, cfg)
	apply(t, s, func(e *combat.Encounter) ([]combat.Event, error) {
		v, _ := combat.ParseInitiative("18")
		_, evs, err := e.Add(combat.CombatantSpec{Name: "Thorin", Kind: combat.PC, Initiative: v, MaxHP: 44})
		return evs, err
	})
	apply(t, s, addOp("Goblin", 12, 7))
	apply(t, s, start)
	apply(t, s, func(e *combat.Encounter) ([]combat.Event, error) {
		return e.Damage(e.Combatants[0].ID, 13)
	})
	out := apply(t, s, func(e *combat.Encounter) ([]combat.Event, error) { return e.EndWith(true) })
	if out.ArchivedTo == "" || out.SaveErr != nil {
		t.Fatalf("end with carry-over: archive=%q saveErr=%v", out.ArchivedTo, out.SaveErr)
	}
	if u, r := s.UndoDepth(); u != 0 || r != 0 {
		t.Errorf("undo stack should reset at End: %d/%d", u, r)
	}
	if e := s.Encounter(); e.Status != combat.Setup || len(e.Combatants) != 1 || e.Combatants[0].Name != "Thorin" || e.Combatants[0].HP != 31 || e.Combatants[0].Initiative != 0 {
		t.Fatalf("carried-over encounter: %+v", e)
	}
	s.Close()

	// Reopening immediately should resume the carried-over Setup (D58).
	s2, resumed := open(t, cfg)
	if !resumed || s2.Encounter().Status != combat.Setup || len(s2.Encounter().Combatants) != 1 || s2.Encounter().Combatants[0].HP != 31 {
		t.Fatalf("resumed=%v encounter=%+v", resumed, s2.Encounter())
	}
	s2.Close()
}

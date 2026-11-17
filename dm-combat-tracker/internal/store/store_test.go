package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

var t0 = time.Date(2026, 11, 14, 19, 42, 5, 0, time.UTC)

func clock() time.Time { return t0 }

// busyEncounter builds an active fight with conditions, an effect, temp HP and
// turn history, so a round trip exercises every serialized field.
func busyEncounter(t *testing.T) *combat.Encounter {
	t.Helper()
	e := combat.New(clock)
	ck := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	thorin, _, err := e.Add(combat.CombatantSpec{Name: "Thorin", Kind: combat.PC, Initiative: combat.InitiativeFromInt(18), MaxHP: 44, TempHP: 5, Note: "dwarf"})
	ck(err)
	init, _ := combat.ParseInitiative("12")
	gobs, _, err := e.AddGroup(combat.GroupSpec{CombatantSpec: combat.CombatantSpec{Name: "Goblin", Initiative: init, MaxHP: 7}, Count: 3})
	ck(err)
	r := 3
	_, _, err = e.AddCondition(thorin, "Blessed", &r, "from cleric")
	ck(err)
	_, _, err = e.AddCondition(gobs[1], "Prone", nil, "")
	ck(err)
	fxr := 10
	_, _, err = e.AddEffect(combat.EffectSpec{Name: "Spirit Guardians", SourceID: thorin, Rounds: &fxr, Targets: gobs[:2], Note: "3d8"})
	ck(err)
	_, err = e.Start()
	ck(err)
	for range 6 { // wraps into round 2 and ticks Blessed
		_, err = e.Next()
		ck(err)
	}
	_, err = e.Damage(gobs[0], 9)
	ck(err)
	e.LogPath = "dmct-log-2026-11-14.md"
	return e
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dmct-state.json")
	want := busyEncounter(t)
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("Load returned nil for an existing file")
	}
	// Compare the encoded form: the clock func isn't comparable.
	gj, _ := json.MarshalIndent(got, "", "  ")
	wj, _ := json.MarshalIndent(want, "", "  ")
	if string(gj) != string(wj) {
		t.Fatalf("round trip mismatch\n got: %s\nwant: %s", gj, wj)
	}
	if got.Round != 2 || len(got.Combatants) != 4 || len(got.Effects) != 1 {
		t.Fatalf("unexpected loaded state: round %d, %d combatants, %d effects", got.Round, len(got.Combatants), len(got.Effects))
	}
	if len(got.History) == 0 {
		t.Fatal("expected turn history to survive the round trip")
	}

	// The loaded encounter keeps working, including rewinding across the save.
	got.SetClock(clock)
	if _, err := got.Previous(); err != nil {
		t.Fatalf("Previous after load: %v", err)
	}
	if _, err := got.Next(); err != nil {
		t.Fatalf("Next after load: %v", err)
	}
}

func TestSaveLeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dmct-state.json")
	for range 3 {
		if err := Save(path, busyEncounter(t)); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "dmct-state.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("dir = %v, want only dmct-state.json", names)
	}
}

func TestInterruptedSaveKeepsPreviousFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dmct-state.json")
	first := combat.New(clock)
	if _, _, err := first.Add(combat.CombatantSpec{Name: "Mira", MaxHP: 27}); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)

	rename = func(string, string) error { return errors.New("power cut") }
	t.Cleanup(func() { rename = os.Rename })

	if err := Save(path, busyEncounter(t)); err == nil || !strings.Contains(err.Error(), "power cut") {
		t.Fatalf("Save err = %v, want the rename failure", err)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Fatal("previous state file was changed by a failed save")
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temp file left behind after a failed save (stat err %v)", err)
	}
}

func TestSaveOverwritesStaleTempFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dmct-state.json")
	// Left over from a crash mid-write; longer than the real content.
	if err := os.WriteFile(path+".tmp", []byte(strings.Repeat("x", 100000)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, combat.New(clock)); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, clock); err != nil {
		t.Fatalf("load after save over stale tmp: %v", err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	enc, err := Load(filepath.Join(t.TempDir(), "nope.json"), clock)
	if err != nil || enc != nil {
		t.Fatalf("Load(missing) = %v, %v; want nil, nil", enc, err)
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	cases := map[string]string{
		"not json":       "{",
		"no version":     `{"status":"setup"}`,
		"future version": `{"schemaVersion":99,"status":"setup"}`,
		"bad status":     `{"schemaVersion":1,"status":"paused"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "s.json")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			if enc, err := Load(path, clock); err == nil {
				t.Fatalf("Load accepted %q: %+v", body, enc)
			}
		})
	}
}

func TestArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dmct-state.json")
	var got []string
	for range 3 {
		if err := Save(path, combat.New(clock)); err != nil {
			t.Fatal(err)
		}
		dst, err := Archive(path, t0)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, filepath.Base(dst))
	}
	want := []string{
		"dmct-state-20261114-194205.json",
		"dmct-state-20261114-194205-2.json",
		"dmct-state-20261114-194205-3.json",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("archives = %v, want %v", got, want)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("state file still present after archiving")
	}
}

// A file saved before rulesets existed loads as 5e and is upgraded to the
// current schema; the chosen ruleset survives a save (RS1).
func TestLoadRuleset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dmct-state.json")
	if err := os.WriteFile(path, []byte(`{"schemaVersion":1,"status":"setup"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	enc, err := Load(path, clock)
	if err != nil {
		t.Fatal(err)
	}
	if enc.Ruleset != "5e" || enc.SchemaVersion != combat.SchemaVersion {
		t.Fatalf("ruleset %q, schema %d", enc.Ruleset, enc.SchemaVersion)
	}
	if _, err := enc.SetRuleset("3.5e"); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, enc); err != nil {
		t.Fatal(err)
	}
	if enc, err = Load(path, clock); err != nil || enc.Ruleset != "3.5e" {
		t.Fatalf("after save: %+v, %v", enc, err)
	}

	if err := os.WriteFile(path, []byte(`{"schemaVersion":2,"status":"setup","ruleset":"4e"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, clock); err == nil || !strings.Contains(err.Error(), `unknown ruleset "4e"`) {
		t.Fatalf("unknown ruleset: %v", err)
	}
}

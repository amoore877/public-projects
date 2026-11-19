package store

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoadRulesets(t *testing.T) {
	t.Cleanup(func() { _ = combat.UseRulesets(nil) })
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"b-pf.json":   `{"name":"pf1","hpFloor":-10}`,
		"a-home.JSON": `{"name":"home5"}`,
		"notes.txt":   "not a ruleset",
	})
	if err := os.Mkdir(filepath.Join(dir, "old.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	names, err := LoadRulesets(dir)
	if err != nil || !slices.Equal(names, []string{"home5", "pf1"}) {
		t.Fatalf("names %v, err %v", names, err)
	}
	if r, ok := combat.LookupRuleset("pf1"); !ok || r.HPFloor != -10 {
		t.Errorf("pf1 not available: %+v", r)
	}

	// A state file naming a loaded ruleset now loads.
	path := filepath.Join(t.TempDir(), "state.json")
	writeFiles(t, filepath.Dir(path), map[string]string{"state.json": `{"schemaVersion":2,"status":"setup","ruleset":"PF1"}`})
	if enc, err := Load(path, nil); err != nil || enc.Ruleset != "pf1" {
		t.Errorf("load: %+v, %v", enc, err)
	}
}

func TestLoadRulesetsRefusals(t *testing.T) {
	t.Cleanup(func() { _ = combat.UseRulesets(nil) })
	for _, tc := range []struct {
		files map[string]string
		want  string
	}{
		{map[string]string{"bad.json": `{"name":"x","hpFloor":3}`}, "bad.json: \"hpFloor\" is 3"},
		{map[string]string{"five.json": `{"name":"5E"}`}, `five.json: ruleset "5E" has the same name as the built-in 5e`},
		{map[string]string{"a.json": `{"name":"pf"}`, "b.json": `{"name":"pf"}`}, `b.json: ruleset "pf" has the same name as ruleset pf`},
	} {
		dir := t.TempDir()
		writeFiles(t, dir, tc.files)
		_, err := LoadRulesets(dir)
		if err == nil || !strings.Contains(err.Error(), filepath.Join(dir, "")) || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%v: err %v", tc.files, err)
		}
		if len(combat.RulesetNames()) != 2 {
			t.Errorf("%v: a refused folder left rulesets loaded: %v", tc.files, combat.RulesetNames())
		}
	}

	if _, err := LoadRulesets(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing folder: %v", err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"locked.json": `{"name":"x"}`})
	if err := os.Chmod(filepath.Join(dir, "locked.json"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRulesets(dir); err == nil || !strings.Contains(err.Error(), "locked.json") {
		if os.Getuid() != 0 { // root can read it anyway
			t.Errorf("unreadable file: %v", err)
		}
	}
}

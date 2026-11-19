package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// run's failures all happen before the terminal is touched, so they can be
// tested in-process. The program itself is driven end to end, through a
// pseudo-terminal, in e2e_unix_test.go.

func TestRunRejectsBadArguments(t *testing.T) {
	t.Chdir(t.TempDir())
	stderr := captureStderr(t)
	if err := run([]string{"--nope"}); err == nil || !strings.Contains(err.Error(), "not defined") {
		t.Errorf("unknown flag: %v", err)
	}
	if err := run([]string{"goblins.json"}); err == nil || !strings.Contains(err.Error(), `unexpected argument "goblins.json"`) {
		t.Errorf("stray argument: %v", err)
	}
	if err := run([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
	if err := run([]string{"--ruleset", "4e"}); err == nil || !strings.Contains(err.Error(), `unknown ruleset "4e" (choose 5e or 3.5e)`) {
		t.Errorf("unknown ruleset: %v", err)
	}
	if out := stderr(); strings.Count(out, "usage: dmct [--state FILE] [--log FILE] [--ruleset NAME]") != 3 {
		t.Errorf("each should print the usage line:\n%s", out)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("nothing should be created: %v", entries)
	}
}

func TestRunRefusesUnreadableState(t *testing.T) {
	for name, content := range map[string]string{
		"corrupt": "{",
		"newer":   `{"schemaVersion":99,"status":"active"}`,
		"unknown": `{"schemaVersion":1,"status":"paused"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fight.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			err := run([]string{"--state", path})
			if err == nil || !strings.Contains(err.Error(), "The state file was left as it is") {
				t.Fatalf("err = %v", err)
			}
			if got, _ := os.ReadFile(path); string(got) != content {
				t.Errorf("state file was changed to %q", got)
			}
		})
	}
}

// captureStderr redirects os.Stderr (where flag prints usage) until the
// returned func is called, which gives back what was written.
func captureStderr(t *testing.T) func() string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stderr")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = f
	restore := func() { os.Stderr = old; f.Close() }
	t.Cleanup(restore)
	return func() string {
		restore()
		b, _ := os.ReadFile(path)
		return string(b)
	}
}

// --ruleset can't change a fight that is already under way; the state file
// is fine, so the message doesn't suggest fixing it.
func TestRunRefusesRulesetChangeMidFight(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fight.json")
	content := `{"schemaVersion":2,"status":"active","ruleset":"5e","round":1}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run([]string{"--state", path, "--ruleset", "3.5e"})
	if err == nil || !strings.Contains(err.Error(), "holds a 5e fight in progress") ||
		!strings.Contains(err.Error(), "Run without --ruleset") || strings.Contains(err.Error(), "Fix or move it") {
		t.Fatalf("err = %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != content {
		t.Errorf("state file was changed to %q", got)
	}
}

func writeRuleset(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Ruleset files come from --rulesets, or else the config folder, which may
// be missing (D38).
func TestRunRulesetFiles(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Cleanup(func() { _ = combat.UseRulesets(nil) })
	config := t.TempDir()
	old := userConfigDir
	t.Cleanup(func() { userConfigDir = old })
	userConfigDir = func() (string, error) { return config, nil }

	// No folder at all is fine; the choices are the built-ins.
	if err := run([]string{"--ruleset", "pf1"}); err == nil || !strings.Contains(err.Error(), `unknown ruleset "pf1" (choose 5e or 3.5e)`) {
		t.Errorf("err = %v", err)
	}

	// The default folder is read.
	writeRuleset(t, filepath.Join(config, "dmct", "rulesets"), "pf1.json", `{"name":"pf1"}`)
	if err := run([]string{"--ruleset", "4e"}); err == nil || !strings.Contains(err.Error(), "(choose 5e or 3.5e or pf1)") {
		t.Errorf("err = %v", err)
	}

	// A folder named with --rulesets must exist.
	missing := filepath.Join(t.TempDir(), "nope")
	if err := run([]string{"--rulesets", missing}); err == nil || !strings.Contains(err.Error(), "read rulesets") ||
		!strings.Contains(err.Error(), "use --rulesets DIR") {
		t.Errorf("err = %v", err)
	}

	// A bad file stops start-up with the file and the reason.
	bad := t.TempDir()
	writeRuleset(t, bad, "x.json", `{"name":"x","states":[{"name":"bleeding","max":0}]}`)
	if err := run([]string{"--rulesets", bad}); err == nil || !strings.Contains(err.Error(), filepath.Join(bad, "x.json")) ||
		!strings.Contains(err.Error(), `needs a "tag"`) {
		t.Errorf("err = %v", err)
	}

	// A fight in a ruleset that isn't loaded names the folder to look in.
	state := filepath.Join(t.TempDir(), "fight.json")
	writeRuleset(t, filepath.Dir(state), "fight.json", `{"schemaVersion":2,"status":"active","ruleset":"grim","round":1}`)
	err := run([]string{"--state", state})
	if !errors.Is(err, combat.ErrUnknownRuleset) || !strings.Contains(err.Error(), "Ruleset files are read from "+filepath.Join(config, "dmct", "rulesets")) {
		t.Errorf("err = %v", err)
	}

	// With no config folder there is no default; files come only from
	// --rulesets.
	userConfigDir = func() (string, error) { return "", errors.New("no config") }
	if got := defaultRulesetDir(); got != "" {
		t.Errorf("default dir %q", got)
	}
	if err := run([]string{"--ruleset", "pf1"}); err == nil || !strings.Contains(err.Error(), "(choose 5e or 3.5e)") {
		t.Errorf("err = %v", err)
	}
}

func TestDisplayDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home folder")
	}
	for in, want := range map[string]string{
		"":                                 "(none)",
		filepath.Join(home, "a", "b"):      filepath.Join("~", "a", "b"),
		home:                               home,
		filepath.Join(home, "..", "other"): filepath.Join(home, "..", "other"),
		"/elsewhere/rules":                 "/elsewhere/rules",
	} {
		if got := displayDir(in); got != want {
			t.Errorf("displayDir(%q) = %q, want %q", in, got, want)
		}
	}
}

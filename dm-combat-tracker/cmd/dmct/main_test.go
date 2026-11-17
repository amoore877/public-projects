package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

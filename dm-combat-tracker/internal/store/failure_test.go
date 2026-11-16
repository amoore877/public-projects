package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func TestSaveIntoMissingFolderFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gone", "dmct-state.json")
	err := Save(path, combat.New(clock))
	if err == nil || !strings.Contains(err.Error(), "save state") {
		t.Fatalf("err = %v, want a save error", err)
	}
}

func TestLoadUnreadableStateFails(t *testing.T) {
	// A directory where the file should be: it exists but can't be read.
	if _, err := Load(t.TempDir(), clock); err == nil || !strings.Contains(err.Error(), "read state") {
		t.Fatalf("err = %v, want a read error", err)
	}
}

// If the rename fails, the state file stays where it is.
func TestArchiveRenameFailureKeepsState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dmct-state.json")
	if err := Save(path, combat.New(clock)); err != nil {
		t.Fatal(err)
	}
	rename = func(string, string) error { return errors.New("disk on fire") }
	t.Cleanup(func() { rename = os.Rename })
	if dst, err := Archive(path, t0); err == nil || dst != "" || !strings.Contains(err.Error(), "disk on fire") {
		t.Fatalf("Archive = %q, %v", dst, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file should be untouched: %v", err)
	}
}

func TestArchiveCannotInspectFolder(t *testing.T) {
	// The "folder" is a regular file, so looking for a free name fails with
	// something other than not-exist.
	notDir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notDir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Archive(filepath.Join(notDir, "dmct-state.json"), t0); err == nil || !strings.Contains(err.Error(), "archive state") {
		t.Fatalf("err = %v, want an archive error", err)
	}
}

func TestArchiveGivesUpWhenEveryNameIsTaken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dmct-state.json")
	if err := Save(path, combat.New(clock)); err != nil {
		t.Fatal(err)
	}
	stem := "dmct-state-" + t0.Format("20060102-150405")
	for n := 1; n < 1000; n++ {
		name := stem + ".json"
		if n > 1 {
			name = fmt.Sprintf("%s-%d.json", stem, n)
		}
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Archive(path, t0); err == nil || !strings.Contains(err.Error(), "too many archives") {
		t.Fatalf("err = %v, want too many archives", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file should be untouched: %v", err)
	}
}

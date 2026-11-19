// Package store loads and atomically saves the encounter state file (R7, D6)
// and archives it when combat ends (D13).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// Swappable for tests that simulate a failure partway through a save.
var rename = os.Rename

// Load reads the encounter at path. It returns (nil, nil) if the file doesn't
// exist. The returned encounter uses clock (nil means time.Now).
func Load(path string, clock combat.Clock) (*combat.Encounter, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var enc combat.Encounter
	if err := json.Unmarshal(data, &enc); err != nil {
		return nil, fmt.Errorf("parse state %s: %w", path, err)
	}
	switch {
	case enc.SchemaVersion < 1:
		return nil, fmt.Errorf("state %s: missing schemaVersion", path)
	case enc.SchemaVersion > combat.SchemaVersion:
		return nil, fmt.Errorf("state %s: schema version %d is newer than this build supports (%d)",
			path, enc.SchemaVersion, combat.SchemaVersion)
	}
	if enc.Ruleset == "" {
		enc.Ruleset = combat.DefaultRuleset // saved before rulesets existed (RS1)
	}
	if r, ok := combat.LookupRuleset(enc.Ruleset); ok {
		enc.Ruleset = r.Name
	} else {
		return nil, fmt.Errorf("state %s: %w %q", path, combat.ErrUnknownRuleset, enc.Ruleset)
	}
	enc.SchemaVersion = combat.SchemaVersion
	switch enc.Status {
	case combat.Setup, combat.Active, combat.Ended:
	default:
		return nil, fmt.Errorf("state %s: unknown status %q", path, enc.Status)
	}
	enc.SetClock(clock)
	return &enc, nil
}

// Save writes the encounter to path atomically: it writes <path>.tmp, fsyncs
// it, renames it over path, then fsyncs the directory. If any step fails, the
// previous file at path is left as it was and the temp file is removed.
func Save(path string, enc *combat.Encounter) error {
	data, err := json.MarshalIndent(enc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode state: %w", err)
	}
	data = append(data, '\n')

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("save state: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("save state: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	if err := rename(tmp, path); err != nil {
		return fmt.Errorf("save state: %w", err)
	}
	ok = true
	syncDir(filepath.Dir(path))
	return nil
}

// syncDir makes the rename durable. Best effort: some platforms and
// filesystems don't support fsync on a directory.
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		_ = d.Close()
	}
}

// Archive renames the state file to dmct-state-<YYYYMMDD-HHMMSS>.json in the
// same directory (D13) and returns the new path. If that name is taken (two
// fights ended in the same second), a -2, -3, … suffix is added.
func Archive(path string, at time.Time) (string, error) {
	dir := filepath.Dir(path)
	stem := "dmct-state-" + at.Format("20060102-150405")
	for n := 1; n < 1000; n++ {
		name := stem
		if n > 1 {
			name = fmt.Sprintf("%s-%d", stem, n)
		}
		dst := filepath.Join(dir, name+".json")
		if _, err := os.Lstat(dst); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("archive state: %w", err)
		}
		if err := rename(path, dst); err != nil {
			return "", fmt.Errorf("archive state: %w", err)
		}
		syncDir(dir)
		return dst, nil
	}
	return "", fmt.Errorf("archive state: too many archives named %s*", stem)
}

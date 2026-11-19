package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// LoadRulesets reads every *.json file in dir as a ruleset and makes them
// available alongside the built-ins (RS9, D38). Files are read in name
// order; other files and subdirectories are ignored. A dir that doesn't
// exist returns an error matching os.ErrNotExist. The first bad file, or a
// name already taken, is refused with the file's path and the reason, and
// then no file rulesets are available.
func LoadRulesets(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir) // sorted by name
	if err != nil {
		return nil, fmt.Errorf("read rulesets: %w", err)
	}
	var rs []combat.Ruleset
	var names []string
	for _, ent := range entries {
		if ent.IsDir() || !strings.EqualFold(filepath.Ext(ent.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, ent.Name())
		r, err := loadRuleset(path)
		if err == nil {
			err = combat.UseRulesets(append(rs, r))
		}
		if err != nil {
			_ = combat.UseRulesets(nil)
			return nil, fmt.Errorf("ruleset %s: %w", path, err)
		}
		rs = append(rs, r)
		names = append(names, r.Name)
	}
	return names, nil
}

func loadRuleset(path string) (combat.Ruleset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return combat.Ruleset{}, err
	}
	return combat.ParseRuleset(data)
}

// Command dmct is a terminal combat tracker for the DM's side of the screen.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return // usage was printed; asking for help isn't a failure
		}
		fmt.Fprintln(os.Stderr, "dmct:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("dmct", flag.ContinueOnError)
	statePath := fs.String("state", session.DefaultStatePath, "state file (autosaved after every change)")
	logPath := fs.String("log", "", "combat log file (default ./dmct-log-YYYY-MM-DD.md, dated when combat starts)")
	ruleset := fs.String("ruleset", "", "game ruleset for a fight that hasn't started: "+strings.Join(combat.RulesetNames(), " or ")+", or one from --rulesets (default "+combat.DefaultRuleset+")")
	rulesetDir := fs.String("rulesets", "", "folder of ruleset files, *.json (default "+displayDir(defaultRulesetDir())+")")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: dmct [--state FILE] [--log FILE] [--ruleset NAME] [--rulesets DIR]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	dir, err := loadRulesets(*rulesetDir)
	if err != nil {
		return err
	}
	if _, ok := combat.LookupRuleset(*ruleset); *ruleset != "" && !ok {
		return combat.UnknownRulesetError(*ruleset)
	}

	s, resumed, err := session.Open(session.Config{StatePath: *statePath, LogPath: *logPath, Ruleset: *ruleset})
	if errors.Is(err, session.ErrRulesetInUse) {
		return fmt.Errorf("%w\nRun without --ruleset to carry on with it, or with --state OTHER.json to start a new fight.", err)
	}
	if errors.Is(err, combat.ErrUnknownRuleset) {
		return fmt.Errorf("%w\nRuleset files are read from %s; put its file back there, or use --rulesets DIR.", err, displayDir(dir))
	}
	if err != nil {
		return fmt.Errorf("%w\nThe state file was left as it is. Fix or move it, or run with --state OTHER.json to start a new fight.", err)
	}
	defer s.Close()

	resumedFrom := ""
	if resumed {
		resumedFrom = *statePath
	}
	_, err = tea.NewProgram(tui.New(s, resumedFrom)).Run()
	return err
}

// defaultRulesetDir is where ruleset files live unless --rulesets says
// otherwise (D38): dmct/rulesets in the user's config folder. It is "" if
// the system has no config folder.
func defaultRulesetDir() string {
	base, err := userConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "dmct", "rulesets")
}

// Swappable for tests.
var userConfigDir = os.UserConfigDir

// loadRulesets loads the ruleset files and returns the folder they came
// from. The default folder may be missing; a folder named with --rulesets
// may not, since a typo there would otherwise go unnoticed.
func loadRulesets(dir string) (string, error) {
	_ = combat.UseRulesets(nil) // can't fail: no names to clash
	named := dir != ""
	if !named {
		dir = defaultRulesetDir()
		if dir == "" {
			return "", nil
		}
	}
	_, err := store.LoadRulesets(dir)
	if errors.Is(err, fs.ErrNotExist) && !named {
		return dir, nil
	}
	if err != nil {
		return dir, fmt.Errorf("%w\nFix or move the file, or use --rulesets DIR.", err)
	}
	return dir, nil
}

// displayDir shortens a folder under the home folder to ~/... for messages.
func displayDir(dir string) string {
	if dir == "" {
		return "(none)"
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rel, err := filepath.Rel(home, dir); err == nil && !strings.HasPrefix(rel, "..") && rel != "." {
			return filepath.Join("~", rel)
		}
	}
	return dir
}

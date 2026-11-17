// Command dmct is a terminal combat tracker for the DM's side of the screen.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
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
	ruleset := fs.String("ruleset", "", "game ruleset for a fight that hasn't started: "+strings.Join(combat.RulesetNames(), " or ")+" (default "+combat.DefaultRuleset+")")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: dmct [--state FILE] [--log FILE] [--ruleset NAME]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	if _, ok := combat.LookupRuleset(*ruleset); *ruleset != "" && !ok {
		return fmt.Errorf("unknown ruleset %q (choose %s)", *ruleset, strings.Join(combat.RulesetNames(), " or "))
	}

	s, resumed, err := session.Open(session.Config{StatePath: *statePath, LogPath: *logPath, Ruleset: *ruleset})
	if errors.Is(err, session.ErrRulesetInUse) {
		return fmt.Errorf("%w\nRun without --ruleset to carry on with it, or with --state OTHER.json to start a new fight.", err)
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

// Command dmct is a terminal combat tracker for the DM's side of the screen.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/session"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/tui"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stderr, "dmct:", err)
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("dmct", flag.ContinueOnError)
	statePath := fs.String("state", session.DefaultStatePath, "state file (autosaved after every change)")
	logPath := fs.String("log", "", "combat log file (default ./dmct-log-YYYY-MM-DD.md, dated when combat starts)")
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: dmct [--state FILE] [--log FILE]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		fs.Usage()
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	s, resumed, err := session.Open(session.Config{StatePath: *statePath, LogPath: *logPath})
	if err != nil {
		return err
	}
	defer s.Close()

	resumedFrom := ""
	if resumed {
		resumedFrom = *statePath
	}
	_, err = tea.NewProgram(tui.New(s, resumedFrom)).Run()
	return err
}

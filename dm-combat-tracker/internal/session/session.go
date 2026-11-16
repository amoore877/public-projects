// Package session is the app layer between the UI and the domain (D11): it
// applies combat operations, autosaves the state file after each change (D6),
// sends events to the Markdown log (D7), and archives the state when combat
// ends (D13). The TUI and command bar call Apply; neither touches files.
package session

import (
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/journal"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
)

// DefaultStatePath is the state file used when --state isn't given.
const DefaultStatePath = "dmct-state.json"

// Config is how the session finds its files.
type Config struct {
	StatePath string       // defaults to DefaultStatePath
	LogPath   string       // "" means dmct-log-YYYY-MM-DD.md for the day each combat starts
	Clock     combat.Clock // nil means time.Now
}

// Session owns the current encounter and its files.
type Session struct {
	cfg Config
	enc *combat.Encounter
	log *journal.Writer
}

// Open resumes the encounter in the state file, or starts a new one in Setup.
// A state file left in Ended (a crash between ending and archiving) is
// archived first, then a new encounter begins.
func Open(cfg Config) (s *Session, resumed bool, err error) {
	if cfg.StatePath == "" {
		cfg.StatePath = DefaultStatePath
	}
	if cfg.Clock == nil {
		cfg.Clock = time.Now
	}
	enc, err := store.Load(cfg.StatePath, cfg.Clock)
	if err != nil {
		return nil, false, err
	}
	if enc != nil && enc.Status == combat.Ended {
		if _, err := store.Archive(cfg.StatePath, cfg.Clock()); err != nil {
			return nil, false, err
		}
		enc = nil
	}
	resumed = enc != nil
	if enc == nil {
		enc = combat.New(cfg.Clock)
	}
	return &Session{cfg: cfg, enc: enc}, resumed, nil
}

// Encounter returns the current encounter, for display. Change it only
// through Apply.
func (s *Session) Encounter() *combat.Encounter { return s.enc }

// Outcome reports what happened after an operation succeeded. SaveErr and
// LogErr are problems to show the DM; the change itself has already been
// made and is never rolled back.
type Outcome struct {
	Events     []combat.Event
	SaveErr    error  // the state file couldn't be written
	LogErr     error  // the log couldn't be written (best-effort)
	Logged     bool   // log entries were written (false in Setup, where only notes are logged)
	ArchivedTo string // set when combat ended and the state file was archived
}

// Op is one domain operation, e.g.
//
//	func(e *combat.Encounter) ([]combat.Event, error) { return e.Damage(id, 7) }
type Op func(*combat.Encounter) ([]combat.Event, error)

// Apply runs op on the encounter. If op fails, nothing is saved or logged and
// its error is returned. Otherwise the state is saved, the events are logged,
// and, if combat ended, the state file is archived and a new encounter begins.
func (s *Session) Apply(op Op) (Outcome, error) {
	logAt := journal.ResumeFor(s.enc)
	evs, err := op(s.enc)
	if err != nil {
		return Outcome{}, err
	}
	out := Outcome{Events: evs}
	if len(evs) == 0 {
		return out, nil
	}

	ended := false
	for _, ev := range evs {
		switch ev.(type) {
		case combat.CombatStarted, combat.NoteLogged:
			s.bindLog()
		case combat.CombatEnded:
			ended = true
		}
	}

	out.SaveErr = store.Save(s.cfg.StatePath, s.enc)
	out.Logged, out.LogErr = s.writeLog(evs, logAt)

	if ended {
		s.closeLog()
		if out.SaveErr == nil {
			out.ArchivedTo, out.SaveErr = store.Archive(s.cfg.StatePath, s.cfg.Clock())
		}
		s.enc = combat.New(s.cfg.Clock)
	}
	return out, nil
}

// bindLog fixes the encounter's log file the first time it needs one, so a
// fight resumed on a later date keeps writing to the same log.
func (s *Session) bindLog() {
	if s.enc.LogPath != "" {
		return
	}
	s.enc.LogPath = s.cfg.LogPath
	if s.enc.LogPath == "" {
		s.enc.LogPath = "dmct-log-" + s.cfg.Clock().Format("2006-01-02") + ".md"
	}
}

// writeLog appends evs to the encounter's log. at is where the log stood
// before the operation, used if the writer has to be (re)opened, e.g. on the
// first change after a restart. It reports whether entries were written.
func (s *Session) writeLog(evs []combat.Event, at journal.Resume) (bool, error) {
	if s.enc.LogPath == "" {
		return false, nil // nothing in Setup is logged except notes, which bind the log
	}
	if s.log == nil {
		w, err := journal.Open(s.enc.LogPath, at)
		if err != nil {
			return false, err
		}
		s.log = w
	}
	if err := s.log.Write(evs); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Session) closeLog() {
	if s.log != nil {
		_ = s.log.Close()
		s.log = nil
	}
}

// Close releases the log file. State is already saved after every change.
func (s *Session) Close() error {
	s.closeLog()
	return nil
}

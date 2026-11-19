package session

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/journal"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/store"
)

// UndoLimit is how many steps Undo can go back (D40).
const UndoLimit = 100

var (
	ErrNothingToUndo = errors.New("nothing to undo")
	ErrNothingToRedo = errors.New("nothing to redo")
)

// step is one change the DM made: one successful Apply that emitted events
// (D39, D40).
type step struct {
	before, after []byte         // the encounter as JSON, either side of the change
	events        []combat.Event // what the change emitted, for the UI to describe
	logged        []string       // the log lines it wrote (journal.Texts); nil if none
}

func snapshot(enc *combat.Encounter) []byte {
	data, err := json.Marshal(enc)
	if err != nil {
		// The encounter is plain data that Save writes after every change;
		// failing here means a programming error, not a bad file.
		panic(fmt.Sprintf("snapshot encounter: %v", err))
	}
	return data
}

// record keeps a step for undo. A change that ends combat clears the
// history instead (D43).
func (s *Session) record(st step, ended bool) {
	s.redo = nil
	if ended {
		s.undo = nil
		return
	}
	s.undo = append(s.undo, st)
	if len(s.undo) > UndoLimit {
		s.undo = append([]step(nil), s.undo[len(s.undo)-UndoLimit:]...)
	}
}

// UndoDepth reports how many steps can be undone and redone.
func (s *Session) UndoDepth() (undo, redo int) { return len(s.undo), len(s.redo) }

// Undo puts the encounter back as it was before the last change (UR1). If
// that change wrote log lines, an "Undone: …" line says so (D42). The state
// is saved as after any change. The Outcome's Events are the undone
// change's, so the UI can name it; Undid is set.
func (s *Session) Undo() (Outcome, error) {
	if len(s.undo) == 0 {
		return Outcome{}, ErrNothingToUndo
	}
	st := s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]

	out := Outcome{Events: st.events, Undid: true}
	if len(st.logged) > 0 {
		text := "Undone: " + st.logged[0]
		if n := len(st.logged) - 1; n > 0 {
			text += fmt.Sprintf(" (+%d more)", n)
		}
		meta := s.enc.Meta()
		out.Logged, out.LogErr = s.logWith(func(w *journal.Writer) error { return w.Entry(meta, text) })
	}
	out.SaveErr = s.restore(st.before)
	s.redo = append(s.redo, st)
	return out, nil
}

// Redo makes the last undone change again (UR2), writing its log lines
// again marked "(redone)" (D42).
func (s *Session) Redo() (Outcome, error) {
	if len(s.redo) == 0 {
		return Outcome{}, ErrNothingToRedo
	}
	st := s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]

	out := Outcome{Events: st.events, Redid: true}
	if len(st.logged) > 0 {
		at := s.cfg.Clock()
		out.Logged, out.LogErr = s.logWith(func(w *journal.Writer) error { return w.WriteRedone(st.events, at) })
	}
	out.SaveErr = s.restore(st.after)
	s.undo = append(s.undo, st)
	return out, nil
}

// restore replaces the encounter with a snapshot and saves it. A log file
// once bound stays bound, so lines after an undo land in the same file.
func (s *Session) restore(data []byte) error {
	var enc combat.Encounter
	if err := json.Unmarshal(data, &enc); err != nil {
		panic(fmt.Sprintf("restore encounter: %v", err)) // our own snapshot
	}
	enc.SetClock(s.cfg.Clock)
	if enc.LogPath == "" {
		enc.LogPath = s.enc.LogPath
	}
	s.enc = &enc
	if s.log != nil {
		s.log.Resync(journal.ResumeFor(s.enc))
	}
	return store.Save(s.cfg.StatePath, s.enc)
}

// logWith runs write on the log, opening it first if needed. It reports
// whether the write happened.
func (s *Session) logWith(write func(*journal.Writer) error) (bool, error) {
	if s.enc.LogPath == "" {
		return false, nil
	}
	if s.log == nil {
		w, err := journal.Open(s.enc.LogPath, journal.ResumeFor(s.enc))
		if err != nil {
			return false, err
		}
		s.log = w
	}
	if err := write(s.log); err != nil {
		return false, err
	}
	return true, nil
}

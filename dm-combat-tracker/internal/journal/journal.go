// Package journal writes the append-only Markdown combat log (R6, D7). It
// consumes combat events; it never reads the file back or rewrites it.
package journal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// Resume tells a Writer where an encounter's log already stands, so a resumed
// fight doesn't repeat its "# Combat" or "## Round" headings.
type Resume struct {
	Header bool // the "# Combat — …" heading for this encounter is already written
	Round  int  // the last "## Round N" heading written; 0 for none
}

// ResumeFor derives the log position from a loaded encounter. Once combat is
// active, its header and current round heading have been written. In Setup,
// the app binds LogPath only when it first logs something (a note), so a set
// LogPath means the header is there.
func ResumeFor(enc *combat.Encounter) Resume {
	switch {
	case enc.Status == combat.Active:
		return Resume{Header: true, Round: enc.Round}
	case enc.LogPath != "":
		return Resume{Header: true}
	}
	return Resume{}
}

// Writer appends log entries for one encounter.
type Writer struct {
	f        *os.File
	header   bool
	round    int
	trailing int // newlines at the end of the file (0, 1 or 2+), for spacing
}

// Open opens (creating if needed) the log at path for appending.
func Open(path string, at Resume) (*Writer, error) {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	w := &Writer{f: f, header: at.Header, round: at.Round, trailing: 2}
	if st, err := f.Stat(); err == nil && st.Size() > 0 {
		// We don't read the file back, so assume it ends in a single newline.
		w.trailing = 1
	}
	return w, nil
}

// Close closes the log file.
func (w *Writer) Close() error { return w.f.Close() }

// Write appends the entries for evs in a single write. Events that aren't
// worth a log line are skipped. On error nothing about the writer's position
// changes, so the caller can carry on (logging is best-effort).
func (w *Writer) Write(evs []combat.Event) error {
	return w.flush(w.renderer(), evs)
}

// WriteRedone writes evs again for a redo (D42): each entry is marked
// "(redone)" and stamped at, the time of the redo.
func (w *Writer) WriteRedone(evs []combat.Event, at time.Time) error {
	r := w.renderer()
	r.suffix, r.at = " (redone)", &at
	return w.flush(r, evs)
}

// Entry writes one entry of the session's own, such as an undo line (D42).
func (w *Writer) Entry(m combat.EventMeta, text string) error {
	r := w.renderer()
	r.entry(m, text)
	return w.flush(r, nil)
}

// Resync sets where the writer thinks the log stands, after the encounter
// was put back to an earlier state (an undo or redo). The next round
// heading is written again if the restored round is before it.
func (w *Writer) Resync(at Resume) {
	w.header, w.round = at.Header, at.Round
}

func (w *Writer) renderer() *renderer {
	return &renderer{header: w.header, round: w.round, trailing: w.trailing}
}

func (w *Writer) flush(r *renderer, evs []combat.Event) error {
	for _, ev := range evs {
		r.event(ev)
	}
	if r.b.Len() == 0 {
		return nil
	}
	if _, err := w.f.WriteString(r.b.String()); err != nil {
		return fmt.Errorf("write log: %w", err)
	}
	w.header, w.round, w.trailing = r.header, r.round, r.trailing
	return nil
}

// Texts lists what writing evs puts in the log, one line per entry
// without its time and turn stamp, plus "Round N started" for a round
// heading. Nil means the events write nothing. The session uses it to name
// an undone step (D42).
func Texts(evs []combat.Event) []string {
	r := renderer{header: true}
	for _, ev := range evs {
		r.event(ev)
	}
	return r.texts
}

type renderer struct {
	b        strings.Builder
	header   bool
	round    int
	trailing int

	suffix string     // appended to every entry (" (redone)")
	at     *time.Time // overrides the events' own time, for a redo
	texts  []string   // what was written, for Texts
}

func (r *renderer) raw(s string) {
	r.b.WriteString(s)
	switch {
	case strings.HasSuffix(s, "\n\n"):
		r.trailing = 2
	case strings.HasSuffix(s, "\n"):
		r.trailing = 1
	default:
		r.trailing = 0
	}
}

// block writes a heading as its own paragraph: a blank line before and after.
func (r *renderer) block(s string) {
	for n := r.trailing; n < 2; n++ {
		r.b.WriteByte('\n')
	}
	r.raw(s + "\n\n")
}

func (r *renderer) ensureHeader(m combat.EventMeta) {
	if r.header {
		return
	}
	r.header = true
	r.block("# Combat — " + m.At.Format("2006-01-02 15:04"))
}

func (r *renderer) roundHeading(n int) {
	if n == r.round || n < 1 {
		return
	}
	r.round = n
	r.block("## Round " + strconv.Itoa(n))
}

func (r *renderer) entry(m combat.EventMeta, text string) {
	r.ensureHeader(m)
	stamp := "Setup"
	if m.Round > 0 {
		stamp = "R" + strconv.Itoa(m.Round)
		if m.TurnHolder != "" {
			stamp += " · " + oneLine(m.TurnHolder)
		}
	}
	at := m.At
	if r.at != nil {
		at = *r.at
	}
	text = oneLine(text) + r.suffix
	r.texts = append(r.texts, text)
	r.raw(fmt.Sprintf("- `%s` **[%s]** %s\n", at.Format("15:04"), stamp, text))
}

func (r *renderer) event(ev combat.Event) {
	m := ev.Meta()
	// During Setup only manual notes are logged; the lineup is summarized
	// when combat starts.
	if m.Round == 0 {
		if n, ok := ev.(combat.NoteLogged); ok {
			r.entry(m, "📝 "+n.Text)
		}
		return
	}
	switch ev.(type) {
	case combat.CombatStarted:
		r.ensureHeader(m)
		r.roundHeading(m.Round)
	case combat.RoundStarted:
		r.ensureHeader(m)
		r.roundHeading(m.Round)
		r.texts = append(r.texts, "Round "+strconv.Itoa(m.Round)+" started")
		return
	}
	if text, ok := logText(ev); ok {
		r.entry(m, text)
	}
}

// logText is the log's wording for an event, whatever the round, or false
// for an event the log leaves out.
func logText(ev combat.Event) (string, bool) {
	switch e := ev.(type) {
	case combat.CombatStarted:
		return "Combat started: " + lineup(e.Order), true
	case combat.CombatEnded:
		return "Combat ended after " + plural(e.Rounds, "round"), true
	case combat.TurnRewound:
		return "Turn rewound to " + e.Name, true

	case combat.CombatantAdded:
		return fmt.Sprintf("%s joined at initiative %s (%d/%d HP)", e.Name, e.Initiative, e.HP, e.MaxHP), true
	case combat.CombatantRemoved:
		return e.Name + " removed from combat", true
	case combat.HealthStateChanged:
		switch e.To {
		case "":
			// Every built-in state covers HP 0 and below, so leaving one
			// for none means HP is above 0. A ruleset file may say otherwise.
			if combat.IsNonlethalState(e.From) || e.HP <= 0 {
				return fmt.Sprintf("%s is no longer %s", e.Name, e.From), true
			}
			return e.Name + " is back above 0 HP", true
		case "down":
			if e.HP == 0 { // 5e, which can't go below 0
				return e.Name + " dropped to 0 HP", true
			}
			fallthrough
		default:
			if combat.IsNonlethalState(e.To) {
				return fmt.Sprintf("%s is %s (%d HP, %d nonlethal)", e.Name, e.To, e.HP, e.Nonlethal), true
			}
			return fmt.Sprintf("%s is %s (%d HP)", e.Name, e.To, e.HP), true
		}
	case combat.MaxHPSet:
		return fmt.Sprintf("%s max HP %d → %d", e.Name, e.From, e.To), true

	case combat.ConditionApplied:
		var extra []string
		if e.Remaining != nil {
			extra = append(extra, plural(*e.Remaining, "round"))
		}
		if e.Note != "" {
			extra = append(extra, e.Note)
		}
		by := ""
		if e.Source != "" {
			by = " by " + e.Source
		}
		return fmt.Sprintf("%s applied to %s%s%s", e.Condition, e.Target, by, paren(strings.Join(extra, "; "))), true
	case combat.ConditionRemoved:
		return fmt.Sprintf("%s removed from %s", e.Condition, e.Target), true
	case combat.ConditionExpired:
		return fmt.Sprintf("%s expired on %s", e.Condition, e.Target), true

	case combat.EffectStarted:
		var extra []string
		if e.Source != "" {
			extra = append(extra, e.Source)
		}
		if e.Remaining != nil {
			extra = append(extra, plural(*e.Remaining, "round"))
		}
		text := "Effect started: " + e.Name + paren(strings.Join(extra, ", "))
		if len(e.Targets) > 0 {
			text += " on " + strings.Join(e.Targets, ", ")
		}
		return text, true
	case combat.EffectTargetsChanged:
		var parts []string
		if len(e.Added) > 0 {
			parts = append(parts, "now also on "+strings.Join(e.Added, ", "))
		}
		if len(e.Removed) > 0 {
			parts = append(parts, "no longer on "+strings.Join(e.Removed, ", "))
		}
		if len(parts) > 0 {
			return e.Name + " " + strings.Join(parts, "; "), true
		}
	case combat.EffectEnded:
		return "Effect ended early: " + e.Name + paren(e.Reason), true
	case combat.EffectExpired:
		return "Effect expired: " + e.Name, true

	case combat.NoteLogged:
		return "📝 " + e.Text, true

		// Not logged: TurnStarted (the turn holder is on every line), HP
		// arithmetic (Damaged, Healed, HPSet, TempHPSet, NonlethalDamaged,
		// NonlethalSet; changes of state are logged via HealthStateChanged),
		// InitiativeChanged and RowNoteSet.
	}
	return "", false
}

// lineup summarizes the starting order, collapsing runs of numbered group
// members: "Thorin (18), Goblin 1–4 (12.4–12.1), Mira (9)".
func lineup(order []combat.OrderEntry) string {
	var parts []string
	for i := 0; i < len(order); {
		o := order[i]
		if o.IsEffect {
			parts = append(parts, fmt.Sprintf("◆ %s (%s)", o.Name, o.Initiative))
			i++
			continue
		}
		base, n, ok := splitNumbered(o.Name)
		j := i + 1
		if ok {
			for j < len(order) && !order[j].IsEffect {
				b2, n2, ok2 := splitNumbered(order[j].Name)
				if !ok2 || b2 != base || n2 != n+(j-i) {
					break
				}
				j++
			}
		}
		if j-i >= 2 {
			last := order[j-1]
			parts = append(parts, fmt.Sprintf("%s %d–%d (%s–%s)", base, n, n+(j-i-1), o.Initiative, last.Initiative))
		} else {
			parts = append(parts, fmt.Sprintf("%s (%s)", o.Name, o.Initiative))
		}
		i = j
	}
	return strings.Join(parts, ", ")
}

// splitNumbered splits "Goblin 12" into ("Goblin", 12).
func splitNumbered(name string) (string, int, bool) {
	i := strings.LastIndexByte(name, ' ')
	if i <= 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil || n < 0 || name[i+1] == '+' || name[i+1] == '-' {
		return "", 0, false
	}
	return name[:i], n, true
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return strconv.Itoa(n) + " " + unit + "s"
}

func paren(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

// oneLine keeps an entry on a single Markdown list line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/command"
)

// This file holds the editing hotkeys (milestone 4). Each one opens an
// overlay that asks for what it needs, then makes a single session.Apply
// call, so autosave and the log behave exactly as for turn changes.

// selectedCombatant returns the selected row's combatant, or warns and
// returns nil when the selection is empty or an effect row.
func (m *Model) selectedCombatant() *combat.Combatant {
	r, ok := m.sess.Encounter().Row(m.selected)
	switch {
	case !ok:
		m.warn("No combatant selected")
		return nil
	case r.Combatant == nil:
		m.warn("Select a combatant row")
		return nil
	}
	return r.Combatant
}

// --- damage, heal, temp HP, nonlethal, max HP -----------------------------

func (m *Model) promptHP(kind string) overlay {
	c := m.selectedCombatant()
	if c == nil {
		return nil
	}
	id, name := c.ID, c.Name
	p := &inputPrompt{help: "=N sets HP"}
	switch kind {
	case "d":
		p.label = "Damage " + name + ":"
	case "h":
		p.label = "Heal " + name + ":"
	case "t":
		p.label = "Temp HP for " + name + ":"
		if c.TempHP > 0 {
			p.label = fmt.Sprintf("Temp HP for %s (has %d):", name, c.TempHP)
		}
		p.help = "N replaces · +N adds"
	case "D":
		if !m.sess.Encounter().Rules().Nonlethal {
			m.warn(fmt.Sprintf("The %s ruleset doesn't track nonlethal damage", m.sess.Encounter().Rules().Name))
			return nil
		}
		p.label = "Nonlethal damage to " + name + ":"
		if c.Nonlethal > 0 {
			p.label = fmt.Sprintf("Nonlethal damage to %s (has %d):", name, c.Nonlethal)
		}
		p.help = "=N sets the total"
	case "M":
		p.label = "Max HP for " + name + ":"
		p.ed = newLineEdit(strconv.Itoa(c.MaxHP))
		p.help = "a lower max brings HP down; a higher one doesn't heal"
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		set := kind != "t" && kind != "M" && strings.HasPrefix(text, "=")
		add := kind == "t" && strings.HasPrefix(text, "+")
		floor := 0
		switch {
		case set && kind != "D":
			floor = m.sess.Encounter().Rules().HPFloor
		case kind == "M":
			floor = 1
		}
		if set || add {
			text = text[1:]
		}
		n, err := parseAmount(text, floor)
		if err != nil {
			return p, err
		}
		_, err = m.run(func(e *combat.Encounter) ([]combat.Event, error) {
			switch {
			case kind == "D" && set:
				return e.SetNonlethal(id, n)
			case kind == "D":
				return e.NonlethalDamage(id, n)
			case kind == "M":
				return e.SetMaxHP(id, n)
			case set:
				return e.SetHP(id, n)
			case kind == "d":
				return e.Damage(id, n)
			case kind == "h":
				return e.Heal(id, n)
			case add:
				return e.AddTempHP(id, n)
			default:
				return e.SetTempHP(id, n)
			}
		})
		if err != nil {
			return p, err
		}
		return nil, nil
	}
	return p
}

// parseAmount reads a whole number of at least floor: 0 for an amount, or
// the ruleset's HP floor for a set HP.
func parseAmount(s string, floor int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("Enter a number")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < floor || strings.HasPrefix(s, "+") {
		return 0, fmt.Errorf("%q isn't a whole number of %d or more", s, floor)
	}
	return n, nil
}

// --- conditions -----------------------------------------------------------

func (m *Model) promptAddCondition() overlay {
	c := m.selectedCombatant()
	if c == nil {
		return nil
	}
	id, target := c.ID, c.Name
	var name string
	var rounds *int
	var source combat.RowID

	note := &inputPrompt{help: "optional, e.g. DC 14 CON save ends"}
	note.submit = func(m *Model, text string) (overlay, error) {
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			_, evs, err := e.AddConditionFrom(id, name, rounds, source, text)
			return evs, err
		})
		return nil, nil
	}

	dur := &inputPrompt{help: "rounds; blank = until removed"}
	dur.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		rounds = nil
		if text != "" {
			n, err := strconv.Atoi(text)
			if err != nil || n < 1 {
				return dur, fmt.Errorf("%q isn't a number of rounds (1 or more)", text)
			}
			rounds = &n
		}
		note.label = fmt.Sprintf("%s on %s · note:", name, target)
		return m.askConditionSource(id, target, rounds, &source, note), nil
	}

	pick := &inputPrompt{label: "Condition on " + target + ":", suggest: m.suggestConditions}
	pick.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		if text == "" {
			return pick, errors.New("Type a condition name, or tab through the presets")
		}
		name = m.sess.Encounter().Rules().CanonicalCondition(text)
		dur.label = fmt.Sprintf("%s on %s · duration:", name, target)
		return dur, nil
	}
	return pick
}

// askConditionSource asks whether a timed condition was caused by the
// creature whose turn it is (RS7, G8), then goes on to next. It asks only when
// that is a combatant other than the bearer; otherwise there is no source.
// Typing any other source is left to :cond … by SOURCE.
func (m *Model) askConditionSource(bearer combat.RowID, target string, rounds *int, source *combat.RowID, next overlay) overlay {
	*source = 0
	enc := m.sess.Encounter()
	holder, ok := enc.Combatant(enc.Current)
	if rounds == nil || enc.Status != combat.Active || !ok || holder.ID == bearer {
		return next
	}
	holderID := holder.ID
	return &choicePrompt{
		question: "counts down on whose turn?",
		enter:    "n",
		choices: []choice{
			{key: "s", label: holder.Name + " caused it", run: func(*Model) overlay { *source = holderID; return next }},
			{key: "n", label: target + "'s own", run: func(*Model) overlay { return next }},
		},
	}
}

// suggestConditions lists the ruleset's presets that start with text (all of
// them for an empty text), then any that contain it.
func (m *Model) suggestConditions(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	var prefix, inner []string
	for _, c := range m.sess.Encounter().Rules().Conditions {
		lc := strings.ToLower(c)
		switch {
		case strings.HasPrefix(lc, text):
			prefix = append(prefix, c)
		case strings.Contains(lc, text):
			inner = append(inner, c)
		}
	}
	return append(prefix, inner...)
}

func (m *Model) promptRemoveCondition() overlay {
	c := m.selectedCombatant()
	if c == nil {
		return nil
	}
	if len(c.Conditions) == 0 {
		if len(m.sess.Encounter().LinkedEffects(c.ID)) > 0 {
			m.warn(c.Name + " has only effect conditions; end the effect or change its targets instead")
		} else {
			m.warn(c.Name + " has no conditions")
		}
		return nil
	}
	id, target := c.ID, c.Name
	conds := append([]*combat.Condition(nil), c.Conditions...)
	var names, listed []string
	for i, cond := range conds {
		names = append(names, cond.Name)
		listed = append(listed, fmt.Sprintf("%d %s", i+1, cond.Name))
	}
	p := &inputPrompt{
		label: "Remove from " + target + ":",
		help:  "name or number: " + strings.Join(listed, ", "),
		suggest: func(text string) []string {
			var out []string
			for _, n := range names {
				if strings.HasPrefix(strings.ToLower(n), strings.ToLower(strings.TrimSpace(text))) {
					out = append(out, n)
				}
			}
			return out
		},
	}
	if len(conds) == 1 {
		p.ed = newLineEdit(conds[0].Name)
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		var cond *combat.Condition
		if n, err := strconv.Atoi(text); err == nil && n >= 1 && n <= len(conds) {
			cond = conds[n-1]
		} else {
			for _, x := range conds {
				if strings.EqualFold(x.Name, text) {
					cond = x
					break
				}
			}
		}
		if cond == nil {
			return p, fmt.Errorf("%s has no condition %q", target, text)
		}
		condID := cond.ID
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.RemoveCondition(id, condID) })
		return nil, nil
	}
	return p
}

// --- initiative and notes -------------------------------------------------

func (m *Model) promptInitiative() overlay {
	r, ok := m.sess.Encounter().Row(m.selected)
	if !ok {
		m.warn("Nothing selected")
		return nil
	}
	id := r.ID()
	p := &inputPrompt{
		label: "Initiative for " + r.Name() + ":",
		ed:    newLineEdit(r.Initiative().String()),
		help:  "decimals break ties, e.g. 15.2",
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		v, err := combat.ParseInitiative(text)
		if err != nil {
			return p, err
		}
		m.setInitiative(id, v, combat.AskToMoveEffects)
		return nil, nil
	}
	return p
}

// setInitiative changes a row's initiative. In a ruleset that pins effects
// to their counts, moving their source first asks whether they move too
// (D33): yes for a mistyped initiative, no for Delay or Ready.
func (m *Model) setInitiative(id combat.RowID, v combat.Initiative, move combat.SourceMove) {
	_, err := m.run(func(e *combat.Encounter) ([]combat.Event, error) { return e.SetInitiative(id, v, move) })
	var me *combat.MoveEffectsError
	switch {
	case errors.As(err, &me):
		m.chain = m.askMoveEffects(me)
	case err != nil:
		m.fail(err)
	}
	m.syncSelection()
}

func (m *Model) askMoveEffects(me *combat.MoveEffectsError) overlay {
	enc := m.sess.Encounter()
	name := ""
	if c, ok := enc.Combatant(me.Combatant); ok {
		name = c.Name
	}
	var fx []string
	for _, fid := range me.Effects {
		if f, ok := enc.Effect(fid); ok {
			fx = append(fx, f.Name)
		}
	}
	id, v := me.Combatant, me.To
	return &choicePrompt{
		question: fmt.Sprintf("%s is the source of %s: move them too?", name, strings.Join(fx, ", ")),
		choices: []choice{
			{key: "y", label: "move them (a correction)", run: func(m *Model) overlay { m.setInitiative(id, v, combat.MoveEffects); return nil }},
			{key: "n", label: "leave them (delay, ready)", run: func(m *Model) overlay { m.setInitiative(id, v, combat.LeaveEffects); return nil }},
		},
	}
}

func (m *Model) promptNote() overlay {
	r, ok := m.sess.Encounter().Row(m.selected)
	if !ok {
		m.warn("Nothing selected")
		return nil
	}
	id, note := r.ID(), ""
	if r.Combatant != nil {
		note = r.Combatant.Note
	} else {
		note = r.Effect.Note
	}
	p := &inputPrompt{label: "Note for " + r.Name() + ":", ed: newLineEdit(note), help: "blank clears it"}
	p.submit = func(m *Model, text string) (overlay, error) {
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.SetNote(id, text) })
		return nil, nil
	}
	return p
}

func (m *Model) promptLog() overlay {
	p := &inputPrompt{label: "Log:", help: "a line for the combat log"}
	p.submit = func(m *Model, text string) (overlay, error) {
		if strings.TrimSpace(text) == "" {
			return p, errors.New("Type something to log, or esc to cancel")
		}
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.LogNote(text) })
		return nil, nil
	}
	return p
}

// --- adding ---------------------------------------------------------------

// groupName matches "Goblin x4" / "Goblin ×4".
var groupName = regexp.MustCompile(`^(.+?)\s+[xX×]\s*(\d+)$`)

func (m *Model) promptAdd() overlay {
	var (
		name  string
		count int // 0 for a single combatant
		init  combat.Initiative
		inits []combat.Initiative
		maxHP int
		hp    *int
		temp  int
		who   string // "Goblin ×4", for the step labels
	)

	finish := func(m *Model, kind combat.Kind) overlay {
		var added []combat.RowID
		spec := combat.CombatantSpec{Name: name, Kind: kind, Initiative: init, MaxHP: maxHP, HP: hp, TempHP: temp}
		ok := m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			if count == 0 {
				id, evs, err := e.Add(spec)
				added = []combat.RowID{id}
				return evs, err
			}
			ids, evs, err := e.AddGroup(combat.GroupSpec{CombatantSpec: spec, Count: count, Initiatives: inits})
			added = ids
			return evs, err
		})
		if ok && len(added) > 0 {
			m.selected = added[0]
			m.syncSelection()
		}
		return nil
	}

	kind := &choicePrompt{enter: "n", choices: []choice{
		{key: "n", label: "NPC", run: func(m *Model) overlay { return finish(m, combat.NPC) }},
		{key: "p", label: "PC", run: func(m *Model) overlay { return finish(m, combat.PC) }},
	}}

	hpStep := &inputPrompt{help: "max HP, or CURRENT/MAX; +N adds temp HP"}
	hpStep.submit = func(m *Model, text string) (overlay, error) {
		cur, mx, tmp, err := command.ParseHP(text, m.sess.Encounter().Rules().HPFloor)
		if err != nil {
			return hpStep, err
		}
		maxHP, hp, temp = mx, cur, tmp
		kind.question = "Add " + who + " · kind?"
		return kind, nil
	}

	initStep := &inputPrompt{}
	initStep.submit = func(m *Model, text string) (overlay, error) {
		var err error
		init, inits, err = command.ParseAddInitiative(text, count)
		if err != nil {
			return initStep, err
		}
		hpStep.label = "Add " + who + " · HP:"
		return hpStep, nil
	}

	nameStep := &inputPrompt{label: "Add — name:", help: "Goblin x4 adds a numbered group"}
	nameStep.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		name, count = text, 0
		if g := groupName.FindStringSubmatch(text); g != nil {
			n, err := strconv.Atoi(g[2])
			if err != nil || n < 1 || n > 999 {
				return nameStep, errors.New("Group size must be 1–999")
			}
			name, count = strings.TrimSpace(g[1]), n
		}
		if name == "" {
			return nameStep, errors.New("Enter a name")
		}
		who = name
		if count > 0 {
			who = fmt.Sprintf("%s ×%d", name, count)
			initStep.help = fmt.Sprintf("whole number to share (decimals filled in), or %d values separated by commas", count)
		} else {
			initStep.help = "decimals break ties, e.g. 15.2"
		}
		initStep.label = "Add " + who + " · initiative:"
		return initStep, nil
	}
	return nameStep
}

// --- removing and ending --------------------------------------------------

func (m *Model) promptRemove() overlay {
	r, ok := m.sess.Encounter().Row(m.selected)
	if !ok {
		m.warn("Nothing selected")
		return nil
	}
	id, name := r.ID(), r.Name()
	return confirm("Remove "+name+" from combat?", func(m *Model) { m.remove(id, name, combat.AskAboutEffects) })
}

// remove takes a combatant out; if it is the source of effects, it asks
// whether to keep or end them (R5) before changing anything.
func (m *Model) remove(id combat.RowID, name string, sourced combat.SourcedEffects) {
	_, err := m.run(func(e *combat.Encounter) ([]combat.Event, error) { return e.Remove(id, sourced) })
	var nd *combat.NeedsDecisionError
	switch {
	case errors.As(err, &nd):
		var fx []string
		for _, fid := range nd.Effects {
			if f, ok := m.sess.Encounter().Effect(fid); ok {
				fx = append(fx, f.Name)
			}
		}
		m.chain = &choicePrompt{
			question: fmt.Sprintf("%s is the source of %s: keep or end?", name, strings.Join(fx, ", ")),
			choices: []choice{
				{key: "k", label: "keep (unsourced)", run: func(m *Model) overlay { m.remove(id, name, combat.KeepEffects); return nil }},
				{key: "e", label: "end them", run: func(m *Model) overlay { m.remove(id, name, combat.EndEffects); return nil }},
			},
		}
	case err != nil:
		m.fail(err)
	}
	m.syncSelection()
}

func (m *Model) promptEndCombat() overlay {
	enc := m.sess.Encounter()
	q := "End combat? The state file is archived and a new combat begins."
	if enc.Status == combat.Active {
		q = fmt.Sprintf("End combat after %s? The state file is archived.", plural(enc.Round, "round", "rounds"))
	}
	return confirm(q, func(m *Model) { m.endCombat() })
}

// endCombat ends the fight; the session archives the state file and a new
// combat begins in Setup (D13).
func (m *Model) endCombat() {
	out, err := m.run(func(e *combat.Encounter) ([]combat.Event, error) { return e.End() })
	if err != nil {
		m.fail(err)
		return
	}
	m.selected, m.selIdx, m.offset = 0, 0, 0
	m.syncSelection()
	if m.msg == "" && out.ArchivedTo != "" {
		m.info("Combat ended · archived as " + out.ArchivedTo)
	}
}

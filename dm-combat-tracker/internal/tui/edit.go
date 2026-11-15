package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
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

// --- damage, heal, temp HP ------------------------------------------------

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
		p.help = "replaces current temp HP"
		if c.TempHP > 0 {
			p.ed = newLineEdit(strconv.Itoa(c.TempHP))
		}
	}
	p.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		set := kind != "t" && strings.HasPrefix(text, "=")
		n, err := parseAmount(strings.TrimPrefix(text, "="))
		if err != nil {
			return p, err
		}
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			switch {
			case set:
				return e.SetHP(id, n)
			case kind == "d":
				return e.Damage(id, n)
			case kind == "h":
				return e.Heal(id, n)
			default:
				return e.SetTempHP(id, n)
			}
		})
		return nil, nil
	}
	return p
}

func parseAmount(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("Enter a number")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%q isn't a whole number of 0 or more", s)
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

	note := &inputPrompt{help: "optional, e.g. DC 14 CON save ends"}
	note.submit = func(m *Model, text string) (overlay, error) {
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) {
			_, evs, err := e.AddCondition(id, name, rounds, text)
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
		return note, nil
	}

	pick := &inputPrompt{label: "Condition on " + target + ":", suggest: suggestConditions}
	pick.submit = func(m *Model, text string) (overlay, error) {
		text = strings.TrimSpace(text)
		if text == "" {
			return pick, errors.New("Type a condition name, or tab through the presets")
		}
		name = canonicalCondition(text)
		dur.label = fmt.Sprintf("%s on %s · duration:", name, target)
		return dur, nil
	}
	return pick
}

// suggestConditions lists the presets that start with text (all of them for
// an empty text), then any that contain it.
func suggestConditions(text string) []string {
	text = strings.ToLower(strings.TrimSpace(text))
	var prefix, inner []string
	for _, c := range combat.DefaultConditions {
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

// canonicalCondition uses the preset's spelling for a case-insensitive match
// ("prone" → "Prone"); custom names are kept as typed.
func canonicalCondition(s string) string {
	for _, c := range combat.DefaultConditions {
		if strings.EqualFold(c, s) {
			return c
		}
	}
	return s
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
		m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.SetInitiative(id, v) })
		return nil, nil
	}
	return p
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
		if m.apply(func(e *combat.Encounter) ([]combat.Event, error) { return e.LogNote(text) }) && m.msg == "" {
			m.info("Logged to " + m.sess.Encounter().LogPath)
		}
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
		who   string // "Goblin ×4", for the step labels
	)

	finish := func(m *Model, kind combat.Kind) overlay {
		var added []combat.RowID
		spec := combat.CombatantSpec{Name: name, Kind: kind, Initiative: init, MaxHP: maxHP, HP: hp}
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

	hpStep := &inputPrompt{help: "max HP, or CURRENT/MAX"}
	hpStep.submit = func(m *Model, text string) (overlay, error) {
		cur, mx, err := parseHP(text)
		if err != nil {
			return hpStep, err
		}
		maxHP, hp = mx, cur
		kind.question = "Add " + who + " · kind?"
		return kind, nil
	}

	initStep := &inputPrompt{}
	initStep.submit = func(m *Model, text string) (overlay, error) {
		var err error
		init, inits, err = parseAddInitiative(text, count)
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

// parseAddInitiative reads one initiative, or for a group either one whole
// number (auto-filled decimals) or one value per member.
func parseAddInitiative(text string, count int) (combat.Initiative, []combat.Initiative, error) {
	parts := strings.Split(text, ",")
	if count == 0 || len(parts) == 1 {
		v, err := combat.ParseInitiative(text)
		if err != nil {
			return 0, nil, err
		}
		if count > 0 {
			if _, err := combat.GroupFill(v, count); err != nil {
				return 0, nil, errors.New("A shared group initiative must be a whole number; or give one value per member, separated by commas")
			}
		}
		return v, nil, nil
	}
	if len(parts) != count {
		return 0, nil, fmt.Errorf("Need %d initiatives, got %d", count, len(parts))
	}
	out := make([]combat.Initiative, count)
	for i, p := range parts {
		v, err := combat.ParseInitiative(p)
		if err != nil {
			return 0, nil, err
		}
		out[i] = v
	}
	return out[0], out, nil
}

// parseHP reads "44" (full health) or "31/44".
func parseHP(text string) (cur *int, maxHP int, err error) {
	curS, maxS, split := strings.Cut(strings.TrimSpace(text), "/")
	if !split {
		maxS = curS
	}
	maxHP, err = strconv.Atoi(strings.TrimSpace(maxS))
	if err != nil || maxHP < 1 {
		return nil, 0, errors.New("Max HP must be a whole number of 1 or more")
	}
	if split {
		n, err := strconv.Atoi(strings.TrimSpace(curS))
		if err != nil || n < 0 || n > maxHP {
			return nil, 0, fmt.Errorf("Current HP must be 0–%d", maxHP)
		}
		cur = &n
	}
	return cur, maxHP, nil
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
	return confirm(q, func(m *Model) {
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
	})
}

// Package command parses ":" command-bar input (D9) into combat operations.
// It only turns text into operations; the TUI runs them through the session
// exactly as it runs the hotkeys, so both input paths behave the same.
package command

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
	"github.com/amoore877/public-projects/dm-combat-tracker/internal/dice"
)

// Op is one domain operation; it has the same shape as session.Op.
type Op func(*combat.Encounter) ([]combat.Event, error)

// Kind tells the UI about commands that need more than running Op.
type Kind int

const (
	Edit      Kind = iota // a plain change; run Op
	Turn                  // :next, :prev, :start: the selection follows the turn (D22)
	Remove                // :rm: run through the UI's remove flow, which asks about sourced effects
	EndCombat             // :endcombat: combat ends and the state file is archived
	Undo                  // :undo: take back the last change (UR1); no Op
	Redo                  // :redo: make the last undone change again (UR2); no Op
	Roll                  // :roll: roll Dice, or the last roll when nil (DR2); no Op, nothing changes (D45)
	InitAll               // :initall: start the quick initiative pass (QI1); no Op
)

// Command is a parsed command.
type Command struct {
	Kind Kind
	Op   Op
	// Focus is the row to select afterwards (the first target), or zero.
	Focus combat.RowID
	// Target is the combatant to remove, for Kind Remove.
	Target combat.RowID
	// Created, if set, holds the first row Op added once it has run (:add, :fx).
	Created *combat.RowID
	// Dice is the expression to roll, for Kind Roll; nil rolls the last one again.
	Dice *dice.Expr
	// Clear is true for `:endcombat clear`, which starts an empty Setup instead
	// of carrying PCs over (D57).
	Clear bool
}

// Spec documents one command, for the help overlay.
type Spec struct {
	Usage   string
	Example string
	Summary string
}

// Specs lists every command in the order the help overlay shows them.
var Specs = []Spec{
	{":add NAME [xN] INIT HP [pc|npc] [-- NOTE]", ":add Goblin x4 12 7", "add a combatant or numbered group; HP is MAX or CUR/MAX, +N for temp HP"},
	{":rm TARGET", ":rm goblin 2", "remove a combatant"},
	{":dmg TARGETS N", ":dmg g1,g3 9", "damage (temp HP first)"},
	{":heal TARGETS N", ":heal thorin 8", "heal, up to max HP"},
	{":hp TARGET N", ":hp g2 3", "set HP directly"},
	{":temp TARGETS [+]N", ":temp mira +5", "set temp HP; +N adds to it"},
	{":nl TARGETS [=]N", ":nl orc 6", "nonlethal damage; =N sets the total (rulesets that track it)"},
	{":maxhp TARGET N", ":maxhp orc 24", "set max HP; a lower max brings HP down"},
	{":cond TARGETS NAME [ROUNDS] [by SOURCE] [-- NOTE]", ":cond g3 dazed 1 by mira", "add a condition; with a source it counts down on the source's turn"},
	{":uncond TARGET NAME", ":uncond g3 prone", "remove a condition"},
	{":fx NAME [ROUNDS] [from SOURCE] [at INIT] [on TARGETS]", ":fx Spirit Guardians 10 from mira on g1,g3", "start an effect"},
	{":fxadd FX TARGETS", ":fxadd spirit g2", "add effect targets"},
	{":fxrm FX TARGETS", ":fxrm spirit g1", "remove effect targets"},
	{":end FX [-- REASON]", ":end spirit -- concentration broken", "end an effect early"},
	{":init TARGET VALUE", ":init g2 12.35", "set initiative"},
	{":initall", "", "quick initiative pass over every combatant"},
	{":note TARGET TEXT", ":note g1 fleeing", "set the side note (blank clears)"},
	{":log TEXT", ":log Thorin shoves the goblin", "write a log entry"},
	{":ruleset NAME", ":ruleset 3.5e", "choose the game ruleset (5e, 3.5e or one from a file); only before combat starts"},
	{":next  :prev  :start  :endcombat [clear]", "", "turn controls; :endcombat keeps PCs unless clear is given"},
	{":undo  :redo", "", "take back the last change / make it again"},
	{":roll [DICE]", ":roll 2d6+2+1d4", "roll dice (d20 is 1d20); no dice rolls the last again. Changes nothing"},
}

// Verbs lists the command names, for completion.
var Verbs = []string{"add", "rm", "dmg", "heal", "hp", "temp", "nl", "maxhp", "cond", "uncond", "fx", "fxadd", "fxrm", "end", "init", "initall", "note", "log", "ruleset", "next", "prev", "start", "endcombat", "undo", "redo", "roll"}

var aliases = map[string]string{
	"remove": "rm", "damage": "dmg", "condition": "cond", "effect": "fx",
	"nonlethal": "nl", "max": "maxhp",
	"n": "next", "p": "prev", "previous": "prev",
}

// Parse reads one command line (with or without the leading ":") against
// the current encounter, resolving names to rows.
func Parse(enc *combat.Encounter, line string) (Command, error) {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), ":"))
	verb, rest, _ := strings.Cut(line, " ")
	verb = strings.ToLower(verb)
	if a, ok := aliases[verb]; ok {
		verb = a
	}
	rest = strings.TrimSpace(rest)
	words := strings.Fields(rest)

	switch verb {
	case "":
		return Command{}, errors.New("type a command, e.g. :dmg g1 7")
	case "next", "prev", "start", "undo", "redo", "initall":
		if rest != "" {
			return Command{}, fmt.Errorf(":%s takes no arguments", verb)
		}
		switch verb {
		case "undo":
			return Command{Kind: Undo}, nil
		case "redo":
			return Command{Kind: Redo}, nil
		case "initall":
			return Command{Kind: InitAll}, nil
		case "next":
			return Command{Kind: Turn, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.Next() }}, nil
		case "prev":
			return Command{Kind: Turn, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.Previous() }}, nil
		default:
			return Command{Kind: Turn, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.Start() }}, nil
		}
	case "endcombat":
		clear := false
		if rest != "" {
			if strings.EqualFold(rest, "clear") {
				clear = true
			} else {
				return Command{}, errors.New("usage: :endcombat [clear]")
			}
		}
		keepPCs := !clear
		return Command{Kind: EndCombat, Clear: clear, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.EndWith(keepPCs) }}, nil
	case "roll":
		if rest == "" {
			return Command{Kind: Roll}, nil
		}
		e, err := dice.Parse(rest)
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: Roll, Dice: &e}, nil
	case "add":
		return parseAdd(enc, rest)
	case "rm":
		id, err := resolve(enc, rest, combatantRow)
		if err != nil {
			return Command{}, err
		}
		return Command{Kind: Remove, Target: id}, nil
	case "dmg", "heal", "temp", "hp", "nl", "maxhp":
		return parseHP(enc, verb, words)
	case "cond":
		return parseCond(enc, rest)
	case "uncond":
		return parseUncond(enc, words)
	case "fx":
		return parseFX(enc, words)
	case "fxadd", "fxrm":
		return parseFXTargets(enc, verb, words)
	case "end":
		text, reason, _ := strings.Cut(rest, "--")
		id, err := resolve(enc, text, effectRow)
		if err != nil {
			return Command{}, err
		}
		reason = strings.TrimSpace(reason)
		return Command{Focus: id, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.EndEffect(id, reason) }}, nil
	case "init":
		if len(words) < 2 {
			return Command{}, errors.New("usage: :init TARGET VALUE")
		}
		v, err := combat.ParseInitiative(words[len(words)-1])
		if err != nil {
			return Command{}, err
		}
		id, err := resolve(enc, strings.Join(words[:len(words)-1], " "), anyRow)
		if err != nil {
			return Command{}, err
		}
		return Command{Focus: id, Op: func(e *combat.Encounter) ([]combat.Event, error) {
			return e.SetInitiative(id, v, combat.AskToMoveEffects)
		}}, nil
	case "note":
		if len(words) == 0 {
			return Command{}, errors.New("usage: :note TARGET TEXT")
		}
		// The note may be empty (clears it), so try the whole line as a name too.
		ids, restWords, err := splitTarget(enc, words, anyRow, 0)
		if err != nil {
			return Command{}, err
		}
		if len(ids) != 1 {
			return Command{}, errors.New(":note takes one target")
		}
		id, note := ids[0], strings.Join(restWords, " ")
		return Command{Focus: id, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.SetNote(id, note) }}, nil
	case "log":
		if rest == "" {
			return Command{}, errors.New("usage: :log TEXT")
		}
		return Command{Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.LogNote(rest) }}, nil
	case "ruleset":
		if _, ok := combat.LookupRuleset(rest); !ok {
			return Command{}, fmt.Errorf("usage: :ruleset %s (now %s)", strings.Join(combat.RulesetNames(), "|"), enc.Rules().Name)
		}
		return Command{Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.SetRuleset(rest) }}, nil
	}
	return Command{}, fmt.Errorf("unknown command :%s (press ? for the list)", verb)
}

// each runs f for every id, concatenating the events. Targets and amounts
// are checked while parsing, so f doesn't fail part way through a list.
func each(ids []combat.RowID, f func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error)) Op {
	return func(e *combat.Encounter) ([]combat.Event, error) {
		var out []combat.Event
		for _, id := range ids {
			evs, err := f(e, id)
			if err != nil {
				return out, err
			}
			out = append(out, evs...)
		}
		return out, nil
	}
}

func parseHP(enc *combat.Encounter, verb string, words []string) (Command, error) {
	usage := map[string]string{"temp": "TARGETS [+]N", "nl": "TARGETS [=]N", "hp": "TARGET N", "maxhp": "TARGET N"}[verb]
	if usage == "" {
		usage = "TARGETS N"
	}
	if len(words) < 2 {
		return Command{}, fmt.Errorf("usage: :%s %s", verb, usage)
	}
	if verb == "nl" && !enc.Rules().Nonlethal {
		return Command{}, fmt.Errorf("%w (%s)", combat.ErrNoNonlethal, enc.Rules().Name)
	}
	raw := words[len(words)-1]
	text := raw
	adding := verb == "temp" && strings.HasPrefix(text, "+")
	setting := verb == "nl" && strings.HasPrefix(text, "=")
	if adding || setting {
		text = text[1:]
	}
	floor := 0 // amounts are never negative; a set HP can be, down to the ruleset's floor
	switch verb {
	case "hp":
		floor = enc.Rules().HPFloor
	case "maxhp":
		floor = 1
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < floor || strings.HasPrefix(text, "+") {
		return Command{}, fmt.Errorf("%q isn't a whole number of %d or more", raw, floor)
	}
	ids, err := resolveList(enc, strings.Join(words[:len(words)-1], " "), combatantRow)
	if err != nil {
		return Command{}, err
	}
	if (verb == "hp" || verb == "maxhp") && len(ids) > 1 {
		return Command{}, fmt.Errorf(":%s takes one target", verb)
	}
	op := each(ids, func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) {
		switch {
		case verb == "dmg":
			return e.Damage(id, n)
		case verb == "heal":
			return e.Heal(id, n)
		case verb == "hp":
			return e.SetHP(id, n)
		case verb == "maxhp":
			return e.SetMaxHP(id, n)
		case verb == "nl" && setting:
			return e.SetNonlethal(id, n)
		case verb == "nl":
			return e.NonlethalDamage(id, n)
		case adding:
			return e.AddTempHP(id, n)
		}
		return e.SetTempHP(id, n)
	})
	return Command{Focus: ids[0], Op: op}, nil
}

func parseCond(enc *combat.Encounter, rest string) (Command, error) {
	head, note, _ := strings.Cut(rest, "--")
	note = strings.TrimSpace(note)
	words := strings.Fields(head)
	if len(words) < 2 {
		return Command{}, errors.New("usage: :cond TARGETS NAME [ROUNDS] [by SOURCE] [-- NOTE]")
	}
	ids, nameWords, err := splitTarget(enc, words, combatantRow, 1)
	if err != nil {
		return Command{}, err
	}
	var source combat.RowID
	for k := len(nameWords) - 2; k >= 1; k-- {
		if strings.EqualFold(nameWords[k], "by") {
			if source, err = resolve(enc, strings.Join(nameWords[k+1:], " "), combatantRow); err != nil {
				return Command{}, fmt.Errorf("source: %w", err)
			}
			nameWords = nameWords[:k]
			break
		}
	}
	var rounds *int
	if len(nameWords) > 1 {
		if n, err := strconv.Atoi(nameWords[len(nameWords)-1]); err == nil {
			if n < 1 {
				return Command{}, errors.New("duration must be at least 1 round")
			}
			rounds, nameWords = &n, nameWords[:len(nameWords)-1]
		}
	}
	name := enc.Rules().CanonicalCondition(strings.Join(nameWords, " "))
	op := each(ids, func(e *combat.Encounter, id combat.RowID) ([]combat.Event, error) {
		_, evs, err := e.AddConditionFrom(id, name, rounds, source, note)
		return evs, err
	})
	return Command{Focus: ids[0], Op: op}, nil
}

func parseUncond(enc *combat.Encounter, words []string) (Command, error) {
	if len(words) < 2 {
		return Command{}, errors.New("usage: :uncond TARGET NAME")
	}
	// Try each split, longest target first, keeping the first that has the condition.
	var lastErr error
	for k := len(words) - 1; k >= 1; k-- {
		id, err := resolve(enc, strings.Join(words[:k], " "), combatantRow)
		if err != nil {
			if lastErr == nil || k == 1 {
				lastErr = err
			}
			continue
		}
		c, _ := enc.Combatant(id)
		name := strings.Join(words[k:], " ")
		cond, ok := c.FindCondition(name)
		if !ok {
			lastErr = fmt.Errorf("%s has no condition %q", c.Name, name)
			continue
		}
		condID := cond.ID
		return Command{Focus: id, Op: func(e *combat.Encounter) ([]combat.Event, error) { return e.RemoveCondition(id, condID) }}, nil
	}
	return Command{}, lastErr
}

func parseFX(enc *combat.Encounter, words []string) (Command, error) {
	usage := errors.New("usage: :fx NAME [ROUNDS] [from SOURCE] [at INIT] [on TARGETS]")
	// Split at the keywords; the name is everything before the first one.
	parts := map[string][]string{}
	cur := "name"
	for _, w := range words {
		switch lw := strings.ToLower(w); lw {
		case "from", "at", "on":
			if _, seen := parts[lw]; !seen && cur != lw {
				cur = lw
				parts[cur] = []string{}
				continue
			}
		}
		parts[cur] = append(parts[cur], w)
	}
	nameWords := parts["name"]
	if len(nameWords) == 0 {
		return Command{}, usage
	}
	spec := combat.EffectSpec{}
	if len(nameWords) > 1 {
		if n, err := strconv.Atoi(nameWords[len(nameWords)-1]); err == nil {
			if n < 1 {
				return Command{}, errors.New("duration must be at least 1 round")
			}
			spec.Rounds, nameWords = &n, nameWords[:len(nameWords)-1]
		}
	}
	spec.Name = strings.Join(nameWords, " ")
	if src, ok := parts["from"]; ok {
		id, err := resolve(enc, strings.Join(src, " "), combatantRow)
		if err != nil {
			return Command{}, fmt.Errorf("source: %w", err)
		}
		spec.SourceID = id
	}
	if at, ok := parts["at"]; ok {
		v, err := combat.ParseInitiative(strings.Join(at, ""))
		if err != nil {
			return Command{}, err
		}
		spec.Initiative = &v
	}
	if spec.SourceID == 0 && spec.Initiative == nil {
		return Command{}, errors.New("an effect needs a source (from NAME) or an initiative (at N)")
	}
	if on, ok := parts["on"]; ok {
		ids, err := resolveList(enc, strings.Join(on, " "), combatantRow)
		if err != nil {
			return Command{}, err
		}
		spec.Targets = ids
	}
	created := new(combat.RowID)
	return Command{Created: created, Op: func(e *combat.Encounter) ([]combat.Event, error) {
		id, evs, err := e.AddEffect(spec)
		*created = id
		return evs, err
	}}, nil
}

func parseFXTargets(enc *combat.Encounter, verb string, words []string) (Command, error) {
	if len(words) < 2 {
		return Command{}, fmt.Errorf("usage: :%s FX TARGETS", verb)
	}
	fx, rest, err := splitTarget(enc, words, effectRow, 1)
	if err != nil {
		return Command{}, err
	}
	if len(fx) != 1 {
		return Command{}, fmt.Errorf(":%s takes one effect", verb)
	}
	ids, err := resolveList(enc, strings.Join(rest, " "), combatantRow)
	if err != nil {
		return Command{}, err
	}
	id := fx[0]
	return Command{Focus: id, Op: func(e *combat.Encounter) ([]combat.Event, error) {
		if verb == "fxadd" {
			return e.AddTargets(id, ids)
		}
		return e.RemoveTargets(id, ids)
	}}, nil
}

// groupToken matches a group size such as "x4" or "×4".
var groupToken = regexp.MustCompile(`^[xX×](\d+)$`)

func parseAdd(enc *combat.Encounter, rest string) (Command, error) {
	usage := errors.New("usage: :add NAME [xN] INIT HP [pc|npc] [-- NOTE]")
	head, note, _ := strings.Cut(rest, "--")
	words := strings.Fields(head)
	kind := combat.NPC
	if n := len(words); n > 0 {
		switch strings.ToLower(words[n-1]) {
		case "pc":
			kind, words = combat.PC, words[:n-1]
		case "npc":
			words = words[:n-1]
		}
	}
	if len(words) < 3 {
		return Command{}, usage
	}
	hpText := strings.TrimSuffix(strings.ToLower(words[len(words)-1]), "hp")
	initText := words[len(words)-2]
	nameWords := words[:len(words)-2]

	count := 0
	if g := groupToken.FindStringSubmatch(nameWords[len(nameWords)-1]); g != nil && len(nameWords) > 1 {
		count, _ = strconv.Atoi(g[1])
		if count < 1 || count > 999 {
			return Command{}, errors.New("group size must be 1–999")
		}
		nameWords = nameWords[:len(nameWords)-1]
	}
	name := strings.Join(nameWords, " ")

	cur, maxHP, temp, err := ParseHP(hpText, enc.Rules().HPFloor)
	if err != nil {
		return Command{}, err
	}
	init, inits, err := ParseAddInitiative(initText, count)
	if err != nil {
		return Command{}, err
	}
	spec := combat.CombatantSpec{Name: name, Kind: kind, Initiative: init, MaxHP: maxHP, HP: cur, TempHP: temp, Note: strings.TrimSpace(note)}
	created := new(combat.RowID)
	return Command{Created: created, Op: func(e *combat.Encounter) ([]combat.Event, error) {
		if count == 0 {
			id, evs, err := e.Add(spec)
			*created = id
			return evs, err
		}
		ids, evs, err := e.AddGroup(combat.GroupSpec{CombatantSpec: spec, Count: count, Initiatives: inits})
		if len(ids) > 0 {
			*created = ids[0]
		}
		return evs, err
	}}, nil
}

// ParseAddInitiative reads one initiative, or for a group (count > 0) either
// one whole number to share with auto-filled decimals or one value per
// member, comma-separated (D25).
func ParseAddInitiative(text string, count int) (combat.Initiative, []combat.Initiative, error) {
	parts := strings.Split(text, ",")
	if count == 0 || len(parts) == 1 {
		v, err := combat.ParseInitiative(text)
		if err != nil {
			return 0, nil, err
		}
		if count > 0 {
			if _, err := combat.GroupFill(v, count); err != nil {
				return 0, nil, errors.New("a shared group initiative must be a whole number; or give one value per member, separated by commas")
			}
		}
		return v, nil, nil
	}
	if len(parts) != count {
		return 0, nil, fmt.Errorf("need %d initiatives, got %d", count, len(parts))
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

// ParseHP reads "44" (full health) or "31/44", either with an optional
// "+5" of temp HP on the end (R3). The current HP can go down to floor, the
// ruleset's HP floor, so "-4/22" is accepted in 3.5e (RS2).
func ParseHP(text string, floor int) (cur *int, maxHP, temp int, err error) {
	text, tempS, hasTemp := strings.Cut(strings.TrimSpace(text), "+")
	if hasTemp {
		temp, err = strconv.Atoi(strings.TrimSpace(tempS))
		if err != nil || temp < 0 {
			return nil, 0, 0, errors.New("temp HP must be a whole number of 0 or more")
		}
	}
	curS, maxS, split := strings.Cut(text, "/")
	if !split {
		maxS = curS
	}
	maxHP, err = strconv.Atoi(strings.TrimSpace(maxS))
	if err != nil || maxHP < 1 {
		return nil, 0, 0, errors.New("max HP must be a whole number of 1 or more")
	}
	if split {
		n, err := strconv.Atoi(strings.TrimSpace(curS))
		if err != nil || n < floor || n > maxHP {
			if floor < 0 {
				return nil, 0, 0, fmt.Errorf("current HP must be from %d to %d", floor, maxHP)
			}
			return nil, 0, 0, fmt.Errorf("current HP must be %d–%d", floor, maxHP)
		}
		cur = &n
	}
	return cur, maxHP, temp, nil
}

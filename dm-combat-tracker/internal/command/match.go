package command

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

// rowKind narrows which rows a name may match.
type rowKind int

const (
	anyRow rowKind = iota
	combatantRow
	effectRow
)

func (k rowKind) noun() string {
	switch k {
	case combatantRow:
		return "combatant"
	case effectRow:
		return "effect"
	}
	return "row"
}

// AmbiguousError is returned when a name matches more than one row.
type AmbiguousError struct {
	Query      string
	Candidates []string
}

func (a *AmbiguousError) Error() string {
	return fmt.Sprintf("%q matches %s; be more specific", a.Query, strings.Join(a.Candidates, ", "))
}

// normalize lower-cases s and drops spaces: "Goblin 2" → "goblin2".
func normalize(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if !unicode.IsSpace(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// splitNumber splits a trailing number off: "goblin12" → "goblin", "12".
func splitNumber(s string) (base, num string) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	return s[:i], s[i:]
}

// resolve finds the one row of the given kind that query names. Matching is
// case-insensitive with spaces optional (D27): an exact name wins; otherwise
// a name prefix ("band" → Bandit Captain) or a short base with the number
// ("gob2", "g2" → Goblin 2).
func resolve(enc *combat.Encounter, query string, kind rowKind) (combat.RowID, error) {
	q := normalize(query)
	if q == "" {
		return 0, fmt.Errorf("missing %s name", kind.noun())
	}
	qBase, qNum := splitNumber(q)
	var exact, loose []combat.Row
	for _, r := range enc.Order() {
		if (kind == combatantRow && r.Combatant == nil) || (kind == effectRow && r.Effect == nil) {
			continue
		}
		n := normalize(r.Name())
		if n == q {
			exact = append(exact, r)
			continue
		}
		nBase, nNum := splitNumber(n)
		switch {
		case strings.HasPrefix(n, q):
			loose = append(loose, r)
		case qNum != "" && qBase != "" && nNum == qNum && strings.HasPrefix(nBase, qBase):
			loose = append(loose, r)
		}
	}
	pick := exact
	if len(pick) == 0 {
		pick = loose
	}
	switch len(pick) {
	case 0:
		return 0, fmt.Errorf("no %s matches %q", kind.noun(), strings.TrimSpace(query))
	case 1:
		return pick[0].ID(), nil
	}
	names := make([]string, len(pick))
	for i, r := range pick {
		names[i] = r.Name()
	}
	return 0, &AmbiguousError{Query: strings.TrimSpace(query), Candidates: names}
}

// resolveList resolves a comma-separated list of names.
func resolveList(enc *combat.Encounter, text string, kind rowKind) ([]combat.RowID, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("missing %s name", kind.noun())
	}
	var out []combat.RowID
	for _, part := range strings.Split(text, ",") {
		id, err := resolve(enc, part, kind)
		if err != nil {
			return nil, err
		}
		dup := false
		for _, x := range out {
			dup = dup || x == id
		}
		if !dup {
			out = append(out, id)
		}
	}
	return out, nil
}

// splitTarget finds where a single target ends and the rest of the arguments
// begin, for commands like ":note goblin 1 tired, hungry" where both parts can
// contain spaces.
func splitTarget(enc *combat.Encounter, words []string, kind rowKind, minRest int) ([]combat.RowID, []string, error) {
	return splitTargets(enc, words, kind, minRest, false)
}

// splitTargetList is splitTarget for commands whose target argument is a
// comma-separated list and whose trailing arguments never contain commas
// (such as :cond after stripping "-- NOTE"), so every comma belongs to the
// target list.
func splitTargetList(enc *combat.Encounter, words []string, kind rowKind, minRest int) ([]combat.RowID, []string, error) {
	return splitTargets(enc, words, kind, minRest, true)
}

func splitTargets(enc *combat.Encounter, words []string, kind rowKind, minRest int, allCommasInTargets bool) ([]combat.RowID, []string, error) {
	minK := 1
	if allCommasInTargets {
		for i, w := range words {
			if strings.Contains(w, ",") {
				minK = i + 1
				if strings.HasSuffix(w, ",") {
					minK = i + 2
				}
			}
		}
	}
	var ambErr, firstErr, fallbackErr error
	for k := len(words) - minRest; k >= minK; k-- {
		ids, err := resolveList(enc, strings.Join(words[:k], " "), kind)
		if err == nil {
			return ids, words[k:], nil
		}
		if _, ok := err.(*AmbiguousError); ok && ambErr == nil {
			ambErr = err
		}
		if !strings.HasSuffix(words[k-1], ",") {
			firstErr = err
		}
		fallbackErr = err
	}
	switch {
	case ambErr != nil:
		return nil, nil, ambErr
	case firstErr != nil:
		return nil, nil, firstErr
	case fallbackErr != nil:
		return nil, nil, fallbackErr
	}
	return nil, nil, fmt.Errorf("missing %s name", kind.noun())
}

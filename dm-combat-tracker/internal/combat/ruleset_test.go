package combat

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func TestRulesetLookup(t *testing.T) {
	for in, want := range map[string]string{"5e": "5e", "5E": "5e", " 5 ": "5e", "3.5e": "3.5e", "3.5": "3.5e"} {
		if r, ok := LookupRuleset(in); !ok || r.Name != want {
			t.Errorf("LookupRuleset(%q) = %q, %v", in, r.Name, ok)
		}
	}
	for _, in := range []string{"", "4e", "pathfinder", "35"} {
		if _, ok := LookupRuleset(in); ok {
			t.Errorf("LookupRuleset(%q) should fail", in)
		}
	}
	if got := RulesetNames(); !slices.Equal(got, []string{"5e", "3.5e"}) || got[0] != DefaultRuleset {
		t.Errorf("names %v", got)
	}
}

func TestSetRuleset(t *testing.T) {
	e := newEnc()
	if e.Ruleset != "5e" || e.Rules().Name != "5e" {
		t.Fatalf("a new encounter should be 5e, got %q", e.Ruleset)
	}
	wantKinds(t, must(t)(e.SetRuleset("5E"))) // already chosen: nothing happens

	evs := must(t)(e.SetRuleset("3.5"))
	wantKinds(t, evs, "RulesetChanged")
	if ch := evs[0].(RulesetChanged); ch.From != "5e" || ch.To != "3.5e" || e.Ruleset != "3.5e" {
		t.Errorf("change %+v, ruleset %q", ch, e.Ruleset)
	}
	_, err := e.SetRuleset("4e")
	wantErr(t, err, ErrInvalid)
	if !strings.Contains(err.Error(), `unknown ruleset "4e" (choose 5e or 3.5e)`) || e.Ruleset != "3.5e" {
		t.Errorf("err %v, ruleset %q", err, e.Ruleset)
	}

	// Once combat starts the choice is fixed (RS1), though naming the
	// current ruleset is still fine.
	add(t, e, "Orc", "12", 20)
	must(t)(e.Start())
	_, err = e.SetRuleset("5e")
	wantErr(t, err, ErrNotSetup)
	wantKinds(t, must(t)(e.SetRuleset("3.5e")))
	if e.Ruleset != "3.5e" {
		t.Errorf("ruleset %q", e.Ruleset)
	}
}

func TestRulesetConditions(t *testing.T) {
	five, _ := LookupRuleset("5e")
	three, _ := LookupRuleset("3.5e")
	if !slices.Contains(five.Conditions, "Poisoned") || slices.Contains(five.Conditions, "Shaken") {
		t.Errorf("5e list: %v", five.Conditions)
	}
	for _, c := range []string{"Shaken", "Sickened", "Nauseated", "Dazed", "Fatigued", "Stable", "Concentrating"} {
		if !slices.Contains(three.Conditions, c) {
			t.Errorf("3.5e list is missing %s", c)
		}
	}
	// HP states are shown from HP, not picked (RS8).
	for _, c := range []string{"Disabled", "Dying", "Dead", "Staggered", "Poisoned"} {
		if slices.Contains(three.Conditions, c) {
			t.Errorf("3.5e list shouldn't offer %s", c)
		}
	}
	if got := three.CanonicalCondition("flat-footed"); got != "Flat-Footed" {
		t.Errorf("canonical: %q", got)
	}
	if got := three.CanonicalCondition("poisoned"); got != "poisoned" {
		t.Errorf("a name outside the preset stays as typed, got %q", got)
	}
}

// An encounter with no ruleset field (a state file from before rulesets)
// behaves as 5e.
func TestMissingRulesetIsDefault(t *testing.T) {
	var e Encounter
	if err := json.Unmarshal([]byte(`{"schemaVersion":1,"status":"setup"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Rules().Name != "5e" {
		t.Errorf("rules %q", e.Rules().Name)
	}
}

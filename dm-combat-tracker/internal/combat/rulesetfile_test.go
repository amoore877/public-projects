package combat

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

const pfFile = `{
  "name": "pf1",
  "hpFloor": -20,
  "nonlethal": true,
  "states": [
    {"name": "disabled", "tag": "dsbld", "min": 0, "max": 0, "acts": true},
    {"name": "dying", "min": -19, "max": -1, "remind": true},
    {"name": "dead", "max": -20}
  ],
  "conditions": [" Shaken ", "Sickened"]
}`

func useRulesets(t *testing.T, rs ...Ruleset) {
	t.Helper()
	if err := UseRulesets(rs); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = UseRulesets(nil) })
}

func TestParseRuleset(t *testing.T) {
	r, err := ParseRuleset([]byte(pfFile))
	if err != nil {
		t.Fatal(err)
	}
	if r.Name != "pf1" || r.HPFloor != -20 || !r.Nonlethal || r.FixedAnchor || len(r.States) != 3 {
		t.Fatalf("parsed %+v", r)
	}
	if s := r.State(-20); s == nil || s.Name != "dead" {
		t.Errorf("state at -20: %+v", s)
	}
	if s := r.State(0); s == nil || !s.Acts || s.Label() != "DSBLD" {
		t.Errorf("state at 0: %+v", s)
	}
	if !slices.Equal(r.Conditions, []string{"Shaken", "Sickened"}) {
		t.Errorf("conditions are trimmed: %q", r.Conditions)
	}

	// The smallest file: a name. No states, no presets, floor 0.
	r, err = ParseRuleset([]byte(`{"name":"bare"}`))
	if err != nil || r.HPFloor != 0 || r.State(-5) != nil {
		t.Errorf("bare: %+v, %v", r, err)
	}
}

func TestParseRulesetRefusals(t *testing.T) {
	for file, want := range map[string]string{
		`{`:                         "unexpected EOF",
		`{"name":"x"} {}`:           "unexpected data after the ruleset",
		`{"name":"x","hpFlor":-10}`: `unknown field "hpFlor"`,
		`{"name":"x","states":[{"name":"down","maximum":0}]}`: `unknown field "maximum"`,
		`{"hpFloor":-10}`:                                                                                  `"name" is required`,
		`{"name":"pathfinder1e"}`:                                                                          "longer than 10 characters",
		`{"name":"my rules"}`:                                                                              "has a space in it",
		`{"name":"x","hpFloor":5}`:                                                                         "can't be above 0",
		`{"name":"x","states":[{"max":0}]}`:                                                                "state 1 has no name",
		`{"name":"x","states":[{"name":"bleeding","max":0}]}`:                                              `state "bleeding" needs a "tag" of at most 5`,
		`{"name":"x","states":[{"name":"bleeding","tag":"bleedy","max":0}]}`:                               `has tag "bleedy"`,
		`{"name":"x","states":[{"name":"Staggered","max":0}]}`:                                             "reserved for nonlethal damage",
		`{"name":"x","states":[{"name":"down","min":0,"max":-1}]}`:                                         "min 0 above max -1",
		`{"name":"x","states":[{"name":"gone","max":-11}],"hpFloor":-10}`:                                  "below the HP floor -10",
		`{"name":"x","states":[{"name":"down","max":0},{"name":"DOWN","min":5}]}`:                          `state "DOWN" is listed twice`,
		`{"name":"x","hpFloor":-10,"states":[{"name":"down","max":0},{"name":"dying","min":-5,"max":-1}]}`: `states "down" and "dying" cover some`,
		`{"name":"x","conditions":["Prone",""]}`:                                                           "condition 2 is blank",
		`{"name":"x","conditions":["Prone","prone "]}`:                                                     `condition "prone" is listed twice`,
	} {
		if _, err := ParseRuleset([]byte(file)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err %v, want %q", file, err, want)
		}
	}
}

func TestOverlap(t *testing.T) {
	st := func(lo, hi *int) *HealthState { return &HealthState{Min: lo, Max: hi} }
	for _, tc := range []struct {
		a, b *HealthState
		want bool
	}{
		{st(nil, intPtr(0)), st(intPtr(1), nil), false},
		{st(nil, intPtr(0)), st(intPtr(0), nil), true},
		{st(nil, nil), st(intPtr(3), intPtr(3)), true},
		{st(intPtr(-9), intPtr(-1)), st(intPtr(0), intPtr(0)), false},
		{st(intPtr(0), intPtr(0)), st(intPtr(-9), intPtr(-1)), false},
		{st(nil, intPtr(-10)), st(nil, intPtr(-20)), true},
	} {
		if got := overlap(tc.a, tc.b); got != tc.want {
			t.Errorf("overlap(%v-%v, %v-%v) = %v", tc.a.Min, tc.a.Max, tc.b.Min, tc.b.Max, got)
		}
	}
}

// The built-ins come from the same file form, with the v1 values (RS9).
func TestBuiltinsFromFiles(t *testing.T) {
	five, _ := LookupRuleset("5e")
	if five.HPFloor != 0 || five.Nonlethal || five.FixedAnchor || len(five.States) != 1 || five.State(0).Name != "down" || five.State(1) != nil {
		t.Errorf("5e: %+v", five)
	}
	three, _ := LookupRuleset("3.5e")
	if three.HPFloor != -10 || !three.Nonlethal || !three.FixedAnchor {
		t.Errorf("3.5e: %+v", three)
	}
	for hp, want := range map[int]string{1: "", 0: "disabled", -1: "dying", -9: "dying", -10: "dead"} {
		if got := stateName(three.State(hp)); got != want {
			t.Errorf("3.5e at %d: %q, want %q", hp, got, want)
		}
	}
	if s := three.State(-5); !s.Remind || s.Acts {
		t.Errorf("dying: %+v", s)
	}
}

func TestMustParseBuiltinsPanicsOnABadFile(t *testing.T) {
	old := builtinOrder
	t.Cleanup(func() { builtinOrder = old })
	builtinOrder = []string{"rulesets/missing.json"}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "built-in ruleset rulesets/missing.json") {
			t.Errorf("recovered %v", r)
		}
	}()
	mustParseBuiltins()
}

func TestUseRulesets(t *testing.T) {
	pf, err := ParseRuleset([]byte(pfFile))
	if err != nil {
		t.Fatal(err)
	}
	useRulesets(t, pf)
	if got := RulesetNames(); !slices.Equal(got, []string{"5e", "3.5e", "pf1"}) {
		t.Errorf("names %v", got)
	}
	if r, ok := LookupRuleset("PF1"); !ok || r.HPFloor != -20 {
		t.Errorf("lookup: %+v %v", r, ok)
	}
	if r, ok := LookupRuleset("pf1e"); !ok || r.Name != "pf1" {
		t.Errorf("the trailing e is optional for files too: %+v %v", r, ok)
	}

	e := newEnc()
	wantKinds(t, must(t)(e.SetRuleset("pf1")), "RulesetChanged")
	if e.Rules().HPFloor != -20 {
		t.Errorf("rules %+v", e.Rules())
	}
	_, err = e.SetRuleset("4e")
	if !errors.Is(err, ErrUnknownRuleset) || !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "(choose 5e or 3.5e or pf1)") {
		t.Errorf("err %v", err)
	}

	// Names can't clash with a built-in or each other, and a refusal
	// leaves the loaded set as it was.
	for _, names := range [][]string{{"5E"}, {"3.5"}, {"pf", "PFe"}} {
		var rs []Ruleset
		for _, n := range names {
			rs = append(rs, Ruleset{Name: n})
		}
		if err := UseRulesets(rs); err == nil || !strings.Contains(err.Error(), "has the same name as") {
			t.Errorf("%v: err %v", names, err)
		}
	}
	if _, ok := LookupRuleset("pf1"); !ok {
		t.Error("a refused set replaced the loaded one")
	}

	must0(t, UseRulesets(nil))
	if _, ok := LookupRuleset("pf1"); ok || len(RulesetNames()) != 2 {
		t.Error("UseRulesets(nil) should leave only the built-ins")
	}
}

func must0(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

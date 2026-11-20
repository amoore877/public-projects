package dice

import (
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
)

func seeded() *rand.Rand { return rand.New(rand.NewPCG(1, 2)) }

func TestParse(t *testing.T) {
	for in, want := range map[string]string{
		"4d6+4":           "4d6+4",
		"2d6+2+1d4":       "2d6+2+1d4",
		"d20":             "1d20",
		"d20+5":           "1d20+5",
		"D8 - 1":          "1d8-1",
		" 2D6 + 1d4 - 2 ": "2d6+1d4-2",
		"10-1d4":          "10-1d4",
		"-1d4+10":         "-1d4+10",
		"+3":              "3",
		"7":               "7",
		"100d1000":        "100d1000",
		"d2":              "1d2",
		"1d6+1d6+1d6+1d6": "1d6+1d6+1d6+1d6",
		"2\t+d6":          "2+1d6",
		"  d20":           "1d20",
		"0003d06":         "3d6",
	} {
		e, err := Parse(in)
		if err != nil {
			t.Errorf("Parse(%q): %v", in, err)
			continue
		}
		if got := e.String(); got != want {
			t.Errorf("Parse(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTerms(t *testing.T) {
	e, err := Parse("2d6+2-d4")
	if err != nil {
		t.Fatal(err)
	}
	want := []Term{{Sign: 1, Count: 2, Sides: 6}, {Sign: 1, Value: 2}, {Sign: -1, Count: 1, Sides: 4}}
	if !reflect.DeepEqual(e.Terms, want) {
		t.Errorf("terms %+v, want %+v", e.Terms, want)
	}
}

func TestParseRefuses(t *testing.T) {
	for in, want := range map[string]string{
		"":                             "type dice to roll",
		"   ":                          "type dice to roll",
		"4d":                           `the number of sides is missing after "4d"`,
		"d":                            `the number of sides is missing after "d"`,
		"4dd6":                         `the number of sides is missing after "4d"`,
		"2d6+":                         `something to add is missing after "2d6+"`,
		"2d6-":                         "something to add is missing",
		"2d6++3":                       `'+' can't follow "2d6+"`,
		"2d6d4":                        `'d' can't follow "2d6"`,
		"2d6 3":                        `put + or - between "2d6" and '3'`,
		"2 d6":                         "put + or - between",
		"2d 6":                         "put + or - between",
		"x":                            "a roll can't start with 'x'",
		"2d6*2":                        `'*' can't follow "2d6"`,
		"4d6kh3":                       `'k' can't follow "4d6"`,
		"d%":                           "the number of sides is missing",
		"0d6":                          `"0d" rolls no dice`,
		"101d6":                        "100 dice in a term at most",
		"1d1001":                       "1000 sides on a die at most",
		"1d1":                          "a die needs 2 sides or more",
		"1d0":                          "a die needs 2 sides or more",
		"10001":                        "10000 a number at most",
		"99999999999999999999d6":       "100 dice in a term at most",
		"2d6+é":                        `'é' can't follow "2d6+"`,
		strings.Repeat("1+", 20) + "1": "20 terms at most",
	} {
		_, err := Parse(in)
		if err == nil {
			t.Errorf("Parse(%q) should be refused", in)
			continue
		}
		if !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%q): %q, want it to contain %q", in, err, want)
		}
	}
	if _, err := Parse(strings.Repeat("1+", 19) + "1"); err != nil {
		t.Errorf("20 terms: %v", err)
	}
}

func TestRoll(t *testing.T) {
	e, err := Parse("2d6+2+1d4-1d3")
	if err != nil {
		t.Fatal(err)
	}
	r := e.Roll(seeded())
	// The same seed rolls the same faces in the same order.
	again := e.Roll(seeded())
	if !reflect.DeepEqual(r, again) {
		t.Errorf("not repeatable: %+v vs %+v", r, again)
	}
	if len(r.Terms) != 4 || len(r.Terms[0].Faces) != 2 || r.Terms[1].Faces != nil || len(r.Terms[2].Faces) != 1 {
		t.Fatalf("terms %+v", r.Terms)
	}
	total := 0
	for _, rt := range r.Terms {
		for _, f := range rt.Faces {
			if f < 1 || f > rt.Term.Sides {
				t.Errorf("face %d out of 1–%d", f, rt.Term.Sides)
			}
		}
		total += rt.Sum()
	}
	want := r.Terms[0].Faces[0] + r.Terms[0].Faces[1] + 2 + r.Terms[2].Faces[0] - r.Terms[3].Faces[0]
	if r.Total != total || r.Total != want {
		t.Errorf("total %d, sum of terms %d, want %d", r.Total, total, want)
	}
	if got := r.String(); !strings.HasPrefix(got, "2d6+2+1d4-1d3 = ") {
		t.Errorf("String %q", got)
	}
}

// Every face comes up, and nothing outside the die.
func TestRollCoversTheDie(t *testing.T) {
	e, _ := Parse("100d6")
	seen := map[int]bool{}
	r := e.Roll(seeded())
	for _, f := range r.Terms[0].Faces {
		seen[f] = true
	}
	if len(seen) != 6 || !seen[1] || !seen[6] {
		t.Errorf("faces seen %v", seen)
	}
}

func TestRollNumberOnly(t *testing.T) {
	e, _ := Parse("10-3")
	if r := e.Roll(seeded()); r.Total != 7 || r.String() != "10-3 = 7" {
		t.Errorf("%+v", r)
	}
}

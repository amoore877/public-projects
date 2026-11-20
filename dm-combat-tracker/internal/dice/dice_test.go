package dice

import (
	"fmt"
	"math"
	"math/rand/v2"
	"reflect"
	"strings"
	"testing"
	"time"
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

func TestBreakdown(t *testing.T) {
	roll := func(expr string, faces ...[]int) Result {
		e, err := Parse(expr)
		if err != nil {
			t.Fatal(err)
		}
		r := Result{Expr: e}
		k := 0
		for _, term := range e.Terms {
			rt := Rolled{Term: term}
			if term.Dice() {
				rt.Faces = faces[k]
				k++
			}
			r.Terms = append(r.Terms, rt)
			r.Total += rt.Sum()
		}
		return r
	}
	for _, c := range []struct {
		r    Result
		want string
	}{
		{roll("2d6+2+1d4", []int{5, 3}, []int{4}), "[5 3] +2 [4]"},
		{roll("4d6+4", []int{6, 1, 3, 3}), "[6 1 3 3] +4"},
		{roll("1d8-1", []int{6}), "[6] -1"},
		{roll("10-1d4", []int{3}), "10 -[3]"},
		{roll("-1d4+10", []int{2}), "-[2] +10"},
		{roll("-3+d20", []int{20}), "-3 [20]"},
		{roll("7"), "7"},
	} {
		if got := c.r.Breakdown(); got != c.want {
			t.Errorf("%s: %q, want %q", c.r.Expr, got, c.want)
		}
	}
	// A real roll's breakdown matches its faces, in roll order.
	e, _ := Parse("3d20")
	r := e.Roll(seeded())
	f := r.Terms[0].Faces
	if want := fmt.Sprintf("[%d %d %d]", f[0], f[1], f[2]); r.Breakdown() != want {
		t.Errorf("%q, want %q", r.Breakdown(), want)
	}
}

// Across many rolls of every common die (and the largest allowed), every
// face stays within 1..sides, every face comes up, and totals stay in
// range. This runs on a fixed seed and on the clock-seeded source the app
// uses; with about 200 of each face expected, a face missing by chance is
// out of the question (e^-200).
//
// The spread check (chi-square against a fair die) runs on the fixed seed
// only, so the test always gives the same answer.
func TestRollBoundsAndSpread(t *testing.T) {
	const perFace = 200 // expected count of each face
	for _, src := range []struct {
		name   string
		r      *rand.Rand
		spread bool
	}{
		{"seeded", seeded(), true},
		{"clock", rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 1)), false},
	} {
		for _, sides := range []int{2, 3, 4, 6, 8, 10, 12, 20, 100, MaxSides} {
			e := Expr{Terms: []Term{{Sign: 1, Count: MaxDice, Sides: sides}, {Sign: -1, Value: 1}}}
			counts := make([]int, sides+1)
			rolls := perFace * sides / MaxDice
			for range rolls {
				res := e.Roll(src.r)
				if lo, hi := MaxDice-1, MaxDice*sides-1; res.Total < lo || res.Total > hi {
					t.Fatalf("%s %s: total %d outside %d–%d", src.name, e, res.Total, lo, hi)
				}
				for _, f := range res.Terms[0].Faces {
					if f < 1 || f > sides {
						t.Fatalf("%s d%d: face %d out of range", src.name, sides, f)
					}
					counts[f]++
				}
			}
			n := float64(rolls * MaxDice)
			want := n / float64(sides)
			chi := 0.0
			for f := 1; f <= sides; f++ {
				if counts[f] == 0 {
					t.Errorf("%s d%d: face %d never came up in %.0f rolls", src.name, sides, f, n)
				}
				d := float64(counts[f]) - want
				chi += d * d / want
			}
			if limit := chiSquareLimit(sides - 1); src.spread && chi > limit {
				t.Errorf("%s d%d: chi-square %.1f over %.1f, faces look uneven: %v", src.name, sides, chi, limit, counts[1:])
			}
		}
	}
}

// chiSquareLimit is roughly the value a fair die's chi-square exceeds one
// time in ten million (z = 5.2), by the Wilson–Hilferty approximation. It
// errs high for few degrees of freedom (33.6 against the exact 28.4 for
// one), which only makes the check more lenient.
func chiSquareLimit(df int) float64 {
	k := float64(df)
	c := 2 / (9 * k)
	return k * math.Pow(1-c+5.2*math.Sqrt(c), 3)
}

// The approximation is never stricter than the exact one-in-ten-million
// point (so a fair die can't fail more often than that), and not much looser.
// Exact values computed from the regularized incomplete gamma function.
func TestChiSquareLimit(t *testing.T) {
	for df, exact := range map[int]float64{1: 28.37, 2: 32.24, 5: 40.86, 11: 54.39, 19: 69.78, 99: 190.23, 999: 1249.01} {
		if got := chiSquareLimit(df); got < exact || got > exact*1.2 {
			t.Errorf("df %d: %.2f, want %.2f–%.2f", df, got, exact, exact*1.2)
		}
	}
}

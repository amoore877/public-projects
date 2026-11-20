// Package dice parses and rolls dice expressions such as "2d6+2+1d4"
// (DR1–DR3, D46). It is pure: no I/O, and the random source is passed in.
// A roll never touches the encounter (D45).
package dice

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"unicode"
)

// Limits on an expression (D46). They catch typos like "20d" or "1d2000"
// and keep a roll's breakdown short enough to read.
const (
	MaxDice   = 100   // dice in one term
	MaxSides  = 1000  // sides on one die
	MinSides  = 2     // a one-sided die isn't a roll
	MaxTerms  = 20    // terms in one expression
	MaxNumber = 10000 // a whole-number term
)

// Term is one part of an expression: Count dice of Sides sides, or the whole
// number Value when Count is 0. Sign is +1 or -1.
type Term struct {
	Sign  int
	Count int
	Sides int
	Value int
}

// Dice reports whether the term rolls dice.
func (t Term) Dice() bool { return t.Count > 0 }

// Expr is a parsed expression: its terms in the order typed.
type Expr struct {
	Terms []Term
}

// Rolled is one term after a roll.
type Rolled struct {
	Term  Term
	Faces []int // each die's face in roll order; nil for a whole-number term
}

// Sum is the term's signed contribution to the total.
func (r Rolled) Sum() int {
	if !r.Term.Dice() {
		return r.Term.Sign * r.Term.Value
	}
	n := 0
	for _, f := range r.Faces {
		n += f
	}
	return r.Term.Sign * n
}

// Result is a rolled expression.
type Result struct {
	Expr  Expr
	Total int
	Terms []Rolled // in the order typed
}

// Parse reads an expression: terms (NdS or a whole number) joined by + or -.
// A "d" with no count means one die; spaces and letter case are ignored.
func Parse(s string) (Expr, error) {
	// Spaces are dropped, except that one between two parts of a term or
	// two terms ("2d6 3", "2 d6") is refused: dropping it would read 2d63.
	var b strings.Builder
	space := false
	for _, r := range s {
		r = unicode.ToLower(r)
		switch {
		case unicode.IsSpace(r):
			space = b.Len() > 0
			continue
		case space && termChar(r) && termChar(rune(b.String()[b.Len()-1])):
			return Expr{}, fmt.Errorf("put + or - between %q and %q", b.String(), r)
		}
		space = false
		b.WriteRune(r)
	}
	src := b.String()
	if src == "" {
		return Expr{}, errors.New("type dice to roll, e.g. 2d6+3")
	}

	var e Expr
	i := 0
	for i < len(src) {
		sign := 1
		switch src[i] {
		case '+':
			i++
		case '-':
			sign = -1
			i++
		default:
			if len(e.Terms) > 0 {
				return Expr{}, unexpected(src, i)
			}
		}
		if i == len(src) {
			return Expr{}, fmt.Errorf("something to add is missing after %q", src)
		}
		t, next, err := parseTerm(src, i)
		if err != nil {
			return Expr{}, err
		}
		t.Sign = sign
		e.Terms = append(e.Terms, t)
		if len(e.Terms) > MaxTerms {
			return Expr{}, fmt.Errorf("%d terms at most in a roll", MaxTerms)
		}
		i = next
	}
	return e, nil
}

// parseTerm reads "N", "NdS" or "dS" starting at i.
func parseTerm(src string, i int) (Term, int, error) {
	count, j := digits(src, i)
	if j == len(src) || src[j] != 'd' {
		if j == i {
			return Term{}, 0, unexpected(src, i)
		}
		n, err := number(count, MaxNumber, "a number")
		if err != nil {
			return Term{}, 0, err
		}
		return Term{Value: n}, j, nil
	}
	n := 1
	if count != "" {
		var err error
		if n, err = number(count, MaxDice, "dice in a term"); err != nil {
			return Term{}, 0, err
		}
		if n < 1 {
			return Term{}, 0, fmt.Errorf("%q rolls no dice", src[i:j+1])
		}
	}
	sides, k := digits(src, j+1)
	if sides == "" {
		return Term{}, 0, fmt.Errorf("the number of sides is missing after %q", src[:j+1])
	}
	s, err := number(sides, MaxSides, "sides on a die")
	if err != nil {
		return Term{}, 0, err
	}
	if s < MinSides {
		return Term{}, 0, fmt.Errorf("a die needs %d sides or more", MinSides)
	}
	return Term{Count: n, Sides: s}, k, nil
}

// digits returns the run of ASCII digits starting at i and the index after it.
func digits(src string, i int) (string, int) {
	j := i
	for j < len(src) && src[j] >= '0' && src[j] <= '9' {
		j++
	}
	return src[i:j], j
}

// number converts a run of digits, refusing anything above limit.
func number(s string, limit int, what string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil || n > limit {
		return 0, fmt.Errorf("%d %s at most", limit, what)
	}
	return n, nil
}

func unexpected(src string, i int) error {
	r := []rune(src[i:])[0]
	if i == 0 {
		return fmt.Errorf("a roll can't start with %q", r)
	}
	return fmt.Errorf("%q can't follow %q", r, src[:i])
}

// String writes the expression in its normal form: "d20 + 5" is "1d20+5".
func (e Expr) String() string {
	var b strings.Builder
	for i, t := range e.Terms {
		switch {
		case t.Sign < 0:
			b.WriteByte('-')
		case i > 0:
			b.WriteByte('+')
		}
		if t.Dice() {
			fmt.Fprintf(&b, "%dd%d", t.Count, t.Sides)
		} else {
			b.WriteString(strconv.Itoa(t.Value))
		}
	}
	return b.String()
}

// Roll rolls every die in order, using r.
func (e Expr) Roll(r *rand.Rand) Result {
	res := Result{Expr: e, Terms: make([]Rolled, len(e.Terms))}
	for i, t := range e.Terms {
		rt := Rolled{Term: t}
		if t.Dice() {
			rt.Faces = make([]int, t.Count)
			for j := range rt.Faces {
				rt.Faces[j] = r.IntN(t.Sides) + 1
			}
		}
		res.Terms[i] = rt
		res.Total += rt.Sum()
	}
	return res
}

// String is the result as the status line shows it: "2d6+2+1d4 = 14".
func (r Result) String() string {
	return fmt.Sprintf("%s = %d", r.Expr, r.Total)
}

// termChar reports whether r can be part of a term: a digit or "d".
func termChar(r rune) bool { return r == 'd' || (r >= '0' && r <= '9') }

// Breakdown lists what each term rolled, in set order (DR4, D48): terms as
// typed, each term's faces in roll order. A dice term is "[5 3]", or
// "-[3]" when subtracted; a whole number keeps its sign ("+2", "-1"), except
// a positive first term. "2d6+2+1d4" might give "[5 3] +2 [4]".
func (r Result) Breakdown() string {
	parts := make([]string, len(r.Terms))
	for i, rt := range r.Terms {
		var s string
		if rt.Term.Dice() {
			faces := make([]string, len(rt.Faces))
			for j, f := range rt.Faces {
				faces[j] = strconv.Itoa(f)
			}
			s = "[" + strings.Join(faces, " ") + "]"
			if rt.Term.Sign < 0 {
				s = "-" + s
			}
		} else {
			s = strconv.Itoa(rt.Term.Value)
			switch {
			case rt.Term.Sign < 0:
				s = "-" + s
			case i > 0:
				s = "+" + s
			}
		}
		parts[i] = s
	}
	return strings.Join(parts, " ")
}

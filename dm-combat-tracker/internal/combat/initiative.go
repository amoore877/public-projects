package combat

import (
	"fmt"
	"strconv"
	"strings"
)

// Initiative is a fixed-point initiative value in thousandths (D4):
// 15.3 is stored as 15300. Fixed point keeps comparisons and round trips exact.
type Initiative int64

const initScale = 1000

// InitiativeFromInt returns the initiative for a whole number.
func InitiativeFromInt(n int) Initiative { return Initiative(int64(n) * initScale) }

// ParseInitiative parses a decimal such as "15", "15.3", "-1.25" or "12.125".
// At most three decimal places are accepted.
func ParseInitiative(s string) (Initiative, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("initiative: empty value")
	}
	neg := false
	body := s
	switch body[0] {
	case '-':
		neg, body = true, body[1:]
	case '+':
		body = body[1:]
	}
	whole, frac, hasDot := strings.Cut(body, ".")
	if whole == "" && (!hasDot || frac == "") {
		return 0, fmt.Errorf("initiative: %q is not a number", s)
	}
	if hasDot && frac == "" {
		return 0, fmt.Errorf("initiative: %q is not a number", s)
	}
	if len(frac) > 3 {
		return 0, fmt.Errorf("initiative: %q has more than 3 decimal places", s)
	}
	if !allDigits(whole) || !allDigits(frac) {
		return 0, fmt.Errorf("initiative: %q is not a number", s)
	}
	var w int64
	if whole != "" {
		var err error
		w, err = strconv.ParseInt(whole, 10, 64)
		if err != nil || w > 1_000_000_000 {
			return 0, fmt.Errorf("initiative: %q is out of range", s)
		}
	}
	var f int64
	if frac != "" {
		padded := frac + strings.Repeat("0", 3-len(frac))
		f, _ = strconv.ParseInt(padded, 10, 64)
	}
	v := w*initScale + f
	if neg {
		v = -v
	}
	return Initiative(v), nil
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// String renders the value with trailing zeros trimmed: 15300 → "15.3", 15000 → "15".
func (i Initiative) String() string {
	v := int64(i)
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	whole, frac := v/initScale, v%initScale
	if frac == 0 {
		return fmt.Sprintf("%s%d", sign, whole)
	}
	fs := strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
	return fmt.Sprintf("%s%d.%s", sign, whole, fs)
}

// IsWhole reports whether the value has no fractional part.
func (i Initiative) IsWhole() bool { return int64(i)%initScale == 0 }

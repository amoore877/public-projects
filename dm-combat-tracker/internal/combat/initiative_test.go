package combat

import (
	"reflect"
	"testing"
)

func TestParseInitiative(t *testing.T) {
	tests := []struct {
		in   string
		want Initiative
		str  string
	}{
		{"15", 15000, "15"},
		{"15.3", 15300, "15.3"},
		{"15.30", 15300, "15.3"},
		{"12.125", 12125, "12.125"},
		{"12.05", 12050, "12.05"},
		{"0", 0, "0"},
		{"-1", -1000, "-1"},
		{"-1.25", -1250, "-1.25"},
		{".5", 500, "0.5"},
		{"+3", 3000, "3"},
		{" 7 ", 7000, "7"},
	}
	for _, tt := range tests {
		got, err := ParseInitiative(tt.in)
		if err != nil {
			t.Errorf("ParseInitiative(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseInitiative(%q) = %d, want %d", tt.in, got, tt.want)
		}
		if got.String() != tt.str {
			t.Errorf("String(%q) = %q, want %q", tt.in, got.String(), tt.str)
		}
	}
	for _, bad := range []string{"", "abc", "1.2345", "1.", ".", "-", "1e3", "1,5", "--1"} {
		if _, err := ParseInitiative(bad); err == nil {
			t.Errorf("ParseInitiative(%q): want error", bad)
		}
	}
}

func TestGroupFill(t *testing.T) {
	tests := []struct {
		base int
		n    int
		want []string
	}{
		{12, 6, []string{"12.6", "12.5", "12.4", "12.3", "12.2", "12.1"}},
		{12, 1, []string{"12.1"}},
		{12, 9, []string{"12.9", "12.8", "12.7", "12.6", "12.5", "12.4", "12.3", "12.2", "12.1"}},
		{-1, 3, []string{"-1.1", "-1.2", "-1.3"}},
		{0, 2, []string{"0.2", "0.1"}},
	}
	for _, tt := range tests {
		got, err := GroupFill(InitiativeFromInt(tt.base), tt.n)
		if err != nil {
			t.Fatal(err)
		}
		var s []string
		for _, v := range got {
			s = append(s, v.String())
		}
		if !reflect.DeepEqual(s, tt.want) {
			t.Errorf("GroupFill(%d, %d) = %v, want %v", tt.base, tt.n, s, tt.want)
		}
	}

	got, _ := GroupFill(InitiativeFromInt(12), 12)
	if got[0].String() != "12.12" || got[11].String() != "12.01" {
		t.Errorf("12 x12: first %s last %s", got[0], got[11])
	}
	got, _ = GroupFill(InitiativeFromInt(5), 150)
	if got[0].String() != "5.15" || got[149].String() != "5.001" {
		t.Errorf("5 x150: first %s last %s", got[0], got[149])
	}

	if _, err := GroupFill(15500, 3); err == nil {
		t.Error("fractional base: want error")
	}
	if _, err := GroupFill(0, 1000); err == nil {
		t.Error("n=1000: want error")
	}
}

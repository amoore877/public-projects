package combat

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 14, 19, 42, 0, 0, time.UTC)

func fixedClock() Clock { return func() time.Time { return t0 } }

func newEnc() *Encounter { return New(fixedClock()) }

func ini(t *testing.T, s string) Initiative {
	t.Helper()
	v, err := ParseInitiative(s)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func add(t *testing.T, e *Encounter, name, init string, hp int) RowID {
	t.Helper()
	id, _, err := e.Add(CombatantSpec{Name: name, Initiative: ini(t, init), MaxHP: hp})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// must is used as must(t)(e.Op(...)) so a multi-value call can be passed.
func must(t *testing.T) func([]Event, error) []Event {
	t.Helper()
	return func(evs []Event, err error) []Event {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return evs
	}
}

func orderNames(e *Encounter) string {
	var names []string
	for _, r := range e.Order() {
		names = append(names, r.Name())
	}
	return strings.Join(names, ",")
}

func currentName(e *Encounter) string {
	if r, ok := e.Row(e.Current); ok {
		return r.Name()
	}
	return ""
}

// kinds returns the concrete type names of events, e.g. "RoundStarted".
func kinds(evs []Event) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = reflect.TypeOf(ev).Name()
	}
	return out
}

func wantKinds(t *testing.T, evs []Event, want ...string) {
	t.Helper()
	if got := kinds(evs); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
		t.Fatalf("events = %v, want %v", got, want)
	}
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("err = %v, want %v", err, target)
	}
}

func rounds(n int) *int { return &n }

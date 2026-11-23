package tui

import (
	"strings"
	"testing"

	"github.com/amoore877/public-projects/dm-combat-tracker/internal/combat"
)

func TestFuzzyJump(t *testing.T) {
	// Empty table warns and does nothing.
	mEmpty := New(newSession(t), "")
	press(mEmpty, "/")
	if mEmpty.overlay != nil || !strings.Contains(mEmpty.msg, "Nothing to jump to") {
		t.Fatalf("empty jump: overlay=%v msg=%q", mEmpty.overlay, mEmpty.msg)
	}

	s := newSession(t, fight()...)
	m := sized(New(s, ""), 80, 24)
	orig := m.Selected() // Thorin
	undoBefore, redoBefore := s.UndoDepth()

	// Live prefix jump + tab/up/down cycling across goblins.
	press(m, "/")
	typeText(m, "gob")
	if got := rowName(m, m.Selected()); got != "Goblin 1" {
		t.Fatalf("after /gob: selected %q, want Goblin 1", got)
	}
	if sc := screen(m); !strings.Contains(sc, "1/3") {
		t.Errorf("expected 1/3 match count:\n%s", sc)
	}
	press(m, "tab")
	if got := rowName(m, m.Selected()); got != "Goblin 2" {
		t.Fatalf("after tab: selected %q, want Goblin 2", got)
	}
	press(m, "down")
	if got := rowName(m, m.Selected()); got != "Goblin 3" {
		t.Fatalf("after down: selected %q, want Goblin 3", got)
	}
	special(m, "shift+tab")
	if got := rowName(m, m.Selected()); got != "Goblin 2" {
		t.Fatalf("after shift+tab: selected %q, want Goblin 2", got)
	}
	press(m, "up")
	if got := rowName(m, m.Selected()); got != "Goblin 1" {
		t.Fatalf("after up: selected %q, want Goblin 1", got)
	}

	// Esc restores the original selection.
	press(m, "esc")
	if m.overlay != nil || m.Selected() != orig {
		t.Fatalf("esc should restore %d, got %d (overlay=%v)", orig, m.Selected(), m.overlay)
	}

	// Subsequence (fuzzy) match on effect row ("sg" -> Spirit Guardians), enter keeps it.
	press(m, "/")
	typeText(m, "sg")
	if got := rowName(m, m.Selected()); got != "Spirit Guardians" {
		t.Fatalf("after /sg: selected %q, want Spirit Guardians", got)
	}
	press(m, "enter")
	if m.overlay != nil || rowName(m, m.Selected()) != "Spirit Guardians" {
		t.Fatalf("enter should keep Spirit Guardians, got %q", rowName(m, m.Selected()))
	}

	// Substring match ("cap" -> Bandit Captain), no match restores orig, paste works.
	press(m, "/")
	m.Update(pasteMsg("  cap "))
	if got := rowName(m, m.Selected()); got != "Bandit Captain" {
		t.Fatalf("after paste cap: selected %q, want Bandit Captain", got)
	}
	typeText(m, "zzz")
	if sc := screen(m); !strings.Contains(sc, "No match") {
		t.Errorf("expected No match:\n%s", sc)
	}
	if rowName(m, m.Selected()) != "Spirit Guardians" {
		t.Errorf("no match should fall back to starting row Spirit Guardians, got %q", rowName(m, m.Selected()))
	}
	// Backspace back to empty clears query and keeps starting row.
	special(m, "ctrl+u")
	if sc := screen(m); !strings.Contains(sc, "type a name to jump") {
		t.Errorf("empty query hint:\n%s", sc)
	}
	press(m, "enter")

	// Jumping produces no undo steps.
	if u, r := s.UndoDepth(); u != undoBefore || r != redoBefore {
		t.Errorf("jump should not push undo steps: %d/%d (was %d/%d)", u, r, undoBefore, redoBefore)
	}
}

func rowName(m *Model, id combat.RowID) string {
	if r, ok := m.sess.Encounter().Row(id); ok {
		return r.Name()
	}
	return ""
}

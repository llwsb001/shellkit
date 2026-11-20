package core

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func tempHome(t *testing.T) {
	t.Helper()
	t.Setenv("SHELLKIT_HOME", t.TempDir())
}

func TestAddListRemove(t *testing.T) {
	tempHome(t)

	if got, err := Load(); err != nil || len(got) != 0 {
		t.Fatalf("empty home: %v %d", err, len(got))
	}

	a, err := Add("buy oat milk", []string{"Shopping", "shopping", " "})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == "" || len(a.Tags) != 1 || a.Tags[0] != "shopping" {
		t.Fatalf("bad note %+v", a)
	}
	b, err := Add("read the Go spec", nil)
	if err != nil {
		t.Fatal(err)
	}

	notes, err := List("", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].ID != b.ID {
		t.Fatalf("want newest first, got %+v", notes)
	}
	if one, _ := List("OAT", 0); len(one) != 1 || one[0].ID != a.ID {
		t.Fatalf("search should be case-insensitive: %+v", one)
	}
	if one, _ := List("shopping", 0); len(one) != 1 {
		t.Fatalf("tag search: %+v", one)
	}
	if none, _ := List("nothing-here", 0); len(none) != 0 {
		t.Fatalf("expected no match: %+v", none)
	}
	if head, _ := List("", 1); len(head) != 1 || head[0].ID != b.ID {
		t.Fatalf("limit: %+v", head)
	}

	if err := Remove(a.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := Count(); n != 1 {
		t.Fatalf("count after remove = %d", n)
	}
	if err := Remove(a.ID); err == nil || !strings.Contains(err.Error(), "no note") {
		t.Fatalf("removing twice should fail, got %v", err)
	}
}

func TestAddRejectsEmptyAndOversize(t *testing.T) {
	tempHome(t)
	if _, err := Add("   \n\t", nil); err == nil {
		t.Fatal("empty text accepted")
	}
	if _, err := Add(strings.Repeat("x", 5001), nil); err == nil {
		t.Fatal("oversize text accepted")
	}
	if _, err := Add(strings.Repeat("x", 5000), nil); err != nil {
		t.Fatalf("5000 runes should be allowed: %v", err)
	}
}

func TestLoadReportsCorruptFile(t *testing.T) {
	tempHome(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := List("", 0); err == nil || !strings.Contains(err.Error(), "notes.json") {
		t.Fatalf("corrupt file should name itself: %v", err)
	}
}

func TestIDsAreUnique(t *testing.T) {
	tempHome(t)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		n, err := Add("same text", nil)
		if err != nil {
			t.Fatal(err)
		}
		if seen[n.ID] {
			t.Fatalf("duplicate id %s", n.ID)
		}
		seen[n.ID] = true
	}
}

func TestConcurrentAddsDoNotLoseWrites(t *testing.T) {
	tempHome(t)
	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := Add(fmt.Sprintf("note %d", i), nil); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if got, _ := Count(); got != n {
		t.Fatalf("%d concurrent adds left %d notes", n, got)
	}
}

func TestListSortsByTime(t *testing.T) {
	tempHome(t)
	// A hand-edited file whose order does not match its timestamps.
	body := `[
  {"id":"a","text":"older","at":"2026-01-01T00:00:00Z"},
  {"id":"b","text":"newer","at":"2026-02-01T00:00:00Z"}
]`
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(File(), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	notes, err := List("", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].ID != "b" {
		t.Fatalf("want newest first, got %+v", notes)
	}
}

func TestTagsAreCapped(t *testing.T) {
	tempHome(t)
	n, err := Add("caps", []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", strings.Repeat("x", 40)})
	if err != nil {
		t.Fatal(err)
	}
	if len(n.Tags) > 8 {
		t.Fatalf("tag cap ignored: %d tags", len(n.Tags))
	}
	for _, tag := range n.Tags {
		if len([]rune(tag)) > 32 {
			t.Fatalf("long tag kept: %q", tag)
		}
	}
}

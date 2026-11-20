package cli

import (
	"strings"
	"testing"
)

func TestSplitAdd(t *testing.T) {
	cases := []struct {
		args    []string
		text    string
		tags    []string
		wantErr bool
	}{
		{nil, "", nil, true},
		{[]string{"buy", "oat", "milk"}, "buy oat milk", nil, false},
		{[]string{"-t", "Shopping", "buy"}, "buy", []string{"Shopping"}, false},
		{[]string{"--tag=food", "buy"}, "buy", []string{"food"}, false},
		{[]string{"buy", "--tag", "x", "milk"}, "buy milk", []string{"x"}, false},
		{[]string{"--verbose", "buy"}, "", nil, true},
		{[]string{"-t"}, "", nil, true},
	}
	for _, c := range cases {
		text, tags, err := splitAdd(c.args)
		if c.wantErr {
			if err == nil {
				t.Errorf("%v: expected an error", c.args)
			}
			continue
		}
		if err != nil {
			t.Errorf("%v: %v", c.args, err)
			continue
		}
		if text != c.text || strings.Join(tags, ",") != strings.Join(c.tags, ",") {
			t.Errorf("%v: got %q %v", c.args, text, tags)
		}
	}
}

func TestParseList(t *testing.T) {
	cases := []struct {
		args  []string
		q     string
		limit int
		err   bool
	}{
		{nil, "", 0, false},
		{[]string{"oat", "milk"}, "oat milk", 0, false},
		{[]string{"oat", "-n", "3"}, "oat", 3, false},
		{[]string{"-n", "2"}, "", 2, false},
		{[]string{"-n"}, "", 0, true},
		{[]string{"-n", "abc"}, "", 0, true},
		{[]string{"-n", "-1"}, "", 0, true},
	}
	for _, c := range cases {
		q, limit, err := parseList(c.args)
		if c.err {
			if err == nil {
				t.Errorf("%v: expected an error", c.args)
			}
			continue
		}
		if err != nil || q != c.q || limit != c.limit {
			t.Errorf("%v: got %q %d %v", c.args, q, limit, err)
		}
	}
}

func TestParseListLimitForms(t *testing.T) {
	if _, limit, err := parseList([]string{"--limit=3", "q"}); err != nil || limit != 3 {
		t.Fatalf("--limit=3: %d %v", limit, err)
	}
	if _, _, err := parseList([]string{"--limit=x"}); err == nil {
		t.Fatal("--limit=x accepted")
	}
}

func TestSplitAddEndOfFlags(t *testing.T) {
	text, tags, err := splitAdd([]string{"-t", "x", "--", "-5", "degrees"})
	if err != nil || text != "-5 degrees" || len(tags) != 1 {
		t.Fatalf("got %q %v %v", text, tags, err)
	}
	if _, _, err := splitAdd([]string{"-t", "-n", "x"}); err == nil {
		t.Fatal("a flag-looking tag should be rejected")
	}
}

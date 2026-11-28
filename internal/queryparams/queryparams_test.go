package queryparams

import "testing"

func TestParsePreservesOrderAndDuplicates(t *testing.T) {
	rows := Parse("http://x/p?b=2&a=1&a=3#frag")
	if len(rows) != 3 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Key != "b" || rows[0].Value != "2" || rows[1].Key != "a" || rows[1].Value != "1" || rows[2].Value != "3" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestBuildRoundTrip(t *testing.T) {
	const raw = "http://x/p?a=1&b=x+y#frag"
	rows := Parse(raw)
	if got := Build(raw, rows); got != raw {
		t.Fatalf("Build(Parse(%q)) = %q", raw, got)
	}
}

func TestBuildDropsEmptyRows(t *testing.T) {
	got := Build("http://x/p?a=1", []Row{{Key: "", Value: ""}, {Key: "k", Value: ""}})
	if got != "http://x/p?k" {
		t.Fatalf("Build = %q", got)
	}
}

func TestSplit(t *testing.T) {
	base, q, frag := Split("http://x/p?a=1#f")
	if base != "http://x/p" || q != "a=1" || frag != "f" {
		t.Fatalf("Split = %q %q %q", base, q, frag)
	}
}

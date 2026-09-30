package main

import "testing"

func TestVersionText(t *testing.T) {
	got := versionText("1.2.3", "abc123", "2026-09-30")
	want := "captain-compose 1.2.3 (commit abc123, built 2026-09-30)"
	if got != want {
		t.Fatalf("versionText() = %q, want %q", got, want)
	}
}

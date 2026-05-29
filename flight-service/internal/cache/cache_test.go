package cache

import "testing"

func TestFlightKey(t *testing.T) {
	if got := flightKey(42); got != "flight:42" {
		t.Fatalf("flightKey() = %q, want %q", got, "flight:42")
	}
}

func TestSearchKey(t *testing.T) {
	got := searchKey("SVO", "LED", "2026-04-01")
	want := "search:SVO:LED:2026-04-01"
	if got != want {
		t.Fatalf("searchKey() = %q, want %q", got, want)
	}
}

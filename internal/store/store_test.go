package store

import (
	"errors"
	"path/filepath"
	"testing"
)

func sampleEntry() Entry {
	return Entry{
		Namespace: "billing",
		Locale:    "en-US",
		Region:    "US",
		Prefix:    "/us/checkout",
		Content:   "Pay now",
	}
}

func TestPutEntryAndRoute(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, found, err := st.Route("billing", "en-US")
	if err != nil || !found {
		t.Fatalf("route: found=%v err=%v", found, err)
	}
	if got != sampleEntry() {
		t.Fatalf("route = %+v, want %+v", got, sampleEntry())
	}
}

func TestIdenticalRepeatSucceeds(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("repeat put: %v", err)
	}

	got, found, err := st.Route("billing", "en-US")
	if err != nil || !found {
		t.Fatalf("route: found=%v err=%v", found, err)
	}
	if got.Content != "Pay now" || got.Prefix != "/us/checkout" {
		t.Fatalf("repeat altered state: %+v", got)
	}
}

func TestConflictsLeaveStateUntouched(t *testing.T) {
	cases := []struct {
		name  string
		entry Entry
	}{
		{"different content", Entry{Namespace: "billing", Locale: "en-US", Region: "US", Prefix: "/us/checkout", Content: "Pay later"}},
		{"different region", Entry{Namespace: "billing", Locale: "en-US", Region: "CA", Prefix: "/ca", Content: "Pay now"}},
		{"different prefix via same region", Entry{Namespace: "billing", Locale: "en-US", Region: "US", Prefix: "/us/other", Content: "Pay now"}},
		{"region mapped to other prefix by new locale", Entry{Namespace: "billing", Locale: "fr-FR", Region: "US", Prefix: "/us/other", Content: "Payer"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			st, err := Open(filepath.Join(t.TempDir(), "store.db"))
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer st.Close()

			if err := st.PutEntry(sampleEntry()); err != nil {
				t.Fatalf("put: %v", err)
			}
			err = st.PutEntry(tc.entry)
			if !IsConflict(err) {
				t.Fatalf("second put err = %v, want conflict", err)
			}

			got, found, routeErr := st.Route("billing", "en-US")
			if routeErr != nil || !found || got != sampleEntry() {
				t.Fatalf("original entry changed: %+v found=%v err=%v", got, found, routeErr)
			}
			if prefix, ok, _ := st.PrefixForRegion("US"); !ok || prefix != "/us/checkout" {
				t.Fatalf("region mapping changed: %q ok=%v", prefix, ok)
			}
			if _, found, _ := st.Route("billing", "fr-FR"); found {
				t.Fatalf("conflicting entry was left behind")
			}
		})
	}
}

func TestSameRegionSamePrefixDifferentLocaleIsAllowed(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("put: %v", err)
	}
	other := Entry{Namespace: "billing", Locale: "fr-FR", Region: "US", Prefix: "/us/checkout", Content: "Payer"}
	if err := st.PutEntry(other); err != nil {
		t.Fatalf("shared region/prefix put: %v", err)
	}
}

func TestNoFallback(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("put: %v", err)
	}
	for _, locale := range []string{"en", "EN-us", "fr-FR", "en-GB"} {
		if _, found, err := st.Route("billing", locale); err != nil {
			t.Fatalf("route %s: %v", locale, err)
		} else if found {
			t.Fatalf("route %s unexpectedly found an entry", locale)
		}
	}
}

func TestEntriesSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.PutEntry(sampleEntry()); err != nil {
		t.Fatalf("put: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	st2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer st2.Close()
	got, found, err := st2.Route("billing", "en-US")
	if err != nil || !found {
		t.Fatalf("route after reopen: found=%v err=%v", found, err)
	}
	if got != sampleEntry() {
		t.Fatalf("after reopen = %+v, want %+v", got, sampleEntry())
	}
}

func TestConflictErrorSentinel(t *testing.T) {
	if !errors.Is(ConflictError{}, ConflictError{}) {
		t.Fatal("ConflictError should match itself")
	}
}

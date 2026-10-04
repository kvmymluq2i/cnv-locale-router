package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenCreatesUsableStore(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()
	if err := st.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func sampleEntry() MessageEntry {
	return MessageEntry{
		Namespace: "checkout",
		Locale:    "en-US",
		Region:    "US",
		Prefix:    "/us",
		Content:   "Hello",
	}
}

func TestSaveMessageEntryPersistsAndReplays(t *testing.T) {
	st := openTemp(t)
	entry := sampleEntry()

	saved, replayed, err := st.SaveMessageEntry(entry)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if replayed {
		t.Fatalf("first save reported replay")
	}
	if saved != entry {
		t.Fatalf("saved = %+v, want %+v", saved, entry)
	}

	again, replayed, err := st.SaveMessageEntry(entry)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !replayed {
		t.Fatalf("identical resubmit not reported as replay")
	}
	if again != entry {
		t.Fatalf("replayed = %+v, want %+v", again, entry)
	}
}

func TestSaveMessageEntryConflictsOnChangedValues(t *testing.T) {
	st := openTemp(t)
	entry := sampleEntry()
	if _, _, err := st.SaveMessageEntry(entry); err != nil {
		t.Fatalf("save: %v", err)
	}

	for name, mutate := range map[string]func(*MessageEntry){
		"region":  func(e *MessageEntry) { e.Region = "GB" },
		"prefix":  func(e *MessageEntry) { e.Prefix = "/other" },
		"content": func(e *MessageEntry) { e.Content = "Changed" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := entry
			mutate(&changed)
			if _, _, err := st.SaveMessageEntry(changed); !errors.Is(err, ErrConflict) {
				t.Fatalf("err = %v, want ErrConflict", err)
			}
		})
	}
}

func TestSaveMessageEntryConflictsWhenRegionBoundToOtherPrefix(t *testing.T) {
	st := openTemp(t)
	if _, _, err := st.SaveMessageEntry(sampleEntry()); err != nil {
		t.Fatalf("save: %v", err)
	}

	other := MessageEntry{
		Namespace: "cart",
		Locale:    "fr",
		Region:    "US",
		Prefix:    "/different",
		Content:   "Bonjour",
	}
	if _, _, err := st.SaveMessageEntry(other); !errors.Is(err, ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}

	// The failed registration must leave no entry behind.
	if _, err := st.FindMessageEntry("cart", "fr"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("find after conflict: err = %v, want sql.ErrNoRows", err)
	}
}

func TestSaveMessageEntrySharesPrefixAcrossNamespaces(t *testing.T) {
	st := openTemp(t)
	if _, _, err := st.SaveMessageEntry(sampleEntry()); err != nil {
		t.Fatalf("save: %v", err)
	}

	other := MessageEntry{
		Namespace: "cart",
		Locale:    "en-US",
		Region:    "US",
		Prefix:    "/us",
		Content:   "Hi there",
	}
	if _, _, err := st.SaveMessageEntry(other); err != nil {
		t.Fatalf("save with shared prefix: %v", err)
	}
}

func TestFindMessageEntrySurvivesReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	entry := sampleEntry()
	if _, _, err := st.SaveMessageEntry(entry); err != nil {
		t.Fatalf("save: %v", err)
	}
	st.Close()

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	got, err := reopened.FindMessageEntry(entry.Namespace, entry.Locale)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got != entry {
		t.Fatalf("got %+v, want %+v", got, entry)
	}
}

func TestFindMessageEntryMissingReturnsNoRows(t *testing.T) {
	st := openTemp(t)
	if _, err := st.FindMessageEntry("nope", "en-US"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}

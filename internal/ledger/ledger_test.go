package ledger

import (
	"path/filepath"
	"testing"
)

func TestAddHasAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "processed.json")

	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.Has(42) {
		t.Fatal("fresh ledger should not contain id 42")
	}
	l.Add(42)
	if !l.Has(42) {
		t.Fatal("ledger should contain 42 after Add")
	}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}

	// Reload from disk: 42 should persist, 99 should not.
	l2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !l2.Has(42) {
		t.Error("reloaded ledger missing id 42")
	}
	if l2.Has(99) {
		t.Error("reloaded ledger unexpectedly has id 99")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if l.Has(1) {
		t.Error("missing-file ledger should be empty")
	}
}

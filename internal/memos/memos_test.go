package memos

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T, recordingsDir string) {
	t.Helper()
	dbPath := filepath.Join(recordingsDir, "CloudRecordings.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE ZFOLDER (Z_PK INTEGER PRIMARY KEY, ZENCRYPTEDNAME TEXT)`,
		`CREATE TABLE ZCLOUDRECORDING (
			Z_PK INTEGER PRIMARY KEY,
			ZDATE REAL, ZDURATION REAL,
			ZENCRYPTEDTITLE TEXT, ZCUSTOMLABEL TEXT,
			ZPATH TEXT, ZFOLDER INTEGER)`,
		`INSERT INTO ZFOLDER (Z_PK, ZENCRYPTEDNAME) VALUES (1, 'Ideas')`,
		// ZDATE 769105800 == 2025-05-15T... in Core Data epoch (2001-based).
		`INSERT INTO ZCLOUDRECORDING
			(Z_PK, ZDATE, ZDURATION, ZENCRYPTEDTITLE, ZCUSTOMLABEL, ZPATH, ZFOLDER)
			VALUES (10, 769105800.0, 12.5, 'My Memo', NULL, '20250515 010000.m4a', 1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup stmt failed: %v\n%s", err, s)
		}
	}
}

func TestListReturnsConvertedMemo(t *testing.T) {
	dir := t.TempDir()
	newTestDB(t, dir)
	// create the audio file so resolveAudioPath finds it
	audio := filepath.Join(dir, "20250515 010000.m4a")
	if err := os.WriteFile(audio, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := New(dir)
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d memos, want 1", len(got))
	}
	m := got[0]
	if m.ID != 10 {
		t.Errorf("ID = %d, want 10", m.ID)
	}
	if m.Title != "My Memo" {
		t.Errorf("Title = %q, want My Memo", m.Title)
	}
	if m.Folder != "Ideas" {
		t.Errorf("Folder = %q, want Ideas", m.Folder)
	}
	if m.AudioPath != audio {
		t.Errorf("AudioPath = %q, want %q", m.AudioPath, audio)
	}
	// 769105800 + 978307200 = 1747413000 unix
	wantUnix := int64(769105800 + coreDataEpochOffset)
	if m.Date.Unix() != wantUnix {
		t.Errorf("Date.Unix() = %d, want %d", m.Date.Unix(), wantUnix)
	}
	if m.Duration != 12500*time.Millisecond {
		t.Errorf("Duration = %v, want 12.5s", m.Duration)
	}
}

func TestListMissingAudioLeavesPathEmpty(t *testing.T) {
	dir := t.TempDir()
	newTestDB(t, dir)
	// note: no audio file created
	got, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AudioPath != "" {
		t.Errorf("AudioPath = %q, want empty when file absent", got[0].AudioPath)
	}
}

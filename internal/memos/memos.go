// Package memos reads Apple Voice Memos recordings from the CloudRecordings.db
// Core Data SQLite database (read-only) and resolves their audio file paths.
package memos

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mattwynne/voice-memo-capture/internal/output"
)

const coreDataEpochOffset = 978307200 // 2001-01-01 UTC in Unix seconds

var audioExts = []string{".m4a", ".qta"}

// Memo embeds the output.Memo fields plus what we need for filtering/skip.
type Memo struct {
	output.Memo
	Folder string
}

// Store reads recordings from a given Recordings directory.
type Store struct {
	recordingsDir string
}

func New(recordingsDir string) *Store {
	return &Store{recordingsDir: recordingsDir}
}

func (s *Store) dbPath() string {
	return filepath.Join(s.recordingsDir, "CloudRecordings.db")
}

const listSQL = `
	SELECT r.Z_PK AS id,
	       r.ZDATE AS zdate,
	       r.ZDURATION AS duration,
	       COALESCE(r.ZENCRYPTEDTITLE, r.ZCUSTOMLABEL) AS title,
	       r.ZPATH AS path,
	       f.ZENCRYPTEDNAME AS folder
	FROM ZCLOUDRECORDING r
	LEFT JOIN ZFOLDER f ON r.ZFOLDER = f.Z_PK
	ORDER BY r.ZDATE DESC`

// List returns all recordings, newest first, with audio paths resolved.
func (s *Store) List() ([]Memo, error) {
	db, err := sql.Open("sqlite", "file:"+s.dbPath()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(listSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memos []Memo
	for rows.Next() {
		var (
			id       int64
			zdate    sql.NullFloat64
			duration sql.NullFloat64
			title    sql.NullString
			path     sql.NullString
			folder   sql.NullString
		)
		if err := rows.Scan(&id, &zdate, &duration, &title, &path, &folder); err != nil {
			return nil, err
		}
		m := Memo{Folder: folder.String}
		m.ID = id
		m.Title = title.String
		m.Date = time.Unix(int64(zdate.Float64)+coreDataEpochOffset, 0).UTC()
		m.Duration = time.Duration(duration.Float64 * float64(time.Second))
		m.AudioPath = s.resolveAudioPath(path.String)
		memos = append(memos, m)
	}
	return memos, rows.Err()
}

// resolveAudioPath returns the absolute path to the recording's audio file, or
// "" if it isn't present on disk yet (e.g. not downloaded from iCloud). ZPATH
// may name one extension while the real file uses the other.
func (s *Store) resolveAudioPath(zpath string) string {
	if zpath == "" {
		return ""
	}
	direct := filepath.Join(s.recordingsDir, zpath)
	if fileExists(direct) {
		return direct
	}
	stem := zpath[:len(zpath)-len(filepath.Ext(zpath))]
	for _, ext := range audioExts {
		cand := filepath.Join(s.recordingsDir, stem+ext)
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// IsPermissionError reports whether err is the macOS TCC "operation not
// permitted" / permission-denied condition (Full Disk Access not granted).
func IsPermissionError(err error) bool {
	if errors.Is(err, fs.ErrPermission) {
		return true
	}
	if err == nil {
		return false
	}

	// When macOS TCC denies access to the Voice Memos group container,
	// modernc.org/sqlite can surface SQLite code 14 with the misleading text
	// "unable to open database file: out of memory (14)" rather than an
	// os.ErrPermission-wrapping error.
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "unable to open database file") &&
		strings.Contains(s, "(14)")
}

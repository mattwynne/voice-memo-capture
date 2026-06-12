// Package ledger tracks which memo IDs have already been written, so runs are
// idempotent. Stored as a small JSON file.
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Ledger struct {
	ids map[int64]bool
}

type fileShape struct {
	Processed []int64 `json:"processed"`
}

// Load reads the ledger at path. A missing file yields an empty ledger.
func Load(path string) (*Ledger, error) {
	l := &Ledger{ids: map[int64]bool{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	var fileData fileShape
	if err := json.Unmarshal(data, &fileData); err != nil {
		return nil, err
	}
	for _, id := range fileData.Processed {
		l.ids[id] = true
	}
	return l, nil
}

func (l *Ledger) Has(id int64) bool { return l.ids[id] }

func (l *Ledger) Add(id int64) { l.ids[id] = true }

// Save writes the ledger to path, creating parent directories as needed.
func (l *Ledger) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ids := make([]int64, 0, len(l.ids))
	for id := range l.ids {
		ids = append(ids, id)
	}
	data, err := json.MarshalIndent(fileShape{Processed: ids}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

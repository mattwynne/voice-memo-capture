// Package transcript extracts Apple's on-device Voice Memos transcript that is
// embedded as JSON inside the recording's audio file.
//
// Ported from Pedram Amini's CC0 gist (see CREDITS.md). Apple writes the
// transcript JSON into the file after on-device transcription completes. Two
// container layouts exist in the wild (older .m4a udta `tsrp` atom; newer .qta
// meta/ilst), so rather than walk both, we scan the raw bytes for the unique
// JSON sentinel and brace-balance forward to the first parseable object.
package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

var sentinel = []byte(`{"attributedString":`)

type parsed struct {
	AttributedString struct {
		Runs []json.RawMessage `json:"runs"`
	} `json:"attributedString"`
}

// Extract returns the transcript text for the audio file at path. The second
// return value is false (with nil error) when the file has no embedded
// transcript yet — the caller should skip and retry later.
func Extract(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	raw, ok := scan(data)
	if !ok {
		return "", false, nil
	}
	text, err := flatten(raw)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// scan finds the first byte range starting at the sentinel that forms a valid,
// brace-balanced JSON object (string-literal aware).
func scan(data []byte) ([]byte, bool) {
	from := 0
	for {
		rel := bytes.Index(data[from:], sentinel)
		if rel < 0 {
			return nil, false
		}
		start := from + rel
		depth := 0
		inStr := false
		escape := false
		for j := start; j < len(data); j++ {
			b := data[j]
			if inStr {
				switch {
				case escape:
					escape = false
				case b == 0x5C: // backslash
					escape = true
				case b == 0x22: // "
					inStr = false
				}
				continue
			}
			switch b {
			case 0x22: // "
				inStr = true
			case 0x7B: // {
				depth++
			case 0x7D: // }
				depth--
				if depth == 0 {
					candidate := data[start : j+1]
					if json.Valid(candidate) {
						return candidate, true
					}
					// invalid here; resume searching after this sentinel
					from = start + len(sentinel)
					goto nextSentinel
				}
			}
		}
		// reached EOF without closing; try after this sentinel
		from = start + len(sentinel)
	nextSentinel:
	}
}

// flatten joins the even-indexed (spoken-text) entries of the runs array.
func flatten(raw []byte) (string, error) {
	var p parsed
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 0; i < len(p.AttributedString.Runs); i += 2 {
		var tok string
		if err := json.Unmarshal(p.AttributedString.Runs[i], &tok); err != nil {
			continue // non-string token: skip defensively
		}
		sb.WriteString(tok)
	}
	return sb.String(), nil
}

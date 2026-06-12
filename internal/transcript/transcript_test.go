package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

// buildFixture wraps a valid transcript JSON object in filler bytes, the way
// it appears embedded in a real .m4a/.qta file.
func buildFixture(jsonObj string) []byte {
	out := []byte{0x00, 0x01, 0x02, 'f', 'r', 'e', 'e'} // leading binary filler
	out = append(out, []byte(jsonObj)...)
	out = append(out, []byte{0xFF, 0xFE, 0x00}...) // trailing binary filler
	return out
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "memo.m4a")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractReturnsJoinedTokens(t *testing.T) {
	// runs alternates token, attr-index, token, attr-index, ...
	obj := `{"attributedString":{"runs":["Hello ",0,"world",1]},"locale":"en-US"}`
	path := writeTemp(t, buildFixture(obj))

	text, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Extract reported no transcript, want one")
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestExtractNoTranscript(t *testing.T) {
	path := writeTemp(t, []byte("no transcript here at all"))
	_, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Extract reported a transcript, want none")
	}
}

func TestExtractSkipsInvalidJSONSentinelAndFindsLater(t *testing.T) {
	// First sentinel is followed by broken JSON; a later one is valid.
	bad := `{"attributedString":{"runs":[BROKEN`
	good := `{"attributedString":{"runs":["ok",0]}}`
	data := append([]byte(bad), []byte(good)...)
	path := writeTemp(t, data)

	text, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || text != "ok" {
		t.Errorf("got (%q, %v), want (\"ok\", true)", text, ok)
	}
}

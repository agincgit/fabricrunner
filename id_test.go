package fabricrunner

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestNewIDAtCreatesUUIDv7(t *testing.T) {
	t.Parallel()

	wantTime := time.Date(2026, time.September, 7, 12, 0, 0, 123_000_000, time.UTC)
	id, err := newIDAt(wantTime, bytes.NewReader(bytes.Repeat([]byte{0xab}, 16)))
	if err != nil {
		t.Fatalf("newIDAt() error = %v", err)
	}
	if _, err := ParseID(id.String()); err != nil {
		t.Fatalf("ParseID(%q) error = %v", id, err)
	}
	if id[14] != '7' {
		t.Fatalf("version nibble = %q, want 7", id[14])
	}
	if !strings.Contains("89ab", string(id[19])) {
		t.Fatalf("variant nibble = %q, want RFC 9562 variant", id[19])
	}
}

func TestUUIDv7SortsByTime(t *testing.T) {
	t.Parallel()

	early, err := newIDAt(time.UnixMilli(1), bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	late, err := newIDAt(time.UnixMilli(2), bytes.NewReader(make([]byte, 16)))
	if err != nil {
		t.Fatal(err)
	}
	if early >= late {
		t.Fatalf("IDs are not time ordered: %q >= %q", early, late)
	}
}

func TestParseIDRejectsInvalidValues(t *testing.T) {
	t.Parallel()

	for _, value := range []string{
		"",
		"not-a-uuid",
		"00000000-0000-4000-8000-000000000000",
		"00000000-0000-7000-0000-000000000000",
	} {
		if _, err := ParseID(value); !errors.Is(err, ErrInvalidID) {
			t.Errorf("ParseID(%q) error = %v, want ErrInvalidID", value, err)
		}
	}
}

package fabricrunner

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"time"
)

// ID is a UUIDv7 identifier. Its text representation sorts by creation time.
type ID string

var ErrInvalidID = errors.New("invalid fabricrunner ID")

// NewID creates a UUIDv7 using the current UTC time and cryptographic entropy.
func NewID() (ID, error) {
	return newIDAt(time.Now().UTC(), rand.Reader)
}

// ParseID validates and returns a UUIDv7 identifier.
func ParseID(value string) (ID, error) {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return "", fmt.Errorf("%w: malformed UUID", ErrInvalidID)
	}

	compact := value[0:8] + value[9:13] + value[14:18] + value[19:23] + value[24:36]
	var raw [16]byte
	if _, err := hex.Decode(raw[:], []byte(compact)); err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidID, err)
	}
	if raw[6]>>4 != 0x7 {
		return "", fmt.Errorf("%w: UUID version is not 7", ErrInvalidID)
	}
	if raw[8]&0xc0 != 0x80 {
		return "", fmt.Errorf("%w: UUID variant is not RFC 9562", ErrInvalidID)
	}

	return ID(value), nil
}

func newIDAt(now time.Time, entropy io.Reader) (ID, error) {
	millis := now.UnixMilli()
	if millis < 0 || uint64(millis) >= 1<<48 {
		return "", fmt.Errorf("%w: timestamp outside UUIDv7 range", ErrInvalidID)
	}

	var raw [16]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return "", fmt.Errorf("create ID: %w", err)
	}

	ms := uint64(millis)
	raw[0] = byte(ms >> 40)
	raw[1] = byte(ms >> 32)
	raw[2] = byte(ms >> 24)
	raw[3] = byte(ms >> 16)
	raw[4] = byte(ms >> 8)
	raw[5] = byte(ms)
	raw[6] = raw[6]&0x0f | 0x70
	raw[8] = raw[8]&0x3f | 0x80

	var encoded [36]byte
	hex.Encode(encoded[0:8], raw[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], raw[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], raw[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], raw[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], raw[10:16])

	return ID(encoded[:]), nil
}

func (id ID) String() string { return string(id) }

func (id ID) IsZero() bool { return id == "" }

func (id ID) Validate() error {
	if id.IsZero() {
		return fmt.Errorf("%w: empty", ErrInvalidID)
	}
	_, err := ParseID(string(id))
	return err
}

package canonical

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const VersionV1 uint16 = 1

var identifierPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,127}$`)

type Commitment [32]byte

func (c Commitment) String() string { return "0x" + hex.EncodeToString(c[:]) }

func ParseCommitment(value []byte) (Commitment, error) {
	if len(value) != sha256.Size {
		return Commitment{}, fmt.Errorf("canonical: commitment must be 32 bytes, got %d", len(value))
	}
	var result Commitment
	copy(result[:], value)
	return result, nil
}

func Hash(canonicalBytes []byte) Commitment { return sha256.Sum256(canonicalBytes) }

type encoder struct{ bytes.Buffer }

func newEncoder(objectType byte) *encoder {
	e := &encoder{}
	e.WriteString("HTRL")
	e.u16(VersionV1)
	e.WriteByte(objectType)
	return e
}

func (e *encoder) u16(value uint16) {
	var encoded [2]byte
	binary.BigEndian.PutUint16(encoded[:], value)
	e.Write(encoded[:])
}

func (e *encoder) lengthPrefixed(value string) {
	var encoded [4]byte
	binary.BigEndian.PutUint32(encoded[:], uint32(len(value)))
	e.Write(encoded[:])
	e.WriteString(value)
}

func (e *encoder) identifier(value string) error {
	if !identifierPattern.MatchString(value) {
		return fmt.Errorf("canonical: invalid identifier %q", value)
	}
	e.lengthPrefixed(value)
	return nil
}

func (e *encoder) nullableIdentifier(value *string) error {
	if value == nil {
		e.WriteByte(0)
		return nil
	}
	e.WriteByte(1)
	return e.identifier(*value)
}

func (e *encoder) text(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("canonical: text is not valid UTF-8")
	}
	if !norm.NFC.IsNormalString(value) {
		return errors.New("canonical: text is not Unicode NFC")
	}
	e.lengthPrefixed(value)
	return nil
}

func (e *encoder) timestamp(value time.Time) error {
	if value.IsZero() {
		return errors.New("canonical: timestamp is required")
	}
	nanos := value.UnixNano()
	if !time.Unix(0, nanos).Equal(value) {
		return errors.New("canonical: timestamp is outside signed Unix-nanosecond range")
	}
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], uint64(nanos))
	e.Write(encoded[:])
	return nil
}

func (e *encoder) nullableTimestamp(value *time.Time) error {
	if value == nil {
		e.WriteByte(0)
		return nil
	}
	e.WriteByte(1)
	return e.timestamp(*value)
}

func (e *encoder) commitment(value Commitment) { e.Write(value[:]) }

func (e *encoder) nullableCommitment(value *Commitment) {
	if value == nil {
		e.WriteByte(0)
		return
	}
	e.WriteByte(1)
	e.commitment(*value)
}

func NormalizeText(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", errors.New("canonical: text is not valid UTF-8")
	}
	return norm.NFC.String(value), nil
}

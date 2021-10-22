package protocol

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestParseNewPacketEmptySecret(t *testing.T) {
	packet := NewPacket('C', 32837, "willy_nilly", "special.golang.org", "")
	asBytes, err := packet.Bytes()
	if err != nil {
		t.Error(err)
	}
	parsed, err := ParsePacket(asBytes)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Command != packet.Command {
		t.Fatalf("parsed command %s", string(parsed.Command))
	}
	if parsed.MessageID != packet.MessageID {
		t.Fatalf("parsed message id %d", parsed.MessageID)
	}
	if strings.TrimSpace(string(parsed.Namespace)) != "willy_nilly" {
		t.Fatalf("parsed Namespace %s", string(parsed.Namespace))
	}
	if strings.TrimSpace(string(parsed.DataValue)) != "special.golang.org" {
		t.Fatalf("parsed value %s", string(parsed.DataValue))
	}
}

func TestParseNewPacketFromPartsWishSecret(t *testing.T) {
	secret := []byte{115, 117, 112, 101, 114, 115, 101, 99, 114, 101, 116}
	packet := NewPacketFromParts(
		'C',
		[]byte{23, 104, 3, 0}, // message id
		[]byte{119, 105, 108, 108, 121, 95, 110, 105, 108, 108, 121}, // namespace
		[]byte{114, 97, 110, 100, 111, 109},                          // data value
		secret, // with auth
	)
	asBytes, err := packet.Bytes()
	if err != nil {
		t.Error(err)
	}
	parsed, err := ParsePacket(asBytes)
	if err != nil {
		t.Fatal(err)
	}

	if parsed.Command != 'C' {
		t.Fatalf("parsed command %s", string(parsed.Command))
	}
	if parsed.MessageID != 223255 {
		t.Fatalf("parsed message id %d", parsed.MessageID)
	}
	if parsed.NamespaceString() != "willy_nilly" {
		t.Fatalf("parsed Namespace %s", string(parsed.Namespace))
	}
	if parsed.DataValueString() != "random" {
		t.Fatalf("parsed value %s", string(parsed.DataValue))
	}
	assert.Nil(t, parsed.Validate(secret))
}

func TestParsePacketSizeTooLarge(t *testing.T) {
	packet := NewPacket('C', 3321, "somebody", "will replace", "")
	b, err := packet.Bytes() // will be 1500
	if err != nil {
		t.Fatal(err)
	}
	// now overload packet size
	b = append(b, []byte("ksdkfjjskdfjaksdjfkasdkjf")...)
	assert.Equal(t, 1525, len(b)) // pre check

	packet, err = ParsePacket(b)
	assert.Error(t, ErrInvalidPacketSize, err)
	assert.Equal(t, DataValueSize, len(packet.DataValue),
		"should have still parsed packet and dropped bytes")
}

func TestParsePacketSizeTooSmall(t *testing.T) {
	packet := NewPacket('C', 32837, "willy_nilly", "special.golang.org", "")
	b, err := packet.Bytes() // will be 1500
	if err != nil {
		t.Fatal(err)
	}
	// now make packet size short by 200 bytes
	b = b[0:1300]
	assert.Equal(t, 1300, len(b)) // pre check

	packet, err = ParsePacket(b)
	assert.Error(t, ErrInvalidPacketSize, err)
	assert.Equal(t, DataValueSize, len(packet.DataValue),
		"should have still parsed packet padded end bytes")

	// now try packet that is way too small - should not panic

	tinyBytePacket := []byte{'C', 99, ' ', 'a', 'b', 'c', ' ', 'd', 'e', 'f'}
	tinyPacket, err := ParsePacket(tinyBytePacket)
	assert.Error(t, ErrInvalidPacketSize)
	assert.Nil(t, tinyPacket)
}

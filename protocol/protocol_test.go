package protocol

import (
	"github.com/stretchr/testify/assert"
	"strings"
	"testing"
)

func TestParsePacket(t *testing.T) {
	t.Run("packet parsing", func(t *testing.T) {
		packet := &Packet{
			Command:   'C',
			MessageID: 32837,
			Namespace: []byte("willy_nilly"),
			DataValue: []byte("special.golang.org"),
		}
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
	})
}

func TestParsePacketSizeTooLarge(t *testing.T) {
	packet := &Packet{
		Command:   'C',
		MessageID: 3321,
		Namespace: []byte("somebody"),
		DataValue: []byte("will replace"),
	}
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
	packet := &Packet{
		Command:   'C',
		MessageID: 3321,
		Namespace: []byte("somebody"),
		DataValue: []byte("will replace"),
	}
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
}

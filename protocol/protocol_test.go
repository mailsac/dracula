package protocol

import (
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

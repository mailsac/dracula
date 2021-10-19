package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"unsafe"
)

const (
	PacketSize    = 1500
	namespaceSize = 64
	DataValueSize = 1428

	CmdCount byte = 'C'
	CmdPut   byte = 'P'
	ResError byte = 'E'

	space       byte = ' '
	spaceIndex1      = 1
	spaceIndex2      = 6
	spaceIndex3      = 71
)

var (
	ErrInvalidPacketSize  = errors.New("bad packet: size must be 1500 bytes")
	ErrInvalidCommandByte = errors.New("bad packet: invalid command byte")
	ErrProtocolSpace1     = errors.New("bad packet: expected space 1")
	ErrProtocolSpace2     = errors.New("bad packet: expected space 1")
	ErrProtocolSpace3     = errors.New("bad packet: expected space 3")
	ErrBadOutputSize      = errors.New("wrong data size during packet construction")
)

// IsRequestCmd indicates if the server should accept this as a command
func IsRequestCmd(c byte) bool {
	return c == CmdCount || c == CmdPut
}

// IsResponseCmd indicates if the client should accept this as a command
func IsResponseCmd(c byte) bool {
	return c == ResError || c == CmdCount || c == CmdPut
}

type Packet struct {
	Command   byte
	MessageID uint32 // fixed 4 byte number
	Namespace []byte // fixed 64 byte string
	DataValue []byte // fixed 1428 byte string
}

func (p *Packet) NamespaceString() string {
	return strings.TrimSpace(string(p.Namespace))
}

func (p *Packet) DataValueString() string {
	return strings.TrimSpace(string(p.DataValue))
}

// ParsePacket parses a packet like:
//    [Command char][space][Message ID uint32][space][Namespace 64 bytes][space][data remaining bytes]
//
// The MTU of 1500 is the maximum allowed packet size. That means the data key can only be 1428
// bytes max.
//
// The goal here is fast and simple tracking of expirable keys, so the trade off is that
// the keys have limited length.
//
// An invalid packet will still be returned, in which case error will not be nil.
func ParsePacket(buf []byte) (*Packet, error) {
	// if not meeting minimum packet size where we , cannot parse packet below
	if len(buf) < spaceIndex3+2 {
		return &Packet{}, ErrInvalidPacketSize
	}
	// allows shorter packet to be turned into 1500 byte total packet
	endAt := int(math.Min(float64(len(buf)), PacketSize))
	initialData := buf[spaceIndex3+1:endAt]
	rightSizeData := *padRight(&initialData, DataValueSize)

	p := Packet{
		Command:   buf[0],                               // then a space
		MessageID: binary.LittleEndian.Uint32(buf[2:6]), // then a space
		Namespace: buf[spaceIndex2+1 : spaceIndex3],     // then a space
		DataValue: rightSizeData,
	}

	if len(buf) != PacketSize {
		return &p, ErrInvalidPacketSize
	}

	//fmt.Println("ParsePacket() message id", p.MessageID, "|")
	//fmt.Println("ParsePacket() Namespace", p.Namespace, "|", len(p.Namespace))

	commandIsValid := IsRequestCmd(p.Command) || IsResponseCmd(p.Command)
	if !commandIsValid {
		return &p, ErrInvalidCommandByte
	}

	// expected spaces at fixed spots
	if buf[spaceIndex1] != space {
		fmt.Println("packet space 1 was", buf[spaceIndex1], "instead of", space, "\n", buf)
		return &p, ErrProtocolSpace1
	}
	if buf[spaceIndex2] != space {
		fmt.Println("packet space 2 was", buf[spaceIndex2], "instead of", space, "\n", buf)
		return &p, ErrProtocolSpace2
	}
	if buf[spaceIndex3] != space {
		fmt.Println("packet space 3 was", buf[spaceIndex3], "instead of", space, "\n", buf)
		return &p, ErrProtocolSpace3
	}

	return &p, nil
}

// Bytes formats the packet for transport. The first 8 bytes are a header.
// The last byte should be a line break. The data is a UTF-8 string.
func (p *Packet) Bytes() ([]byte, error) {
	//fmt.Println("Bytes()       message id", p.MessageID, "|")
	//fmt.Println("Bytes()       Namespace", p.Namespace, "|", len(p.Namespace))

	mID := (*[4]byte)(unsafe.Pointer(&p.MessageID))
	ns := *padRight(&p.Namespace, namespaceSize)
	val := *padRight(&p.DataValue, DataValueSize)

	out := []byte{
		p.Command,
		' ',
		mID[0], mID[1], mID[2], mID[3],
		' ',
	}
	out = append(out, ns...)
	out = append(out, ' ')
	out = append(out, val...)
	if len(out) != PacketSize {
		fmt.Println("packet size outputted was", len(out))
		return nil, ErrBadOutputSize
	}

	return out, nil
}

// padRight adds char space to make buffer reach desired size
func padRight(in *[]byte, finalSize int) *[]byte {
	inputLen := len(*in)
	if inputLen >= finalSize {
		return in // not copied if correcti size
	}
	out := make([]byte, finalSize)
	copy(out[0:inputLen], *in)
	for i := inputLen; i < finalSize; i++ {
		out[i] = space
	}
	if len(out) != finalSize {
		log.Panicf("bad final size, expected %d, got %d", finalSize, len(out))
	}
	return &out
}

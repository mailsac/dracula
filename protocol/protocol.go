package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/OneOfOne/xxhash"
	"log"
	"math"
	"net"
	"strings"
)

const (
	PacketSize    = 1500
	NamespaceSize = 64
	DataValueSize = 1419

	CmdCount          byte = 'C'
	CmdPut            byte = 'P'
	CmdPutReplicate   byte = 'R'
	CmdCountNamespace byte = 'N'
	CmdCountServer    byte = 'S'

	CmdTCPOnlyKeys     byte = 'K'
	CmdTCPOnlyValues   byte = 'V'
	CmdTCPOnlyStore    byte = 'T'
	CmdTCPOnlyRetrieve byte = 'V'

	ResError byte = 'E'

	space       byte = ' '
	spaceIndex1      = 1
	spaceIndex2      = 10
	spaceIndex3      = 15
	spaceIndex4      = 80
)

var (
	ErrInvalidPacketSize  = errors.New("bad packet: size must be 1500 bytes")
	ErrInvalidCommandByte = errors.New("bad packet: invalid command byte")
	ErrProtocolSpace1     = errors.New("bad packet: expected space 1")
	ErrProtocolSpace2     = errors.New("bad packet: expected space 1")
	ErrProtocolSpace3     = errors.New("bad packet: expected space 3")
	ErrBadHash            = errors.New("auth failed: packet hash invalid")
	ErrBadOutputSize      = errors.New("wrong data size during packet construction")
)

var StopSymbol = []byte("\n.\n")

// IsRequestCmd indicates if the server should accept this as a command
func IsRequestCmd(c byte) bool {
	return c == CmdCount || c == CmdPut || c == CmdCountNamespace || c == CmdCountServer || c == CmdPutReplicate
}

// IsResponseCmd indicates if the client should accept this as a command
func IsResponseCmd(c byte) bool {
	return c == ResError || IsRequestCmd(c) // any request will be ack'd back
}

type Packet struct {
	Command        byte
	HashBytes      []byte // fixed 8 byte number
	Hash           uint64 // fixed 8 byte number
	MessageIDBytes []byte // fixed 4 byte number
	MessageID      uint32 // fixed 4 byte number
	Namespace      []byte // fixed 64 byte string
	DataValue      []byte // fixed 1419 byte string

	RequestClient *net.TCPConn
}

// NewPacket is a friendlier way to construct a packet and will provide conversions inline
func NewPacket(command byte, messageID uint32, namespace, dataValue, preSharedKey string) *Packet {
	ns := []byte(namespace)
	dv := []byte(dataValue)
	p := NewPacketFromParts(command, Uint32ToBytes(messageID), ns, dv, []byte(preSharedKey))
	return p
}

func NewPacketFromParts(command byte, messageID, namespace, dataValue, preSharedKey []byte) *Packet {
	// TODO: consider adding validations
	p := &Packet{
		Command:        command,
		MessageID:      Uint32FromBytes(messageID),
		MessageIDBytes: messageID,
		Namespace:      *padRight(&namespace, NamespaceSize),
		DataValue:      *padRight(&dataValue, DataValueSize),
	}
	p.SetHash(preSharedKey)
	return p
}

func (p *Packet) NamespaceString() string {
	return strings.TrimSpace(string(p.Namespace))
}

func (p *Packet) DataValueString() string {
	return strings.TrimSpace(string(p.DataValue))
}

// ParsePacket parses a packet like:
//    [Command char][space][xxhash of pre shared key + id + ns + data][space][Message ID uint32][space][Namespace 64 bytes][space][data remaining bytes]
//
// The MTU of 1500 is the maximum allowed packet size. That means the data key can only be 1419
// bytes max.
//
// The goal here is fast and simple tracking of expirable keys, so the trade off is that
// the keys have limited length.
//
// An invalid packet may still be returned it can be parsed partially, in which case error will not be nil.
// If a packet is way too small, the pointer will be nil.
func ParsePacket(buf []byte) (*Packet, error) {
	// if not meeting minimum packet size where we, cannot parse packet below
	if len(buf) < spaceIndex4+2 {
		return nil, ErrInvalidPacketSize
	}
	hBytes := buf[spaceIndex1+1 : spaceIndex2]
	idBytes := buf[spaceIndex2+1 : spaceIndex3]
	nsBytes := buf[spaceIndex3+1 : spaceIndex4]
	// allows shorter packet to be turned into 1500 byte total packet
	endAt := int(math.Min(float64(len(buf)), PacketSize))
	messageIData := buf[spaceIndex4+1 : endAt]
	rightSizeData := *padRight(&messageIData, DataValueSize)
	p := Packet{
		Command: buf[0], // then a space

		HashBytes: hBytes,
		Hash:      Uint64FromBytes(hBytes),

		MessageIDBytes: idBytes,                  // stored for hashing purposes later
		MessageID:      Uint32FromBytes(idBytes), // then a space

		Namespace: nsBytes, // then a space

		DataValue: rightSizeData,
	}

	if len(buf) != PacketSize {
		return &p, ErrInvalidPacketSize
	}

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
	if buf[spaceIndex4] != space {
		fmt.Println("packet space 4 was", buf[spaceIndex4], "instead of", space, "\n", buf)
		return &p, ErrProtocolSpace3
	}

	return &p, nil
}

// bytes formats the packet for transport. The first 8 bytes are a header.
//// The last byte should be a line break. The data is a UTF-8 string.
func (p *Packet) bytes() []byte {
	//fmt.Println("Bytes()       message id", p.MessageID, "|")
	//fmt.Println("Bytes()       Namespace", p.Namespace, "|", len(p.Namespace))
	if len(p.HashBytes) < 8 {
		panic("Packet.Bytes() called before setting Packet hash!")
	}
	if len(p.MessageIDBytes) < 4 {
		panic("Packet.Bytes() called without MessageIDBytes!")
	}

	namespace := *padRight(&p.Namespace, NamespaceSize)
	dataValue := *padRight(&p.DataValue, DataValueSize)

	out := []byte{
		p.Command,
		' ',
		p.HashBytes[0], p.HashBytes[1], p.HashBytes[2], p.HashBytes[3], p.HashBytes[4], p.HashBytes[5], p.HashBytes[6], p.HashBytes[7],
		' ',
		p.MessageIDBytes[0], p.MessageIDBytes[1], p.MessageIDBytes[2], p.MessageIDBytes[3],
		' ',
	}
	out = append(out, namespace...)
	out = append(out, ' ')
	out = append(out, dataValue...)

	return out
}

// Bytes formats the packet for UDP transport.
func (p *Packet) Bytes() ([]byte, error) {
	out := p.bytes()
	if len(out) != PacketSize {
		fmt.Println("packet size outputted was", len(out))
		return nil, ErrBadOutputSize
	}

	return out, nil
}

// TCPBytes formats the packet for TCP transport.
func (p *Packet) BytesTCP() ([]byte, error) {
	out := p.bytes()
	return out, nil
}

// HashPacket returns an 8 byte slice
func HashPacket(p *Packet, preSharedKey []byte) []byte {
	hasher := xxhash.New64()
	// omit the spaces and hash the Message ID, Namespace, and DataValue
	bytesToHash := append(preSharedKey, p.MessageIDBytes...)
	bytesToHash = append(bytesToHash, p.Namespace...)
	bytesToHash = append(bytesToHash, p.DataValue...)
	return hasher.Sum(bytesToHash)
}

// SetHash puts the hash on a packet
func (p *Packet) SetHash(preSharedKey []byte) {
	// omit the spaces and hash the Message ID, Namespace, and DataValue
	bytesToHash := append(preSharedKey, p.MessageIDBytes...)
	bytesToHash = append(bytesToHash, p.Namespace...)
	bytesToHash = append(bytesToHash, p.DataValue...)
	p.HashBytes = HashPacket(p, preSharedKey)
	p.Hash = Uint64FromBytes(p.HashBytes)
}

// Validate returns an error is the packet's hash does not authenticate against the preSharedKey.
func (p *Packet) Validate(preSharedKey []byte) error {
	expectedHash := Uint64FromBytes(HashPacket(p, preSharedKey))
	if p.Hash != expectedHash {
		fmt.Printf("packet hash fail, packet: %d, server: %d \n", p.Hash, expectedHash)
		return ErrBadHash
	}
	return nil
}

// padRight adds char space to make buffer reach desired size. If `in` is larger
// than `finalSize`, nothing happens.
func padRight(in *[]byte, finalSize int) *[]byte {
	inputLen := len(*in)
	if inputLen >= finalSize {
		return in // not copied if already correct size
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

func Uint32FromBytes(fourBytes []byte) uint32 {
	return binary.LittleEndian.Uint32(fourBytes)
}

// Uint32ToBytes returns a 4 byte slice
func Uint32ToBytes(u uint32) []byte {
	fourBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(fourBytes, u)
	return fourBytes
}

func Uint64FromBytes(eightBytes []byte) uint64 {
	return binary.LittleEndian.Uint64(eightBytes)
}

// Uint64ToBytes returns an 8 byte slice
func Uint64ToBytes(u uint64) []byte {
	eightBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(eightBytes, u)
	return eightBytes
}

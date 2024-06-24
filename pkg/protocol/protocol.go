package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"

	"github.com/OneOfOne/xxhash"
)

const (
	PacketSize int = 1500

	CommandOffset   int = 0
	MessageIDOffset int = 1
	NamespaceOffset int = 5
	DataValueOffset int = 69
	HashOffset      int = 1492

	CmdCount          byte = 'C'
	CmdPut            byte = 'P'
	CmdPutReplicate   byte = 'R'
	CmdCountNamespace byte = 'N'
	CmdCountServer    byte = 'S'

	CmdTCPOnlyKeys       byte = 'K'
	CmdTCPOnlyValues     byte = 'V'
	CmdTCPOnlyStore      byte = 'T'
	CmdTCPOnlyRetrieve   byte = 'I'
	CmdTCPOnlyNamespaces byte = 'L'
)

var (
	ErrInvalidPacketSizeTooSmall = errors.New("bad packet: too small, size must be 1500 bytes")
	ErrInvalidPacketSizeTooLarge = errors.New("bad packet: too large, size must be 1500 bytes")
	ErrInvalidCommandByte        = errors.New("bad packet: invalid command byte")
	ErrProtocolSpace1            = errors.New("bad packet: expected space 1")
	ErrProtocolSpace2            = errors.New("bad packet: expected space 1")
	ErrProtocolSpace3            = errors.New("bad packet: expected space 3")
	ErrBadHash                   = errors.New("auth failed: packet hash invalid")
	ErrBadOutputSize             = errors.New("wrong data size during packet construction")
	ErrBadReadCall               = errors.New("packet reader error")
	ErrBadReadSize               = errors.New("invalid packet read size")
	ErrBadWriteCall              = errors.New("packet writer error")
	ErrBadWriteSize              = errors.New("invalid packet write size")
)

// Packet zero copy structure for 1500 byte messages
type Packet struct {
	buffer []byte
}

// NewPacket creates a packet from an existing slice of 1500 bytes or initializes. If
// buffer is nil, a new buffer is allocated.
func NewPacket(buffer []byte) *Packet {
	packet := &Packet{
		buffer: buffer,
	}
	if packet.buffer == nil {
		packet.buffer = make([]byte, PacketSize)
	}
	return packet
}

// GetCommand gets command byte
func (p *Packet) GetCommand() byte {
	return p.buffer[CommandOffset]
}

// SetCommand sets command byte
func (p *Packet) SetCommand(command byte) {
	p.buffer[CommandOffset] = command
}

// GetMessageID gets message ID
func (p *Packet) GetMessageID() uint32 {
	return binary.LittleEndian.Uint32(p.buffer[MessageIDOffset:NamespaceOffset])
}

// SetMessageID sets message ID
func (p *Packet) SetMessageID(messageID uint32) {
	binary.LittleEndian.PutUint32(p.buffer[MessageIDOffset:NamespaceOffset], messageID)
}

// GetNamespace gets namespace
func (p *Packet) GetNamespace() string {
	return p.getString(NamespaceOffset, 64)
}

// SetNamespace sets namespace
func (p *Packet) SetNamespace(namespace string) {
	p.setString(NamespaceOffset, 64, namespace)
}

// GetDataValue gets data value
func (p *Packet) GetDataValue() string {
	return p.getString(DataValueOffset, 1423)
}

// SetDataValue sets data value
func (p *Packet) SetDataValue(dataValue string) {
	p.setString(DataValueOffset, 1423, dataValue)
}

// GetHash gets hash
func (p *Packet) GetHash() uint64 {
	return binary.LittleEndian.Uint64(p.buffer[HashOffset:PacketSize])
}

// GetHash sets hash
func (p *Packet) SetHash(hash uint64) {
	binary.LittleEndian.PutUint64(p.buffer[HashOffset:PacketSize], hash)
}

// Sign hashes the packet with a given key and stores it in the packet hash value
func (p *Packet) Sign(key []byte) {
	hash := p.hash(key)
	p.SetHash(hash)
}

// Verify checks the stored hash from the packet using a given key
func (p *Packet) Verify(key []byte) error {
	hash := p.hash(key)
	if hash != p.GetHash() {
		return ErrBadHash
	}
	return nil
}

// getString decodes string up to maxSize length starting from offset
func (p *Packet) getString(offset, maxSize int) string {
	stringSize := bytes.IndexByte(p.buffer[offset:offset+maxSize], 0)
	if stringSize == -1 {
		stringSize = maxSize
	}
	buffer := p.buffer[offset : offset+stringSize]
	return string(buffer)
}

// setString encodes string up to maxSize length starting at offset
func (p *Packet) setString(offset, maxSize int, value string) {
	for index := offset; index < offset+maxSize; index++ {
		p.buffer[index] = 0
	}
	_ = copy(p.buffer[offset:], []byte(value))
}

func (p *Packet) hash(key []byte) uint64 {
	hasher := xxhash.New64()
	_, _ = hasher.Write(key)
	_, _ = hasher.Write(p.buffer[0:HashOffset])
	return hasher.Sum64()
}

// ReadPacket extracts a packet from a reader. Returns error if the reader fails or less than 1500 bytes are read.
func ReadPacket(reader io.Reader) (*Packet, error) {
	buffer := make([]byte, PacketSize)
	size, err := reader.Read(buffer)
	if err != nil {
		return nil, ErrBadReadCall
	} else if size != PacketSize {
		return nil, ErrBadReadSize
	}
	return NewPacket(buffer), nil
}

// ReadPacket insterts a packet into a writer. Returns error if the writer fails or less than 1500 bytes are written.
func WritePacket(writer io.Writer, packet *Packet) error {
	size, err := writer.Write(packet.buffer)
	if err != nil {
		return ErrBadWriteCall
	} else if size != PacketSize {
		return ErrBadWriteSize
	}
	return nil
}

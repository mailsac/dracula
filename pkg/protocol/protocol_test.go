package protocol

import (
	"bytes"
	"errors"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	success  int = 0
	truncate int = 1
	failure  int = 2

	expectedCommand   byte   = CmdCount
	expectedMessageID uint32 = 1
	expectedHash      uint64 = 12345
	expectedDataValue string = "datavalue"
)

var (
	// test the edge case of having a string of max size
	expectedNamespace string = generateAlphaNumericString(64)
)

// Packet buffer manager with configurable error states
type MockPacketStorage struct {
	state  int
	buffer []byte
}

// NewMockPacketStorage creates a new MockPacketStorage with an empty buffer and no error state
func NewMockPacketStorage() *MockPacketStorage {
	return &MockPacketStorage{
		state:  success,
		buffer: make([]byte, PacketSize),
	}
}

// Read either copies from the read buffer or simulates some read error
func (m MockPacketStorage) Read(buffer []byte) (int, error) {
	if m.state == truncate {
		return len(buffer) / 2, nil
	} else if m.state == failure {
		return 0, errors.New("mock error")
	}
	return copy(buffer, m.buffer), nil
}

// Write either copies to the write buffer or simulates some write error
func (m MockPacketStorage) Write(buffer []byte) (int, error) {
	if m.state == truncate {
		return len(buffer) / 2, nil
	} else if m.state == failure {
		return 0, errors.New("mock error")
	}
	return copy(m.buffer, buffer), nil
}

// TestWriteRead verifies that we can write and read a packet from the same block of bytes
func TestWriteRead(t *testing.T) {
	storage := NewMockPacketStorage()
	writePacket := NewPacket(nil)
	writePacket.SetCommand(expectedCommand)
	writePacket.SetMessageID(expectedMessageID)
	writePacket.SetNamespace(expectedNamespace)
	writePacket.SetDataValue(expectedDataValue)
	writePacket.SetHash(expectedHash)

	storage.state = truncate
	err := WritePacket(storage, writePacket)
	assert.ErrorIs(t, err, ErrBadWriteSize)

	storage.state = failure
	err = WritePacket(storage, writePacket)
	assert.ErrorIs(t, err, ErrBadWriteCall)

	storage.state = success
	err = WritePacket(storage, writePacket)
	assert.NoError(t, err)

	storage.state = truncate
	readPacket, err := ReadPacket(storage)
	assert.Nil(t, readPacket)
	assert.ErrorIs(t, err, ErrBadReadSize)

	storage.state = failure
	readPacket, err = ReadPacket(storage)
	assert.Nil(t, readPacket)
	assert.ErrorIs(t, err, ErrBadReadCall)

	storage.state = success
	readPacket, err = ReadPacket(storage)
	assert.NotNil(t, readPacket)
	assert.NoError(t, err)

	assert.Equal(t, expectedCommand, readPacket.GetCommand())
	assert.Equal(t, expectedMessageID, readPacket.GetMessageID())
	assert.Equal(t, expectedNamespace, readPacket.GetNamespace())
	assert.Equal(t, expectedDataValue, readPacket.GetDataValue())
	assert.Equal(t, expectedHash, readPacket.GetHash())
}

// TestSignVerify checks the Sign/Verify functions using various keys and the hashes
func TestSignVerify(t *testing.T) {
	goodKey := []byte("this is the good key")
	badKey := []byte("this is the bad key")
	packet := NewPacket(nil)

	packet.Sign(goodKey)

	err := packet.Verify(badKey)
	assert.Error(t, err)

	err = packet.Verify(goodKey)
	assert.NoError(t, err)
}

func generateAlphaNumericString(size int) string {
	chars := []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789")
	buffer := bytes.NewBuffer(nil)
	buffer.Grow(size)
	for buffer.Len() < size {
		buffer.WriteByte(chars[rand.Intn(len(chars))])
	}
	return buffer.String()
}

package client

import (
	"github.com/mailsac/dracula/protocol"
	"github.com/mailsac/dracula/server"
	"github.com/stretchr/testify/assert"
	"math"
	"sync"
	"testing"
	"time"
)

func TestClient_Auth(t *testing.T) {
	// there are already tests with empty secret as well

	secret := "asdf-jkl-HOHOHO!"
	badSecret := "Brute-Force9"
	s := server.NewServer(60, secret)
	err := s.Listen(9000)
	if err != nil {
		t.Fatal(err)
	}
	s.Debug = true
	defer s.Close()

	goodClient := NewClient("127.0.0.1", 9000, 5, secret)
	err = goodClient.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	goodClient.Debug = true
	defer goodClient.Close()

	badClient := NewClient("127.0.0.1", 9000, 5, badSecret)
	err = badClient.Listen(9002)
	if err != nil {
		t.Fatal(err)
	}
	badClient.Debug = true
	defer badClient.Close()

	// good client checks
	err = goodClient.Put("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	// check it worked with auth
	c, err := goodClient.Count("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	if c != 1 {
		t.Error("expected pre check count=1, got=", c)
	}

	// bad client checks, put same and count same
	err = badClient.Put("asdf", "99.33.22.44")
	assert.Error(t, err)
	assert.Equal(t, "auth failed: packet hash invalid", err.Error())
}

func TestClient_messageIDOverflow(t *testing.T) {
	cl := NewClient("127.0.0.1", 9000, time.Second*5, "")
	cl.messageIDCounter = math.MaxUint32 - 1
	actual := protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(math.MaxUint32), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(0), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(1), actual)
}

func TestClient_messageIDThreadSafe(t *testing.T) {
	cl := NewClient("127.0.0.1", 9000, time.Second*5, "")
	var wg sync.WaitGroup
	const expected uint32 = 5001

	for i := 0; i < 50; i++ {
		wg.Add(1)

		go func() {
			for c := 0; c < 100; c++ {
				cl.makeMessageID()
			}
			wg.Done()
		}()
	}

	// Wait until all the goroutines are done.
	wg.Wait()
	actual := protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, expected, actual)
}

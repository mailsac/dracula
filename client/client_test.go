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
	s := server.NewServer(60, secret)
	s.DebugEnable("9000")
	err := s.Listen(9000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	goodClient := NewClient("127.0.0.1:9000", 5, secret)
	goodClient.DebugEnable("9001")
	err = goodClient.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	defer goodClient.Close()

	// START with good secret so it can connect to server in pool, then switch to bad later
	badClient := NewClient("127.0.0.1:9000", 5, secret)
	err = badClient.Listen(9002)
	if err != nil {
		t.Fatal(err)
	}
	badClient.DebugEnable("9002")
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

	// pre-check
	assert.Equal(t, "[127.0.0.1:9000]", badClient.pool.ListHealthy())
	// change to BAD secret!
	badClient.preSharedKey = []byte("Brute-Force9")
	err = badClient.Put("asdf", "99.33.22.44")
	assert.Error(t, err)
	assert.Equal(t, "auth failed: packet hash invalid", err.Error())
}

func TestClient_Healthcheck(t *testing.T) {
	s1 := server.NewServer(60, "sec1")
	s1.DebugEnable("9000")
	err := s1.Listen(9000)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()

	s2 := server.NewServer(60, "sec1")
	s2.DebugEnable("9100")
	err = s2.Listen(9100)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	c1 := NewClient("127.0.0.1:9000,127.0.0.1:9100,127.0.0.1:99999", 5, "sec1")
	c1.pool.Debug = true
	c1.DebugEnable("9001")
	err = c1.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	assert.Equal(t, "[127.0.0.1:9000 127.0.0.1:9100 127.0.0.1:99999]", c1.pool.ListServers(), "did not parse servers correctly")
	assert.Equal(t, "[127.0.0.1:9000 127.0.0.1:9100]", c1.pool.ListHealthy())
	assert.Equal(t, "[127.0.0.1:99999]", c1.pool.ListUnHealthy())
}

func TestClient_messageIDOverflow(t *testing.T) {
	cl := NewClient("127.0.0.1:9000", time.Second*5, "")
	cl.messageIDCounter = math.MaxUint32 - 1
	actual := protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(math.MaxUint32), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(0), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(1), actual)
}

func TestClient_messageIDThreadSafe(t *testing.T) {
	cl := NewClient("127.0.0.1:9000", time.Second*5, "")
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

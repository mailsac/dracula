package client

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/mailsac/dracula/protocol"
	"github.com/mailsac/dracula/server"
	"github.com/stretchr/testify/assert"
)

func TestClient_Auth(t *testing.T) {
	// there are already tests with empty secret as well

	secret := "asdf-jkl-HOHOHO!"
	s := server.NewServer(60, secret)
	s.DebugEnable("9000")
	err := s.Listen(9000, 9000)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	goodClient := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000", Timeout: 5, PreSharedKey: secret})
	goodClient.DebugEnable("9001")
	err = goodClient.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	defer goodClient.Close()

	// START with good secret so it can connect to server in udpPool, then switch to bad later
	badClient := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000", Timeout: 5, PreSharedKey: secret})
	err = badClient.Listen(9002)
	if err != nil {
		t.Fatal(err)
	}
	badClient.DebugEnable("9002")
	defer badClient.Close()

	// good client checks
	err = goodClient.Put("asdf", "99.33.22.44")
	err = goodClient.Put("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	// check it worked with auth
	c, err := goodClient.Count("asdf", "99.33.22.44")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, c)

	// bad client checks, put same and count same

	// pre-check
	assert.Equal(t, "[127.0.0.1:9000]", badClient.udpPool.ListHealthy())
	// change to BAD secret!
	badClient.preSharedKey = []byte("Brute-Force9")
	err = badClient.Put("asdf", "99.33.22.44")
	assert.Error(t, err)
	assert.Equal(t, "auth failed: packet hash invalid", err.Error())
}

func TestClient_Healthcheck(t *testing.T) {
	s1 := server.NewServer(60, "sec1")
	s1.DebugEnable("9000")
	err := s1.Listen(9000, 9000)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()

	s2 := server.NewServer(60, "sec1")
	s2.DebugEnable("9100")
	err = s2.Listen(9100, 9010)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	c1 := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000,127.0.0.1:9100,127.0.0.1:99999", Timeout: 5, PreSharedKey: "sec1"})
	c1.udpPool.Debug = true
	c1.DebugEnable("9001")
	err = c1.Listen(9001)
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	assert.Equal(t, "[127.0.0.1:9000 127.0.0.1:9100 127.0.0.1:99999]", c1.udpPool.ListServers(), "did not parse servers correctly")
	assert.Equal(t, "[127.0.0.1:9000 127.0.0.1:9100]", c1.udpPool.ListHealthy())
	assert.Equal(t, "[127.0.0.1:99999]", c1.udpPool.ListUnHealthy())
}

func TestClient_messageIDOverflow(t *testing.T) {
	cl := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000", Timeout: time.Second * 5})
	cl.messageIDCounter = math.MaxUint32 - 1
	actual := protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(math.MaxUint32), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(0), actual)
	actual = protocol.Uint32FromBytes(cl.makeMessageID())
	assert.Equal(t, uint32(1), actual)
}

func TestClient_messageIDThreadSafe(t *testing.T) {
	cl := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000", Timeout: time.Second * 5})
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

func TestClient_TcpKeyMatch(t *testing.T) {
	t.Run("returns ordered keys with secret", func(t *testing.T) {
		secret := "asdf-!!?!|asdf"
		s := server.NewServer(60, secret)
		s.DebugEnable("9000")
		err := s.Listen(9000, 9000)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		cl := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9000", RemoteTCPIPPortList: "127.0.0.1:9000", Timeout: time.Second * 5, PreSharedKey: secret})
		assert.NoError(t, cl.Listen(9001))
		defer cl.Close()

		assert.NoError(t, cl.Put("default", "blah"))
		assert.NoError(t, cl.Put("default", "blat"))
		assert.NoError(t, cl.Put("default", "blah:ce")) // out of order
		assert.NoError(t, cl.Put("default", "blah:2"))
		assert.NoError(t, cl.Put("default", "blah:a"))
		assert.NoError(t, cl.Put("default", "blaM!"))    // no match
		assert.NoError(t, cl.Put("other", "blah:other")) // other namespace

		matched, err := cl.KeyMatch("default", "blah*")
		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{"blah", "blah:2", "blah:a", "blah:ce"}, matched)

		// use the pool a bit
		matched, err = cl.KeyMatch("other", "blah*")
		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{"blah:other"}, matched)

		matched, err = cl.KeyMatch("notexisting", "blah*")
		assert.NoError(t, err)
		assert.ElementsMatch(t, []string{}, matched)
	})
}

func TestClient_TcpListNamespaces(t *testing.T) {
	t.Run("returns a list of namespaces", func(t *testing.T) {
		secret := "asdf-!!?!|asdf"
		s := server.NewServer(60, secret)
		s.DebugEnable("9011")
		err := s.Listen(9011, 9011)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()

		cl := NewClient(Config{RemoteUDPIPPortList: "127.0.0.1:9011", RemoteTCPIPPortList: "127.0.0.1:9011", Timeout: time.Second * 2, PreSharedKey: secret})
		assert.NoError(t, cl.Listen(9012))
		defer cl.Close()

		insertValues := map[string]string{
			"namespace0": "key0",
			"namespace1": "key1",
		}

		for namespace, value := range insertValues {
			assert.NoError(t, cl.Put(namespace, value))
		}

		namespaces, err := cl.ListNamespaces()
		assert.Len(t, namespaces, 2)
		assert.NoError(t, err) // out of order
	})
}

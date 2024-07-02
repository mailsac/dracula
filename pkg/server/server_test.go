package server

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/mailsac/dracula/pkg/client"
	"github.com/stretchr/testify/assert"
)

func TestServer_Roundtrip(t *testing.T) {
	// setup
	s := NewServer(60, "")
	s.DebugEnable("9000")
	if err := s.Listen(9000, 9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := client.NewClient(client.Config{
		RemoteUDPIPPortList: "127.0.0.1:9000",
		Timeout:             time.Second,
	})
	c.DebugEnable("9001")
	if err := c.Listen(9001); err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	// set
	for i := 0; i < 5; i++ {
		if err := c.Put("default", "bren.msdc.co"); err != nil {
			t.Fatal(i, err)
		}
	}
	for i := 0; i < 3; i++ {
		if err := c.Put("other", "somebody.com"); err != nil {
			t.Fatal(i, err)
		}
	}

	var wg sync.WaitGroup
	cases := []func(){
		func() {
			if count, err := c.Count("default", "bren.msdc.co"); err != nil {
				t.Fatal(err)
			} else {
				assert.Equal(t, 5, count)
			}
		},
		func() {
			if count, err := c.Count("other", "somebody.com"); err != nil {
				t.Fatal(err)
			} else {
				assert.Equal(t, 3, count)
			}
		}, func() {
			if count, err := c.Count("other", "bren.msdc.co"); err != nil {
				t.Fatal(err)
			} else {
				assert.Equal(t, 0, count)
			}
		}, func() {
			if count, err := c.Count("default", "somebody.com"); err != nil {
				t.Fatal(err)
			} else {
				assert.Equal(t, 0, count)
			}
		}, func() {
			if count, err := c.Count("will_it_bork", "anything"); err != nil {
				t.Fatal(err)
			} else {
				assert.Equal(t, 0, count, "should return count for never seen value")
			}
		},
	}

	// run all multithreaded to try to confuse it
	wg.Add(len(cases))
	for _, testCase := range cases {
		go func(tc func()) {
			tc()
			wg.Done()
		}(testCase)
	}
	wg.Wait()

	assert.Equal(t, 0, c.PendingRequests(), "client did not cleanup callbacks")

	// cleanup
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServer_Replication(t *testing.T) {
	peers := "127.0.0.1:9010,127.0.0.1:9020,127.0.0.1:9030"
	// setup 3 servers
	s1 := NewServerWithPeers(60, "asdf", "127.0.0.1:9010", peers)
	s1.DebugEnable("9010")
	if err := s1.Listen(9010, 9010); err != nil {
		t.Fatal(err)
	}

	s2 := NewServerWithPeers(60, "asdf", "127.0.0.1:9020", peers)
	s1.DebugEnable("9020")
	if err := s2.Listen(9020, 9020); err != nil {
		t.Fatal(err)
	}

	s3 := NewServerWithPeers(60, "asdf", "127.0.0.1:9030", peers)
	s3.DebugEnable("9030")
	if err := s3.Listen(9030, 9030); err != nil {
		t.Fatal(err)
	}

	// 2 clients

	// client 1 listens to server 1 ONLY
	c1 := client.NewClient(client.Config{RemoteUDPIPPortList: "127.0.0.1:9010", PreSharedKey: "asdf"})
	c1.DebugEnable("9001")
	if err := c1.Listen(9001); err != nil {
		t.Fatal(err)
	}

	// client 2 creates server pool with only two servers
	c2 := client.NewClient(client.Config{RemoteUDPIPPortList: "127.0.0.1:9010,127.0.0.1:9020", PreSharedKey: "asdf"})
	c2.DebugEnable("9002")
	if err := c2.Listen(9002); err != nil {
		t.Fatal(err)
	}

	// set
	c1.Put("default", "asdf")
	// c2 should hit multiple pool servers
	c2.Put("default", "asdf")
	c2.Put("default", "asdf")
	c2.Put("default", "asdf")
	c2.Put("default", "asdf")
	c2.Put("default", "asdf")
	c2.Put("default", "jjj")
	c2.Put("asdfasdf", "ppp")
	time.Sleep(30 * time.Millisecond)
	// check server 3 to see whether it got those even though it didn't have any normal clients connected
	assert.Equal(t, 6, s3.store.Count("default", "asdf"))
	assert.Equal(t, 1, s3.store.Count("default", "jjj"))
	assert.Equal(t, 1, s3.store.Count("asdfasdf", "ppp"))

	// check servers 1 and 2 to make sure they didn't double count
	assert.Equal(t, 6, s1.store.Count("default", "asdf"))
	assert.Equal(t, 6, s2.store.Count("default", "asdf"))

	// cleanup
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s3.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c1.Close(); err != nil {
		t.Fatal(err)
	}
	if err := c2.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestServer_MultipleClientsNoPanic(t *testing.T) {
	// setup
	s := NewServer(60, "")
	s.DebugEnable("9000")
	if err := s.Listen(9000, 9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// UDP clients
	c1 := client.NewClient(client.Config{RemoteUDPIPPortList: "127.0.0.1:9000"})
	c1.DebugEnable("9001")
	if err := c1.Listen(9001); err != nil {
		t.Fatal(err)
	}
	defer c1.Close()

	c2 := client.NewClient(client.Config{RemoteUDPIPPortList: "127.0.0.1:9000"})
	c2.DebugEnable("9002")
	if err := c2.Listen(9002); err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// tcp and udp server
	c3 := client.NewClient(client.Config{
		RemoteUDPIPPortList: "127.0.0.1:9000",
		RemoteTCPIPPortList: "127.0.0.1:9000",
	})
	c3.DebugEnable("9003")
	if err := c3.Listen(9003); err != nil {
		t.Fatal(err)
	}
	defer c3.Close()

	var wg sync.WaitGroup
	wg.Add(3)

	// add one first to not crash the tcp test
	c1.Put("default", "pa-penn")
	go func(t *testing.T) {
		c1.Put("default", "a3.com")
		c1.Put("default", "a3.com")
		c1.Put("default", "a3.com")
		c1.Put("default", "a3.com")
		c1.Put("default", "192.168.0.99")
		c1.Put("secondary", "pa.abs.com")
		time.Sleep(time.Millisecond * 250)

		if count, err := c1.Count("default", "a3.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 4, count)
		}

		// other client's
		if count, err := c1.Count("default", "pa.abs.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 3, count)
		}
		// repeated request
		if count, err := c1.Count("default", "pa.abs.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 3, count)
		}
		// second namespace same entryKey
		if count, err := c1.Count("secondary", "pa.abs.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 1, count)
		}

		// wrong namespace
		if count, err := c1.Count("red", "a3.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 0, count)
		}

		// counting across entire server
		if count, err := c2.CountServer(); err != nil {
			t.Error(err)
		} else {
			// expected to count secondary namespace as well
			assert.Equal(t, 12, count, "failed count all server entries")
		}

		wg.Done()
	}(t)

	go func(t *testing.T) {
		c2.Put("default", "pa.abs.com")
		c2.Put("default", "pa.abs.com")
		c2.Put("default", "pa.abs.com")
		c2.Put("default", "192.168.0.98")
		time.Sleep(time.Millisecond * 250)

		if count, err := c2.Count("default", "pa.abs.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 3, count)
		}
		if count, err := c2.Count("default", "192.168.0.99"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 1, count)
		}
		// other client's
		if count, err := c2.Count("default", "192.168.0.99"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 1, count)
		}

		// counting across entire namespace
		if count, err := c2.CountNamespace("default"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 11, count, "failed count default ns entries")
		}

		wg.Done()
	}(t)

	go func(t *testing.T) {
		assert.NoError(t, c3.Put("default", "pa"))
		patterns := []string{"pa*", "192.168.0.99", "192*", "*", "*"}
		for i := 0; i < 5; i++ {
			keys, err := c3.KeyMatch("default", patterns[i])
			assert.NoError(t, err)
			assert.NotEmpty(t, keys)
			time.Sleep(10 * time.Millisecond)
		}

		wg.Done()
	}(t)

	wg.Wait()
}

// consider convert to benchmark
func TestServer_HeavyConcurrency(t *testing.T) {
	// Conditions: many clients reading and writing at once, expire keys very quickly,
	// with large auth key.
	rounds := 300
	putsPerRound := 5
	clientCount := 5
	switchNamespaceEvery := 10

	preSharedKey := helperRandStr(100)
	s := NewServer(2, preSharedKey)
	if err := s.Listen(9000, 9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	startTime := time.Now()
	var wg sync.WaitGroup
	wg.Add(clientCount)

	for cc := 0; cc < clientCount; cc++ {
		port := 9001 + cc
		_c := client.NewClient(client.Config{
			RemoteUDPIPPortList: "127.0.0.1:9000",
			Timeout:             time.Second * 5,
			PreSharedKey:        preSharedKey,
		})
		if err := _c.Listen(port); err != nil {
			t.Fatal(err)
		}
		defer _c.Close()

		go func(c *client.Client) {
			var err error
			var ct int
			ns := helperRandStr(60)
			datav := helperRandStr(1000)
			for i := 0; i < rounds; i++ {
				if i%switchNamespaceEvery == 0 {
					// switch namespace every once in a while
					ns = helperRandStr(60)
				}

				// add some data
				datav = helperRandStr(1000)
				for j := 0; j < putsPerRound; j++ {
					if err = c.Put(ns, datav); err != nil {
						t.Error("put err", err)
					}
				}
				// we just inserted to this namespace, so the response shouldn't ever be zero in the same
				// loop
				if ct, err = c.Count(ns, datav); err != nil {
					t.Error("count err", err)
				} else if ct < 1 {
					t.Error("count missing")
				}
				if ct, err = c.CountNamespace(ns); err != nil {
					t.Error("count ns err", err)
				} else if ct < 1 {
					t.Error("ns count missing")
				}
				if ct, err = c.CountServer(); err != nil {
					t.Error("count server err", err)
				} else if ct < 1 {
					t.Error("server count missing")
				}
			}
			wg.Done()
		}(_c)
	}

	wg.Wait()
	fmt.Println("finished heavy concurrency test after", time.Since(startTime))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}

func helperRandStr(s int) string {
	b := make([]byte, s)
	rand.Read(b)
	return string(b)
}

func Test_helper(t *testing.T) {
	fmt.Println(helperRandStr(7))
	fmt.Println(helperRandStr(19))
	fmt.Println(helperRandStr(88))
}

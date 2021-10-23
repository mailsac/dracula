package server

import (
	"fmt"
	"github.com/mailsac/dracula/client"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func TestServer_Roundtrip(t *testing.T) {
	// setup
	s := NewServer(60, "")
	s.Debug = true
	if err := s.Listen(9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	c := client.NewClient("127.0.0.1", 9000, time.Second, "")
	c.Debug = true
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
		}, func() {

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

func TestServer_MultipleClients(t *testing.T) {
	// setup
	s := NewServer(60, "")
	s.Debug = true
	if err := s.Listen(9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	c1 := client.NewClient("127.0.0.1", 9000, time.Second, "")
	c1.Debug = true
	if err := c1.Listen(9001); err != nil {
		t.Fatal(err)
	}
	defer c1.Close()

	c2 := client.NewClient("127.0.0.1", 9000, time.Second, "")
	c2.Debug = true
	if err := c2.Listen(9002); err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	var wg sync.WaitGroup
	wg.Add(2)

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
			assert.Equal(t, 10, count, "failed count all server entries")
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
			assert.Equal(t, 9, count, "failed count default ns entries")
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
	if err := s.Listen(9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	startTime := time.Now()
	var wg sync.WaitGroup
	wg.Add(clientCount)

	for cc := 0; cc < clientCount; cc++ {
		port := 9001 + cc
		_c := client.NewClient("127.0.0.1", 9000, time.Second*5, preSharedKey)
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
					t.Error(err)
				} else if ct < 1 {
					t.Error("count missing")
				}
				if ct, err = c.CountNamespace(ns); err != nil {
					t.Error(err)
				} else if ct < 1 {
					t.Error("ns count missing")
				}
				if ct, err = c.CountServer(); err != nil {
					t.Error(err)
				} else if ct < 1 {
					t.Error("server count missing")
				}
			}
			wg.Done()
		}(_c)
	}

	wg.Wait()
	fmt.Println("finished heavy concurrency test after", time.Since(startTime))
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

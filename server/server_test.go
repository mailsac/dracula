package server

import (
	"github.com/mailsac/dracula/client"
	"github.com/stretchr/testify/assert"
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

		// wrong namespace
		if count, err := c1.Count("red", "a3.com"); err != nil {
			t.Error(err)
		} else {
			assert.Equal(t, 0, count)
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

		wg.Done()
	}(t)

	wg.Wait()
}

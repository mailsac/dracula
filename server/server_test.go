package server

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path"
	"sync"
	"testing"
	"time"

	"github.com/mailsac/dracula/client"
	"github.com/stretchr/testify/assert"
)

var (
	storageDirectory, _ = os.MkdirTemp(os.TempDir(), "dracula-client-*")
)

func TestServer_Roundtrip(t *testing.T) {
	// setup
	storagePath := path.Join(storageDirectory, "TestServer_Roundtrip.db")
	s := NewServer(60, "", storagePath)
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
	storagePath := path.Join(storageDirectory, "TestServer_Replication1.db")
	s1 := NewServerWithPeers(60, "asdf", "127.0.0.1:9010", peers, storagePath)
	s1.DebugEnable("9010")
	if err := s1.Listen(9010, 9010); err != nil {
		t.Fatal(err)
	}

	storagePath = path.Join(storageDirectory, "TestServer_Replication2.db")
	s2 := NewServerWithPeers(60, "asdf", "127.0.0.1:9020", peers, storagePath)
	s1.DebugEnable("9020")
	if err := s2.Listen(9020, 9020); err != nil {
		t.Fatal(err)
	}

	storagePath = path.Join(storageDirectory, "TestServer_Replication3.db")
	s3 := NewServerWithPeers(60, "asdf", "127.0.0.1:9030", peers, storagePath)
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
	assert.Equal(t, 6, s3.store.CountKey(context.TODO(), "default", "asdf"))
	assert.Equal(t, 1, s3.store.CountKey(context.TODO(), "default", "jjj"))
	assert.Equal(t, 1, s3.store.CountKey(context.TODO(), "asdfasdf", "ppp"))

	// check servers 1 and 2 to make sure they didn't double count
	assert.Equal(t, 6, s1.store.CountKey(context.TODO(), "default", "asdf"))
	assert.Equal(t, 6, s2.store.CountKey(context.TODO(), "default", "asdf"))

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
	storagePath := path.Join(storageDirectory, "TestServer_MultipleClientsNoPanic.db")
	s := NewServer(60, "", storagePath)
	s.DebugEnable("9000")
	if err := s.Listen(9000, 9000); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
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
	storagePath := path.Join(storageDirectory, "TestServer_HeavyConcurrency.db")
	s := NewServer(2, preSharedKey, storagePath)
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
	letters := []byte("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	b := make([]byte, s)
	for i := 0; i < len(b); i++ {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func Test_helper(t *testing.T) {
	fmt.Println(helperRandStr(7))
	fmt.Println(helperRandStr(19))
	fmt.Println(helperRandStr(88))
}

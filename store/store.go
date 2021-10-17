package store

import (
	"github.com/emirpasic/gods/maps/hashmap"
	"github.com/mailsac/throttle-counter/store/tree"
	"sync"
	"time"
)

// Store provides a way to store entries and count them based on namespaces.
// Old entries are garbage collected in a way that attempts to not block for too long.
type Store struct {
	sync.Mutex // mutext locks namespaces
	namespaces            *hashmap.Map
	expireAfterSecs       int64
	cleanupServiceEnabled bool
}

func NewStore(expireAfterSecs int64) *Store {
	s := &Store{
		expireAfterSecs: expireAfterSecs,
		namespaces:      hashmap.New(),
	}
	s.cleanupServiceEnabled = true

	go s.runCleanup()

	return s
}

func (s *Store) EnableCleanup() {
	s.cleanupServiceEnabled = true
}

func (s *Store) DisableCleanup() {
	s.cleanupServiceEnabled = false
}

// runCleanup must run in its own thread
func (s *Store) runCleanup() {
	if !s.cleanupServiceEnabled {
		return
	}

	defer time.AfterFunc(time.Second * 15, s.runCleanup)

	s.Lock()
	keys := s.namespaces.Keys() // they are randomly ordered
	s.Unlock()

	hashSize := len(keys)
	if hashSize < 3 {
		return
	}
	maxNamespaces := hashSize / 3 // only ever clean 1/3

	subtrees := make(map[string]*tree_test_go.Tree)

	s.Lock()
	{
		// pointers to some the subtrees are fetched from
		var ns string
		var found bool
		var subtreeI interface{}

		for i:= 0; i < maxNamespaces; i++ {
			ns = keys[i].(string)
			subtreeI, found = s.namespaces.Get(ns)
			if !found {
				continue
			}
			subtrees[ns] = subtreeI.(*tree_test_go.Tree)
		}
	}
	s.Unlock()

	var subtreeKeys []string
	for ns, subtree := range subtrees {
		subtreeKeys = subtree.Keys()
		// calling Count() on every key will enforce cleanup
		for _, ek := range subtreeKeys {
			subtree.Count(ek)
		}

		subtreeKeys = subtree.Keys() // getting the keys again will indicate if any are left now
		if len(subtreeKeys) == 0 {
			// an empty subtree can be removed from the top level namespaces
			s.Lock()
			s.namespaces.Remove(ns)
			s.Unlock()
			continue
		}
	}

}

func (s *Store) Put(ns, entryKey string) {
	var subtree *tree_test_go.Tree
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		subtree = tree_test_go.NewTree(s.expireAfterSecs)
		s.Lock()
		s.namespaces.Put(ns, subtree)
		s.Unlock()
	} else {
		subtree = subtreeI.(*tree_test_go.Tree)
	}

	subtree.Put(entryKey)
}

func (s *Store) Count(ns, entryKey string) int {
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		return 0
	}
	subtree := subtreeI.(*tree_test_go.Tree)

	return subtree.Count(entryKey)
}

package store

import (
	"github.com/emirpasic/gods/maps/hashmap"
	"github.com/mailsac/dracula/store/tree"
	"sync"
	"time"
)

var runDuration = time.Second * 15

// denominator of how many keys to garbage collect max on a run. if 3 then 1/3 or `<total keys>/3`
const maxNamespacesDenom = 3

// Store provides a way to store entries and count them based on namespaces.
// Old entries are garbage collected in a way that attempts to not block for too long.
type Store struct {
	sync.Mutex            // mutext locks namespaces
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

	defer time.AfterFunc(runDuration, s.runCleanup)

	s.Lock()
	keys := s.namespaces.Keys() // they are randomly ordered
	s.Unlock()

	hashSize := len(keys)
	if hashSize < 3 {
		return
	}
	maxNamespaces := hashSize / maxNamespacesDenom

	subtrees := make(map[string]*tree.Tree)

	s.Lock()
	{
		// pointers to some the subtrees are fetched from
		var ns string
		var found bool
		var subtreeI interface{}

		for i := 0; i < maxNamespaces; i++ {
			ns = keys[i].(string)
			subtreeI, found = s.namespaces.Get(ns)
			if !found {
				continue
			}
			subtrees[ns] = subtreeI.(*tree.Tree)
		}
	}
	s.Unlock()

	var subtreeKeys []string
	for ns, subtree := range subtrees {
		// Keys will cleanup every empty entry key
		subtreeKeys, _ = subtree.Keys()
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
	var subtree *tree.Tree
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		subtree = tree.NewTree(s.expireAfterSecs)
		s.Lock()
		s.namespaces.Put(ns, subtree)
		s.Unlock()
	} else {
		subtree = subtreeI.(*tree.Tree)
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
	subtree := subtreeI.(*tree.Tree)

	return subtree.Count(entryKey)
}

// CountEntries returns the count of all entries for the entire namespace.
// This is an expensive operation.
func (s *Store) CountEntries(ns string) int {
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		return 0
	}
	subtree := subtreeI.(*tree.Tree)

	_, count := subtree.Keys()
	return count
}

// CountServerEntries returns the count of all entries for the entire server.
// This is an extremely expensive operation.
func (s *Store) CountServerEntries() int {
	s.Lock()
	spaces := s.namespaces.Keys() // they are randomly ordered
	s.Unlock()
	var entryCount int
	var c int
	for _, ns := range spaces {
		c = s.CountEntries(ns.(string))
		entryCount += c
	}
	return entryCount
}

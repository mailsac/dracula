package tree

import (
	"github.com/emirpasic/gods/trees/avltree"
	"sync"
	"time"
)

type ExpireAtSecs int64

// Tree is a a thread-safe data structure for tracking expirable items. It automatically expires old entries and keys.
// It does not garbage collect. Items are only expired when interacting with the data structure.
type Tree struct {
	sync.Mutex
	defaultExpireAfterSecs int64
	tree                   *avltree.Tree
}

func NewTree(expireAfterSecs int64) *Tree {
	return &Tree{
		defaultExpireAfterSecs: expireAfterSecs,
		tree:                   avltree.NewWithStringComparator(),
	}
}

func (n *Tree) Keys() []string {
	n.Lock()
	defer n.Unlock()
	var out []string

	keysI := n.tree.Keys()
	if keysI == nil {
		return out
	}
	for _, iface := range keysI {
		out = append(out, iface.(string))
	}

	return out
}

// Count will return the number of entries at `entryKey`. It has the side effect of cleaning up
// stale entries.
func (n *Tree) Count(entryKey string) int {
	n.Lock()
	defer n.Unlock()

	datesSecs := n.getAndCleanupUnsafe(entryKey)
	if datesSecs == nil {
		return 0
	}

	if len(*datesSecs) == 0 {
		n.tree.Remove(entryKey)
		return 0
	}

	datesSecs = removeExpired(datesSecs)
	count := len(*datesSecs)
	n.tree.Put(entryKey, *datesSecs)
	return count
}

func (n *Tree) Put(entryKey string) {
	n.Lock()
	defer n.Unlock()

	datesSecs := n.getAndCleanupUnsafe(entryKey)
	if datesSecs == nil {
		datesSecs = &[]int64{}
	}
	datesSecs = removeExpired(datesSecs)
	secs := time.Now().Unix()
	nextDatesSecs := append(*datesSecs, secs+n.defaultExpireAfterSecs)
	n.tree.Put(entryKey, nextDatesSecs)
}

// getAndCleanupUnsafe does not lock the mutex, so it can be used inside a lock
func (n *Tree) getAndCleanupUnsafe(entryKey string) *[]int64 {
	val, found := n.tree.Get(entryKey)
	if !found {
		return nil
	}
	dates := val.([]int64)
	if len(dates) == 0 {
		// cleanup empty entry
		n.tree.Remove(entryKey)
		return nil
	}
	return &dates // not extra copy
}

func removeExpired(datesSecs *[]int64) *[]int64 {
	secs := time.Now().Unix()
	var out []int64
	for _, ds := range *datesSecs {
		if ds > secs {
			// not expired
			out = append(out, ds)
		}
	}
	return &out
}

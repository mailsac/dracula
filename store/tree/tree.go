package tree

import (
	"github.com/emirpasic/gods/trees/redblacktree"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ExpireAtSecs int64

type entry struct {
	expiresAt []int64
	liveCount int
}

// Tree is a a thread-safe data structure for tracking expirable items. It automatically expires old entries and keys.
// It does not garbage collect. Items are only expired when interacting with the data structure.
type Tree struct {
	sync.Mutex
	defaultExpireAfterSecs int64
	tree                   *redblacktree.Tree
}

func NewTree(expireAfterSecs int64) *Tree {
	return &Tree{
		defaultExpireAfterSecs: expireAfterSecs,
		tree:                   redblacktree.NewWithStringComparator(),
	}
}

// Keys returns a list of all valid keys in the tree, and a sum of every key's valid entries.
// It is expensive because it will result in the entire tree being counted and expired where necessary.
func (n *Tree) Keys() ([]string, int) {
	var outKeys []string
	var outCount int

	n.Lock()
	keysI := n.tree.Keys()
	n.Unlock()

	if keysI == nil {
		return outKeys, outCount
	}

	var key string
	var keyCount int
	for _, iface := range keysI {
		key = iface.(string)
		keyCount = n.Count(key)
		outCount += keyCount
		if keyCount == 0 {
			continue
		}
		outKeys = append(outKeys, key)
	}

	return outKeys, outCount
}

// Count will return the number of entries at `entryKey`. It has the side effect of cleaning up
// stale entries and entry keys.
func (n *Tree) Count(entryKey string) int {
	n.Lock()
	defer n.Unlock()

	item, found := n.getAndCleanupUnsafe(entryKey)
	if !found {
		return 0
	}

	if item.liveCount == 0 {
		n.tree.Remove(entryKey)
		return 0
	}

	item = removeExpired(item)
	if item.liveCount == 0 {
		n.tree.Remove(entryKey)
		return 0
	}

	n.tree.Put(entryKey, item)
	return item.liveCount
}

// KeyMatch crawls the subtree to return keys starting with the `keyPattern` string.
func (n *Tree) KeyMatch(keyPattern string) []string {
	var out []string
	var wg sync.WaitGroup
	re, err := regexp.Compile(strings.ReplaceAll(keyPattern, "*", "(^|$|.+)"))
	if err != nil {
		return []string{err.Error()}
	}

	wg.Add(1)
	go func() {
		iterator := n.tree.Iterator()
		var k string
		var kOk bool
		existed := iterator.Next()
		for existed {
			k, kOk = iterator.Key().(string)
			if !kOk {
				break
			}
			existed = iterator.Next()
			if re.MatchString(k) {
				if n.Count(k) > 0 {
					out = append(out, k)
				}
			}
		}
		wg.Done()
	}()
	wg.Wait()

	return out
}

func (n *Tree) Put(entryKey string) int {
	n.Lock()
	defer n.Unlock()

	item, found := n.getAndCleanupUnsafe(entryKey)
	if !found {
		item = entry{}
	}
	item = removeExpired(item)
	secs := time.Now().Unix()
	item.expiresAt = append(item.expiresAt, secs+n.defaultExpireAfterSecs)
	item.liveCount = len(item.expiresAt)
	n.tree.Put(entryKey, item)
	return item.liveCount
}

// getAndCleanupUnsafe does not lock the mutex, so it can be used inside a lock
func (n *Tree) getAndCleanupUnsafe(entryKey string) (entry, bool) {
	val, found := n.tree.Get(entryKey)
	if !found {
		return entry{}, false
	}
	item := val.(entry)
	if item.liveCount == 0 {
		// cleanup empty entry
		n.tree.Remove(entryKey)
		return entry{}, false
	}
	return item, true
}

func removeExpired(item entry) entry {
	if len(item.expiresAt) == 0 {
		item.liveCount = 0
		return item
	}

	currentTime := time.Now().Unix()
	firstValid := -1

	// Because entries are appended using time.Now().Unix(), they are chronologically sorted.
	for i, removeAt := range item.expiresAt {
		if removeAt > currentTime {
			firstValid = i
			break
		}
	}

	if firstValid == -1 {
		item.expiresAt = nil
		item.liveCount = 0
		return item
	}

	if firstValid > 0 {
		item.expiresAt = item.expiresAt[firstValid:]
	}

	// Prevent memory leaks: Slicing a large array keeps the entire backing array in memory.
	if cap(item.expiresAt) > 1024 && len(item.expiresAt) < cap(item.expiresAt)/4 {
		newSlice := make([]int64, len(item.expiresAt))
		copy(newSlice, item.expiresAt)
		item.expiresAt = newSlice
	}

	item.liveCount = len(item.expiresAt)
	return item
}

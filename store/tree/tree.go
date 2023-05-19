package tree

import (
	"github.com/emirpasic/gods/trees/redblacktree"
	"regexp"
	"strings"
	"sync"
	"time"
)

type ExpireAtSecs int64

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
	if len(*datesSecs) == 0 {
		return datesSecs
	}
	currentTime := time.Now().Unix()
	var out []int64
	// TODO: these are already sorted, so we can discard earlier entries
	for _, removeAt := range *datesSecs {
		if removeAt > currentTime {
			// KEEP - not expired
			out = append(out, removeAt)
		}
	}
	return &out
}

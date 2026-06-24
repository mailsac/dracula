package tree

import (
	"github.com/stretchr/testify/assert"
	"math/rand"
	"testing"
	"time"
)

func TestTree_removeExpired(t *testing.T) {
	keep1 := time.Now().Unix() + 5
	keep2 := time.Now().Unix() + 200

	// Because entries are appended using time.Now().Unix(), they are chronologically sorted.
	// The slice-based implementation requires this sorting to find the first unexpired item.
	entries := []int64{
		time.Now().Unix() - 60,
		time.Now().Unix() - 2,
		keep1,
		keep2,
	}

	result := removeExpired(entry{expiresAt: entries, liveCount: len(entries)})
	assert.Equal(t, 2, result.liveCount)
	assert.Equal(t, keep1, result.expiresAt[0])
	assert.Equal(t, keep2, result.expiresAt[1])

	t.Run("memory reallocation when capacity is huge but length is small", func(t *testing.T) {
		largeCapEntries := make([]int64, 0, 2000)
		for i := 0; i < 1900; i++ {
			largeCapEntries = append(largeCapEntries, time.Now().Unix()-100)
		}
		for i := 0; i < 5; i++ {
			largeCapEntries = append(largeCapEntries, time.Now().Unix()+100)
		}

		result := removeExpired(entry{expiresAt: largeCapEntries, liveCount: len(largeCapEntries)})
		assert.Equal(t, 5, result.liveCount)
		assert.Equal(t, 5, len(result.expiresAt))
		assert.LessOrEqual(t, cap(result.expiresAt), 1024, "Should have reallocated to a smaller array")
	})

	t.Run("returns empty when all expired", func(t *testing.T) {
		allExpired := []int64{
			time.Now().Unix() - 60,
			time.Now().Unix() - 2,
		}
		result := removeExpired(entry{expiresAt: allExpired, liveCount: len(allExpired)})
		assert.Equal(t, 0, result.liveCount)
		assert.Equal(t, 0, len(result.expiresAt))
	})
}

func TestTree_Count(t *testing.T) {
	t.Run("count returns number of non-expired and removes expired values", func(t *testing.T) {
		tr := NewTree(2)
		keys, entryCount := tr.Keys()
		assert.Equal(t, 0, len(keys))
		assert.Equal(t, 0, entryCount)
		tr.Put("willy")
		tr.Put("willy")
		tr.Put("Uncle Brick")
		tr.Put("pander")
		tr.Put("pander")
		tr.Put("pander")

		keys, entryCount = tr.Keys()
		assert.Equal(t, 3, len(keys))
		assert.Equal(t, 6, entryCount)

		// called twice in a row to make sure Count() doesn't trigger deletion or something weird
		assert.Equal(t, 2, tr.Count("willy"))
		assert.Equal(t, 2, tr.Count("willy"))
		assert.Equal(t, 1, tr.Count("Uncle Brick"))
		assert.Equal(t, 1, tr.Count("Uncle Brick"))
		assert.Equal(t, 3, tr.Count("pander"))
		assert.Equal(t, 3, tr.Count("pander"))

		// wrong case check
		assert.Equal(t, 0, tr.Count("uncle brick"))
		// unknowns
		assert.Equal(t, 0, tr.Count("p"))
		assert.Equal(t, 0, tr.Count("733"))

		time.Sleep(2 * time.Second)

		// count should force the tree to purge expired values
		assert.Equal(t, 0, tr.Count("willy"))
		assert.Equal(t, 0, tr.Count("Uncle Brick"))
		assert.Equal(t, 0, tr.Count("pander"))

		keys, entryCount = tr.Keys()
		assert.Equal(t, 0, len(keys)) // keys should be expired, but calling this will also expire them
		assert.Equal(t, 0, entryCount)

		// now check that Keys() expires old keys
		tr.Put("billy")
		tr.Put("billy")
		assert.Equal(t, 2, tr.Count("billy"))
		keys, entryCount = tr.Keys()
		assert.Equal(t, 1, len(keys))
		assert.Equal(t, 2, entryCount)
		time.Sleep(2 * time.Second)
		keys, entryCount = tr.Keys()
		assert.Equal(t, 0, len(keys), "Keys() should expire keys")
		assert.Equal(t, 0, entryCount, "Keys() should expire entries")
	})
	t.Run("put returns live count without needing a second scan", func(t *testing.T) {
		tr := NewTree(2)
		assert.Equal(t, 1, tr.Put("willy"))
		assert.Equal(t, 2, tr.Put("willy"))
		assert.Equal(t, 1, tr.Put("other"))
		assert.Equal(t, 2, tr.Count("willy"))
	})
}

func TestTree_KeyMatch(t *testing.T) {
	t.Run("returns only matches for a keyPattern", func(t *testing.T) {
		tr := NewTree(60)
		// we only put one key, but multiple counts
		tr.Put("asdf")
		tr.Put("asdf")
		tr.Put("asdf")

		tr.Put("a:sdf")
		tr.Put("a")
		tr.Put("a:")

		tr.Put("mn:blah")
		tr.Put("na:blahblah")
		tr.Put("bla")

		tr.Put("a:elvis:5")
		tr.Put("b:elvis:8:elvis")
		tr.Put("e:elvis")
		tr.Put("c:elvis:1")

		// curveballs
		tr.Put("") // doesn't work
		tr.Put(".+")
		tr.Put("nil")

		assert.Equal(t, 3, len(tr.KeyMatch("^a:*")))

		m := tr.KeyMatch("*bla*")
		assert.Equalf(t, 3, len(m), "%+v", m)

		assert.ElementsMatch(t, []string{"a:elvis:5", "b:elvis:8:elvis", "e:elvis", "c:elvis:1"}, tr.KeyMatch("*elvis*"))

		// everything
		all := tr.KeyMatch("*")
		assert.Equalf(t, 14, len(all), "%+v", all)
	})
	t.Run("it does not crash when empty", func(t *testing.T) {
		tr := NewTree(5)

		assert.Equal(t, 0, len(tr.KeyMatch("asdf")))
	})
	t.Run("it does not return the key when all its values are expired", func(t *testing.T) {
		tr := NewTree(1)
		tr.Put("a")
		tr.tree.Put("a", entry{})

		tr.Put("cdbe")
		tr.Put("cd:aa")

		result := tr.KeyMatch("cd")
		assert.ElementsMatch(t, []string{"cdbe", "cd:aa"}, result)

	})
	t.Run("works with a huge tree", func(t *testing.T) {
		charset := "abcdefghijklmnopqrstuvwxyz"

		tr := NewTree(10)
		for i := 0; i < 100_000; i++ {
			tr.Put(
				string(charset[rand.Intn(len(charset))]) +
					string(charset[rand.Intn(len(charset))]) +
					string(charset[rand.Intn(len(charset))]))
		}
		assert.NotEmpty(t, tr.KeyMatch("a"))
	})

}

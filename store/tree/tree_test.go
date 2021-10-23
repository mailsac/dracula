package tree

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestTree_removeExpired(t *testing.T) {
	keep1 := time.Now().Unix() + 5
	keep2 := time.Now().Unix() + 200
	entries := []int64{
		time.Now().Unix() - 2, // expired
		time.Now().Unix() - 60, // expired
		keep1, // KEEP
		keep2, // KEEP
		time.Now().Unix() - 1, // expired
	}

	result := removeExpired(&entries)
	assert.Equal(t, 2, len(*result))
	assert.Equal(t, (*result)[0], keep1)
	assert.Equal(t, (*result)[1], keep2)
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
}

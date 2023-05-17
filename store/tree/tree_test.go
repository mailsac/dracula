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
	entries := []int64{
		time.Now().Unix() - 2,  // expired
		time.Now().Unix() - 60, // expired
		keep1,                  // KEEP
		keep2,                  // KEEP
		time.Now().Unix() - 1,  // expired
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
		tr.tree.Put("a", []string{})

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

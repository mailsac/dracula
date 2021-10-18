package tree

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestTree_Count(t *testing.T) {
	t.Run("count returns number of non-expired and removes expired values", func(t *testing.T) {
		tr := NewTree(2)
		assert.Equal(t, 0, len(tr.Keys()))
		tr.Put("willy")
		tr.Put("willy")
		tr.Put("Uncle Brick")
		tr.Put("pander")
		tr.Put("pander")
		tr.Put("pander")

		assert.Equal(t, 3, len(tr.Keys()))

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

		assert.Equal(t, 0, len(tr.Keys())) // keys should be expired, but calling this will also expire them

		// now check that Keys() expires old keys
		tr.Put("billy")
		tr.Put("billy")
		assert.Equal(t, 2, tr.Count("billy"))
		assert.Equal(t, 1, len(tr.Keys()))
		time.Sleep(2 * time.Second)
		assert.Equal(t, 0, len(tr.Keys()), "Keys() should expire keys")
	})
}

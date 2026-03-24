package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_NamespacesUsesSnapshot(t *testing.T) {
	s := NewStore(60)
	s.DisableCleanup()

	assert.Equal(t, []string{}, s.Namespaces())

	assert.Equal(t, 1, s.Put("namespace0", "key0"))
	assert.Equal(t, 2, s.Put("namespace0", "key0"))
	assert.Equal(t, 1, s.Put("namespace1", "key1"))

	assert.ElementsMatch(t, []string{"namespace0", "namespace1"}, s.Namespaces())
}

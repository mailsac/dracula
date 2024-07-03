package store

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	badPath := os.TempDir()
	st, err := New(DefaultStoragePath, time.Second, nil)
	assert.NotNil(t, st)
	assert.NoError(t, err)
	defer st.Close()
	_, err = New(badPath, time.Second, nil)
	assert.Error(t, err)
}

package waitingmessage

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrMessageIDExists = errors.New("message ID already exists")
	ErrMessageExpired  = errors.New("message expired")
	ErrNoMessage       = errors.New("message not found or was garbage collected")

	cleanupEveryDefault = time.Second * 10
)

type Callback func([]byte, error)

type waitingMessage struct {
	Callback    Callback
	CreatedSecs int64
}

type ResponseCache struct {
	sync.Mutex
	cache        map[uint32]waitingMessage
	disposed     bool
	cleanupEvery time.Duration
	timeoutSecs  int64 // cached for fewer conversions
	// TimedOutMessages channel can be listened over for when messages did not receive a response by the timeout deadline
	// or a little later (in practice)
	TimedOutMessages chan Callback
}

func NewCache(timeout time.Duration) *ResponseCache {
	cleanupEvery := cleanupEveryDefault
	if cleanupEvery > timeout {
		cleanupEvery = timeout
	}

	rc := &ResponseCache{
		cache:            make(map[uint32]waitingMessage),
		timeoutSecs:      int64(timeout.Seconds()),
		cleanupEvery:     cleanupEvery,
		TimedOutMessages: make(chan Callback),
	}

	rc.checkCleanup()
	return rc
}

// Len returns the count of the number of entries
func (rc *ResponseCache) Len() int {
	rc.Lock()
	defer rc.Unlock()
	return len(rc.cache)
}

func (rc *ResponseCache) Add(messageID uint32, cb Callback) error {
	rc.Lock()
	defer rc.Unlock()

	if _, exists := rc.cache[messageID]; exists {
		return ErrMessageIDExists
	}

	rc.cache[messageID] = waitingMessage{
		Callback:    cb,
		CreatedSecs: time.Now().Unix(),
	}
	return nil
}

// Pull removes the expected message command if exists or returns an error
func (rc *ResponseCache) Pull(messageID uint32) (Callback, error) {
	rc.Lock()
	message, exists := rc.cache[messageID]
	defer rc.Unlock()

	if !exists {
		return nil, ErrNoMessage
	}
	// can only pull a message once
	delete(rc.cache, messageID)

	isExpired := message.CreatedSecs < (time.Now().Unix() - rc.timeoutSecs)
	if isExpired {
		return nil, ErrMessageExpired
	}

	// ok
	return message.Callback, nil
}

// Dispose stops the cleanup operation and allows the whole cache to be to be garbage collected by go's runtime.
// Also stops the channel
func (rc *ResponseCache) Dispose() {
	if rc != nil {
		rc.disposed = true
		close(rc.TimedOutMessages)
	}
}

func (rc *ResponseCache) checkCleanup() {
	if rc == nil || rc.disposed {
		// item was disposed
		return
	}

	rc.Lock()
	defer rc.Unlock()

	entryCount := len(rc.cache)
	oneQuarter := entryCount / 3 // only crawl 1/3 of the entries at a time
	cleanupWhenOlderThanSecs := time.Now().Unix() - rc.timeoutSecs
	i := 0
	var removeTheseKeys []uint32
	var shouldCleanup bool
	for messageID, entry := range rc.cache {
		i++
		shouldCleanup = entry.CreatedSecs < cleanupWhenOlderThanSecs
		if shouldCleanup {
			removeTheseKeys = append(removeTheseKeys, messageID)
		}

		if i > oneQuarter {
			break
		}
	}

	var messageID uint32
	var cb Callback
	for i = 0; i < len(removeTheseKeys); i++ {
		messageID = removeTheseKeys[i]
		cb = rc.cache[messageID].Callback
		delete(rc.cache, messageID)
		if !rc.disposed {
			rc.TimedOutMessages <- cb
		}
	}

	time.AfterFunc(rc.cleanupEvery, rc.checkCleanup)
}

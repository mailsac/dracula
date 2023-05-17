package store

import (
	"github.com/emirpasic/gods/maps/hashmap"
	"github.com/mailsac/dracula/store/tree"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
	"sync"
	"time"
)

var runDuration = time.Second * 15

// denominator of how many namespaces to garbage collect max on a run. if 3 then 1/3 or `<total keys>/3`
const maxNamespacesDenom = 2

type Metrics struct {
	registry                          *prometheus.Registry
	maxNamespacesDenom                prometheus.Gauge
	namespacesTotalCount              prometheus.Gauge
	namespacesGarbageCollected        prometheus.Gauge
	keysRemainingInGCNamespaces       prometheus.Gauge
	countTotalRemainingInGCNamespaces prometheus.Gauge
	gcPauseTime                       prometheus.Gauge
}

func (m *Metrics) ListenAndServe(promHostPort string) error {
	http.Handle(
		"/metrics", promhttp.HandlerFor(
			m.registry,
			promhttp.HandlerOpts{
				EnableOpenMetrics: true,
			}),
	)
	// To test: curl -H 'Accept: application/openmetrics-text' localhost:8080/metrics
	err := http.ListenAndServe(promHostPort, nil)
	return err
}

// Store provides a way to store entries and count them based on namespaces.
// Old entries are garbage collected in a way that attempts to not block for too long.
type Store struct {
	sync.Mutex            // mutext locks namespaces
	namespaces            *hashmap.Map
	expireAfterSecs       int64
	cleanupServiceEnabled bool
	LastMetrics           *Metrics
}

func NewStore(expireAfterSecs int64) *Store {
	registry := prometheus.NewRegistry()
	maxNamespacesDenomGauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_max_namespaces_denom",
		Help: "Denominator/portion of namespaces to be garbage collected each cleanup run",
	})
	namespacesTotalCount := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_namespaces_count",
		Help: "Number of top level key namespaces",
	})
	namespacesGarbageCollected := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_namespaces_gc_count",
		Help: "Number of namespaces which had keys garbage collected during last cleanup run",
	})
	keysRemainingInGCNamespaces := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_keys_in_gc_namespaces",
		Help: "Count of unexpired keys in last garbage collected namespaces",
	})
	countTotalRemainingInGCNamespaces := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_key_sum_in_gc_namespaces",
		Help: "Count of key values in last garbage collected namespace valid keys",
	})
	gcPauseTime := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "dracula_gc_pause_millis",
		Help: "How long last garbage collection took in milliseconds",
	})
	registry.MustRegister(maxNamespacesDenomGauge, namespacesTotalCount, namespacesGarbageCollected, keysRemainingInGCNamespaces, countTotalRemainingInGCNamespaces, gcPauseTime)

	s := &Store{
		expireAfterSecs: expireAfterSecs,
		namespaces:      hashmap.New(),
		LastMetrics: &Metrics{
			registry:                          registry,
			maxNamespacesDenom:                maxNamespacesDenomGauge,
			namespacesTotalCount:              namespacesTotalCount,
			namespacesGarbageCollected:        namespacesGarbageCollected,
			keysRemainingInGCNamespaces:       keysRemainingInGCNamespaces,
			countTotalRemainingInGCNamespaces: countTotalRemainingInGCNamespaces,
			gcPauseTime:                       gcPauseTime,
		},
	}
	s.cleanupServiceEnabled = true
	s.LastMetrics.maxNamespacesDenom.Set(maxNamespacesDenom)

	go s.runCleanup()

	return s
}

func (s *Store) EnableCleanup() {
	s.cleanupServiceEnabled = true
}

func (s *Store) DisableCleanup() {
	s.cleanupServiceEnabled = false
}

// runCleanup must run in its own thread
func (s *Store) runCleanup() {
	if !s.cleanupServiceEnabled {
		return
	}

	start := time.Now()

	defer time.AfterFunc(runDuration, s.runCleanup)

	s.Lock()
	keys := s.namespaces.Keys() // they are randomly ordered
	s.Unlock()

	hashSize := len(keys)
	maxNamespaces := hashSize / maxNamespacesDenom

	s.LastMetrics.namespacesTotalCount.Set(float64(hashSize))
	s.LastMetrics.namespacesGarbageCollected.Set(float64(maxNamespaces))

	subtrees := make(map[string]*tree.Tree)

	s.Lock()
	{
		// pointers to some the subtrees are fetched from
		var ns string
		var found bool
		var subtreeI interface{}

		for i := 0; i < maxNamespaces; i++ {
			ns = keys[i].(string)
			subtreeI, found = s.namespaces.Get(ns)
			if !found {
				continue
			}
			subtrees[ns] = subtreeI.(*tree.Tree)
		}
	}
	s.Unlock()

	var subtreeKeys []string
	var knownKeysCount int
	var subtreeKeyTrackCount int
	var tally int
	for ns, subtree := range subtrees {
		// Keys will cleanup every empty entry key
		subtreeKeys, subtreeKeyTrackCount = subtree.Keys()
		if len(subtreeKeys) == 0 {
			// an empty subtree can be removed from the top level namespaces
			s.Lock()
			s.namespaces.Remove(ns)
			s.Unlock()
			continue
		}
		knownKeysCount += len(subtreeKeys)
		tally += subtreeKeyTrackCount
	}

	s.LastMetrics.keysRemainingInGCNamespaces.Set(float64(knownKeysCount))
	s.LastMetrics.countTotalRemainingInGCNamespaces.Set(float64(tally))
	s.LastMetrics.gcPauseTime.Set(float64(time.Since(start).Milliseconds()))
}

func (s *Store) Put(ns, entryKey string) {
	var subtree *tree.Tree
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		subtree = tree.NewTree(s.expireAfterSecs)
		s.Lock()
		s.namespaces.Put(ns, subtree)
		s.Unlock()
	} else {
		subtree = subtreeI.(*tree.Tree)
	}

	subtree.Put(entryKey)
}

func (s *Store) Count(ns, entryKey string) int {
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		return 0
	}
	subtree := subtreeI.(*tree.Tree)

	return subtree.Count(entryKey)
}

// CountEntries returns the count of all entries for the entire namespace.
// This is an expensive operation.
func (s *Store) CountEntries(ns string) int {
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()
	if !found {
		return 0
	}
	subtree := subtreeI.(*tree.Tree)

	_, count := subtree.Keys()
	return count
}

// KeyMatch crawls the subtree to return keys containing keyPattern string.
func (s *Store) KeyMatch(ns string, keyPattern string) []string {
	s.Lock()
	subtreeI, found := s.namespaces.Get(ns)
	s.Unlock()

	if !found {
		return []string{}
	}
	subtree := subtreeI.(*tree.Tree)

	return subtree.KeyMatch(keyPattern)
}

// CountServerEntries returns the count of all entries for the entire server.
// This is an extremely expensive operation.
func (s *Store) CountServerEntries() int {
	s.Lock()
	spaces := s.namespaces.Keys() // they are randomly ordered
	s.Unlock()
	var entryCount int
	var c int
	for _, ns := range spaces {
		c = s.CountEntries(ns.(string))
		entryCount += c
	}
	return entryCount
}

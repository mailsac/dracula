package store

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/maxtek6/keybase-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	DefaultStoragePath string        = ""
	cleanupInterval    time.Duration = time.Second * 15
)

var runDuration = time.Second * 15

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

type Store struct {
	kb              *keybase.Keybase
	log             *log.Logger
	cleanupTicker   *time.Ticker
	shutdownChannel chan struct{}
	exitChannel     chan struct{}
}

func New(storagePath string, keyDuration time.Duration, log *log.Logger) (*Store, error) {
	kbOptions := []keybase.Option{keybase.WithTTL(keyDuration)}
	if storagePath != DefaultStoragePath {
		kbOptions = append(kbOptions, keybase.WithStorage(storagePath))
	}
	kb, err := keybase.Open(context.TODO(), kbOptions...)
	if err != nil {
		return nil, err
	}
	store := &Store{
		kb:            kb,
		log:           log,
		cleanupTicker: time.NewTicker(keyDuration * 10),
	}
	go store.backgroundService()
	return store, nil
}

func (s *Store) backgroundService() {
	ok := true
	for ok {
		select {
		case <-s.cleanupTicker.C:

		case <-s.shutdownChannel:
			s.cleanupTicker.Stop()
			ok = false
		}
	}
	s.exitChannel <- struct{}{}
}

func (s *Store) Close() {
	s.shutdownChannel <- struct{}{}
	<-s.exitChannel
	s.kb.Close()
}

func (s *Store) Put(ctx context.Context, namespace, key string) {
	err := s.kb.Put(ctx, namespace, key)
	if err != nil {
		s.log.Printf("put error: %v", err)
	}
}

func (s *Store) CountKey(ctx context.Context, namespace, key string) int {
	count, err := s.kb.CountKey(ctx, namespace, key, true)
	if err != nil {
		s.log.Printf("count key error: %v", err)
		return 0
	}
	return count
}

func (s *Store) MatchKey(ctx context.Context, namespace, pattern string) []string {
	matches, err := s.kb.MatchKey(ctx, namespace, pattern, true, false)
	if err != nil {
		s.log.Printf("count key error: %v", err)
		return nil
	}
	return matches
}

func (s *Store) CountKeys(ctx context.Context, namespace string) int {
	count, err := s.kb.CountKeys(ctx, namespace, true, false)
	if err != nil {
		s.log.Printf("count keys error: %v", err)
		return 0
	}
	return count
}

func (s *Store) CountEntries(ctx context.Context) int {
	count, err := s.kb.CountEntries(ctx, true, false)
	if err != nil {
		s.log.Printf("count entries error: %v", err)
		return 0
	}
	return count
}

func (s *Store) GetNamespaces(ctx context.Context) []string {
	namespaces, err := s.kb.GetNamespaces(context.TODO(), true)
	if err != nil {
		s.log.Printf("get namespaces error: %v", err)
		return nil
	}
	return namespaces
}

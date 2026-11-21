package account

import (
	"clearcare/insurance"
	"context"
	"database/sql"
	"os"
	"sync"
	"time"
)

type RowScanner interface{ Scan(dest ...any) error }
type SQLTx interface {
	Exec(query string, args ...any) (sql.Result, error)
	QueryRow(query string, args ...any) RowScanner
	Commit() error
	Rollback() error
}

// SQLStore is the persistence boundary for account and product state. The local
// adapter owns SQLite schema and connection details. A future Postgres adapter
// will own Postgres schema and placeholder translation behind this boundary.
type SQLStore interface {
	Initialize() error
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) RowScanner
	Begin() (SQLTx, error)
	Close() error
}

func New(store SQLStore) (*Service, error) {
	if err := store.Initialize(); err != nil {
		store.Close()
		return nil, err
	}
	ai := insurance.NewOpenAI()
	s := &Service{
		db:         store,
		attempts:   map[string][]time.Time{},
		insurance:  &insurance.Engine{AI: ai, Catalog: insurance.LoadCatalog(os.Getenv("CLEARCARE_PLAN_CATALOG")), Fetcher: insurance.NewFetcher()},
		jobCancels: map[string]context.CancelFunc{},
		jobSem:     make(chan struct{}, 2),
		supabase:   newSupabaseVerifier(),
		healthAI:   ai,
	}
	if err := s.initFeatures(); err != nil {
		store.Close()
		return nil, err
	}
	if err := s.pruneExpired(); err != nil {
		store.Close()
		return nil, err
	}
	documents, err := newPrivateDocuments()
	if err != nil {
		store.Close()
		return nil, err
	}
	s.documents = documents
	return s, nil
}

type Service struct {
	db         SQLStore
	mu         sync.Mutex
	attempts   map[string][]time.Time
	insurance  *insurance.Engine
	jobsMu     sync.Mutex
	jobCancels map[string]context.CancelFunc
	jobSem     chan struct{}
	jobWG      sync.WaitGroup
	supabase   *supabaseVerifier
	healthAI   healthcareAI
	documents  *privateDocuments
}

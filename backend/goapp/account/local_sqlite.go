package account

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

type localSQLiteStore struct{ db *sql.DB }
type localSQLiteTx struct{ tx *sql.Tx }

func (s *localSQLiteStore) Initialize() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS users (id TEXT PRIMARY KEY, email TEXT UNIQUE NOT NULL, name TEXT NOT NULL, password TEXT NOT NULL, city TEXT DEFAULT '', plan TEXT DEFAULT ''); CREATE TABLE IF NOT EXISTS sessions (token TEXT PRIMARY KEY, user_id TEXT NOT NULL, csrf TEXT NOT NULL, expires REAL NOT NULL); CREATE TABLE IF NOT EXISTS reviews (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, title TEXT NOT NULL, created TEXT NOT NULL, payload TEXT NOT NULL); CREATE TABLE IF NOT EXISTS insurance_cards (user_id TEXT PRIMARY KEY, content_type TEXT NOT NULL, data BLOB NOT NULL, updated TEXT NOT NULL); CREATE TABLE IF NOT EXISTS insurance_workflows (user_id TEXT PRIMARY KEY, revision TEXT NOT NULL, result TEXT NOT NULL, document BLOB); CREATE TABLE IF NOT EXISTS ai_review_analyses (user_id TEXT NOT NULL, request_hash TEXT NOT NULL, payload TEXT NOT NULL, created INTEGER NOT NULL, PRIMARY KEY(user_id,request_hash));`)
	return err
}
func (s *localSQLiteStore) Exec(query string, args ...any) (sql.Result, error) {
	return s.db.Exec(query, args...)
}
func (s *localSQLiteStore) Query(query string, args ...any) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}
func (s *localSQLiteStore) QueryRow(query string, args ...any) RowScanner {
	return s.db.QueryRow(query, args...)
}
func (s *localSQLiteStore) Begin() (SQLTx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &localSQLiteTx{tx: tx}, nil
}
func (s *localSQLiteStore) Close() error { return s.db.Close() }

func (t *localSQLiteTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.tx.Exec(query, args...)
}
func (t *localSQLiteTx) QueryRow(query string, args ...any) RowScanner {
	return t.tx.QueryRow(query, args...)
}
func (t *localSQLiteTx) Commit() error   { return t.tx.Commit() }
func (t *localSQLiteTx) Rollback() error { return t.tx.Rollback() }

// OpenLocalSQLite is the development adapter. Hosted deployments should inject
// an external SQLStore instead of relying on the service filesystem.
func OpenLocalSQLite(dsn string) (*Service, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return New(&localSQLiteStore{db: db})
}

// Open is retained for local callers and existing tests.
func Open(path string) (*Service, error) { return OpenLocalSQLite(path) }

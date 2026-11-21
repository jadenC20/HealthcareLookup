package account

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

type postgresStore struct{ db *sql.DB }
type postgresTx struct{ tx *sql.Tx }

// Application SQL uses question-mark parameters. All statements are owned by
// the application; quoted literals must not be rewritten as parameters.
func postgresSQL(query string) string {
	query = strings.ReplaceAll(query, " REAL ", " DOUBLE PRECISION ")
	var out strings.Builder
	quoted := false
	parameter := 0
	for i := 0; i < len(query); i++ {
		c := query[i]
		if c == '\'' {
			out.WriteByte(c)
			if quoted && i+1 < len(query) && query[i+1] == '\'' {
				out.WriteByte(query[i+1])
				i++
				continue
			}
			quoted = !quoted
		} else if c == '?' && !quoted {
			parameter++
			fmt.Fprintf(&out, "$%d", parameter)
		} else {
			out.WriteByte(c)
		}
	}
	return out.String()
}

func OpenPostgres(dsn string) (*Service, error) {
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid Postgres connection configuration")
	}
	// Supabase transaction pooling cannot retain named prepared statements.
	config.DefaultQueryExecMode = pgx.QueryExecModeExec
	config.RuntimeParams["search_path"] = "clearcare"
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to Postgres: %w", err)
	}
	return New(&postgresStore{db: db})
}

func (s *postgresStore) Initialize() error {
	// Private business tables are not exposed by Supabase's public REST schema.
	// Provision the schema separately; the runtime role deliberately has no
	// database-wide CREATE permission.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS clearcare.users(id TEXT PRIMARY KEY,email TEXT UNIQUE NOT NULL,name TEXT NOT NULL,password TEXT NOT NULL,city TEXT DEFAULT '',plan TEXT DEFAULT '');
 CREATE TABLE IF NOT EXISTS clearcare.sessions(token TEXT PRIMARY KEY,user_id TEXT NOT NULL,csrf TEXT NOT NULL,expires DOUBLE PRECISION NOT NULL);
 CREATE TABLE IF NOT EXISTS clearcare.reviews(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,title TEXT NOT NULL,created TEXT NOT NULL,payload TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS clearcare.insurance_cards(user_id TEXT PRIMARY KEY,content_type TEXT NOT NULL,data BYTEA NOT NULL,updated TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS clearcare.insurance_workflows(user_id TEXT PRIMARY KEY,revision TEXT NOT NULL,result TEXT NOT NULL,document BYTEA);
 CREATE TABLE IF NOT EXISTS clearcare.ai_review_analyses(user_id TEXT NOT NULL,request_hash TEXT NOT NULL,payload TEXT NOT NULL,created BIGINT NOT NULL,PRIMARY KEY(user_id,request_hash));`)
	return err
}
func (s *postgresStore) Exec(q string, a ...any) (sql.Result, error) {
	return s.db.Exec(postgresSQL(q), a...)
}
func (s *postgresStore) Query(q string, a ...any) (*sql.Rows, error) {
	return s.db.Query(postgresSQL(q), a...)
}
func (s *postgresStore) QueryRow(q string, a ...any) RowScanner {
	return s.db.QueryRow(postgresSQL(q), a...)
}
func (s *postgresStore) Begin() (SQLTx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	return &postgresTx{tx: tx}, nil
}
func (s *postgresStore) Close() error { return s.db.Close() }
func (t *postgresTx) Exec(q string, a ...any) (sql.Result, error) {
	return t.tx.Exec(postgresSQL(q), a...)
}
func (t *postgresTx) QueryRow(q string, a ...any) RowScanner {
	return t.tx.QueryRow(postgresSQL(q), a...)
}
func (t *postgresTx) Commit() error   { return t.tx.Commit() }
func (t *postgresTx) Rollback() error { return t.tx.Rollback() }

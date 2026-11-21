package account

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{0, `CREATE TABLE IF NOT EXISTS private_documents(object_key TEXT PRIMARY KEY,user_id TEXT NOT NULL,kind TEXT NOT NULL,created TEXT NOT NULL); CREATE INDEX IF NOT EXISTS idx_private_documents_user ON private_documents(user_id);`},
	{1, `CREATE TABLE IF NOT EXISTS patient_preferences(user_id TEXT PRIMARY KEY,payload TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS feature_runs(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,feature TEXT NOT NULL,created TEXT NOT NULL,result TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS charge_observations(id TEXT PRIMARY KEY,facility_id TEXT NOT NULL,code TEXT NOT NULL,charge REAL NOT NULL,benchmark REAL NOT NULL,difference REAL NOT NULL,price_type TEXT NOT NULL,source_url TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS observation_owners(observation_id TEXT PRIMARY KEY,user_id TEXT NOT NULL,run_id TEXT NOT NULL);`},
	{2, `CREATE INDEX IF NOT EXISTS idx_sessions_user_expires ON sessions(user_id,expires);
CREATE INDEX IF NOT EXISTS idx_reviews_user_created ON reviews(user_id,created DESC);
CREATE INDEX IF NOT EXISTS idx_feature_runs_user_created ON feature_runs(user_id,created DESC,id DESC);
CREATE INDEX IF NOT EXISTS idx_feature_runs_user_feature_created ON feature_runs(user_id,feature,created DESC);
CREATE INDEX IF NOT EXISTS idx_observation_owners_user_run ON observation_owners(user_id,run_id);
CREATE TABLE IF NOT EXISTS feature_jobs(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,feature TEXT NOT NULL,request_hash TEXT NOT NULL,status TEXT NOT NULL,progress INTEGER NOT NULL,stage TEXT NOT NULL,created TEXT NOT NULL,updated TEXT NOT NULL,result_id TEXT NOT NULL DEFAULT '',result TEXT NOT NULL DEFAULT '',error TEXT NOT NULL DEFAULT '',cancel_requested INTEGER NOT NULL DEFAULT 0);
CREATE INDEX IF NOT EXISTS idx_feature_jobs_user_created ON feature_jobs(user_id,created DESC,id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_feature_jobs_active_request ON feature_jobs(user_id,feature,request_hash) WHERE status IN ('queued','running');`},
}

func (s *Service) initFeatures() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY,applied TEXT NOT NULL)`); err != nil {
		return err
	}
	for _, m := range migrations {
		var found int
		err := s.db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version=?`, m.version).Scan(&found)
		if err == nil {
			continue
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err = tx.Exec(m.sql); err == nil {
			_, err = tx.Exec(`INSERT INTO schema_migrations(version,applied) VALUES(?,?)`, m.version, time.Now().UTC().Format(time.RFC3339))
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", m.version, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`UPDATE feature_jobs SET status='interrupted',stage='service_restarted',error='The service restarted before this job completed.',updated=? WHERE status IN ('queued','running')`, time.Now().UTC().Format(time.RFC3339))
	return err
}

func retentionDays() int {
	days := 90
	if v, err := strconv.Atoi(os.Getenv("CLEARCARE_RESULT_RETENTION_DAYS")); err == nil && v >= 1 && v <= 3650 {
		days = v
	}
	return days
}
func (s *Service) pruneExpired() error {
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays()).Format(time.RFC3339)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`DELETE FROM charge_observations WHERE id IN (SELECT observation_id FROM observation_owners WHERE run_id IN (SELECT id FROM feature_runs WHERE created<?))`, cutoff); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM observation_owners WHERE run_id IN (SELECT id FROM feature_runs WHERE created<?)`, cutoff); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM feature_runs WHERE created<?`, cutoff); err != nil {
		return err
	}
	if _, err = tx.Exec(`DELETE FROM feature_jobs WHERE updated<? AND status NOT IN ('queued','running')`, cutoff); err != nil {
		return err
	}
	return tx.Commit()
}

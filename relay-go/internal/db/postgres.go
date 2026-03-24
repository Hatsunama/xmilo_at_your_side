package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	Pool *pgxpool.Pool
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, err
	}
	s := &Store{Pool: pool}
	if err := s.migrate(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		// --- v1: core sessions ---
		`CREATE TABLE IF NOT EXISTS schema_version (
            version INTEGER PRIMARY KEY,
            applied_at TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS device_users (
            device_user_id TEXT PRIMARY KEY,
            device_name    TEXT NOT NULL,
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS sessions (
            session_id     TEXT PRIMARY KEY,
            device_user_id TEXT NOT NULL REFERENCES device_users(device_user_id),
            expires_at     TIMESTAMPTZ NOT NULL,
            entitled       BOOLEAN NOT NULL DEFAULT TRUE,
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS llm_turns (
            id             BIGSERIAL PRIMARY KEY,
            device_user_id TEXT REFERENCES device_users(device_user_id),
            task_id        TEXT NOT NULL,
            phase          TEXT NOT NULL,
            prompt         TEXT NOT NULL,
            intent         TEXT NOT NULL,
            target_room    TEXT NOT NULL,
            created_at     TIMESTAMPTZ NOT NULL
        )`,

		// --- v2: access, email, entitlements ---
		`CREATE TABLE IF NOT EXISTS users (
            user_id        TEXT PRIMARY KEY,
            email          TEXT UNIQUE NOT NULL,
            email_verified BOOLEAN NOT NULL DEFAULT FALSE,
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS email_verifications (
            token          TEXT PRIMARY KEY,
            email          TEXT NOT NULL,
            device_user_id TEXT NOT NULL REFERENCES device_users(device_user_id),
            expires_at     TIMESTAMPTZ NOT NULL,
            verified_at    TIMESTAMPTZ
        )`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
            code         TEXT PRIMARY KEY,
            granted_days INTEGER NOT NULL DEFAULT 5,
            created_at   TIMESTAMPTZ NOT NULL,
            created_by   TEXT,
            used_by      TEXT REFERENCES device_users(device_user_id),
            used_at      TIMESTAMPTZ
        )`,
		// grant_type: 'trial' | 'invite' | 'subscription'
		// trial: started_at NULL = email verified, clock not yet running
		//        started_at SET  = first /llm/turn called, expires_at = started_at + 12h
		// invite: started_at = redemption time, expires_at = started_at + N days
		// subscription: started_at = purchase time, expires_at NULL = managed by RevenueCat events
		`CREATE TABLE IF NOT EXISTS entitlement_grants (
            id             BIGSERIAL PRIMARY KEY,
            device_user_id TEXT NOT NULL REFERENCES device_users(device_user_id),
            grant_type     TEXT NOT NULL,
            started_at     TIMESTAMPTZ,
            expires_at     TIMESTAMPTZ,
            active         BOOLEAN NOT NULL DEFAULT TRUE,
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS error_reports (
            id             BIGSERIAL PRIMARY KEY,
            device_user_id TEXT,
            error_type     TEXT NOT NULL,
            message        TEXT NOT NULL,
            context_data   JSONB,
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS ai_content_reports (
            id             BIGSERIAL PRIMARY KEY,
            device_user_id TEXT NOT NULL,
            task_id        TEXT,
            event_type     TEXT NOT NULL,
            report_reason  TEXT NOT NULL,
            report_note    TEXT NOT NULL DEFAULT '',
            output_text    TEXT NOT NULL,
            prompt_text    TEXT NOT NULL DEFAULT '',
            runtime_id     TEXT NOT NULL DEFAULT '',
            model_name     TEXT NOT NULL DEFAULT '',
            status         TEXT NOT NULL DEFAULT 'new',
            review_note    TEXT NOT NULL DEFAULT '',
            created_at     TIMESTAMPTZ NOT NULL
        )`,
		// Index for quick entitlement checks
		`CREATE INDEX IF NOT EXISTS idx_entitlement_device
            ON entitlement_grants(device_user_id)
            WHERE active = TRUE`,
		`ALTER TABLE llm_turns ADD COLUMN IF NOT EXISTS device_user_id TEXT REFERENCES device_users(device_user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_llm_turns_device_user ON llm_turns(device_user_id)`,
		`ALTER TABLE ai_content_reports ADD COLUMN IF NOT EXISTS review_note TEXT NOT NULL DEFAULT ''`,
		`CREATE TABLE IF NOT EXISTS two_factor_credentials (
            email        TEXT PRIMARY KEY,
            secret       TEXT NOT NULL,
            enabled      BOOLEAN NOT NULL DEFAULT FALSE,
            created_at   TIMESTAMPTZ NOT NULL,
            enabled_at   TIMESTAMPTZ,
            updated_at   TIMESTAMPTZ NOT NULL
        )`,
		`CREATE TABLE IF NOT EXISTS two_factor_recovery_codes (
            email        TEXT NOT NULL,
            code_hash    TEXT NOT NULL,
            created_at   TIMESTAMPTZ NOT NULL,
            used_at      TIMESTAMPTZ,
            PRIMARY KEY (email, code_hash)
        )`,
		`CREATE TABLE IF NOT EXISTS two_factor_trusted_devices (
            email          TEXT PRIMARY KEY,
            device_user_id TEXT NOT NULL REFERENCES device_users(device_user_id),
            verified_at    TIMESTAMPTZ NOT NULL
        )`,
	}

	for _, stmt := range statements {
		if _, err := s.Pool.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO schema_version(version, applied_at) VALUES(2, $1) ON CONFLICT (version) DO NOTHING`,
		time.Now().UTC())
	return err
}

func (s *Store) Close() {
	s.Pool.Close()
}

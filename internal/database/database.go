package database

import (
	"context"
	"database/sql"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func Open(ctx context.Context, databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func Migrate(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT NOT NULL,
	email TEXT NOT NULL UNIQUE,
	phone_number TEXT NOT NULL DEFAULT '',
	password_hash TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS phone_number TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS users_email_idx ON users (email);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL UNIQUE,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS password_reset_tokens_user_id_idx ON password_reset_tokens (user_id);
CREATE INDEX IF NOT EXISTS password_reset_tokens_token_hash_idx ON password_reset_tokens (token_hash);

CREATE TABLE IF NOT EXISTS wallets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	type TEXT NOT NULL,
	currency TEXT NOT NULL DEFAULT 'IDR',
	balance NUMERIC(20, 2) NOT NULL DEFAULT 0,
	account_number TEXT NOT NULL DEFAULT '',
	credit_limit NUMERIC(20, 2) NOT NULL DEFAULT 0,
	due_day INTEGER,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS wallets_user_id_idx ON wallets (user_id);

CREATE TABLE IF NOT EXISTS transactions (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	wallet_id UUID NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
	to_wallet_id UUID REFERENCES wallets(id) ON DELETE SET NULL,
	type TEXT NOT NULL,
	title TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT '',
	note TEXT NOT NULL DEFAULT '',
	amount NUMERIC(20, 2) NOT NULL,
	transaction_date DATE NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS transactions_user_id_idx ON transactions (user_id);
CREATE INDEX IF NOT EXISTS transactions_wallet_id_idx ON transactions (wallet_id);
CREATE INDEX IF NOT EXISTS transactions_transaction_date_idx ON transactions (transaction_date);

CREATE TABLE IF NOT EXISTS couples (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	name TEXT NOT NULL,
	invite_code TEXT NOT NULL UNIQUE,
	created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS couple_members (
	couple_id UUID NOT NULL REFERENCES couples(id) ON DELETE CASCADE,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL DEFAULT 'member',
	joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	PRIMARY KEY (couple_id, user_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS couple_members_user_id_unique_idx ON couple_members (user_id);
CREATE INDEX IF NOT EXISTS couple_members_couple_id_idx ON couple_members (couple_id);

CREATE TABLE IF NOT EXISTS budgets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	group_name TEXT NOT NULL DEFAULT 'Needs',
	category TEXT NOT NULL,
	period_month TEXT NOT NULL,
	limit_amount NUMERIC(20, 2) NOT NULL DEFAULT 0,
	icon TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE (user_id, category, period_month)
);

CREATE INDEX IF NOT EXISTS budgets_user_id_idx ON budgets (user_id);
CREATE INDEX IF NOT EXISTS budgets_period_month_idx ON budgets (period_month);
ALTER TABLE budgets ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'personal';
ALTER TABLE budgets ADD COLUMN IF NOT EXISTS couple_id UUID REFERENCES couples(id) ON DELETE CASCADE;
ALTER TABLE budgets DROP CONSTRAINT IF EXISTS budgets_user_id_category_period_month_key;
DROP INDEX IF EXISTS budgets_scope_unique_idx;
CREATE UNIQUE INDEX IF NOT EXISTS budgets_personal_unique_idx
	ON budgets (user_id, category, period_month)
	WHERE scope = 'personal';
CREATE UNIQUE INDEX IF NOT EXISTS budgets_couple_unique_idx
	ON budgets (couple_id, category, period_month)
	WHERE scope = 'couple';

ALTER TABLE transactions ADD COLUMN IF NOT EXISTS scope TEXT NOT NULL DEFAULT 'personal';
ALTER TABLE transactions ADD COLUMN IF NOT EXISTS couple_id UUID REFERENCES couples(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS transactions_scope_idx ON transactions (scope);
CREATE INDEX IF NOT EXISTS transactions_couple_id_idx ON transactions (couple_id);

CREATE TABLE IF NOT EXISTS goals (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	category TEXT NOT NULL DEFAULT 'Savings',
	target_amount NUMERIC(20, 2) NOT NULL,
	current_amount NUMERIC(20, 2) NOT NULL DEFAULT 0,
	linked_wallet_id UUID REFERENCES wallets(id) ON DELETE SET NULL,
	deadline DATE,
	icon TEXT NOT NULL DEFAULT '',
	is_emergency BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE goals ADD COLUMN IF NOT EXISTS linked_wallet_id UUID REFERENCES wallets(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS goals_user_id_idx ON goals (user_id);
CREATE INDEX IF NOT EXISTS goals_linked_wallet_id_idx ON goals (linked_wallet_id);

CREATE TABLE IF NOT EXISTS goal_contributions (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	goal_id UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
	wallet_id UUID NOT NULL REFERENCES wallets(id) ON DELETE CASCADE,
	amount NUMERIC(20, 2) NOT NULL,
	note TEXT NOT NULL DEFAULT '',
	contribution_date DATE NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS goal_contributions_user_id_idx ON goal_contributions (user_id);
CREATE INDEX IF NOT EXISTS goal_contributions_goal_id_idx ON goal_contributions (goal_id);

CREATE TABLE IF NOT EXISTS advisor_threads (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	title TEXT NOT NULL,
	period_month TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS advisor_threads_user_id_idx ON advisor_threads (user_id);
CREATE INDEX IF NOT EXISTS advisor_threads_updated_at_idx ON advisor_threads (updated_at);

CREATE TABLE IF NOT EXISTS advisor_messages (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	thread_id UUID NOT NULL REFERENCES advisor_threads(id) ON DELETE CASCADE,
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	role TEXT NOT NULL,
	content TEXT NOT NULL,
	model TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS advisor_messages_thread_id_idx ON advisor_messages (thread_id);
CREATE INDEX IF NOT EXISTS advisor_messages_user_id_idx ON advisor_messages (user_id);
CREATE INDEX IF NOT EXISTS advisor_messages_created_at_idx ON advisor_messages (created_at);

CREATE TABLE IF NOT EXISTS assets (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	asset_type TEXT NOT NULL,
	category TEXT NOT NULL,
	name TEXT NOT NULL,
	quantity NUMERIC(20, 6) NOT NULL DEFAULT 1,
	unit TEXT NOT NULL DEFAULT '',
	purchase_price NUMERIC(20, 2) NOT NULL DEFAULT 0,
	current_price NUMERIC(20, 2) NOT NULL DEFAULT 0,
	current_value NUMERIC(20, 2) NOT NULL DEFAULT 0,
	note TEXT NOT NULL DEFAULT '',
	acquired_at DATE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS assets_user_id_idx ON assets (user_id);
CREATE INDEX IF NOT EXISTS assets_asset_type_idx ON assets (asset_type);

CREATE TABLE IF NOT EXISTS bills (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	wallet_id UUID REFERENCES wallets(id) ON DELETE SET NULL,
	name TEXT NOT NULL,
	category TEXT NOT NULL,
	provider TEXT NOT NULL DEFAULT '',
	amount NUMERIC(20, 2) NOT NULL,
	due_date DATE NOT NULL,
	status TEXT NOT NULL DEFAULT 'upcoming',
	note TEXT NOT NULL DEFAULT '',
	is_recurring BOOLEAN NOT NULL DEFAULT FALSE,
	repeat_interval TEXT NOT NULL DEFAULT '',
	paid_transaction_id UUID REFERENCES transactions(id) ON DELETE SET NULL,
	paid_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS bills_user_id_idx ON bills (user_id);
CREATE INDEX IF NOT EXISTS bills_due_date_idx ON bills (due_date);
CREATE INDEX IF NOT EXISTS bills_status_idx ON bills (status);
ALTER TABLE bills ADD COLUMN IF NOT EXISTS is_recurring BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE bills ADD COLUMN IF NOT EXISTS repeat_interval TEXT NOT NULL DEFAULT '';

ALTER TABLE wallets ALTER COLUMN balance TYPE NUMERIC(20, 2);
ALTER TABLE wallets ALTER COLUMN credit_limit TYPE NUMERIC(20, 2);
ALTER TABLE transactions ALTER COLUMN amount TYPE NUMERIC(20, 2);
ALTER TABLE budgets ALTER COLUMN limit_amount TYPE NUMERIC(20, 2);
ALTER TABLE goals ALTER COLUMN target_amount TYPE NUMERIC(20, 2);
ALTER TABLE goals ALTER COLUMN current_amount TYPE NUMERIC(20, 2);
ALTER TABLE goal_contributions ALTER COLUMN amount TYPE NUMERIC(20, 2);
`)
	return err
}

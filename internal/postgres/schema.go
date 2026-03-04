package postgres

import (
	"context"
)

// schema defines the PostgreSQL schema for chatlog raw data.
const schema = `
CREATE TABLE IF NOT EXISTS accounts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account TEXT NOT NULL UNIQUE,
	platform TEXT NOT NULL,
	version INTEGER NOT NULL,
	data_dir TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	last_synced_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS messages (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	seq BIGINT NOT NULL,
	time TIMESTAMPTZ NOT NULL,
	talker TEXT NOT NULL,
	talker_name TEXT,
	is_chat_room BOOLEAN NOT NULL DEFAULT FALSE,
	sender TEXT NOT NULL,
	sender_name TEXT,
	is_self BOOLEAN NOT NULL DEFAULT FALSE,
	type INTEGER NOT NULL,
	sub_type INTEGER NOT NULL DEFAULT 0,
	content TEXT,
	contents JSONB,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(account_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_messages_account_time ON messages(account_id, time);
CREATE INDEX IF NOT EXISTS idx_messages_talker ON messages(account_id, talker);

CREATE TABLE IF NOT EXISTS contacts (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	user_name TEXT NOT NULL,
	alias TEXT,
	remark TEXT,
	nick_name TEXT,
	is_friend BOOLEAN NOT NULL DEFAULT FALSE,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(account_id, user_name)
);

CREATE TABLE IF NOT EXISTS chat_rooms (
	id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
	account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	owner TEXT,
	remark TEXT,
	nick_name TEXT,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	UNIQUE(account_id, name)
);

CREATE TABLE IF NOT EXISTS chat_room_members (
	chat_room_id UUID NOT NULL REFERENCES chat_rooms(id) ON DELETE CASCADE,
	user_name TEXT NOT NULL,
	display_name TEXT,
	PRIMARY KEY (chat_room_id, user_name)
);
`

// Migrate runs the schema migration.
func (c *Conn) Migrate(ctx context.Context) error {
	_, err := c.pool.Exec(ctx, schema)
	return err
}

package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/sjzar/chatlog/internal/model"
)

// UpsertAccount inserts or returns an existing account. Returns the account ID.
func (c *Conn) UpsertAccount(ctx context.Context, account, platform string, version int, dataDir string) (uuid.UUID, error) {
	var id uuid.UUID
	err := c.pool.QueryRow(ctx, `
		INSERT INTO accounts (account, platform, version, data_dir)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (account) DO UPDATE SET
			platform = EXCLUDED.platform,
			version = EXCLUDED.version,
			data_dir = EXCLUDED.data_dir
		RETURNING id
	`, account, platform, version, dataDir).Scan(&id)
	return id, err
}

// UpsertMessages bulk upserts messages. Uses ON CONFLICT to skip duplicates.
// supplierID is stored as-is; pass empty string for NULL.
func (c *Conn) UpsertMessages(ctx context.Context, accountID uuid.UUID, messages []*model.Message, supplierID string) (int, error) {
	if len(messages) == 0 {
		return 0, nil
	}
	// Convert empty string to nil for proper SQL NULL.
	var sid any
	if supplierID != "" {
		sid = supplierID
	}
	batch := &pgx.Batch{}
	for _, m := range messages {
		contentsJSON, _ := json.Marshal(m.Contents)
		batch.Queue(`
			INSERT INTO messages (account_id, seq, time, talker, talker_name, is_chat_room, sender, sender_name, is_self, type, sub_type, content, contents, supplier_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			ON CONFLICT (account_id, seq) DO UPDATE SET
				time = EXCLUDED.time,
				talker = EXCLUDED.talker,
				talker_name = EXCLUDED.talker_name,
				is_chat_room = EXCLUDED.is_chat_room,
				sender = EXCLUDED.sender,
				sender_name = EXCLUDED.sender_name,
				is_self = EXCLUDED.is_self,
				type = EXCLUDED.type,
				sub_type = EXCLUDED.sub_type,
				content = EXCLUDED.content,
				contents = EXCLUDED.contents,
				supplier_id = EXCLUDED.supplier_id
		`, accountID, m.Seq, m.Time, m.Talker, m.TalkerName, m.IsChatRoom, m.Sender, m.SenderName, m.IsSelf, m.Type, m.SubType, m.Content, contentsJSON, sid)
	}
	br := c.pool.SendBatch(ctx, batch)
	defer br.Close()
	var count int
	for i := 0; i < len(messages); i++ {
		_, err := br.Exec()
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertContacts bulk upserts contacts.
func (c *Conn) UpsertContacts(ctx context.Context, accountID uuid.UUID, contacts []*model.Contact) (int, error) {
	if len(contacts) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, ct := range contacts {
		batch.Queue(`
			INSERT INTO contacts (account_id, user_name, alias, remark, nick_name, is_friend)
			VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT (account_id, user_name) DO UPDATE SET
				alias = EXCLUDED.alias,
				remark = EXCLUDED.remark,
				nick_name = EXCLUDED.nick_name,
				is_friend = EXCLUDED.is_friend
		`, accountID, ct.UserName, ct.Alias, ct.Remark, ct.NickName, ct.IsFriend)
	}
	br := c.pool.SendBatch(ctx, batch)
	defer br.Close()
	var count int
	for i := 0; i < len(contacts); i++ {
		_, err := br.Exec()
		if err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

// UpsertChatRooms bulk upserts chat rooms and their members.
func (c *Conn) UpsertChatRooms(ctx context.Context, accountID uuid.UUID, chatRooms []*model.ChatRoom) (int, error) {
	if len(chatRooms) == 0 {
		return 0, nil
	}
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	var count int
	for _, cr := range chatRooms {
		var crID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO chat_rooms (account_id, name, owner, remark, nick_name)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (account_id, name) DO UPDATE SET
				owner = EXCLUDED.owner,
				remark = EXCLUDED.remark,
				nick_name = EXCLUDED.nick_name
			RETURNING id
		`, accountID, cr.Name, cr.Owner, cr.Remark, cr.NickName).Scan(&crID)
		if err != nil {
			return count, err
		}
		// Upsert members
		for _, u := range cr.Users {
			_, err := tx.Exec(ctx, `
				INSERT INTO chat_room_members (chat_room_id, user_name, display_name)
				VALUES ($1, $2, $3)
				ON CONFLICT (chat_room_id, user_name) DO UPDATE SET display_name = EXCLUDED.display_name
			`, crID, u.UserName, u.DisplayName)
			if err != nil {
				return count, err
			}
		}
		count++
	}
	return count, tx.Commit(ctx)
}

// GetLastSyncedAt returns the last synced timestamp for an account, or zero time if never synced.
func (c *Conn) GetLastSyncedAt(ctx context.Context, accountID uuid.UUID) (time.Time, error) {
	var t *time.Time
	err := c.pool.QueryRow(ctx, `SELECT last_synced_at FROM accounts WHERE id = $1`, accountID).Scan(&t)
	if err == pgx.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	if t == nil || t.IsZero() {
		return time.Time{}, nil
	}
	return *t, nil
}

// SetLastSyncedAt updates the last synced timestamp for an account.
func (c *Conn) SetLastSyncedAt(ctx context.Context, accountID uuid.UUID, t time.Time) error {
	_, err := c.pool.Exec(ctx, `UPDATE accounts SET last_synced_at = $1 WHERE id = $2`, t, accountID)
	return err
}

// GetMaxMessageTimeForTalkers returns the maximum message time per talker for the given account.
// Talkers with no messages are omitted from the result. Used for per-talker incremental sync.
func (c *Conn) GetMaxMessageTimeForTalkers(ctx context.Context, accountID uuid.UUID, talkers []string) (map[string]time.Time, error) {
	if len(talkers) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(talkers))
	args := make([]any, 0, 1+len(talkers))
	args = append(args, accountID)
	for i := range talkers {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, talkers[i])
	}
	query := `SELECT talker, MAX(time) AS max_time
		FROM messages
		WHERE account_id = $1 AND talker IN (` + strings.Join(placeholders, ", ") + `)
		GROUP BY talker`
	rows, err := c.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]time.Time)
	for rows.Next() {
		var talker string
		var t time.Time
		if err := rows.Scan(&talker, &t); err != nil {
			return nil, err
		}
		result[talker] = t
	}
	return result, rows.Err()
}

// UpdateSupplierIDForTalker updates supplier_id for all existing messages of a talker
// when the supplier mapping has changed. Skips update when supplier_id already matches.
func (c *Conn) UpdateSupplierIDForTalker(ctx context.Context, accountID uuid.UUID, talker, supplierID string) error {
	var sid any
	if supplierID != "" {
		sid = supplierID
	}
	_, err := c.pool.Exec(ctx, `
		UPDATE messages SET supplier_id = $1
		WHERE account_id = $2 AND talker = $3 AND (supplier_id IS DISTINCT FROM $4)
	`, sid, accountID, talker, sid)
	return err
}

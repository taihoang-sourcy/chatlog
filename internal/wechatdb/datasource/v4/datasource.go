package v4

import (
	"context"
	"crypto/md5"
	"database/sql"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/wechatdb/datasource/dbm"
	"github.com/sjzar/chatlog/pkg/util"
)

const (
	Message = "message"
	Contact = "contact"
	Session = "session"
	Media   = "media"
	Voice   = "voice"
)

var Groups = []*dbm.Group{
	{
		Name:      Message,
		Pattern:   `^message_([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Contact,
		Pattern:   `^contact\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Session,
		Pattern:   `session\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Media,
		Pattern:   `^hardlink\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Voice,
		Pattern:   `^media_([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
}

// MessageDBInfo stores message database info
type MessageDBInfo struct {
	FilePath  string
	StartTime time.Time
	EndTime   time.Time
}

type DataSource struct {
	path string
	dbm  *dbm.DBManager

	// Message database info
	messageInfos []MessageDBInfo
}

func New(path string) (*DataSource, error) {

	ds := &DataSource{
		path:         path,
		dbm:          dbm.NewDBManager(path),
		messageInfos: make([]MessageDBInfo, 0),
	}

	for _, g := range Groups {
		ds.dbm.AddGroup(g)
	}

	if err := ds.dbm.Start(); err != nil {
		return nil, err
	}

	if err := ds.initMessageDbs(); err != nil {
		return nil, errors.DBInitFailed(err)
	}

	ds.dbm.AddCallback(Message, func(event fsnotify.Event) error {
		if !event.Op.Has(fsnotify.Create) {
			return nil
		}
		if err := ds.initMessageDbs(); err != nil {
			log.Err(err).Msgf("Failed to reinitialize message DBs: %s", event.Name)
		}
		return nil
	})

	return ds, nil
}

func (ds *DataSource) SetCallback(group string, callback func(event fsnotify.Event) error) error {
	if group == "chatroom" {
		group = Contact
	}
	return ds.dbm.AddCallback(group, callback)
}

func (ds *DataSource) initMessageDbs() error {
	dbPaths, err := ds.dbm.GetDBPath(Message)
	if err != nil {
		if strings.Contains(err.Error(), "db file not found") {
			ds.messageInfos = make([]MessageDBInfo, 0)
			return nil
		}
		return err
	}

	// Process each database file
	infos := make([]MessageDBInfo, 0)
	for _, filePath := range dbPaths {
		db, err := ds.dbm.OpenDB(filePath)
		if err != nil {
			log.Err(err).Msgf("failed to get database %s", filePath)
			continue
		}

		// Get start time from Timestamp table
		var startTime time.Time
		var timestamp int64

		row := db.QueryRow("SELECT timestamp FROM Timestamp LIMIT 1")
		if err := row.Scan(&timestamp); err != nil {
			log.Err(err).Msgf("failed to get timestamp from database %s", filePath)
			continue
		}
		startTime = time.Unix(timestamp, 0)

		// Save database info
		infos = append(infos, MessageDBInfo{
			FilePath:  filePath,
			StartTime: startTime,
		})
	}

	// Sort database files by StartTime
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].StartTime.Before(infos[j].StartTime)
	})

	// Set end time
	for i := range infos {
		if i == len(infos)-1 {
			infos[i].EndTime = time.Now().Add(time.Hour)
		} else {
			infos[i].EndTime = infos[i+1].StartTime
		}
	}
	if len(ds.messageInfos) > 0 && len(infos) < len(ds.messageInfos) {
		log.Warn().Msgf("message db count decreased from %d to %d, skip init", len(ds.messageInfos), len(infos))
		return nil
	}
	ds.messageInfos = infos
	return nil
}

// getDBInfosForTimeRange gets database info for the time range
func (ds *DataSource) getDBInfosForTimeRange(startTime, endTime time.Time) []MessageDBInfo {
	var dbs []MessageDBInfo
	for _, info := range ds.messageInfos {
		if info.StartTime.Before(endTime) && info.EndTime.After(startTime) {
			dbs = append(dbs, info)
		}
	}
	return dbs
}

func (ds *DataSource) GetMessages(ctx context.Context, startTime, endTime time.Time, talker string, sender string, keyword string, limit, offset int) ([]*model.Message, error) {
	if talker == "" {
		return nil, errors.ErrTalkerEmpty
	}

	// Parse talker param, supports multiple talkers (comma-separated)
	talkers := util.Str2List(talker, ",")
	if len(talkers) == 0 {
		return nil, errors.ErrTalkerEmpty
	}

	// Find database files in time range
	dbInfos := ds.getDBInfosForTimeRange(startTime, endTime)
	if len(dbInfos) == 0 {
		return nil, errors.TimeRangeNotFound(startTime, endTime)
	}

	// Parse sender param, supports multiple senders (comma-separated)
	senders := util.Str2List(sender, ",")

	// Precompile regex (if keyword provided)
	var regex *regexp.Regexp
	if keyword != "" {
		var err error
		regex, err = regexp.Compile(keyword)
		if err != nil {
			return nil, errors.QueryFailed("invalid regex pattern", err)
		}
	}

	// Query messages from each relevant database, filter while reading
	filteredMessages := []*model.Message{}

	for _, dbInfo := range dbInfos {
		// Check if context is cancelled
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		db, err := ds.dbm.OpenDB(dbInfo.FilePath)
		if err != nil {
			log.Error().Msgf("database %s not opened", dbInfo.FilePath)
			continue
		}

		// Query for each talker
		for _, talkerItem := range talkers {
			// Build table name
			_talkerMd5Bytes := md5.Sum([]byte(talkerItem))
			talkerMd5 := hex.EncodeToString(_talkerMd5Bytes[:])
			tableName := "Msg_" + talkerMd5

			// Check if table exists
			var exists bool
			err = db.QueryRowContext(ctx,
				"SELECT 1 FROM sqlite_master WHERE type='table' AND name=?",
				tableName).Scan(&exists)

			if err != nil {
				if err == sql.ErrNoRows {
					// Table does not exist, skip to next talker
					continue
				}
				return nil, errors.QueryFailed("", err)
			}

			// Build query conditions
			conditions := []string{"create_time >= ? AND create_time <= ?"}
			args := []interface{}{startTime.Unix(), endTime.Unix()}
			log.Debug().Msgf("Table name: %s", tableName)
			log.Debug().Msgf("Start time: %d, End time: %d", startTime.Unix(), endTime.Unix())

			query := fmt.Sprintf(`
				SELECT m.sort_seq, m.server_id, m.local_type, n.user_name, m.create_time, m.message_content, m.packed_info_data, m.status
				FROM %s m
				LEFT JOIN Name2Id n ON m.real_sender_id = n.rowid
				WHERE %s 
				ORDER BY m.sort_seq ASC
			`, tableName, strings.Join(conditions, " AND "))

			// Execute query
			rows, err := db.QueryContext(ctx, query, args...)
			if err != nil {
				// If table does not exist, SQLite will return error
				if strings.Contains(err.Error(), "no such table") {
					continue
				}
				log.Err(err).Msgf("failed to query messages from database %s", dbInfo.FilePath)
				continue
			}

			// Process query results, filter while reading
			for rows.Next() {
				var msg model.MessageV4
				err := rows.Scan(
					&msg.SortSeq,
					&msg.ServerID,
					&msg.LocalType,
					&msg.UserName,
					&msg.CreateTime,
					&msg.MessageContent,
					&msg.PackedInfoData,
					&msg.Status,
				)
				if err != nil {
					rows.Close()
					return nil, errors.ScanRowFailed(err)
				}

				// Convert message to standard format
				message := msg.Wrap(talkerItem)

				// Apply sender filter
				if len(senders) > 0 {
					senderMatch := false
					for _, s := range senders {
						if message.Sender == s {
							senderMatch = true
							break
						}
					}
					if !senderMatch {
						continue // Sender mismatch, skip message
					}
				}

				// Apply keyword filter
				if regex != nil {
					plainText := message.PlainTextContent()
					if !regex.MatchString(plainText) {
						continue // Keyword mismatch, skip message
					}
				}

				// Passed all filters, keep message
				filteredMessages = append(filteredMessages, message)

				// Check if pagination limit reached
				if limit > 0 && len(filteredMessages) >= offset+limit {
					// Got enough messages, can return early
					rows.Close()

					// Sort all messages by time
					sort.Slice(filteredMessages, func(i, j int) bool {
						return filteredMessages[i].Seq < filteredMessages[j].Seq
					})

					// Handle pagination
					if offset >= len(filteredMessages) {
						return []*model.Message{}, nil
					}
					end := offset + limit
					if end > len(filteredMessages) {
						end = len(filteredMessages)
					}
					return filteredMessages[offset:end], nil
				}
			}
			rows.Close()
		}
	}

	// Sort all messages by time
	sort.Slice(filteredMessages, func(i, j int) bool {
		return filteredMessages[i].Seq < filteredMessages[j].Seq
	})

	// Handle pagination
	if limit > 0 {
		if offset >= len(filteredMessages) {
			return []*model.Message{}, nil
		}
		end := offset + limit
		if end > len(filteredMessages) {
			end = len(filteredMessages)
		}
		return filteredMessages[offset:end], nil
	}

	return filteredMessages, nil
}

// Contacts
func (ds *DataSource) GetContacts(ctx context.Context, key string, limit, offset int) ([]*model.Contact, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT username, local_type, alias, remark, nick_name 
				FROM contact 
				WHERE username = ? OR alias = ? OR remark = ? OR nick_name = ?`
		args = []interface{}{key, key, key, key}
	} else {
		// Query all contacts
		query = `SELECT username, local_type, alias, remark, nick_name FROM contact`
	}

	// Add sorting and pagination
	query += ` ORDER BY username`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// 执行查询
	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	contacts := []*model.Contact{}
	for rows.Next() {
		var contactV4 model.ContactV4
		err := rows.Scan(
			&contactV4.UserName,
			&contactV4.LocalType,
			&contactV4.Alias,
			&contactV4.Remark,
			&contactV4.NickName,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		contacts = append(contacts, contactV4.Wrap())
	}

	return contacts, nil
}

// Chat rooms
func (ds *DataSource) GetChatRooms(ctx context.Context, key string, limit, offset int) ([]*model.ChatRoom, error) {
	var query string
	var args []interface{}

	// 执行查询
	db, err := ds.dbm.GetDB(Contact)
	if err != nil {
		return nil, err
	}

	if key != "" {
		// Query by keyword
		query = `SELECT username, owner, ext_buffer FROM chat_room WHERE username = ?`
		args = []interface{}{key}

		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, errors.QueryFailed(query, err)
		}
		defer rows.Close()

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV4 model.ChatRoomV4
			err := rows.Scan(
				&chatRoomV4.UserName,
				&chatRoomV4.Owner,
				&chatRoomV4.ExtBuffer,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV4.Wrap())
		}

		// If chat room not found, try looking up via contacts
		if len(chatRooms) == 0 {
			contacts, err := ds.GetContacts(ctx, key, 1, 0)
			if err == nil && len(contacts) > 0 && strings.HasSuffix(contacts[0].UserName, "@chatroom") {
				// Try again to find chat room by username
				rows, err := db.QueryContext(ctx,
					`SELECT username, owner, ext_buffer FROM chat_room WHERE username = ?`,
					contacts[0].UserName)

				if err != nil {
					return nil, errors.QueryFailed(query, err)
				}
				defer rows.Close()

				for rows.Next() {
					var chatRoomV4 model.ChatRoomV4
					err := rows.Scan(
						&chatRoomV4.UserName,
						&chatRoomV4.Owner,
						&chatRoomV4.ExtBuffer,
					)

					if err != nil {
						return nil, errors.ScanRowFailed(err)
					}

					chatRooms = append(chatRooms, chatRoomV4.Wrap())
				}

				// If chat room record doesn't exist but contact record does, create a mock chat room object
				if len(chatRooms) == 0 {
					chatRooms = append(chatRooms, &model.ChatRoom{
						Name:             contacts[0].UserName,
						Users:            make([]model.ChatRoomUser, 0),
						User2DisplayName: make(map[string]string),
					})
				}
			}
		}

		return chatRooms, nil
	} else {
		// Query all chat rooms
		query = `SELECT username, owner, ext_buffer FROM chat_room`

		// Add sorting and pagination
		query += ` ORDER BY username`
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT %d", limit)
			if offset > 0 {
				query += fmt.Sprintf(" OFFSET %d", offset)
			}
		}

		// Execute query
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, errors.QueryFailed(query, err)
		}
		defer rows.Close()

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV4 model.ChatRoomV4
			err := rows.Scan(
				&chatRoomV4.UserName,
				&chatRoomV4.Owner,
				&chatRoomV4.ExtBuffer,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV4.Wrap())
		}

		return chatRooms, nil
	}
}

// Recent sessions
func (ds *DataSource) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT username, summary, last_timestamp, last_msg_sender, last_sender_display_name 
				FROM SessionTable 
				WHERE username = ? OR last_sender_display_name = ?
				ORDER BY sort_timestamp DESC`
		args = []interface{}{key, key}
	} else {
		// Query all sessions
		query = `SELECT username, summary, last_timestamp, last_msg_sender, last_sender_display_name 
				FROM SessionTable 
				ORDER BY sort_timestamp DESC`
	}

	// Add pagination
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// 执行查询
	db, err := ds.dbm.GetDB(Session)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	sessions := []*model.Session{}
	for rows.Next() {
		var sessionV4 model.SessionV4
		err := rows.Scan(
			&sessionV4.Username,
			&sessionV4.Summary,
			&sessionV4.LastTimestamp,
			&sessionV4.LastMsgSender,
			&sessionV4.LastSenderDisplayName,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		sessions = append(sessions, sessionV4.Wrap())
	}

	return sessions, nil
}

func (ds *DataSource) GetMedia(ctx context.Context, _type string, key string) (*model.Media, error) {
	if key == "" {
		return nil, errors.ErrKeyEmpty
	}

	var table string
	switch _type {
	case "image":
		table = "image_hardlink_info_v3"
		// v4 table used since version 4.1.0
		if !ds.IsExist(Media, table) {
			table = "image_hardlink_info_v4"
		}
	case "video":
		table = "video_hardlink_info_v3"
		if !ds.IsExist(Media, table) {
			table = "video_hardlink_info_v4"
		}
	case "file":
		table = "file_hardlink_info_v3"
		if !ds.IsExist(Media, table) {
			table = "file_hardlink_info_v4"
		}
	case "voice":
		return ds.GetVoice(ctx, key)
	default:
		return nil, errors.MediaTypeUnsupported(_type)
	}

	query := fmt.Sprintf(`
	SELECT 
		f.md5,
		f.file_name,
		f.file_size,
		f.modify_time,
		IFNULL(d1.username,""),
		IFNULL(d2.username,"")
	FROM 
		%s f
	LEFT JOIN 
		dir2id d1 ON d1.rowid = f.dir1
	LEFT JOIN 
		dir2id d2 ON d2.rowid = f.dir2
	`, table)
	query += " WHERE f.md5 = ? OR f.file_name LIKE ? || '%'"
	args := []interface{}{key, key}

	// 执行查询
	db, err := ds.dbm.GetDB(Media)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	var media *model.Media
	for rows.Next() {
		var mediaV4 model.MediaV4
		err := rows.Scan(
			&mediaV4.Key,
			&mediaV4.Name,
			&mediaV4.Size,
			&mediaV4.ModifyTime,
			&mediaV4.Dir1,
			&mediaV4.Dir2,
		)
		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}
		mediaV4.Type = _type
		media = mediaV4.Wrap()

		// Prefer returning HD image
		if _type == "image" && strings.HasSuffix(mediaV4.Name, "_h.dat") {
			break
		}
	}

	if media == nil {
		return nil, errors.ErrMediaNotFound
	}

	return media, nil
}

func (ds *DataSource) IsExist(_db string, table string) bool {
	db, err := ds.dbm.GetDB(_db)
	if err != nil {
		return false
	}
	var tableName string
	query := "SELECT name FROM sqlite_master WHERE type='table' AND name=?;"
	if err = db.QueryRow(query, table).Scan(&tableName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false
		}
		return false
	}
	return true
}

func (ds *DataSource) GetVoice(ctx context.Context, key string) (*model.Media, error) {
	if key == "" {
		return nil, errors.ErrKeyEmpty
	}

	query := `
	SELECT voice_data
	FROM VoiceInfo
	WHERE svr_id = ? 
	`
	args := []interface{}{key}

	dbs, err := ds.dbm.GetDBs(Voice)
	if err != nil {
		return nil, errors.DBConnectFailed("", err)
	}

	for _, db := range dbs {
		rows, err := db.QueryContext(ctx, query, args...)
		if err != nil {
			return nil, errors.QueryFailed(query, err)
		}
		defer rows.Close()

		for rows.Next() {
			var voiceData []byte
			err := rows.Scan(
				&voiceData,
			)
			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}
			if len(voiceData) > 0 {
				return &model.Media{
					Type: "voice",
					Key:  key,
					Data: voiceData,
				}, nil
			}
		}
	}

	return nil, errors.ErrMediaNotFound
}

func (ds *DataSource) Close() error {
	return ds.dbm.Close()
}

package darwinv3

import (
	"context"
	"crypto/md5"
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
	Message  = "message"
	Contact  = "contact"
	ChatRoom = "chatroom"
	Session  = "session"
	Media    = "media"
)

var Groups = []*dbm.Group{
	{
		Name:      Message,
		Pattern:   `^msg_([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Contact,
		Pattern:   `^wccontact_new2\.db$`,
		BlackList: []string{},
	},
	{
		Name:      ChatRoom,
		Pattern:   `group_new\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Session,
		Pattern:   `^session_new\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Media,
		Pattern:   `^hldata\.db$`,
		BlackList: []string{},
	},
}

type DataSource struct {
	path string
	dbm  *dbm.DBManager

	talkerDBMap      map[string]string
	user2DisplayName map[string]string
}

func New(path string) (*DataSource, error) {
	ds := &DataSource{
		path:             path,
		dbm:              dbm.NewDBManager(path),
		talkerDBMap:      make(map[string]string),
		user2DisplayName: make(map[string]string),
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
	if err := ds.initChatRoomDb(); err != nil {
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
	ds.dbm.AddCallback(ChatRoom, func(event fsnotify.Event) error {
		if !event.Op.Has(fsnotify.Create) {
			return nil
		}
		if err := ds.initChatRoomDb(); err != nil {
			log.Err(err).Msgf("Failed to reinitialize chatroom DB: %s", event.Name)
		}
		return nil
	})

	return ds, nil
}

func (ds *DataSource) SetCallback(group string, callback func(event fsnotify.Event) error) error {
	return ds.dbm.AddCallback(group, callback)
}

func (ds *DataSource) initMessageDbs() error {

	dbPaths, err := ds.dbm.GetDBPath(Message)
	if err != nil {
		if strings.Contains(err.Error(), "db file not found") {
			ds.talkerDBMap = make(map[string]string)
			return nil
		}
		return err
	}
	// Process each database file
	talkerDBMap := make(map[string]string)
	for _, filePath := range dbPaths {
		db, err := ds.dbm.OpenDB(filePath)
		if err != nil {
			log.Err(err).Msgf("failed to get database %s", filePath)
			continue
		}

		// Get all table names
		rows, err := db.Query("SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'Chat_%'")
		if err != nil {
			log.Err(err).Msgf("no Chat table in database %s", filePath)
			continue
		}

		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				log.Err(err).Msgf("failed to scan table names in database %s", filePath)
				continue
			}

			// Extract possible talker info from table name
			talkerMd5 := extractTalkerFromTableName(tableName)
			if talkerMd5 == "" {
				continue
			}
			talkerDBMap[talkerMd5] = filePath
		}
		rows.Close()
	}
	ds.talkerDBMap = talkerDBMap
	return nil
}

func (ds *DataSource) initChatRoomDb() error {
	db, err := ds.dbm.GetDB(ChatRoom)
	if err != nil {
		if strings.Contains(err.Error(), "db file not found") {
			ds.user2DisplayName = make(map[string]string)
			return nil
		}
		return err
	}

	rows, err := db.Query("SELECT m_nsUsrName, IFNULL(nickname,\"\") FROM GroupMember")
	if err != nil {
		log.Err(err).Msg("failed to get chat room members")
		return nil
	}

	user2DisplayName := make(map[string]string)
	for rows.Next() {
		var user string
		var nickName string
		if err := rows.Scan(&user, &nickName); err != nil {
			log.Err(err).Msg("failed to scan table names")
			continue
		}
		user2DisplayName[user] = nickName
	}
	rows.Close()
	ds.user2DisplayName = user2DisplayName

	return nil
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

	// Query for each talker
	for _, talkerItem := range talkers {
		// Check if context is cancelled
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// In darwinv3, need to find corresponding database first
		_talkerMd5Bytes := md5.Sum([]byte(talkerItem))
		talkerMd5 := hex.EncodeToString(_talkerMd5Bytes[:])
		dbPath, ok := ds.talkerDBMap[talkerMd5]
		if !ok {
			// If corresponding database not found, skip this talker
			continue
		}

		db, err := ds.dbm.OpenDB(dbPath)
		if err != nil {
			log.Error().Msgf("database %s not opened", dbPath)
			continue
		}

		tableName := fmt.Sprintf("Chat_%s", talkerMd5)

		// Build query conditions
		query := fmt.Sprintf(`
			SELECT msgCreateTime, msgContent, messageType, mesDes
			FROM %s 
			WHERE msgCreateTime >= ? AND msgCreateTime <= ? 
			ORDER BY msgCreateTime ASC
		`, tableName)

		// Execute query
		rows, err := db.QueryContext(ctx, query, startTime.Unix(), endTime.Unix())
		if err != nil {
			// If table does not exist, skip this talker
			if strings.Contains(err.Error(), "no such table") {
				continue
			}
			log.Err(err).Msgf("failed to query messages from database %s", dbPath)
			continue
		}

		// Process query results, filter while reading
		for rows.Next() {
			var msg model.MessageDarwinV3
			err := rows.Scan(
				&msg.MsgCreateTime,
				&msg.MsgContent,
				&msg.MessageType,
				&msg.MesDes,
			)
			if err != nil {
				rows.Close()
				log.Err(err).Msgf("failed to scan message row")
				continue
			}

			// Wrap message into common model
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

	// Sort all messages by time
	// FIXME Need to sort by Time for different talkers
	sort.Slice(filteredMessages, func(i, j int) bool {
		return filteredMessages[i].Time.Before(filteredMessages[j].Time)
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

// Extract talker from table name
func extractTalkerFromTableName(tableName string) string {

	if !strings.HasPrefix(tableName, "Chat_") {
		return ""
	}

	if strings.HasSuffix(tableName, "_dels") {
		return ""
	}

	return strings.TrimPrefix(tableName, "Chat_")
}

// GetContacts implements the method to get contact info
func (ds *DataSource) GetContacts(ctx context.Context, key string, limit, offset int) ([]*model.Contact, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT IFNULL(m_nsUsrName,""), IFNULL(nickname,""), IFNULL(m_nsRemark,""), m_uiSex, IFNULL(m_nsAliasName,"") 
				FROM WCContact 
				WHERE m_nsUsrName = ? OR nickname = ? OR m_nsRemark = ? OR m_nsAliasName = ?`
		args = []interface{}{key, key, key, key}
	} else {
		// Query all contacts
		query = `SELECT IFNULL(m_nsUsrName,""), IFNULL(nickname,""), IFNULL(m_nsRemark,""), m_uiSex, IFNULL(m_nsAliasName,"") 
				FROM WCContact`
	}

	// Add sorting and pagination
	query += ` ORDER BY m_nsUsrName`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// Execute query
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
		var contactDarwinV3 model.ContactDarwinV3
		err := rows.Scan(
			&contactDarwinV3.M_nsUsrName,
			&contactDarwinV3.Nickname,
			&contactDarwinV3.M_nsRemark,
			&contactDarwinV3.M_uiSex,
			&contactDarwinV3.M_nsAliasName,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		contacts = append(contacts, contactDarwinV3.Wrap())
	}

	return contacts, nil
}

// GetChatRooms implements the method to get chat room info
func (ds *DataSource) GetChatRooms(ctx context.Context, key string, limit, offset int) ([]*model.ChatRoom, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT IFNULL(m_nsUsrName,""), IFNULL(nickname,""), IFNULL(m_nsRemark,""), IFNULL(m_nsChatRoomMemList,""), IFNULL(m_nsChatRoomAdminList,"") 
				FROM GroupContact 
				WHERE m_nsUsrName = ? OR nickname = ? OR m_nsRemark = ?`
		args = []interface{}{key, key, key}
	} else {
		// Query all chat rooms
		query = `SELECT IFNULL(m_nsUsrName,""), IFNULL(nickname,""), IFNULL(m_nsRemark,""), IFNULL(m_nsChatRoomMemList,""), IFNULL(m_nsChatRoomAdminList,"") 
				FROM GroupContact`
	}

	// Add sorting and pagination
	query += ` ORDER BY m_nsUsrName`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// Execute query
	db, err := ds.dbm.GetDB(ChatRoom)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	chatRooms := []*model.ChatRoom{}
	for rows.Next() {
		var chatRoomDarwinV3 model.ChatRoomDarwinV3
		err := rows.Scan(
			&chatRoomDarwinV3.M_nsUsrName,
			&chatRoomDarwinV3.Nickname,
			&chatRoomDarwinV3.M_nsRemark,
			&chatRoomDarwinV3.M_nsChatRoomMemList,
			&chatRoomDarwinV3.M_nsChatRoomAdminList,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		chatRooms = append(chatRooms, chatRoomDarwinV3.Wrap(ds.user2DisplayName))
	}

		// If chat room not found, try looking up via contacts
	if len(chatRooms) == 0 && key != "" {
		contacts, err := ds.GetContacts(ctx, key, 1, 0)
		if err == nil && len(contacts) > 0 && strings.HasSuffix(contacts[0].UserName, "@chatroom") {
			// Try again to find chat room by username
			rows, err := db.QueryContext(ctx,
				`SELECT IFNULL(m_nsUsrName,""), IFNULL(nickname,""), IFNULL(m_nsRemark,""), IFNULL(m_nsChatRoomMemList,""), IFNULL(m_nsChatRoomAdminList,"") 
				FROM GroupContact 
				WHERE m_nsUsrName = ?`,
				contacts[0].UserName)

			if err != nil {
				return nil, errors.QueryFailed(query, err)
			}
			defer rows.Close()

			for rows.Next() {
				var chatRoomDarwinV3 model.ChatRoomDarwinV3
				err := rows.Scan(
					&chatRoomDarwinV3.M_nsUsrName,
					&chatRoomDarwinV3.Nickname,
					&chatRoomDarwinV3.M_nsRemark,
					&chatRoomDarwinV3.M_nsChatRoomMemList,
					&chatRoomDarwinV3.M_nsChatRoomAdminList,
				)

				if err != nil {
					return nil, errors.ScanRowFailed(err)
				}

				chatRooms = append(chatRooms, chatRoomDarwinV3.Wrap(ds.user2DisplayName))
			}

			// If chat room record doesn't exist but contact record does, create a mock chat room object
			if len(chatRooms) == 0 {
				chatRooms = append(chatRooms, &model.ChatRoom{
					Name:  contacts[0].UserName,
					Users: make([]model.ChatRoomUser, 0),
				})
			}
		}
	}

	return chatRooms, nil
}

// GetSessions implements the method to get session info
func (ds *DataSource) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT m_nsUserName, m_uLastTime 
				FROM SessionAbstract 
				WHERE m_nsUserName = ?`
		args = []interface{}{key}
	} else {
		// Query all sessions
		query = `SELECT m_nsUserName, m_uLastTime 
				FROM SessionAbstract`
	}

	// Add sorting and pagination
	query += ` ORDER BY m_uLastTime DESC`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", offset)
		}
	}

	// Execute query
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
		var sessionDarwinV3 model.SessionDarwinV3
		err := rows.Scan(
			&sessionDarwinV3.M_nsUserName,
			&sessionDarwinV3.M_uLastTime,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		// Wrap into common model
		session := sessionDarwinV3.Wrap()

		// Try to get contact info to enrich session info
		contacts, err := ds.GetContacts(ctx, session.UserName, 1, 0)
		if err == nil && len(contacts) > 0 {
			session.NickName = contacts[0].DisplayName()
		} else {
			// Try to get chat room info
			chatRooms, err := ds.GetChatRooms(ctx, session.UserName, 1, 0)
			if err == nil && len(chatRooms) > 0 {
				session.NickName = chatRooms[0].DisplayName()
			}
		}

		sessions = append(sessions, session)
	}

	return sessions, nil
}

func (ds *DataSource) GetMedia(ctx context.Context, _type string, key string) (*model.Media, error) {
	if key == "" {
		return nil, errors.ErrKeyEmpty
	}
	query := `SELECT 
    r.mediaMd5,
    r.mediaSize,
    r.inodeNumber,
    r.modifyTime,
    d.relativePath,
    d.fileName
FROM 
    HlinkMediaRecord r
JOIN 
    HlinkMediaDetail d ON r.inodeNumber = d.inodeNumber
WHERE 
    r.mediaMd5 = ?`
	args := []interface{}{key}
	// Execute query
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
		var mediaDarwinV3 model.MediaDarwinV3
		err := rows.Scan(
			&mediaDarwinV3.MediaMd5,
			&mediaDarwinV3.MediaSize,
			&mediaDarwinV3.InodeNumber,
			&mediaDarwinV3.ModifyTime,
			&mediaDarwinV3.RelativePath,
			&mediaDarwinV3.FileName,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		// Wrap into common model
		media = mediaDarwinV3.Wrap()
	}

	if media == nil {
		return nil, errors.ErrMediaNotFound
	}

	return media, nil
}

// Close implements the method to close database connection
func (ds *DataSource) Close() error {
	return ds.dbm.Close()
}

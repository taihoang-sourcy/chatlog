package windowsv3

import (
	"context"
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
	Image   = "image"
	Video   = "video"
	File    = "file"
	Voice   = "voice"
)

var Groups = []*dbm.Group{
	{
		Name:      Message,
		Pattern:   `^MSG([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Contact,
		Pattern:   `^MicroMsg\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Image,
		Pattern:   `^HardLinkImage\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Video,
		Pattern:   `^HardLinkVideo\.db$`,
		BlackList: []string{},
	},
	{
		Name:      File,
		Pattern:   `^HardLinkFile\.db$`,
		BlackList: []string{},
	},
	{
		Name:      Voice,
		Pattern:   `^MediaMSG([0-9]?[0-9])?\.db$`,
		BlackList: []string{},
	},
}

// MessageDBInfo holds message database info
type MessageDBInfo struct {
	FilePath  string
	StartTime time.Time
	EndTime   time.Time
	TalkerMap map[string]int
}

// DataSource implements the DataSource interface
type DataSource struct {
	path string
	dbm  *dbm.DBManager

	// Message database info
	messageInfos []MessageDBInfo
}

// New creates a new WindowsV3DataSource
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

// initMessageDbs initializes message databases
func (ds *DataSource) initMessageDbs() error {
	// Get all message database file paths
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

		// Get start time from DBInfo table
		var startTime time.Time

		rows, err := db.Query("SELECT tableIndex, tableVersion, tableDesc FROM DBInfo")
		if err != nil {
			log.Err(err).Msgf("failed to query DBInfo table in database %s", filePath)
			continue
		}

		for rows.Next() {
			var tableIndex int
			var tableVersion int64
			var tableDesc string

			if err := rows.Scan(&tableIndex, &tableVersion, &tableDesc); err != nil {
				log.Err(err).Msg("failed to scan DBInfo row")
				continue
			}

			// Find record with "Start Time" description
			if strings.Contains(tableDesc, "Start Time") {
				startTime = time.Unix(tableVersion/1000, (tableVersion%1000)*1000000)
				break
			}
		}
		rows.Close()

		// Build TalkerMap
		talkerMap := make(map[string]int)
		rows, err = db.Query("SELECT UsrName FROM Name2ID")
		if err != nil {
			log.Err(err).Msgf("failed to query Name2ID table in database %s", filePath)
			continue
		}

		i := 1
		for rows.Next() {
			var userName string
			if err := rows.Scan(&userName); err != nil {
				log.Err(err).Msg("failed to scan Name2ID row")
				continue
			}
			talkerMap[userName] = i
			i++
		}
		rows.Close()

		// Save database info
		infos = append(infos, MessageDBInfo{
			FilePath:  filePath,
			StartTime: startTime,
			TalkerMap: talkerMap,
		})
	}

	// Sort database files by StartTime
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].StartTime.Before(infos[j].StartTime)
	})

	// Set end time
	for i := range infos {
		if i == len(infos)-1 {
			infos[i].EndTime = time.Now()
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

	// Query messages from each relevant database
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
			// Build query conditions
			conditions := []string{"Sequence >= ? AND Sequence <= ?"}
			args := []interface{}{startTime.Unix() * 1000, endTime.Unix() * 1000}

			// Add talker condition
			talkerID, ok := dbInfo.TalkerMap[talkerItem]
			if ok {
				conditions = append(conditions, "TalkerId = ?")
				args = append(args, talkerID)
			} else {
				conditions = append(conditions, "StrTalker = ?")
				args = append(args, talkerItem)
			}

			query := fmt.Sprintf(`
				SELECT MsgSvrID, Sequence, CreateTime, StrTalker, IsSender, 
					Type, SubType, StrContent, CompressContent, BytesExtra
				FROM MSG 
				WHERE %s 
				ORDER BY Sequence ASC
			`, strings.Join(conditions, " AND "))

			// Execute query
			rows, err := db.QueryContext(ctx, query, args...)
			if err != nil {
				// If table does not exist, skip this talker
				if strings.Contains(err.Error(), "no such table") {
					continue
				}
				log.Err(err).Msgf("failed to query messages from database %s", dbInfo.FilePath)
				continue
			}

			// Process query results, filter while reading
			for rows.Next() {
				var msg model.MessageV3
				var compressContent []byte
				var bytesExtra []byte

				err := rows.Scan(
					&msg.MsgSvrID,
					&msg.Sequence,
					&msg.CreateTime,
					&msg.StrTalker,
					&msg.IsSender,
					&msg.Type,
					&msg.SubType,
					&msg.StrContent,
					&compressContent,
					&bytesExtra,
				)
				if err != nil {
					rows.Close()
					return nil, errors.ScanRowFailed(err)
				}
				msg.CompressContent = compressContent
				msg.BytesExtra = bytesExtra

				// Convert message to standard format
				message := msg.Wrap()

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

// GetContacts implements the method to get contact info
func (ds *DataSource) GetContacts(ctx context.Context, key string, limit, offset int) ([]*model.Contact, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT UserName, Alias, Remark, NickName, Reserved1 FROM Contact 
                WHERE UserName = ? OR Alias = ? OR Remark = ? OR NickName = ?`
		args = []interface{}{key, key, key, key}
	} else {
		// Query all contacts
		query = `SELECT UserName, Alias, Remark, NickName, Reserved1 FROM Contact`
	}

	// Add sorting and pagination
	query += ` ORDER BY UserName`
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
		var contactV3 model.ContactV3
		err := rows.Scan(
			&contactV3.UserName,
			&contactV3.Alias,
			&contactV3.Remark,
			&contactV3.NickName,
			&contactV3.Reserved1,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		contacts = append(contacts, contactV3.Wrap())
	}

	return contacts, nil
}

// GetChatRooms implements the method to get chat room info
func (ds *DataSource) GetChatRooms(ctx context.Context, key string, limit, offset int) ([]*model.ChatRoom, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT ChatRoomName, Reserved2, RoomData FROM ChatRoom WHERE ChatRoomName = ?`
		args = []interface{}{key}

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

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV3 model.ChatRoomV3
			err := rows.Scan(
				&chatRoomV3.ChatRoomName,
				&chatRoomV3.Reserved2,
				&chatRoomV3.RoomData,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV3.Wrap())
		}

		// If chat room not found, try looking up via contacts
		if len(chatRooms) == 0 {
			contacts, err := ds.GetContacts(ctx, key, 1, 0)
			if err == nil && len(contacts) > 0 && strings.HasSuffix(contacts[0].UserName, "@chatroom") {
				// Try again to find chat room by username
				rows, err := db.QueryContext(ctx,
					`SELECT ChatRoomName, Reserved2, RoomData FROM ChatRoom WHERE ChatRoomName = ?`,
					contacts[0].UserName)

				if err != nil {
					return nil, errors.QueryFailed(query, err)
				}
				defer rows.Close()

				for rows.Next() {
					var chatRoomV3 model.ChatRoomV3
					err := rows.Scan(
						&chatRoomV3.ChatRoomName,
						&chatRoomV3.Reserved2,
						&chatRoomV3.RoomData,
					)

					if err != nil {
						return nil, errors.ScanRowFailed(err)
					}

					chatRooms = append(chatRooms, chatRoomV3.Wrap())
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
		query = `SELECT ChatRoomName, Reserved2, RoomData FROM ChatRoom`

		// Add sorting and pagination
		query += ` ORDER BY ChatRoomName`
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

		chatRooms := []*model.ChatRoom{}
		for rows.Next() {
			var chatRoomV3 model.ChatRoomV3
			err := rows.Scan(
				&chatRoomV3.ChatRoomName,
				&chatRoomV3.Reserved2,
				&chatRoomV3.RoomData,
			)

			if err != nil {
				return nil, errors.ScanRowFailed(err)
			}

			chatRooms = append(chatRooms, chatRoomV3.Wrap())
		}

		return chatRooms, nil
	}
}

// GetSessions implements the method to get session info
func (ds *DataSource) GetSessions(ctx context.Context, key string, limit, offset int) ([]*model.Session, error) {
	var query string
	var args []interface{}

	if key != "" {
		// Query by keyword
		query = `SELECT strUsrName, nOrder, strNickName, strContent, nTime 
                FROM Session 
                WHERE strUsrName = ? OR strNickName = ?
                ORDER BY nOrder DESC`
		args = []interface{}{key, key}
	} else {
		// Query all sessions
		query = `SELECT strUsrName, nOrder, strNickName, strContent, nTime 
                FROM Session 
                ORDER BY nOrder DESC`
	}

	// Add pagination
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

	sessions := []*model.Session{}
	for rows.Next() {
		var sessionV3 model.SessionV3
		err := rows.Scan(
			&sessionV3.StrUsrName,
			&sessionV3.NOrder,
			&sessionV3.StrNickName,
			&sessionV3.StrContent,
			&sessionV3.NTime,
		)

		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}

		sessions = append(sessions, sessionV3.Wrap())
	}

	return sessions, nil
}

func (ds *DataSource) GetMedia(ctx context.Context, _type string, key string) (*model.Media, error) {
	if key == "" {
		return nil, errors.ErrKeyEmpty
	}

	if _type == "voice" {
		return ds.GetVoice(ctx, key)
	}

	md5key, err := hex.DecodeString(key)
	if err != nil {
		return nil, errors.DecodeKeyFailed(err)
	}

	var dbType string
	var table1, table2 string

	switch _type {
	case "image":
		dbType = Image
		table1 = "HardLinkImageAttribute"
		table2 = "HardLinkImageID"
	case "video":
		dbType = Video
		table1 = "HardLinkVideoAttribute"
		table2 = "HardLinkVideoID"
	case "file":
		dbType = File
		table1 = "HardLinkFileAttribute"
		table2 = "HardLinkFileID"
	default:
		return nil, errors.MediaTypeUnsupported(_type)
	}

	db, err := ds.dbm.GetDB(dbType)
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`
        SELECT 
            a.FileName,
            a.ModifyTime,
            IFNULL(d1.Dir,"") AS Dir1,
            IFNULL(d2.Dir,"") AS Dir2
        FROM 
            %s a
        LEFT JOIN 
            %s d1 ON a.DirID1 = d1.DirId
        LEFT JOIN 
            %s d2 ON a.DirID2 = d2.DirId
        WHERE 
            a.Md5 = ?
    `, table1, table2, table2)
	args := []interface{}{md5key}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, errors.QueryFailed(query, err)
	}
	defer rows.Close()

	var media *model.Media
	for rows.Next() {
		var mediaV3 model.MediaV3
		err := rows.Scan(
			&mediaV3.Name,
			&mediaV3.ModifyTime,
			&mediaV3.Dir1,
			&mediaV3.Dir2,
		)
		if err != nil {
			return nil, errors.ScanRowFailed(err)
		}
		mediaV3.Type = _type
		mediaV3.Key = key
		media = mediaV3.Wrap()
	}

	if media == nil {
		return nil, errors.ErrMediaNotFound
	}

	return media, nil
}

func (ds *DataSource) GetVoice(ctx context.Context, key string) (*model.Media, error) {
	if key == "" {
		return nil, errors.ErrKeyEmpty
	}

	query := `
	SELECT Buf
	FROM Media
	WHERE Reserved0 = ? 
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

// Close implements DataSource interface's Close method
func (ds *DataSource) Close() error {
	return ds.dbm.Close()
}

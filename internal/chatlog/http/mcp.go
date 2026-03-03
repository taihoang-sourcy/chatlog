package http

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"

	"github.com/sjzar/chatlog/internal/chatlog/conf"
	"github.com/sjzar/chatlog/internal/errors"
	"github.com/sjzar/chatlog/pkg/util"
	"github.com/sjzar/chatlog/pkg/version"
)

func (s *Service) initMCPServer() {
	s.mcpServer = server.NewMCPServer(conf.AppName, version.Version)
	s.mcpServer.AddTool(ContactTool, s.handleMCPContact)
	s.mcpServer.AddTool(ChatRoomTool, s.handleMCPChatRoom)
	s.mcpServer.AddTool(RecentChatTool, s.handleMCPRecentChat)
	s.mcpServer.AddTool(ChatLogTool, s.handleMCPChatLog)
	s.mcpServer.AddTool(CurrentTimeTool, s.handleMCPCurrentTime)
	s.mcpSSEServer = server.NewSSEServer(s.mcpServer)
	s.mcpStreamableServer = server.NewStreamableHTTPServer(s.mcpServer)
}

var ContactTool = mcp.NewTool(
	"query_contact",
	mcp.WithDescription(`Query user's contact information. Can search by name, remark, or ID; returns matching contacts. Use when user asks about someone's contact, wants contact info, or needs to find a specific contact. Returns full contact list when param is empty.`),
	mcp.WithString("keyword", mcp.Description("Search keyword for contacts; can be name, remark, or ID.")),
)

var ChatRoomTool = mcp.NewTool(
	"query_chat_room",
	mcp.WithDescription(`Query chat rooms the user participates in. Can search by room name, room ID, or keyword; returns matching chat rooms. Use when user asks about chat rooms, wants room details, or needs to find a specific chat room.`),
	mcp.WithString("keyword", mcp.Description("Search keyword for chat rooms; can be room name, room ID, or description.")),
)

var RecentChatTool = mcp.NewTool(
	"query_recent_chat",
	mcp.WithDescription(`Query recent session list, including personal chats and chat rooms. Use when user wants recent chat history or to see recently contacted people/groups. No params needed; returns recent session list directly.`),
)

var ChatLogTool = mcp.NewTool(
	"query_chat_log",
	mcp.WithDescription(`Retrieve historical chat records. Supports precise queries by time, talker, sender, and keyword. Use when user needs to find specific info or understand past conversations with someone/group.

[MANDATORY MULTI-STEP QUERY PROCESS!]
When querying specific topics or specific sender messages, strictly follow this process; any deviation causes wrong results:

Step 1: Initial message location
- Use keyword param to find specific topics
- Use sender param to find specific sender's messages
- Use wide time range for initial query

Step 2: [MUST EXECUTE] Get context for each key result point
- Must execute independent query for each time point T1, T2, T3... from Step 1 (nearby times can merge into one query)
- Each independent query MUST remove keyword param
- Each independent query MUST remove sender param
- Each uses narrow range "T±15-30 minutes"
- Each independent query keeps only talker param

Step 3: [MUST EXECUTE] Synthesize all context
- Must wait for all Step 2 results before analysis
- Must consider all context before answering user

[STRICT RULES!]
- Forbidden: answering user with only Step 1 results
- Forbidden: using large time range in Step 2 to query all context at once
- Forbidden: skipping Step 2 or Step 3
- Must execute independent context query for each key result point

[EXAMPLES]
Correct: Step 1 chatlog(time="2023-04-01~2023-04-30", talker="WorkGroup", keyword="project progress")
Returns: messages on Apr 5, 12, 20. Step 2: query each date's 1hr window with talker only (no keyword). Step 3: synthesize.

Return format: "Nickname(ID) time\ncontent\nNickname(ID) time\ncontent"
For multiple talkers: "Nickname(ID)\n[TalkerName(Talker)] time\ncontent"

Tips:
1. Use correct time format for queries with hours/minutes
2. For "what did we chat 4-5pm today": time="2023-04-18/16:00~2023-04-18/17:00"
3. Use sender param when querying someone's messages in a chat room
4. Use keyword param when querying messages with specific keywords`),
	mcp.WithString("time", mcp.Description(`Time point or range. Format rules:

Single point:
- Day: "2023-04-18" or "20230418"
- Minute (must include / and :): "2023-04-18/14:30" or "20230418/14:30"

Time range (use ~ to separate):
- Date: "2023-04-01~2023-04-18"
- Same day: "2023-04-18/14:30~2023-04-18/15:45"

Important: Use / and : for hour/minute. Correct: "2023-04-18/16:30". Wrong: "2023-04-18 16:30"`), mcp.Required()),
	mcp.WithString("talker", mcp.Description(`Chat target (contact or group). Can use ID, nickname, or remark. Multiple: "a,b,c". In multi-step queries, this is the only param to keep.`), mcp.Required()),
	mcp.WithString("sender", mcp.Description(`Sender in chat room. Only for chat room queries. Multiple: "a,b". When querying specific sender: Step 1 use sender to locate; Step 2 remove sender, query context around each time point.`)),
	mcp.WithString("keyword", mcp.Description(`Keyword in content. Supports regex. When querying topic: Step 1 use keyword to locate; Step 2 remove keyword, query context around each time point.`)),
)

var CurrentTimeTool = mcp.NewTool(
	"current_time",
	mcp.WithDescription(`Get current system time in RFC3339 format (with local timezone).
Use when: user asks "summarize today's chat", "what did we discuss this week", or relative time like "yesterday", "last week" - to establish reference point. Example: 2025-04-18T21:29:00+08:00. No input params needed.`),
)

type ContactRequest struct {
	Keyword string `json:"keyword"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

func (s *Service) handleMCPContact(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var req ContactRequest
	if err := request.BindArguments(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind arguments")
		log.Error().Interface("request", request.GetRawArguments()).Msg("Failed to bind arguments")
		return errors.ErrMCPTool(err), nil
	}

	list, err := s.db.GetContacts(req.Keyword, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get contacts")
		return errors.ErrMCPTool(err), nil
	}
	buf := &bytes.Buffer{}
	buf.WriteString("UserName,Alias,Remark,NickName\n")
	for _, contact := range list.Items {
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s\n", contact.UserName, contact.Alias, contact.Remark, contact.NickName))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: buf.String(),
			},
		},
	}, nil
}

type ChatRoomRequest struct {
	Keyword string `json:"keyword"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

func (s *Service) handleMCPChatRoom(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	var req ChatRoomRequest
	if err := request.BindArguments(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind arguments")
		log.Error().Interface("request", request.GetRawArguments()).Msg("Failed to bind arguments")
		return errors.ErrMCPTool(err), nil
	}

	list, err := s.db.GetChatRooms(req.Keyword, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get chat rooms")
		return errors.ErrMCPTool(err), nil
	}
	buf := &bytes.Buffer{}
	buf.WriteString("Name,Remark,NickName,Owner,UserCount\n")
	for _, chatRoom := range list.Items {
		buf.WriteString(fmt.Sprintf("%s,%s,%s,%s,%d\n", chatRoom.Name, chatRoom.Remark, chatRoom.NickName, chatRoom.Owner, len(chatRoom.Users)))
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: buf.String(),
			},
		},
	}, nil
}

type RecentChatRequest struct {
	Keyword string `json:"keyword"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
}

func (s *Service) handleMCPRecentChat(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	var req RecentChatRequest
	if err := request.BindArguments(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind arguments")
		log.Error().Interface("request", request.GetRawArguments()).Msg("Failed to bind arguments")
		return errors.ErrMCPTool(err), nil
	}

	data, err := s.db.GetSessions(req.Keyword, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get sessions")
		return errors.ErrMCPTool(err), nil
	}
	buf := &bytes.Buffer{}
	for _, session := range data.Items {
		buf.WriteString(session.PlainText(120))
		buf.WriteString("\n")
	}
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: buf.String(),
			},
		},
	}, nil
}

type ChatLogRequest struct {
	Time    string `form:"time"`
	Talker  string `form:"talker"`
	Sender  string `form:"sender"`
	Keyword string `form:"keyword"`
	Limit   int    `form:"limit"`
	Offset  int    `form:"offset"`
	Format  string `form:"format"`
}

func (s *Service) handleMCPChatLog(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	var req ChatLogRequest
	if err := request.BindArguments(&req); err != nil {
		log.Error().Err(err).Msg("Failed to bind arguments")
		log.Error().Interface("request", request.GetRawArguments()).Msg("Failed to bind arguments")
		return errors.ErrMCPTool(err), nil
	}

	var err error
	start, end, ok := util.TimeRangeOf(req.Time)
	if !ok {
		log.Error().Err(err).Msg("Failed to get messages")
		return errors.ErrMCPTool(err), nil
	}
	if req.Limit < 0 {
		req.Limit = 0
	}

	if req.Offset < 0 {
		req.Offset = 0
	}

	messages, err := s.db.GetMessages(start, end, req.Talker, req.Sender, req.Keyword, req.Limit, req.Offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get messages")
		return errors.ErrMCPTool(err), nil
	}

	buf := &bytes.Buffer{}
	if len(messages) == 0 {
		buf.WriteString("No chat records found matching the query criteria")
	}
	for _, m := range messages {
		buf.WriteString(m.PlainText(strings.Contains(req.Talker, ","), util.PerfectTimeFormat(start, end), ""))
		buf.WriteString("\n")
	}

	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: buf.String(),
			},
		},
	}, nil
}

func (s *Service) handleMCPCurrentTime(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.TextContent{
				Type: "text",
				Text: time.Now().Local().Format(time.RFC3339),
			},
		},
	}, nil
}

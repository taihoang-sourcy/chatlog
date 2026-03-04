# Chatlog

WeChat chat history extraction and management tool. Extracts, decrypts, and serves chat data from local WeChat databases (macOS/Windows, WeChat 3.x/4.x). Provides a TUI, HTTP API, MCP server, PostgreSQL sync, and webhook support.

## Tech Stack

- **Language**: Go 1.24 (CGO required for SQLite3)
- **CLI**: cobra, viper
- **TUI**: tview, tcell
- **HTTP**: gin
- **Databases**: go-sqlite3 (WeChat DBs), pgx/v5 (PostgreSQL sync)
- **MCP**: mark3labs/mcp-go
- **Logging**: zerolog
- **Crypto**: golang.org/x/crypto (AES decryption)
- **Protobuf**: google.golang.org/protobuf

## Project Structure

```
cmd/chatlog/          CLI commands (root, key, decrypt, server, sync, config)
internal/
  chatlog/            Core app orchestration
    app.go            TUI application (tview)
    manager.go        Service orchestrator (start/stop/switch)
    ctx/              Shared application state (thread-safe)
    conf/             Configuration types (TUI + server modes)
    database/         DB service wrapper with state machine
    http/             HTTP API + MCP server (gin)
    wechat/           WeChat service (decrypt, auto-decrypt)
    webhook/          Webhook notification service
  wechat/             WeChat integration (process detection, key extraction)
    process/          Platform-specific process detection (darwin/, windows/)
    key/              Platform-specific key extraction (darwin/, windows/)
    decrypt/          Platform-specific decryption (darwin/, windows/)
  wechatdb/           WeChat database abstraction
    repository/       Data access with in-memory caching
    datasource/       Platform/version-specific SQL (v4/, darwinv3/, windowsv3/)
  model/              Data models (message, contact, session, chatroom, media)
  postgres/           PostgreSQL connection, schema, and sync
  mcp/                MCP protocol implementation
  errors/             Custom error types
pkg/
  filemonitor/        File system watcher (fsnotify)
  config/             Config manager (viper wrapper)
  util/               Utilities (time, strings, os)
    dat2img/          Image decryption (.dat → image)
    silk/             SILK audio → MP3 conversion
    lz4/, zstd/       Compression utilities
docs/                 User-facing documentation
```

## Build & Test

```sh
make build         # Build for current platform (CGO_ENABLED=1)
make test          # Run tests with coverage
make lint          # golangci-lint
make tidy          # go mod tidy
make all           # clean + lint + tidy + test + build
make crossbuild    # Build for darwin/linux/windows × amd64/arm64
```

Binary output: `bin/chatlog`

## Running

```sh
./bin/chatlog                          # TUI mode (interactive)
./bin/chatlog key                      # Extract encryption keys from running WeChat
./bin/chatlog decrypt                  # Decrypt database files
./bin/chatlog server                   # Start HTTP/MCP server (headless)
./bin/chatlog sync                     # Sync data to PostgreSQL
```

## Configuration

- **TUI mode**: `~/.chatlog/chatlog.json`
- **Server mode**: env vars with `CHATLOG_` prefix (e.g. `CHATLOG_HTTP_ADDR`, `CHATLOG_DATA_KEY`)
- See `internal/chatlog/conf/` for config structures

## HTTP API

Base: `http://127.0.0.1:5030` (default)

- `GET /health` — Health check
- `GET /api/v1/chatlog?talker=&time=&format=json|csv` — Messages
- `GET /api/v1/contact?keyword=&format=json` — Contacts
- `GET /api/v1/chatroom?keyword=&format=json` — Chat rooms
- `GET /api/v1/session?format=json` — Sessions
- `GET /image/*key`, `/video/*key`, `/voice/*key`, `/file/*key` — Media
- `ANY /mcp` — MCP (Streamable HTTP)
- `ANY /sse`, `/message` — MCP (SSE transport)

## Key Conventions

- Platform-specific code lives in `darwin/` and `windows/` subdirectories, never mixed
- Version-specific code uses `v3`/`v4` suffixes or subdirectories
- Services accept narrow `Config` interfaces, not concrete types — see `.claude/docs/architectural_patterns.md`
- Services follow Start/Stop lifecycle; Manager orchestrates in dependency order
- Error handling uses `internal/errors` package with helper functions
- Comments in source are a mix of Chinese and English

## Additional Documentation

Check these when working on related areas:

- `.claude/docs/architectural_patterns.md` — DI pattern, service lifecycle, state machine, DataSource strategy, repository caching, callback chain, HTTP conventions, platform code organization
- `docs/postgres_queries.md` — PostgreSQL query examples for analytics
- `docs/mcp.md` — MCP integration guide and tool definitions
- `docs/docker.md` — Docker deployment instructions

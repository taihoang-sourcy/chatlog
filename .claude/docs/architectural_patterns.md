# Architectural Patterns

## Interface-Based Dependency Injection

Services accept narrow `Config` interfaces rather than concrete types. Each service defines only the config methods it needs.

Examples:
- `internal/chatlog/database/service.go:31-36` — DB service `Config` interface (4 methods)
- `internal/chatlog/http/service.go:28-31` — HTTP service `Config` interface (2 methods)
- Both `ctx.Context` and `conf.ServerConfig` satisfy these interfaces

This allows the same service to work in TUI mode (with `ctx.Context`) and server/CLI mode (with `conf.ServerConfig`).

## Service Lifecycle (Start/Stop)

Services follow a consistent lifecycle: construct with `NewService(conf)`, then `Start()` / `Stop()`.

- `internal/chatlog/manager.go:101-123` — Services start in dependency order (DB first, then HTTP)
- `internal/chatlog/manager.go:136-154` — Services stop in reverse dependency order (HTTP first, then DB)
- `internal/chatlog/http/service.go:61-78` — HTTP `Start()` launches goroutine, returns immediately
- `internal/chatlog/http/service.go:91-108` — `Stop()` uses `context.WithTimeout` for graceful shutdown

## State Machine

The database service tracks state transitions to coordinate initialization.

- `internal/chatlog/database/service.go:15-19` — States: `Init → Decrypting → Ready | Error`
- Used by HTTP middleware to reject requests when DB isn't ready (`checkDBStateMiddleware`)

## DataSource Abstraction (Strategy Pattern)

Platform/version-specific database access is abstracted behind a `DataSource` interface.

- `internal/wechatdb/datasource/datasource.go:16-37` — Interface with 6 methods (messages, contacts, chatrooms, sessions, media, callbacks)
- `internal/wechatdb/datasource/datasource.go:39-52` — Factory function dispatches on `(platform, version)` tuple
- Implementations: `v4/` (shared), `darwinv3/`, `windowsv3/`

Same pattern appears for key extraction and decryption:
- `internal/wechat/key/extractor.go` — Key extractor interface
- `internal/wechat/decrypt/decryptor.go` — Decryptor interface

## Repository with In-Memory Cache

The repository layer wraps DataSource with caches for contacts and chatrooms.

- `internal/wechatdb/repository/repository.go:15-39` — Multiple lookup maps (by username, alias, remark, nickname)
- `internal/wechatdb/repository/repository.go:42-71` — Cache initialized on construction; auto-refreshed via file callbacks
- `internal/wechatdb/repository/repository.go:88-106` — fsnotify callbacks re-initialize caches on file changes

## Observer/Callback Pattern

File system changes propagate through a callback chain:

1. `pkg/filemonitor/filemonitor.go` — Watches directories with `fsnotify`, fires callbacks on file changes
2. DataSource registers file groups with the monitor
3. Repository registers callbacks on DataSource for cache invalidation
4. Webhook service registers callbacks on DataSource for message notifications

Example chain: file change → filemonitor → datasource callback → repository cache refresh

## Manager as Orchestrator

`Manager` (`internal/chatlog/manager.go`) orchestrates all services without business logic. It:
- Owns `Context`, `WeChat`, `Database`, `HTTP` services
- Coordinates start/stop sequences
- Handles account switching (stop services → switch context → restart)
- Provides command-mode entry points (`CommandKey`, `CommandDecrypt`, `CommandHTTPServer`, `CommandSync`)

## Context as Shared State

`ctx.Context` (`internal/chatlog/ctx/context.go`) is the single source of truth for application state:
- Thread-safe with `sync.RWMutex`
- Holds current account, keys, directories, service states
- Persists state changes to config file via `UpdateConfig()`
- Satisfies multiple service `Config` interfaces

## HTTP API Conventions

- Routes grouped by concern: base, media, API, MCP (`internal/chatlog/http/route.go:26-31`)
- API endpoints under `/api/v1/` with DB state middleware
- Query params use anonymous structs with `form` tags (`route.go:92-100`)
- Multi-format response: `format=json|csv|plain` (default: plain text)
- Error handling via `errors.Err(c, err)` helper, not inline responses
- Media endpoints redirect to `/data/*path` which serves files from `dataDir`

## MCP Integration

Two MCP transport modes supported:
- Streamable HTTP at `/mcp`
- SSE (Server-Sent Events) at `/sse` + `/message`
- Both backed by the same `MCPServer` instance (`internal/chatlog/http/service.go:23-25`)

## Platform-Specific Code Organization

Platform differences are isolated in leaf packages, not scattered throughout:
- `internal/wechat/process/darwin/`, `internal/wechat/process/windows/` — Process detection
- `internal/wechat/key/darwin/`, `internal/wechat/key/windows/` — Key extraction
- `internal/wechat/decrypt/darwin/`, `internal/wechat/decrypt/windows/` — Decryption
- `internal/wechatdb/datasource/v4/`, `darwinv3/`, `windowsv3/` — SQL queries
- `internal/model/*_darwinv3.go`, `*_v3.go`, `*_v4.go` — Version-specific models

## Configuration Dual-Mode

Two config systems for two execution modes:
- **TUI mode**: `conf.TUIConfig` loaded from `~/.chatlog/chatlog.json` (`internal/chatlog/conf/tui.go`)
- **Server/CLI mode**: `conf.ServerConfig` loaded from env vars with `CHATLOG_` prefix via Viper (`internal/chatlog/conf/server.go`)
- `pkg/config/config.go` — Shared `Manager` wrapping Viper for read/write

## Batch Processing

Sync operations use batched iteration to handle large datasets:
- `internal/chatlog/manager.go:488` — `syncBatchSize = 5000`
- Iterates sessions, then messages per session with offset pagination
- Incremental sync using `lastSyncedAt` checkpoint timestamps

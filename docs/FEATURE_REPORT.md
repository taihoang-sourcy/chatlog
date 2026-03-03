# Chatlog Feature Report: Incremental Export & Multi-Account Support

**Date:** March 3, 2025  
**Scope:** Analysis of chatlog codebase capabilities

---

## 1. Are We Able to Do Incremental Export?

### Summary: **Partially — API supports it, but no built-in incremental export flow**

### Current Capabilities

#### ✅ What Exists

1. **Time-range queries**  
   The message API supports arbitrary `startTime` and `endTime` filters:

   - HTTP API: `GET /api/v1/chatlog` with `time` (e.g. `2023-01-01~2023-01-31`, `last-7d`)
   - Data layer: All datasources (`darwinv3`, `windowsv3`, `v4`) accept `startTime`/`endTime` for message queries

2. **Flexible time formats** (`pkg/util/time.go`)  
   The `TimeRangeOf()` function supports:

   - Explicit ranges: `2006-01-01~2006-01-31`, `2006-01-01 to 2006-01-31`
   - Relative ranges: `last-7d`, `last-30d`, `last-3m`, `last-1y`
   - Single days/months/years: `2023-01-01`, `2023-01`, `2023`
   - `all` for full history

3. **Query filters**  
   Messages can be filtered by talker, sender, keyword, limit, and offset.

4. **Export formats**  
   HTTP API supports `text`, `json`, and `csv` via the `format` parameter.

#### ❌ What Does Not Exist

1. **No export state tracking**  
   - No `lastExport` or similar per-chat/per-account
   - No `ProcessConfig.LastTime` usage for export
   - No checkpoint/watermark for “messages since last export”

2. **No incremental export feature**  
   - No “export only new messages” in the UI
   - No automatic appending to previously exported files
   - No state persisted between export runs

3. **File copy “incremental”** (`pkg/filecopy/filecopy.go`)  
   - The word “incremental” there refers to **file index updates**, not chat export
   - It is about avoiding concurrent access during temp file cleanup, not message export

### How to Do Incremental Export Today

**Manual approach:**

1. Call the chatlog API with an explicit time range.
2. Example: first export `2020-01-01~2024-12-31`, next export `2025-01-01~2025-03-03`.
3. Append or merge results on your side; chatlog does not track or append.

**Example API calls:**

```bash
# Initial export (example: all of 2024)
GET /api/v1/chatlog?time=2024-01-01~2024-12-31&talker=wxid_xxx&format=csv

# Incremental export (example: new messages since last run)
GET /api/v1/chatlog?time=2025-01-01~2025-03-03&talker=wxid_xxx&format=csv
```

The caller must store the last exported time and use it for the next query.

### Recommendations for Full Incremental Export

To support “incremental export” as a feature:

1. **Store export checkpoints**  
   - Per chat: `lastExportedAt` (and possibly last message ID)  
   - E.g. in `ProcessConfig` or a dedicated export state table/config

2. **New endpoint or parameter**  
   - Option A: `?time=incremental` that uses stored checkpoint  
   - Option B: New endpoint `/api/v1/chatlog/incremental`  
   - Option C: `?since=2025-01-01` (alias for “from last checkpoint”)

3. **Append mode**  
   - Option to append to an existing export file instead of overwriting.

---

## 2. Are We Able to Use Multi-Account?

### Summary: **Yes — multi-account is supported**

### Current Capabilities

#### ✅ What Exists

1. **Account switching menu**  
   - Main menu item: “Switch Account” (`internal/chatlog/app.go`).

2. **Two account sources**

   - **Running WeChat processes**  
     - Detected via `wechat.GetWeChatInstances()` / `wechat.GetAccounts()`
     - Lists all running WeChat instances (PID, version, data dir)
     - Switch by selecting a process

   - **History accounts**  
     - Stored in `conf.History` (from TUI config)
     - Each entry is a `ProcessConfig` with: `Account`, `Platform`, `Version`, `DataDir`, `DataKey`, `ImgKey`, `WorkDir`, etc.
     - Switch by selecting a previously used account (even if WeChat is not running)

3. **Context per account**  
   - `ctx.Current`: active WeChat instance
   - `ctx.History`: map of past accounts
   - `ctx.SwitchCurrent(info)` / `ctx.SwitchHistory(account)`

4. **Manager switching**  
   - `Manager.Switch(instance, historyAccount)` switches:
     - Decrypt state
     - HTTP service
     - Current account context
   - UI shows “Switching account...” and updates menu after switch

5. **Persistence**  
   - `last_account` stored in config
   - `history` array of account configs
   - `chatlog.json` in the account’s data dir for per-account config

6. **Default behavior**  
   - If multiple WeChat instances exist, the first one is auto-selected on startup (`Manager.Run`).

### How It Works

```
Switch Account Menu
├── WeChat Processes (running)
│   └── WeChat [PID] [Current] — Version: x.x.x Dir: /path
├── History Accounts
│   └── account_name [Current] — 版本: x.x.x 目录: /path
└── (If none) "No accounts to select"
```

- **Current account** is indicated by `[Current]` in the menu.
- Switching updates the active context; all API calls use the currently selected account.

### Limitations

1. **One account at a time**  
   - APIs (chatlog, contacts, etc.) always operate on the active account.  
   - No parallel export across multiple accounts in one request.

2. **Single HTTP server**  
   - One HTTP port per chatlog instance; no per-account HTTP endpoints.

3. **History vs live**  
   - History accounts use stored `DataDir` (e.g. from migration).  
   - They work even when WeChat is not running, as long as the data dir is valid.

---

## Appendix: Code References

| Feature         | Location |
|----------------|----------|
| Message API    | `internal/chatlog/http/route.go` (handleChatlog) |
| Time parsing   | `pkg/util/time.go` (TimeRangeOf) |
| DataSource     | `internal/wechatdb/datasource/datasource.go` |
| Switch Account | `internal/chatlog/app.go` (selectAccountSelected) |
| Context        | `internal/chatlog/ctx/context.go` |
| Manager.Switch | `internal/chatlog/manager.go` |
| History config | `internal/chatlog/conf/tui.go` (ProcessConfig, ParseHistory) |
| WeChat manager | `internal/wechat/manager.go` (GetAccounts) |

<div align="center">

![chatlog](https://github.com/user-attachments/assets/e085d3a2-e009-4463-b2fd-8bd7df2b50c3)

_Chat history tool to help you easily work with your own messaging data_

[![ImgMCP](https://cdn.imgmcp.com/imgmcp-logo-small.png)](https://imgmcp.com)

[![Go Report Card](https://goreportcard.com/badge/github.com/sjzar/chatlog)](https://goreportcard.com/report/github.com/sjzar/chatlog)
[![GoDoc](https://godoc.org/github.com/sjzar/chatlog?status.svg)](https://godoc.org/github.com/sjzar/chatlog)
[![GitHub release](https://img.shields.io/github/release/sjzar/chatlog.svg)](https://github.com/sjzar/chatlog/releases)
[![GitHub license](https://img.shields.io/github/license/sjzar/chatlog.svg)](https://github.com/sjzar/chatlog/blob/main/LICENSE)


</div>

## Features

- Extract chat data from local database files
- Support for Windows / macOS, compatible with WeChat 3.x / 4.x
- Retrieve data and image keys (Windows < 4.0.3.36 / macOS < 4.0.3.80)
- Decrypt images, voice, and other media; support wxgf format parsing
- Auto-decrypt databases with webhook callbacks for new messages
- Sync chat data to PostgreSQL for analytics, search, and AI pipeline integration
- Terminal UI interface with CLI and Docker deployment options
- HTTP API for querying chat history, contacts, group chats, and recent conversations
- MCP Streamable HTTP protocol for seamless integration with AI assistants
- Multi-account management with account switching

## Quick Start

### Basic Steps

1. **Install Chatlog**: [Download prebuilt binaries](#download-prebuilt-binaries) or [install from source](#install-from-source)
2. **Run**: Execute `chatlog` to launch the Terminal UI
3. **Decrypt data**: Select the `Decrypt data` menu item
4. **Start HTTP service**: Select the `Start HTTP service` menu item
5. **Access data**: Use the [HTTP API](#http-api) or [MCP integration](#mcp-integration) to query chat history

> 💡 **Tip**: If WeChat chat history on your computer is incomplete, you can [migrate data from your phone](#migrate-chat-history-from-phone).

### Quick Troubleshooting

- **macOS users**: You must [temporarily disable SIP](#macos-notes) before retrieving keys
- **Windows users**: Use [Windows Terminal](#windows-notes) if you see display issues
- **AI assistant integration**: See the [MCP integration guide](#mcp-integration)
- **Unable to get keys**: See [FAQ](https://github.com/sjzar/chatlog/issues/197)

## Installation

### Install from Source

```bash
go install github.com/sjzar/chatlog@latest
```

> 💡 **Note**: Some features require cgo; ensure you have a C compiler installed before building.

### Download Prebuilt Binaries

Visit the [Releases](https://github.com/sjzar/chatlog/releases) page to download prebuilt binaries for your system.

## Usage

### Terminal UI Mode

The simplest way to use Chatlog is through the Terminal UI:

```bash
chatlog
```

Controls:
- Use `↑` `↓` keys to navigate menu items
- Press `Enter` to confirm
- Press `Esc` to go back
- Press `Ctrl+C` to exit

### Command Line Mode

For command-line users:

```bash
# Retrieve WeChat data keys
chatlog key

# Decrypt database files
chatlog decrypt

# Start HTTP service
chatlog server

# Sync chat data to PostgreSQL
chatlog sync --postgres-url "postgres://user:pass@localhost:5432/chatlog"
```

### Docker Deployment

Docker runs in an isolated environment, so key retrieval is not supported. You must obtain keys beforehand. Typical use cases include NAS and similar deployments. See the [Docker deployment guide](docs/docker.md) for details.

**0. Get key information**

```shell
# Run chatlog on your host to get keys
$ chatlog key
Data Key: [c0163e***ac3dc6]
Image Key: [38636***653361]
```

**1. Pull the image**

Chatlog provides images from two registries:

**Docker Hub**:
```shell
docker pull sjzar/chatlog:latest
```

**GitHub Container Registry (ghcr)**:
```shell
docker pull ghcr.io/sjzar/chatlog:latest
```

> 💡 **Image URLs**:
> - Docker Hub: https://hub.docker.com/r/sjzar/chatlog
> - GitHub Container Registry: https://ghcr.io/sjzar/chatlog

**2. Run the container**

```shell
$ docker run -d \
  --name chatlog \
  -p 5030:5030 \
  -v /path/to/your/wechat/data:/app/data \
  sjzar/chatlog:latest
```

### Migrate Chat History from Phone

If WeChat chat history on your computer is incomplete, you can migrate data from your phone:

1. Open WeChat on your phone and go to `Me → Settings → General → Chat History Migration & Backup`
2. Select `Migration → Migrate to Computer` and follow the prompts
3. After migration completes, run `chatlog` again to retrieve keys and decrypt data

> This does not affect chat history on your phone; it only copies data to your computer.

## Platform-Specific Notes

### Windows Notes

If you see display issues (garbled text, etc.), run Chatlog using [Windows Terminal](https://github.com/microsoft/terminal).

### macOS Notes

macOS users must temporarily disable SIP (System Integrity Protection) before retrieving keys:

1. **Disable SIP**:
   ```shell
   # Boot into Recovery Mode
   # Intel Mac: Hold Command + R while restarting
   # Apple Silicon: Hold the power button while restarting
   
   # Open Terminal in Recovery Mode and run:
   csrutil disable
   
   # Restart
   ```

2. **Install required tools**:
   ```shell
   # Install Xcode Command Line Tools
   xcode-select --install
   ```

3. **After getting keys**: You can re-enable SIP (`csrutil enable`) without affecting normal use.

> Apple Silicon users: Ensure WeChat, chatlog, and Terminal are not running under Rosetta.

## HTTP API

With the HTTP service running (default `http://127.0.0.1:5030`), you can access data via:

### Chat History Query

```
GET /api/v1/chatlog?time=2023-01-01&talker=wxid_xxx
```

Parameters:
- `time`: Time range, format `YYYY-MM-DD` or `YYYY-MM-DD~YYYY-MM-DD`
- `talker`: Chat identifier (supports wxid, group ID, remark name, nickname, etc.)
- `limit`: Number of records to return
- `offset`: Pagination offset
- `format`: Output format: `json`, `csv`, or plain text

### Other API Endpoints

- **Contacts**: `GET /api/v1/contact`
- **Group chats**: `GET /api/v1/chatroom`
- **Sessions**: `GET /api/v1/session`

### Media Content

Media in chat history is served over HTTP:

- **Images**: `GET /image/<id>`
- **Videos**: `GET /video/<id>`
- **Files**: `GET /file/<id>`
- **Voice**: `GET /voice/<id>`
- **Media (generic)**: `GET /data/<data dir relative path>`

Image, video, and file requests return a 302 redirect to the media URL. Voice requests return the audio directly and transcode SILK to MP3 in real time. Media URLs are relative to the data directory; encrypted images are decrypted on the fly.

## PostgreSQL Storage

Chatlog can sync raw chat data to PostgreSQL for downstream use (analytics, search, AI pipelines, BI tools, etc.).

### Sync Command

```bash
# Use postgres.url from config (configure first with chatlog config postgres)
chatlog sync

# Or specify Postgres URL directly
chatlog sync --postgres-url "postgres://user:pass@localhost:5432/chatlog"

# Sync only a specific account
chatlog sync --account wxid_xxx

# Specify work dir (for single-directory mode)
chatlog sync --work-dir /path/to/decrypted --postgres-url "postgres://..."
```

Environment variable `CHATLOG_POSTGRES_URL` can be used as a fallback for the Postgres URL.

### Webhook Postgres Sink

When `postgres.url` is configured and auto-decrypt is enabled (HTTP server or TUI), new messages are **automatically written to PostgreSQL** in near real time. You do **not** need to configure webhook items—the Postgres sink runs as soon as `postgres.url` is set. If you also configure webhook items, new messages are sent to the HTTP URL in addition to being written to Postgres.

Example config (`chatlog.json` or `chatlog-server.json`):

```json
{
  "postgres": {
    "url": "postgres://user:pass@localhost:5432/chatlog?sslmode=disable"
  },
  "webhook": {
    "host": "localhost:5030",
    "items": [{"url": "http://localhost:8080/webhook", "talker": "wxid_123"}]
  }
}
```

In server mode, set `account` for the Postgres account identifier; otherwise `work_dir` is used.

### Schema and Sample Queries

See [docs/postgres_queries.md](docs/postgres_queries.md) for schema details and SQL examples.

---

## Webhook

Requires auto-decrypt. When matching new messages arrive, they are sent to a configured URL via HTTP POST. If `postgres.url` is set, new messages are also written to PostgreSQL (see [PostgreSQL Storage](#postgresql-storage) above).

> Latency: Local callback ~13 seconds; remote sync callback ~45 seconds.

#### 0. Callback Configuration

For TUI mode, add a `webhook` section to `$HOME/.chatlog/chatlog.json`  
(Windows: `%USERPROFILE%\.chatlog\chatlog.json`)

```json
{
  "history": [],
  "last_account": "wxuser_x",
  "postgres": {
    "url": "postgres://user:pass@localhost:5432/chatlog"
  },
  "webhook": {
    "host": "localhost:5030",                   # Host for image/file URLs in messages
    "items": [
      {
        "url": "http://localhost:8080/webhook", # Required: webhook URL (e.g. n8n)
        "talker": "wxid_123",                   # Required: chat or group to monitor
        "sender": "",                            # Optional: message sender filter
        "keyword": ""                            # Optional: keyword filter
      }
    ]
  }
}
```

With `postgres.url` configured, new messages are written to PostgreSQL automatically (even without webhook items). Webhook triggers also write to Postgres when items are configured.

For server mode, use the `CHATLOG_WEBHOOK` environment variable:

```shell
# Option 1
CHATLOG_WEBHOOK='{"host":"localhost:5030","items":[{"url":"http://localhost:8080/proxy","talker":"wxid_123","sender":"","keyword":""}]}'

# Option 2
CHATLOG_WEBHOOK_HOST="localhost:5030"
CHATLOG_WEBHOOK_ITEMS='[{"url":"http://localhost:8080/proxy","talker":"wxid_123","sender":"","keyword":""}]'
```

#### 1. Test the Webhook

Start chatlog with auto-decrypt enabled to test the callback:

```shell
POST /webhook HTTP/1.1
Host: localhost:8080
Accept-Encoding: gzip
Content-Length: 386
Content-Type: application/json
User-Agent: Go-http-client/1.1

Body:
{
  "keyword": "",
  "lastTime": "2025-08-27 00:00:00",
  "length": 1,
  "messages": [
    {
      "seq": 1756225000000,
      "time": "2025-08-27T00:00:00+08:00",
      "talker": "wxid_123",
      "talkerName": "",
      "isChatRoom": false,
      "sender": "wxid_123",
      "senderName": "Name",
      "isSelf": false,
      "type": 1,
      "subType": 0,
      "content": "Test message",
      "contents": {
        "host": "localhost:5030"
      }
    }
  ],
  "sender": "",
  "talker": "wxid_123"
}
```

## MCP Integration

Chatlog supports the MCP (Model Context Protocol) and integrates with MCP-compatible AI assistants. With the HTTP service running, access it via the Streamable HTTP endpoint:

```
GET /mcp
```

### Quick Integration

Chatlog works with various MCP-capable AI assistants:

- **ChatWise**: Native Streamable HTTP support — add `http://127.0.0.1:5030/mcp` in tool settings
- **Cherry Studio**: Native Streamable HTTP support — add `http://127.0.0.1:5030/mcp` in MCP server settings

For clients without native Streamable HTTP support, use [mcp-proxy](https://github.com/sparfenyuk/mcp-proxy) to forward requests:

- **Claude Desktop**: Via mcp-proxy; configure `claude_desktop_config.json`
- **Monica Code**: Via mcp-proxy; configure the VSCode plugin settings

### Full Integration Guide

See the [MCP integration guide](docs/mcp.md) for platform-specific setup and notes.

## Prompt Examples

We’ve collected prompt examples to help you make better use of Chatlog with AI assistants for querying and analyzing chat history.

See the [Prompt guide](docs/prompt.md) for detailed examples.

Contributions and ideas are welcome. Share your prompts and tips in [Discussions](https://github.com/sjzar/chatlog/discussions).

## Disclaimer

⚠️ **Important: Read and understand the full [Disclaimer](./DISCLAIMER.md) before using this project.**

This project is for learning, research, and personal legitimate use only. It must not be used for any unlawful purpose or to access data without authorization. By downloading, installing, or using this tool, you agree to the terms in the disclaimer and accept all risks and legal responsibility arising from its use.

### Summary (see full disclaimer)

- Use only on chat data you legally own or have authorization to access
- Do not use to obtain, view, or analyze others’ chat history without permission
- Developers are not liable for any loss resulting from use of this tool
- When using third-party LLM services, comply with their terms and privacy policies

**This project is free and open source. Any paid services offered in its name are unauthorized.**

## License

This project is open source under the [Apache-2.0 License](./LICENSE).

## Privacy Policy

This project does not collect any user data. All processing is performed locally on your device. For third-party services, refer to their privacy policies.

## Thanks

- [@0xlane](https://github.com/0xlane) for [wechat-dump-rs](https://github.com/0xlane/wechat-dump-rs)
- [@xaoyaoo](https://github.com/xaoyaoo) for [PyWxDump](https://github.com/xaoyaoo/PyWxDump)
- [@git-jiadong](https://github.com/git-jiadong) for [go-lame](https://github.com/git-jiadong/go-lame) and [go-silk](https://github.com/git-jiadong/go-silk)
- [Anthropic](https://www.anthropic.com/) for the [MCP](https://github.com/modelcontextprotocol) protocol
- All contributors to the Go open source ecosystem

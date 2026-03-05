# Run Decrypt + Sync Daily at 4pm

## 1. Script

`script/decrypt-and-sync.sh` runs decrypt then sync. From project root:

```bash
chmod +x script/decrypt-and-sync.sh
./script/decrypt-and-sync.sh
```

## 2. Crontab

```bash
crontab -e
```

Add (replace path with yours):

```
0 16 * * * /Users/YOUR_USERNAME/Workspaces/Sourcy/chatlog/script/decrypt-and-sync.sh
```

## Prerequisites

- PostgreSQL configured: `chatlog config postgres "postgres://..."`
- Keys in config: run `chatlog key` once

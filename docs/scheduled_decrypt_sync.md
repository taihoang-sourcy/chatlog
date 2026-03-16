# Run Decrypt + Sync Daily at 4pm

## Prerequisites

- PostgreSQL configured: `chatlog config postgres "postgres://..."`
- Keys in config: run `chatlog key` once

---

## macOS / Linux

### 1. Script

`script/decrypt-and-sync.sh` runs decrypt then sync. From project root:

```bash
chmod +x script/decrypt-and-sync.sh
./script/decrypt-and-sync.sh
```

### 2. Crontab

```bash
crontab -e
```

Add (replace path with yours):

```
0 16 * * * /Users/YOUR_USERNAME/Workspaces/Sourcy/chatlog/script/decrypt-and-sync.sh
```

---

## Windows

### 1. Script

`script/decrypt-and-sync.bat` runs decrypt then sync. Requires `chatlog.exe` in the project root (parent of `script/`).

From PowerShell (in project root):

```powershell
.\script\decrypt-and-sync.bat
```

### 2. Task Scheduler (daily at 4pm)

1. **Open Task Scheduler**  
   Windows key → search "Task Scheduler" → open

2. **Create Basic Task**  
   Action → Create Basic Task

3. **Name**  
   Example: `Chatlog Daily Decrypt and Sync`

4. **Trigger**  
   Daily → set time to **4:00 PM** → Next

5. **Action**  
   Start a program → Next

6. **Program/script**  
   Use the full path to the batch file, e.g.  
   `C:\Users\lisie\Downloads\chatlog\chatlog-main\script\decrypt-and-sync.bat`

7. **Start in (optional)**  
   Set to the project root so config paths resolve:  
   `C:\Users\lisie\Downloads\chatlog\chatlog-main`

8. **Finish**  
   Check “Open the Properties dialog” if you want to edit advanced settings.

**Optional:** In task Properties → General → ensure “Run whether user is logged on or not” is unchecked if you want it to use your user session and `%USERPROFILE%\.chatlog` config.

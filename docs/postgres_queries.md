# Sample Queries for Chatlog Postgres Database

Use `talker` as the conversation identifier (WeChat ID for 1:1 chats, group ID for group chats). `account_id` is your WeChat account.

---

## 1. All messages in a conversation (by talker ID)

```sql
SELECT 
  time,
  sender_name,
  sender,
  is_self,
  content,
  type,
  is_chat_room
FROM messages
WHERE account_id = (
  SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f'
)
  AND talker = 'wxid_abc123'   -- Replace with WeChat ID or group ID
ORDER BY time ASC;
```

---

## 2. All messages in a conversation (by account UUID)

```sql
SELECT 
  time,
  COALESCE(sender_name, sender) AS from_user,
  is_self,
  content
FROM messages
WHERE account_id = 'your-account-uuid-here'
  AND talker = 'wxid_abc123'
ORDER BY time ASC;
```

---

## 3. Find conversations and query one

```sql
-- List all conversations for an account
SELECT DISTINCT talker, talker_name, is_chat_room
FROM messages
WHERE account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
ORDER BY talker;

-- Get all messages from a specific conversation
SELECT 
  time AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Bangkok' AS local_time,
  CASE WHEN is_self THEN 'Me' ELSE COALESCE(sender_name, sender) END AS from_user,
  content
FROM messages
WHERE account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
  AND talker = 'wxid_xyz789'
ORDER BY time ASC;
```

---

## 4. Pagination

```sql
SELECT time, sender_name, sender, content
FROM messages
WHERE account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
  AND talker = 'wxid_abc123'
ORDER BY time ASC
LIMIT 100 OFFSET 0;
```

---

## 5. Time range filter

```sql
SELECT time, sender_name, content
FROM messages
WHERE account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
  AND talker = 'wxid_abc123'
  AND time >= '2025-01-01'
  AND time < '2025-02-01'
ORDER BY time ASC;
```

---

## 6. Text messages only

```sql
SELECT time, sender_name, content
FROM messages
WHERE account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
  AND talker = 'wxid_abc123'
  AND type = 1
ORDER BY time ASC;
```

Note: `type = 1` is text; other common types: 3 (image), 34 (voice), 43 (video).

---

## 7. API-like formatted output (single concatenated string)

Returns one string with the entire conversation in the same format.

```sql
SELECT 
talker, 
c.account_id,
to_char(max(m.time) AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Bangkok', 'HH24:MI:SS') as last_message,
string_agg(
  CASE 
    WHEN TRIM(COALESCE(NULLIF(TRIM(c.nick_name), ''), m.sender_name, c.remark, c.alias)) != ''
      THEN TRIM(COALESCE(NULLIF(TRIM(c.nick_name), ''), m.sender_name, c.remark, c.alias))
    ELSE m.sender 
  END 
  || ' ' 
  || to_char(m.time AT TIME ZONE 'UTC' AT TIME ZONE 'Asia/Bangkok', 'HH24:MI:SS')
  || E'\n'
  || COALESCE(NULLIF(TRIM(m.content), ''), m.contents->>'desc', '[Media]')
  || E'\n\n',
  ''
  ORDER BY m.time
) AS conversation
FROM messages m
JOIN contacts c ON c.account_id = m.account_id AND c.user_name = m.sender
-- WHERE m.account_id = (SELECT id FROM accounts WHERE account = 'wxid_pm0q1cfrdmmv12_712f')
  -- AND m.talker = 'wxid_0zwx7rx5gr4k22'
group by 1, 2;
```

Change `'Asia/Bangkok'` to your timezone if needed.

---

## Column reference

| Column       | Description                                  |
|-------------|-----------------------------------------------|
| `talker`    | Conversation identifier (WeChat ID or group)  |
| `account_id`| Which WeChat account                         |
| `sender`    | Sender WeChat ID                             |
| `sender_name` | Display name in the chat                   |
| `is_self`   | True if you sent the message                 |
| `content`   | Text content                                 |
| `contents`  | JSONB for media metadata (md5, url, etc.)   |

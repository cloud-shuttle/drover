# Conversation Search with FTS5

This guide covers how to search and query persisted conversations using SQLite's FTS5 (Full-Text Search) functionality.

## Overview

Drover persists conversation history to SQLite with FTS5 indexing enabled. This enables efficient full-text search across all conversation turns using:

- **BM25 ranking** for relevance scoring
- **Boolean queries** for complex filtering
- **Phrase searches** for exact matching
- **Prefix searches** for autocomplete-style queries

## Database Schema

### Tables

```sql
-- Conversations table
CREATE TABLE conversations (
    id TEXT PRIMARY KEY,
    task_id TEXT NOT NULL,
    worktree TEXT,
    status TEXT DEFAULT 'active',
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    turn_count INTEGER DEFAULT 0,
    total_tokens INTEGER DEFAULT 0,
    last_message_at INTEGER,
    compression_type TEXT DEFAULT 'none'
);

-- Conversation turns table
CREATE TABLE conversation_turns (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    turn_number INTEGER NOT NULL,
    role TEXT NOT NULL,
    content TEXT,
    tool_name TEXT,
    tool_input TEXT,
    tool_output TEXT,
    token_count INTEGER DEFAULT 0,
    created_at INTEGER NOT NULL,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

-- FTS5 virtual table for full-text search
CREATE VIRTUAL TABLE conversation_turns_fts USING fts5(
    content, tool_name, role,
    content=conversation_turns,
    content_rowid=rowid
);
```

## Basic Search Queries

### Search All Conversation Turns

```sql
SELECT
    ct.id,
    ct.conversation_id,
    ct.role,
    substr(ct.content, 1, 100) as excerpt,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE conversation_turns_fts MATCH 'search term'
ORDER BY fts.rank
LIMIT 10;
```

### Search Within a Specific Conversation

```sql
SELECT
    ct.turn_number,
    ct.role,
    substr(ct.content, 1, 100) as excerpt,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE ct.conversation_id = 'conv-abc123'
  AND conversation_turns_fts MATCH 'search term'
ORDER BY fts.rank;
```

### Search by Role

```sql
SELECT
    ct.turn_number,
    ct.role,
    substr(ct.content, 1, 100) as excerpt,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE ct.role = 'assistant'
  AND conversation_turns_fts MATCH 'implementation details'
ORDER BY fts.rank;
```

### Search Tool Uses

```sql
SELECT
    ct.turn_number,
    ct.tool_name,
    substr(ct.tool_input, 1, 100) as excerpt,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE conversation_turns_fts MATCH 'Edit tool:file.go'
ORDER BY fts.rank;
```

## FTS5 Query Syntax

### Simple Term Search

```sql
-- Match any word
WHERE conversation_turns_fts MATCH 'database'

-- Match multiple words (OR logic)
WHERE conversation_turns_fts MATCH 'database migration'

-- Match all words (AND logic - requires phrase)
WHERE conversation_turns_fts MATCH '"database migration"'
```

### Boolean Operators

```sql
-- AND: both terms must be present
WHERE conversation_turns_fts MATCH 'database AND migration'

-- OR: either term can be present
WHERE conversation_turns_fts MATCH 'database OR schema'

-- NOT: exclude term
WHERE conversation_turns_fts MATCH 'migration NOT rollback'

-- Complex boolean
WHERE conversation_turns_fts MATCH '(database OR schema) AND migration NOT rollback'
```

### Phrase Searches

```sql
-- Exact phrase match
WHERE conversation_turns_fts MATCH '"create table conversations"'

-- Phrase with proximity (within 10 words)
WHERE conversation_turns_fts MATCH 'NEAR("create table" conversations, 10)'

-- Ordered proximity
WHERE conversation_turns_fts MATCH '"create table" NOT "drop table"'
```

### Prefix Searches

```sql
-- Autocomplete-style prefix match
WHERE conversation_turns_fts MATCH 'migr*'

-- Multiple prefixes
WHERE conversation_turns_fts MATCH 'migr* schema*'
```

### Column-Specific Searches

```sql
-- Search only content column
WHERE conversation_turns_fts MATCH 'content:database'

-- Search only tool_name column
WHERE conversation_turns_fts MATCH 'tool_name:Edit'

-- Search specific columns with boolean
WHERE conversation_turns_fts MATCH 'content:database AND tool_name:Read'
```

## Practical Examples

### Find Recent Discussion About a Feature

```sql
SELECT
    c.task_id,
    ct.turn_number,
    ct.role,
    substr(ct.content, 1, 150) as excerpt,
    datetime(ct.created_at, 'unixepoch') as created,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
INNER JOIN conversations c ON c.id = ct.conversation_id
WHERE conversation_turns_fts MATCH 'conversation persistence'
  AND ct.created_at > strftime('%s', 'now', '-7 days')
ORDER BY fts.rank, ct.created_at DESC;
```

### Find All Tool Uses on a File

```sql
SELECT
    ct.turn_number,
    ct.tool_name,
    substr(ct.tool_input, 1, 100) as input,
    datetime(ct.created_at, 'unixepoch') as created
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE ct.conversation_id = 'conv-abc123'
  AND conversation_turns_fts MATCH 'tool_name:Edit AND "internal/db/db.go"'
ORDER BY ct.turn_number;
```

### Find Assistant Responses About Errors

```sql
SELECT
    ct.turn_number,
    substr(ct.content, 1, 200) as response,
    fts.rank
FROM conversation_turns_fts fts
INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid
WHERE ct.role = 'assistant'
  AND conversation_turns_fts MATCH 'error OR failed OR "cannot"'
ORDER BY fts.rank
LIMIT 20;
```

### Find Conversations by Task

```sql
SELECT
    c.id as conversation_id,
    c.task_id,
    c.status,
    c.turn_count,
    c.total_tokens,
    substr(ct.content, 1, 150) as first_message
FROM conversations c
INNER JOIN conversation_turns ct ON ct.conversation_id = c.id
WHERE c.task_id = 'task-abc123'
  AND ct.turn_number = 1;
```

## Command Line Usage

### Using sqlite3 CLI

```bash
# Basic search
sqlite3 .drover/drover.db \
  "SELECT ct.role, substr(ct.content, 1, 100) \
   FROM conversation_turns_fts fts \
   INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid \
   WHERE conversation_turns_fts MATCH 'search term' \
   LIMIT 10;"

# Search with context
sqlite3 -header -column .drover/drover.db \
  "SELECT c.task_id, ct.role, substr(ct.content, 1, 80) \
   FROM conversation_turns_fts fts \
   INNER JOIN conversation_turns ct ON ct.rowid = fts.rowid \
   INNER JOIN conversations c ON c.id = ct.conversation_id \
   WHERE conversation_turns_fts MATCH 'database' \
   ORDER BY fts.rank \
   LIMIT 10;"
```

### Using Go API

```go
import "github.com/cloud-shuttle/drover/internal/conversation"

// Get conversation store
store := db.GetConversationStore()

// Search conversation turns
results, err := store.SearchTurns(ctx, "database migration", 10)
if err != nil {
    log.Fatal(err)
}

for _, r := range results {
    fmt.Printf("[%s] %s\n", r.Turn.Role, r.Turn.Content)
}

// Build context with search
ctx, err := store.BuildContext(ctx, taskID, &conversation.BuildContextOptions{
    MaxTokens: 100000,
    SearchQuery: "recent changes",
})
```

## Performance Tips

1. **Use specific terms** - More specific queries return faster results
2. **Limit results** - Always use `LIMIT` to avoid large result sets
3. **Filter by conversation** - Add `conversation_id` filter when possible
4. **Use column filters** - Specify `content:term` instead of searching all columns
5. **Consider token count** - The `total_tokens` column helps estimate context size

## Schema Reference

### Conversation Status Values

| Status | Description |
|--------|-------------|
| `active` | Conversation is ongoing |
| `completed` | Conversation finished successfully |
| `archived` | Conversation archived for reference |

### Conversation Role Values

| Role | Description |
|------|-------------|
| `user` | User message |
| `assistant` | Assistant response |
| `system` | System message |
| `tool` | Tool use/result |

### Token Budget Management

When loading conversation context, use the token-aware loading:

```go
// Load turns that fit within token budget
turns, err := store.GetTurnsByTokenBudget(ctx, conversationID, 100000)

// Build context with token limit
context, err := store.BuildContext(ctx, taskID, &conversation.BuildContextOptions{
    MaxTokens: 100000,
    IncludeSystemMessages: true,
})
```

## Related Documentation

- [Conversation Store Implementation](../internal/conversation/store.go)
- [Conversation Types](../pkg/types/conversation.go)
- [Database Schema](../internal/db/db.go)
- [SQLite FTS5 Documentation](https://www.sqlite.org/fts5.html)

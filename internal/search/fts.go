// Package search provides full-text search using SQLite FTS5
package search

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/glebarez/go-sqlite"
)

// Searcher provides full-text search capabilities
type Searcher struct {
	db     *sql.DB
	logger *log.Logger
}

// Result represents a search result
type Result struct {
	Type       string `json:"type"`       // "task", "output", "epic"
	ID         string `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Match      string `json:"match"`      // Highlighted match snippet
	Rank       float64 `json:"rank"`      // FTS5 rank score
	ProjectID  string `json:"project_id,omitempty"`
	TaskID     string `json:"task_id,omitempty"`
	Timestamp  int64  `json:"timestamp"`
}

// Query represents a search query
type Query struct {
	Query      string `json:"query"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	TypeFilter string `json:"type_filter"` // "task", "output", "epic", or "all"
}

// NewSearcher creates a new searcher instance
func NewSearcher(dbPath string) (*Searcher, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	s := &Searcher{
		db:     db,
		logger: log.New(os.Stdout, "[search] ", log.LstdFlags),
	}

	return s, nil
}

// SetLogger sets the logger for the searcher
func (s *Searcher) SetLogger(logger *log.Logger) {
	s.logger = logger
}

// Close closes the database connection
func (s *Searcher) Close() error {
	return s.db.Close()
}

// InitSchema creates the FTS5 virtual tables
func (s *Searcher) InitSchema() error {
	s.logger.Printf("[search] initializing FTS5 schema")

	// Create FTS5 table for tasks
	if _, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(
			title,
			description,
			content,
			project_id,
			task_id,
			status,
			created_at
		);
	`); err != nil {
		return fmt.Errorf("create tasks_fts table: %w", err)
	}

	// Create FTS5 table for agent outputs
	if _, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS outputs_fts USING fts5(
			output_id,
			task_id,
			agent_type,
			output,
			project_id,
			created_at
		);
	`); err != nil {
		return fmt.Errorf("create outputs_fts table: %w", err)
	}

	// Create FTS5 table for epics
	if _, err := s.db.Exec(`
		CREATE VIRTUAL TABLE IF NOT EXISTS epics_fts USING fts5(
			title,
			description,
			project_id,
			status,
			created_at
		);
	`); err != nil {
		return fmt.Errorf("create epics_fts table: %w", err)
	}

	// Create triggers to keep FTS tables in sync
	if err := s.createTriggers(); err != nil {
		return err
	}

	s.logger.Printf("[search] FTS5 schema initialized")
	return nil
}

// createTriggers creates triggers to keep FTS tables synchronized with source tables
func (s *Searcher) createTriggers() error {
	// First, ensure source tables exist
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			title TEXT,
			description TEXT,
			status TEXT,
			created_at INTEGER,
			updated_at INTEGER
		);
	`); err != nil {
		return fmt.Errorf("create tasks table: %w", err)
	}

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_outputs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			output TEXT NOT NULL,
			project_id TEXT DEFAULT '',
			created_at INTEGER NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create agent_outputs table: %w", err)
	}

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS epics (
			id TEXT PRIMARY KEY,
			title TEXT,
			description TEXT,
			status TEXT,
			created_at INTEGER
		);
	`); err != nil {
		return fmt.Errorf("create epics table: %w", err)
	}

	// Triggers for tasks
	triggers := []string{
		// INSERT trigger
		`CREATE TRIGGER IF NOT EXISTS tasks_ai AFTER INSERT ON tasks BEGIN
			INSERT INTO tasks_fts(title, description, content, project_id, task_id, status, created_at)
			VALUES (NEW.title, NEW.description, '', '', NEW.id, NEW.status, NEW.created_at);
		END;`,
		// UPDATE trigger
		`CREATE TRIGGER IF NOT EXISTS tasks_au AFTER UPDATE ON tasks BEGIN
			UPDATE tasks_fts SET title = NEW.title, description = NEW.description, status = NEW.status
			WHERE task_id = NEW.id;
		END;`,
		// DELETE trigger
		`CREATE TRIGGER IF NOT EXISTS tasks_ad AFTER DELETE ON tasks BEGIN
			DELETE FROM tasks_fts WHERE task_id = OLD.id;
		END;`,
	}

	for _, trigger := range triggers {
		if _, err := s.db.Exec(trigger); err != nil {
			// Log warning but don't fail - triggers may already exist
			s.logger.Printf("[search] warning creating trigger: %v", err)
		}
	}

	return nil
}

// IndexTask indexes a task in the FTS5 table
func (s *Searcher) IndexTask(taskID, title, description string) error {
	// First ensure the tasks table has the task_id column
	if _, err := s.db.Exec(`ALTER TABLE tasks_fts ADD COLUMN task_id TEXT`); err != nil {
		// Column may already exist, ignore error
	}

	// Check if task exists
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE id = ?", taskID).Scan(&count); err != nil {
		return err
	}

	now := time.Now().Unix()
	if count == 0 {
		// Insert new task
		_, err := s.db.Exec(`
			INSERT INTO tasks(id, title, description, status, created_at, updated_at)
			VALUES (?, ?, ?, 'ready', ?, ?)
		`, taskID, title, description, now, now)
		return err
	}

	// Update existing task
	_, err := s.db.Exec(`
		UPDATE tasks SET title = ?, description = ?, updated_at = ?
		WHERE id = ?
	`, title, description, now, taskID)
	return err
}

// IndexOutput indexes an agent output in the FTS5 table
func (s *Searcher) IndexOutput(outputID, taskID, agentType, output string) error {
	// Check if agent_outputs table exists, create if not
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_outputs (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			agent_type TEXT NOT NULL,
			output TEXT NOT NULL,
			project_id TEXT DEFAULT '',
			created_at INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("create agent_outputs table: %w", err)
	}

	// Create FTS triggers for agent_outputs
	if _, err := s.db.Exec(`
		CREATE TRIGGER IF NOT EXISTS outputs_ai AFTER INSERT ON agent_outputs BEGIN
			INSERT INTO outputs_fts(output_id, task_id, agent_type, output, project_id, created_at)
			VALUES (NEW.id, NEW.task_id, NEW.agent_type, NEW.output, NEW.project_id, NEW.created_at);
		END;
	`); err != nil {
		return fmt.Errorf("create outputs trigger: %w", err)
	}

	// Insert or update output
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO agent_outputs(id, task_id, agent_type, output, created_at)
		VALUES (?, ?, ?, ?, ?)
	`, outputID, taskID, agentType, output, time.Now().Unix())

	return err
}

// Search performs a full-text search with the given query
func (s *Searcher) Search(query Query) ([]*Result, error) {
	// Parse and transform the query
	searchQuery, err := s.parseQuery(query.Query)
	if err != nil {
		return nil, err
	}

	// Determine limit and offset
	limit := query.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := query.Offset

	var results []*Result

	// Search based on type filter
	switch query.TypeFilter {
	case "task":
		results, err = s.searchTasks(searchQuery, limit, offset)
	case "output":
		results, err = s.searchOutputs(searchQuery, limit, offset)
	case "epic":
		results, err = s.searchEpics(searchQuery, limit, offset)
	default:
		results, err = s.searchAll(searchQuery, limit, offset)
	}

	if err != nil {
		return nil, err
	}

	return results, nil
}

// searchTasks searches in the tasks FTS table
func (s *Searcher) searchTasks(searchQuery string, limit, offset int) ([]*Result, error) {
	queryStr := `
		SELECT
			t.id,
			t.title,
			ts.description,
			snippet(tasks_fts, 2, '<mark>', '</mark>', '...', 30) as match,
			bm25(tasks_fts) as rank,
			'' as project_id,
			t.created_at
		FROM tasks_fts ts
		INNER JOIN tasks t ON ts.task_id = t.id
		WHERE tasks_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`

	return s.scanResults(queryStr, []interface{}{searchQuery, limit, offset}, "task")
}

// searchOutputs searches in the outputs FTS table
func (s *Searcher) searchOutputs(searchQuery string, limit, offset int) ([]*Result, error) {
	queryStr := `
		SELECT
			ao.id,
			ao.task_id,
			ao.agent_type,
			snippet(outputs_fts, 3, '<mark>', '</mark>', '...', 50) as match,
			bm25(outputs_fts) as rank,
			ao.project_id,
			ao.created_at
		FROM outputs_fts of
		INNER JOIN agent_outputs ao ON of.output_id = ao.id
		WHERE outputs_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`

	return s.scanResults(queryStr, []interface{}{searchQuery, limit, offset}, "output")
}

// searchEpics searches in the epics FTS table
func (s *Searcher) searchEpics(searchQuery string, limit, offset int) ([]*Result, error) {
	// For epics, we need to add a column to match on
	if _, err := s.db.Exec(`ALTER TABLE epics_fts ADD COLUMN epic_id`); err != nil {
		// Column may already exist, ignore error
	}

	queryStr := `
		SELECT
			e.id,
			e.title,
			es.description,
			snippet(epics_fts, 2, '<mark>', '</mark>', '...', 30) as match,
			bm25(epics_fts) as rank,
			'' as project_id,
			e.created_at
		FROM epics_fts es
		INNER JOIN epics e ON es.title = e.title AND es.description = e.description
		WHERE epics_fts MATCH ?
		ORDER BY rank
		LIMIT ? OFFSET ?
	`

	return s.scanResults(queryStr, []interface{}{searchQuery, limit, offset}, "epic")
}

// searchAll searches across all FTS tables
func (s *Searcher) searchAll(searchQuery string, limit, offset int) ([]*Result, error) {
	// Simplified search that searches each table separately
	queryStr := `
		SELECT * FROM (
			SELECT
				'task' as type,
				t.id as id,
				t.title as title,
				ts.description as content,
				snippet(tasks_fts, 2, '<mark>', '</mark>', '...', 30) as match,
				bm25(tasks_fts) as rank,
				'' as project_id,
				'' as task_id,
				t.created_at
			FROM tasks_fts ts
			INNER JOIN tasks t ON ts.task_id = t.id
			WHERE tasks_fts MATCH ?

			UNION ALL

			SELECT
				'output' as type,
				ao.id as id,
				ao.task_id as title,
				ao.output as content,
				snippet(outputs_fts, 3, '<mark>', '</mark>', '...', 50) as match,
				bm25(outputs_fts) as rank,
				ao.project_id as project_id,
				ao.task_id as task_id,
				ao.created_at
			FROM outputs_fts of
			INNER JOIN agent_outputs ao ON of.output_id = ao.id
			WHERE outputs_fts MATCH ?
		)
		ORDER BY rank
		LIMIT ? OFFSET ?
	`

	return s.scanResults(queryStr, []interface{}{searchQuery, searchQuery, limit, offset}, "all")
}

// scanResults scans query results into Result structs
func (s *Searcher) scanResults(query string, args []interface{}, resultType string) ([]*Result, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*Result

	for rows.Next() {
		var r Result
		var typeStr sql.NullString
		var id, title, content sql.NullString
		var match sql.NullString
		var rank sql.NullFloat64
		var projectID sql.NullString
		var taskID sql.NullString
		var createdAt sql.NullInt64

		// Scan based on result type
		if resultType == "all" {
			err = rows.Scan(&typeStr, &id, &title, &content, &match, &rank, &projectID, &taskID, &createdAt)
		} else if resultType == "output" {
			var agentType sql.NullString
			err = rows.Scan(&id, &taskID, &agentType, &match, &rank, &projectID, &createdAt)
			// For output results, title = agent_type
			if agentType.Valid {
				title = agentType
			}
			content = sql.NullString{} // Not set
		} else {
			err = rows.Scan(&id, &title, &content, &match, &rank, &projectID, &createdAt)
		}

		if err != nil {
			s.logger.Printf("[search] scan error: %v", err)
			continue
		}

		r = Result{
			Type:      resultType,
			ID:        id.String,
			Title:     title.String,
			Content:   content.String,
			Match:     match.String,
			Rank:      rank.Float64,
			ProjectID: projectID.String,
			Timestamp: createdAt.Int64,
		}

		if resultType == "output" {
			r.TaskID = taskID.String
		}

		results = append(results, &r)
	}

	return results, nil
}

// parseQuery transforms a natural language query into FTS5 query syntax
func (s *Searcher) parseQuery(query string) (string, error) {
	// Remove common words
	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "as": true, "is": true,
	}

	// Tokenize and clean
	words := strings.Fields(query)
	var searchTerms []string

	for _, word := range words {
		word = strings.ToLower(strings.Trim(word, "\"'"))

		// Skip stop words
		if stopWords[word] {
			continue
		}

		// Handle quoted phrases
		if strings.HasPrefix(word, "\"") {
			searchTerms = append(searchTerms, word)
			continue
		}

		// Add wildcard for prefix matching
		if len(word) > 3 {
			searchTerms = append(searchTerms, word+"*")
		} else {
			searchTerms = append(searchTerms, word)
		}
	}

	if len(searchTerms) == 0 {
		return "", fmt.Errorf("no valid search terms")
	}

	// Join with AND for better relevance
	return strings.Join(searchTerms, " AND "), nil
}

// GetMatch returns the full content for a search result
func (s *Searcher) GetMatch(resultType, id string) (string, error) {
	switch resultType {
	case "task":
		var title, description string
		err := s.db.QueryRow(`
			SELECT title, description FROM tasks WHERE id = ?
		`, id).Scan(&title, &description)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Title: %s\n\nDescription:\n%s", title, description), nil

	case "output":
		var output string
		err := s.db.QueryRow(`
			SELECT output FROM agent_outputs WHERE id = ?
		`, id).Scan(&output)
		if err != nil {
			return "", err
		}
		return output, nil

	case "epic":
		var title, description string
		err := s.db.QueryRow(`
			SELECT title, description FROM epics WHERE id = ?
		`, id).Scan(&title, &description)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Title: %s\n\nDescription:\n%s", title, description), nil

	default:
		return "", fmt.Errorf("unknown result type: %s", resultType)
	}
}

// AdvancedQuery supports boolean operators and phrase matching
type AdvancedQuery struct {
	// Boolean operators
	AND []string
	OR  []string
	NOT []string

	// Phrase matching
	Phrases []string

	// Prefix matching
	Prefixes []string

	// Filters
	MinDate int64
	MaxDate int64
}

// ParseAdvancedQuery parses a complex search query with operators
func (s *Searcher) ParseAdvancedQuery(queryStr string) (*AdvancedQuery, error) {
	q := &AdvancedQuery{}

	// Parse phrase queries "exact phrase"
	phraseRegex := regexp.MustCompile(`"([^"]+)"`)
	phrases := phraseRegex.FindAllStringSubmatch(queryStr, -1)
	for _, match := range phrases {
		if len(match) > 1 {
			q.Phrases = append(q.Phrases, match[1])
		}
	}

	// Remove phrases from query and continue parsing
	cleanQuery := phraseRegex.ReplaceAllString(queryStr, "")

	// Parse boolean operators
	if strings.Contains(cleanQuery, " OR ") {
		parts := strings.Split(cleanQuery, " OR ")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				q.OR = append(q.OR, trimmed)
			}
		}
	} else if strings.Contains(cleanQuery, " -") {
		// NOT operator (exclude terms)
		parts := strings.Fields(cleanQuery)
		var posTerms []string
		for _, part := range parts {
			if strings.HasPrefix(part, "-") {
				q.NOT = append(q.NOT, strings.TrimPrefix(part, "-"))
			} else {
				posTerms = append(posTerms, part)
			}
		}
		q.AND = posTerms
	} else {
		// Simple AND (default)
		q.AND = strings.Fields(cleanQuery)
	}

	return q, nil
}

// BuildFTS5Query builds an FTS5 query from an AdvancedQuery
func (s *Searcher) BuildFTS5Query(q *AdvancedQuery) string {
	var parts []string

	// Add phrases (exact match)
	for _, phrase := range q.Phrases {
		parts = append(parts, fmt.Sprintf("\"%s\"", phrase))
	}

	// Add AND terms
	for _, term := range q.AND {
		parts = append(parts, term+"*")
	}

	// Add OR terms
	if len(q.OR) > 0 {
		orParts := make([]string, len(q.OR))
		for i, term := range q.OR {
			orParts[i] = term + "*"
		}
		parts = append(parts, "("+strings.Join(orParts, " OR ")+")")
	}

	// Add NOT terms (exclude)
	for _, term := range q.NOT {
		parts = append(parts, "NOT "+term+"*")
	}

	if len(parts) == 0 {
		return "*"
	}

	return strings.Join(parts, " AND ")
}

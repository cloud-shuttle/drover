package search

import (
	"path/filepath"
	"testing"
)

// TestInitSchema tests FTS5 table creation
func TestInitSchema(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Verify tables exist
	tables := []string{"tasks_fts", "outputs_fts", "epics_fts"}
	for _, table := range tables {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
			t.Errorf("Table %s not created: %v", table, err)
		}
	}
}

// TestIndexTask tests task indexing
func TestIndexTask(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Index a task
	taskID := "task-1"
	title := "Implement user authentication"
	description := "Create login and signup endpoints with JWT tokens"

	if err := s.IndexTask(taskID, title, description); err != nil {
		t.Fatalf("IndexTask failed: %v", err)
	}

	// Verify it's in the database
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM tasks_fts").Scan(&count); err != nil {
		t.Fatalf("Query failed: %v", err)
	}

	if count != 1 {
		t.Errorf("Expected 1 task in FTS index, got %d", count)
	}
}

// TestSearch tests full-text search
func TestSearch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Index multiple tasks
	tasks := []struct {
		id, title, desc string
	}{
		{"task-1", "Implement user login", "Create login endpoint with email/password"},
		{"task-2", "Implement user signup", "Create signup endpoint with validation"},
		{"task-3", "Add JWT authentication", "Implement JWT tokens for secure auth"},
		{"task-4", "Create password reset", "Allow users to reset forgotten passwords"},
	}

	for _, task := range tasks {
		if err := s.IndexTask(task.id, task.title, task.desc); err != nil {
			t.Fatalf("IndexTask failed: %v", err)
		}
	}

	// Test search
	query := Query{
		Query:  "login",
		Limit:  10,
		Offset: 0,
	}

	results, err := s.Search(query)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results for 'login'")
	}

	// Verify "login" is in the title or content of results
	found := false
	for _, r := range results {
		if contains(r.Title, "login") || contains(r.Content, "login") {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected 'login' to appear in search results")
	}
}

// TestSearchPrefix tests prefix search
func TestSearchPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Index tasks
	tasks := []struct {
		id, title, desc string
	}{
		{"task-1", "Implement authentication", "Add auth system"},
		{"task-2", "Implement author", "Create author profiles"},
		{"task-3", "Create authorization", "Permission system"},
	}

	for _, task := range tasks {
		if err := s.IndexTask(task.id, task.title, task.desc); err != nil {
			t.Fatalf("IndexTask failed: %v", err)
		}
	}

	// Search for "auth" should match "authentication", "author", "authorization"
	query := Query{
		Query: "auth",
		Limit: 10,
	}

	results, err := s.Search(query)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) < 2 {
		t.Errorf("Expected at least 2 results for prefix search 'auth', got %d", len(results))
	}
}

// TestParseQuery tests query transformation
func TestParseQuery(t *testing.T) {
	s := &Searcher{}

	tests := []struct {
		input    string
		contains []string
	}{
		{"login", []string{"login*"}},
		{"login AND signup", []string{"login*", "signup*"}},
		{"the login", []string{"login*"}}, // Stop word removed
		{"login signup", []string{"login*", "signup*"}},
	}

	for _, tt := range tests {
		result, err := s.parseQuery(tt.input)
		if err != nil {
			t.Errorf("parseQuery(%q) failed: %v", tt.input, err)
			continue
		}

		for _, expected := range tt.contains {
			if !contains(result, expected) {
				t.Errorf("parseQuery(%q) = %q, expected to contain %s", tt.input, result, expected)
			}
		}
	}
}

// TestIndexOutput tests agent output indexing
func TestIndexOutput(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Index an agent output
	outputID := "output-1"
	taskID := "task-1"
	agentType := "claude"
	output := "I've implemented the login endpoint with JWT authentication. The code includes password hashing and session management."

	if err := s.IndexOutput(outputID, taskID, agentType, output); err != nil {
		t.Fatalf("IndexOutput failed: %v", err)
	}

	// Search for "JWT" in outputs
	query := Query{
		Query:      "JWT",
		Limit:      10,
		TypeFilter: "output",
	}

	results, err := s.Search(query)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("Expected search results for 'JWT' in outputs")
	}
}

// TestAdvancedQuery tests advanced query parsing
func TestAdvancedQuery(t *testing.T) {
	s := &Searcher{}

	// Test phrase matching
	q, err := s.ParseAdvancedQuery(`"exact phrase" AND term1`)
	if err != nil {
		t.Fatalf("ParseAdvancedQuery failed: %v", err)
	}

	if len(q.Phrases) != 1 {
		t.Errorf("Expected 1 phrase, got %d", len(q.Phrases))
	}

	if q.Phrases[0] != "exact phrase" {
		t.Errorf("Expected phrase 'exact phrase', got %s", q.Phrases[0])
	}

	// Test OR operator
	q, err = s.ParseAdvancedQuery("term1 OR term2")
	if err != nil {
		t.Fatalf("ParseAdvancedQuery failed: %v", err)
	}

	if len(q.OR) != 2 {
		t.Errorf("Expected 2 OR terms, got %d", len(q.OR))
	}

	// Test NOT operator
	q, err = s.ParseAdvancedQuery("term1 -exclude")
	if err != nil {
		t.Fatalf("ParseAdvancedQuery failed: %v", err)
	}

	if len(q.NOT) != 1 {
		t.Errorf("Expected 1 NOT term, got %d", len(q.NOT))
	}

	if q.NOT[0] != "exclude" {
		t.Errorf("Expected NOT term 'exclude', got %s", q.NOT[0])
	}
}

// TestGetMatch tests retrieving full content
func TestGetMatch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	s, err := NewSearcher(dbPath)
	if err != nil {
		t.Fatalf("NewSearcher failed: %v", err)
	}
	defer s.Close()

	if err := s.InitSchema(); err != nil {
		t.Fatalf("InitSchema failed: %v", err)
	}

	// Index a task
	taskID := "task-1"
	title := "Test Task"
	description := "This is a test description for the task"

	if err := s.IndexTask(taskID, title, description); err != nil {
		t.Fatalf("IndexTask failed: %v", err)
	}

	// Get full content
	content, err := s.GetMatch("task", taskID)
	if err != nil {
		t.Fatalf("GetMatch failed: %v", err)
	}

	if !contains(content, title) {
		t.Error("Expected title in content")
	}

	if !contains(content, description) {
		t.Error("Expected description in content")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Package spec provides types and functions for generating epics and tasks from design specifications
package spec

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Parser reads and parses design spec files
type Parser struct{}

// NewParser creates a new spec parser
func NewParser() *Parser {
	return &Parser{}
}

// ParseInput reads from a file or folder and returns combined content
func (p *Parser) ParseInput(path string) (string, []string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", nil, fmt.Errorf("accessing path: %w", err)
	}

	if info.IsDir() {
		return p.parseFolder(path)
	}
	return p.parseFile(path)
}

// parseFile reads a single file
func (p *Parser) parseFile(filePath string) (string, []string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))

	switch ext {
	case ".md", ".markdown":
		return p.parseMarkdown(filePath)
	case ".jsonl":
		return p.parseJSONL(filePath)
	default:
		return "", nil, fmt.Errorf("unsupported file type: %s (supported: .md, .jsonl)", ext)
	}
}

// parseFolder reads all spec files in a folder
func (p *Parser) parseFolder(folderPath string) (string, []string, error) {
	var allContent strings.Builder
	var files []string

	entries, err := os.ReadDir(folderPath)
	if err != nil {
		return "", nil, fmt.Errorf("reading folder: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(folderPath, entry.Name())
		content, _, err := p.parseFile(filePath)
		if err != nil {
			continue // Skip unreadable files
		}

		allContent.WriteString(fmt.Sprintf("# File: %s\n\n", entry.Name()))
		allContent.WriteString(content)
		allContent.WriteString("\n\n---\n\n")
		files = append(files, filePath)
	}

	if allContent.Len() == 0 {
		return "", nil, fmt.Errorf("no valid spec files found in %s", folderPath)
	}

	return allContent.String(), files, nil
}

// parseMarkdown reads a markdown file
func (p *Parser) parseMarkdown(filePath string) (string, []string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("reading file: %w", err)
	}
	return string(content), []string{filePath}, nil
}

// parseJSONL reads a JSONL file (one JSON object per line)
func (p *Parser) parseJSONL(filePath string) (string, []string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", nil, fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var allContent strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var data map[string]interface{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			return "", nil, fmt.Errorf("parsing JSONL line %d: %w", lineNum, err)
		}

		// Convert to markdown format
		if title, ok := data["title"].(string); ok {
			allContent.WriteString(fmt.Sprintf("## %s\n\n", title))
		}
		if desc, ok := data["description"].(string); ok {
			allContent.WriteString(desc + "\n\n")
		}
		if content, ok := data["content"].(string); ok {
			allContent.WriteString(content + "\n\n")
		}
	}

	if err := scanner.Err(); err != nil {
		return "", nil, fmt.Errorf("reading file: %w", err)
	}

	return allContent.String(), []string{filePath}, nil
}

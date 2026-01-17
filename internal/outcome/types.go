// Package outcome provides structured task outcome parsing and verdict extraction
package outcome

import (
	"bufio"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Verdict represents the structured outcome of a task execution
type Verdict string

const (
	// VerdictPass indicates the task completed successfully
	VerdictPass Verdict = "pass"
	// VerdictFail indicates the task failed
	VerdictFail Verdict = "fail"
	// VerdictBlocked indicates the task is blocked by dependencies
	VerdictBlocked Verdict = "blocked"
	// VerdictUnknown indicates the verdict could not be determined
	VerdictUnknown Verdict = "unknown"
)

// String returns the string representation of the verdict
func (v Verdict) String() string {
	return string(v)
}

// IsValid returns true if the verdict is a known value
func (v Verdict) IsValid() bool {
	switch v {
	case VerdictPass, VerdictFail, VerdictBlocked, VerdictUnknown:
		return true
	}
	return false
}

// Icon returns an emoji icon for the verdict
func (v Verdict) Icon() string {
	switch v {
	case VerdictPass:
		return "✅"
	case VerdictFail:
		return "❌"
	case VerdictBlocked:
		return "🚧"
	case VerdictUnknown:
		return "❓"
	}
	return "❓"
}

// Outcome represents the complete structured outcome of a task execution
type Outcome struct {
	Verdict    Verdict            `json:"verdict"`
	Reason     string            `json:"reason,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	Details    string            `json:"details,omitempty"`
	Timestamp  time.Time         `json:"timestamp"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	Blockers   []string          `json:"blockers,omitempty"` // Blocked-by task IDs
	ExitCode   int               `json:"exit_code,omitempty"`
	Changes    *ChangeInfo       `json:"changes,omitempty"`
}

// ChangeInfo represents information about code changes made
type ChangeInfo struct {
	FilesModified int    `json:"files_modified"`
	LinesAdded    int    `json:"lines_added"`
	LinesDeleted  int    `json:"lines_deleted"`
	CommitHash    string `json:"commit_hash,omitempty"`
}

// Parser parses agent output to extract structured outcomes
type Parser struct {
	// Custom patterns for verdict detection
	passPatterns  []*regexp.Regexp
	failPatterns  []*regexp.Regexp
	blockedPatterns []*regexp.Regexp
}

// NewParser creates a new outcome parser with default patterns
func NewParser() *Parser {
	return &Parser{
		passPatterns: compilePatterns(defaultPassPatterns),
		failPatterns: compilePatterns(defaultFailPatterns),
		blockedPatterns: compilePatterns(defaultBlockedPatterns),
	}
}

// defaultPassPatterns are patterns that indicate a successful outcome
var defaultPassPatterns = []string{
	`(?i)success`,
	`(?i)completed successfully`,
	`(?i)task complete`,
	`(?i)all tests passed`,
	`(?i)implementation complete`,
	`(?i)done`,
}

// defaultFailPatterns are patterns that indicate a failed outcome
var defaultFailPatterns = []string{
	`(?i)failed`,
	`(?i)error:`,
	`(?i)unable to`,
	`(?i)cannot`,
	`(?i)task failed`,
	`(?i)execution failed`,
	`(?i)exception`,
	`(?i)panic`,
}

// defaultBlockedPatterns are patterns that indicate a blocked outcome
var defaultBlockedPatterns = []string{
	`(?i)blocked by`,
	`(?i)waiting for`,
	`(?i)depends on`,
	`(?i)blocked:`,
	`(?i)cannot proceed until`,
}

// compilePatterns compiles regex patterns
func compilePatterns(patterns []string) []*regexp.Regexp {
	var compiled []*regexp.Regexp
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		compiled = append(compiled, re)
	}
	return compiled
}

// ParseOutput parses agent output and extracts a structured outcome
func (p *Parser) ParseOutput(output string, exitCode int) *Outcome {
	outcome := &Outcome{
		Timestamp: time.Now(),
		ExitCode:  exitCode,
		Metadata:  make(map[string]string),
	}

	// Check exit code first
	if exitCode != 0 {
		outcome.Verdict = VerdictFail
		outcome.Reason = fmt.Sprintf("Process exited with code %d", exitCode)
		return p.enrichOutcomeFromOutput(outcome, output)
	}

	// Parse output for verdict indicators
	outcome.Verdict = p.detectVerdict(output)

	// Extract summary and details
	p.extractSummaryAndDetails(outcome, output)

	// Detect blockers if verdict is blocked
	if outcome.Verdict == VerdictBlocked {
		outcome.Blockers = p.extractBlockers(output)
	}

	return outcome
}

// detectVerdict analyzes output to determine the verdict
func (p *Parser) detectVerdict(output string) Verdict {
	// Check blocked patterns first (more specific)
	for _, re := range p.blockedPatterns {
		if re.MatchString(output) {
			return VerdictBlocked
		}
	}

	// Check fail patterns
	for _, re := range p.failPatterns {
		if re.MatchString(output) {
			return VerdictFail
		}
	}

	// Check pass patterns
	for _, re := range p.passPatterns {
		if re.MatchString(output) {
			return VerdictPass
		}
	}

	// Default to unknown if no patterns match
	return VerdictUnknown
}

// enrichOutcomeFromOutput enriches an outcome with additional details from output
func (p *Parser) enrichOutcomeFromOutput(outcome *Outcome, output string) *Outcome {
	p.extractSummaryAndDetails(outcome, output)
	if outcome.Verdict == VerdictBlocked {
		outcome.Blockers = p.extractBlockers(output)
	}
	return outcome
}

// extractSummaryAndDetails extracts a summary and detailed information from output
func (p *Parser) extractSummaryAndDetails(outcome *Outcome, output string) {
	lines := strings.Split(output, "\n")

	// Extract first meaningful line as summary
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "=") || strings.HasPrefix(line, "#") {
			continue
		}
		if outcome.Summary == "" {
			// Truncate if too long
			if len(line) > 200 {
				outcome.Summary = line[:200] + "..."
			} else {
				outcome.Summary = line
			}
		}
		break
	}

	// Extract details (last few lines before completion)
	// Look for error messages or final status
	for i := len(lines) - 1; i >= 0 && i > len(lines)-20; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.Contains(strings.ToLower(line), "error") ||
		   strings.Contains(strings.ToLower(line), "failed") ||
		   strings.Contains(strings.ToLower(line), "warning") {
			if outcome.Details == "" {
				outcome.Details = line
			}
			break
		}
	}

	// If no details found but we have output, use a portion of it
	if outcome.Details == "" && len(lines) > 0 {
		// Use last non-empty line
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				outcome.Details = line
				break
			}
		}
	}
}

// extractBlockers extracts task IDs that are blocking this task
func (p *Parser) extractBlockers(output string) []string {
	var blockers []string

	// Pattern for task IDs (e.g., task-1234567890, abc-123-def)
	taskIDPattern := regexp.MustCompile(`[a-zA-Z0-9]+-[0-9]+`)
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.ToLower(line)
		if strings.Contains(line, "blocked by") ||
		   strings.Contains(line, "blocked:") ||
		   strings.Contains(line, "waiting for") ||
		   strings.Contains(line, "depends on") {

			// Extract task IDs from this line
			matches := taskIDPattern.FindAllString(line, -1)
			for _, match := range matches {
				if strings.HasPrefix(match, "task-") || len(match) > 10 {
					blockers = append(blockers, match)
				}
			}
		}
	}

	return uniqueStrings(blockers)
}

// uniqueStrings returns unique strings from a slice
func uniqueStrings(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// ParseWithExitCode is a convenience function that parses output with an exit code
func ParseWithExitCode(output string, exitCode int) *Outcome {
	parser := NewParser()
	return parser.ParseOutput(output, exitCode)
}

// ParseOutput parses output assuming success (exit code 0)
func ParseOutput(output string) *Outcome {
	return ParseWithExitCode(output, 0)
}

// DetermineVerdict quickly determines a verdict from output
func DetermineVerdict(output string, exitCode int) Verdict {
	outcome := ParseWithExitCode(output, exitCode)
	return outcome.Verdict
}

// ExtractSummary extracts a summary from output
func ExtractSummary(output string, maxLines int) string {
	if maxLines <= 0 {
		maxLines = 5
	}

	var summaryLines []string
	scanner := bufio.NewScanner(strings.NewReader(output))

	for scanner.Scan() && len(summaryLines) < maxLines {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "=") {
			summaryLines = append(summaryLines, line)
		}
	}

	return strings.Join(summaryLines, "\n")
}

// FormatOutcome formats an outcome for display
func FormatOutcome(outcome *Outcome) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s %s", outcome.Verdict.Icon(), strings.ToUpper(string(outcome.Verdict))))

	if outcome.Summary != "" {
		sb.WriteString(fmt.Sprintf(": %s", outcome.Summary))
	}

	sb.WriteString("\n")

	if outcome.Reason != "" {
		sb.WriteString(fmt.Sprintf("  Reason: %s\n", outcome.Reason))
	}

	if len(outcome.Blockers) > 0 {
		sb.WriteString(fmt.Sprintf("  Blocked by: %s\n", strings.Join(outcome.Blockers, ", ")))
	}

	if outcome.Details != "" {
		sb.WriteString(fmt.Sprintf("  Details: %s\n", outcome.Details))
	}

	if outcome.Changes != nil {
		sb.WriteString(fmt.Sprintf("  Changes: %d files, +%d/-%d lines\n",
			outcome.Changes.FilesModified,
			outcome.Changes.LinesAdded,
			outcome.Changes.LinesDeleted))
	}

	return sb.String()
}

// MergeOutcomes combines multiple outcomes into a single verdict
// Pass only if all pass, fail if any fail, blocked if any blocked
func MergeOutcomes(outcomes []*Outcome) *Outcome {
	if len(outcomes) == 0 {
		return &Outcome{
			Verdict:   VerdictUnknown,
			Timestamp: time.Now(),
		}
	}

	merged := &Outcome{
		Timestamp: time.Now(),
		Metadata:  make(map[string]string),
	}

	passCount := 0
	failCount := 0
	blockedCount := 0

	for _, o := range outcomes {
		switch o.Verdict {
		case VerdictPass:
			passCount++
		case VerdictFail:
			failCount++
		case VerdictBlocked:
			blockedCount++
		}

		// Collect all blockers
		if len(o.Blockers) > 0 {
			if merged.Blockers == nil {
				merged.Blockers = []string{}
			}
			merged.Blockers = append(merged.Blockers, o.Blockers...)
		}
	}

	// Determine verdict
	if failCount > 0 {
		merged.Verdict = VerdictFail
		merged.Reason = fmt.Sprintf("%d sub-tasks failed", failCount)
	} else if blockedCount > 0 {
		merged.Verdict = VerdictBlocked
		merged.Reason = fmt.Sprintf("%d sub-tasks blocked", blockedCount)
		merged.Blockers = uniqueStrings(merged.Blockers)
	} else if passCount == len(outcomes) {
		merged.Verdict = VerdictPass
		merged.Summary = fmt.Sprintf("All %d sub-tasks passed", passCount)
	} else {
		merged.Verdict = VerdictUnknown
	}

	return merged
}

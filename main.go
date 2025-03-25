package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

type EventType int

const (
	Unknown EventType = iota
	DmgTaken
	DmgDealt
	Heal
	PowerRestored
	DebuffApplied
	BuffApplied
	Interrupt
	CorruptionRemoved
	Death
	Revive
	CombatStart
	CombatEnd
	MobInterrupt
	TempMoraleLost
	TempMoraleNotWasted
	CcBroken
	Benefit
	Comment
)

type eventParser func(string) (*LogEntry, error)

type ParseNotMatchError struct{}

func (p *ParseNotMatchError) Error() string {
	return fmt.Sprintf("This parser did not match")
}

// LogEntry represents a parsed log entry.
type LogEntry struct {
	Timestamp time.Time
	etype     EventType
	Source    string
	// Action      string
	// Modifier    string
	// SubAction   string // hit, heal, benefit
	Target    string
	Skill     string
	Value     int
	ValueType string
	// FinalTarget string
	RawMessage string // The original log line (for debugging)
}

// parseLogLine parses a single line from the log file.
func parseLogLine(line string) (*LogEntry, error) {

	options := map[EventType]eventParser{
		Comment: pComment,
		Benefit: pBenefit,
	}
	var entry *LogEntry
	for _, p := range options {
		e, err := p(line)
		if err != nil {
			var nomatch *ParseNotMatchError
			if errors.As(err, &nomatch) {
				continue
			}
			return nil, fmt.Errorf("Odd error from parsing: %v", err)
		}
		// was success
		entry = e
	}
	if entry == nil {
		return nil, fmt.Errorf("No parsers matched: <%v>", line)
	}
	entry.RawMessage = line

	return entry, nil
}

// extractTimestamp extracts the timestamp from the log line.
func extractTimestamp(line string) (time.Time, string, error) {
	re := regexp.MustCompile(`^\[?(\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2}\s*(?:AM|PM)?)\]?`)
	match := re.FindStringSubmatch(line)
	if len(match) < 2 {
		return time.Time{}, "", fmt.Errorf("timestamp not found")
	}

	remaining := line[len(match):]

	timestampStr := match[1]
	formats := []string{"01/02 03:04:05 PM", "01/02 03:04:05"}
	for _, format := range formats {
		t, err := time.Parse(format, timestampStr)
		if err == nil {
			return t, remaining, nil
		}
	}
	return time.Time{}, "", fmt.Errorf("invalid timestamp format: %s", timestampStr)
}

func main() {
	filePath := "test/input.txt" // Or "Combat_20240708_2.txt"

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		entry, err := parseLogLine(line)
		if err != nil {
			fmt.Println("Error parsing line:", err)
			break
		}
		fmt.Printf("Event Data: %+v\n", entry)
	}

	if scanner.Err() != nil {
		fmt.Println("Error reading file:", scanner.Err())
	}
}

func pComment(line string) (*LogEntry, error) {
	if strings.HasPrefix(line, "###") {
		return &LogEntry{
			etype: Comment,
		}, nil
	}
	return nil, &ParseNotMatchError{}
}

func pBenefit(line string) (*LogEntry, error) {
	if !strings.Contains(line, "applied a benefit") {
		return nil, &ParseNotMatchError{}
	}

	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	re := regexp.MustCompile(`(?P<source>\w+) applied a benefit with (?P<benefitname>.*) on (?P<target>.*).`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as Benefit: <%s>", msg)
	}
	entry.Source = match[re.SubexpIndex("source")]
	entry.Skill = match[re.SubexpIndex("benefitname")]
	entry.Target = match[re.SubexpIndex("target")]
	return entry, nil
}

func pHeal(line string) (*LogEntry, error) {
	if !strings.Contains(line, "applied a heal") {
		return nil, &ParseNotMatchError{}
	}

	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	re := regexp.MustCompile(`(?P<skill>\w+) applied a benefit with (?P<benefitname>.*) on (?P<target>.*).`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as Benefit: <%s>", msg)
	}
	entry.Source = match[re.SubexpIndex("source")]
	entry.Skill = match[re.SubexpIndex("benefitname")]
	entry.Target = match[re.SubexpIndex("target")]
	return entry, nil
}
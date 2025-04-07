package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
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

type Avoid int

const (
	UnknownAvoid Avoid = iota
	Blocked
	Parried
	Evaded
	Resisted
	Immune
	Deflected
	Missed
)

const (
	// whenever parser finds "you", to be replaced later by logic that knows the player's name
	selfplaceholder = "SELF_REPLACE"
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
	Crit      bool
	Dev       bool
	Avoided   Avoid
	// FinalTarget string
	RawMessage string // The original log line (for debugging)
}

// parseLogLine parses a single line from the log file.
func parseLogLine(line string) (*LogEntry, error) {

	options := map[EventType][]eventParser{
		Comment:           {pComment},
		Benefit:           {pBenefit},
		Heal:              {pHeal},
		DmgDealt:          {pDmg, pAvoid, pMiss, pDmgNoValue},
		TempMoraleLost:    {pTempMoraleLost},
		Death:             {pDefeat, pIncapacitate},
		Revive:            {pRevive, pSuccumb}, // no idea why succumb to wounds == revive
		CorruptionRemoved: {pCorrRemove},
		CcBroken:          {pCCBroken},
	}
	var entry *LogEntry
	for _, ps := range options {
		for _, p := range ps {
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
	}
	if entry == nil {
		return nil, fmt.Errorf("No parsers matched: <%v>", line)
	}
	entry.RawMessage = line

	return entry, nil
}

// extractTimestamp extracts the timestamp from the log line.
func extractTimestamp(line string) (time.Time, string, error) {
	re := regexp.MustCompile(`^\[?(\d{2}\/\d{2}\s+\d{2}:\d{2}:\d{2}\s*(?:AM|PM)?)\]? `)
	match := re.FindStringSubmatch(line)
	if len(match) < 2 {
		return time.Time{}, "", fmt.Errorf("timestamp not found")
	}

	remaining := line[len(match[0]):]

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
	errorlines := []string{}
	lines := 0
	for scanner.Scan() {
		lines++
		line := scanner.Text()
		_, err := parseLogLine(line)
		if err != nil {
			fmt.Println("Error parsing line:", err)
			errorlines = append(errorlines, line)
			continue
		}
		// fmt.Printf("Event Data: %+v\n", entry)
	}

	if scanner.Err() != nil {
		fmt.Println("Error reading file:", scanner.Err())
	}
	fmt.Printf("total lines: %v\n", lines)
	fmt.Printf("total errors: %v\n", len(errorlines))
	for i, el := range errorlines {
		if i >= 10 {
			break
		}
		fmt.Println(el)
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
	if match, err := regexp.Match("applied a .*benefit", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}

	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	re := regexp.MustCompile(`(?P<source>\w+) applied a (?P<crit>critical )?benefit with (?P<benefitname>.*) on (?P<target>.*).`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as Benefit: <%s>", msg)
	}
	entry.Source = match[re.SubexpIndex("source")]
	entry.Skill = match[re.SubexpIndex("benefitname")]
	entry.Target = match[re.SubexpIndex("target")]
	entry.Crit = match[re.SubexpIndex("crit")] != ""
	return entry, nil
}

func pHeal(line string) (*LogEntry, error) {
	if match, err := regexp.Match("applied a .*heal", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}

	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	selfheal := regexp.MustCompile(`(?P<skill>\w+) applied a (?<crit>critical )?heal to (?P<target>.*) restoring (?P<value>[\d,]+) points to (?P<type>.*).`)
	match := selfheal.FindStringSubmatch(msg)
	if len(match) != 0 {
		entry.Skill = match[selfheal.SubexpIndex("skill")]
		entry.Target = match[selfheal.SubexpIndex("target")]
		val, err := strconv.Atoi(strings.Replace(match[selfheal.SubexpIndex("value")], ",", "", -1))
		if err != nil {
			return nil, fmt.Errorf("value not convertable to int: %v", err)
		}
		entry.Value = val
		entry.ValueType = match[selfheal.SubexpIndex("type")]
		entry.Crit = match[selfheal.SubexpIndex("crit")] != ""
		return entry, nil
	}
	incHeal := regexp.MustCompile(`(?P<otherplayer>\w+) applied a (?<crit>critical )?heal with (?P<skill>.*?) to (?P<target>.*) restoring (?P<value>[\d,]+) points to (?P<type>.*).`)
	match = incHeal.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as heal: <%v>", line)
	}
	entry.Skill = match[incHeal.SubexpIndex("skill")]
	entry.Target = match[incHeal.SubexpIndex("target")]
	entry.Source = match[incHeal.SubexpIndex("otherplayer")]
	val, err := strconv.Atoi(strings.Replace(match[incHeal.SubexpIndex("value")], ",", "", -1))
	if err != nil {
		return nil, fmt.Errorf("value not convertable to int: %v <%v>", err, msg)
	}
	entry.Value = val
	entry.ValueType = match[incHeal.SubexpIndex("type")]
	entry.Crit = match[incHeal.SubexpIndex("crit")] != ""
	return entry, nil
}

func pDmg(line string) (*LogEntry, error) {
	if match, err := regexp.Match("scored a .*hit.*for.*damage", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	dmg := regexp.MustCompile(`(?P<source>[^ ]+) scored a (partially )?(?<avoided>blocked|parried|evaded)?(?<crit>critical|devastating)? ?hit with (?P<skill>.*?) on (?P<target>.*) for (?P<value>[\d,]+) (?P<type>.*?) ?damage to Morale.`)
	match := dmg.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as dmg dealt: <%v>", line)
	}
	entry.Skill = match[dmg.SubexpIndex("skill")]
	entry.Target = match[dmg.SubexpIndex("target")]
	entry.Source = match[dmg.SubexpIndex("source")]
	val, err := strconv.Atoi(strings.Replace(match[dmg.SubexpIndex("value")], ",", "", -1))
	if err != nil {
		return nil, fmt.Errorf("value not convertable to int: %v <%v>", err, msg)
	}
	entry.Value = val
	entry.ValueType = match[dmg.SubexpIndex("type")]
	entry.Crit = match[dmg.SubexpIndex("crit")] == "critical"
	entry.Dev = match[dmg.SubexpIndex("crit")] == "devastating"

	switch match[dmg.SubexpIndex("crit")] {
	case "blocked":
		entry.Avoided = Blocked
	case "parried":
		entry.Avoided = Parried
	case "evaded":
		entry.Avoided = Evaded
	}
	return entry, nil
}

func pDmgNoValue(line string) (*LogEntry, error) {
	if match, err := regexp.Match("scored a .*hit", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	if match, err := regexp.Match("scored a .*hit.*for.*damage", []byte(line)); err != nil || match {
		return nil, &ParseNotMatchError{}
	}

	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	dmg := regexp.MustCompile(`(?P<player>\w+) scored a (partially )?(?<avoided>blocked|parried|evaded)?(?<crit>critical|devastating)? ?hit with (?P<skill>.*?) on (?P<target>[^ ]+).$`)
	match := dmg.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as dmg no value: <%v>", line)
	}
	entry.Skill = match[dmg.SubexpIndex("skill")]
	entry.Target = match[dmg.SubexpIndex("target")]
	entry.Source = match[dmg.SubexpIndex("player")]
	entry.Value = 0
	entry.Crit = match[dmg.SubexpIndex("crit")] == "critical"
	entry.Dev = match[dmg.SubexpIndex("crit")] == "devastating"

	switch match[dmg.SubexpIndex("crit")] {
	case "blocked":
		entry.Avoided = Blocked
	case "parried":
		entry.Avoided = Parried
	case "evaded":
		entry.Avoided = Evaded
	}
	return entry, nil
}

func pAvoid(line string) (*LogEntry, error) {
	if match, err := regexp.Match("tried to use.*", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	miss := regexp.MustCompile(`(?P<player>\w+) tried to use (?P<skill>.*?) on (?P<target>.*) but (?P<reason>.*) the attempt.`)
	match := miss.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as avoid: <%v>", line)
	}
	entry.Skill = match[miss.SubexpIndex("skill")]
	entry.Target = match[miss.SubexpIndex("target")]
	entry.Source = match[miss.SubexpIndex("player")]
	switch match[miss.SubexpIndex("reason")] {
	case "blocked":
		entry.Avoided = Blocked
	case "parried":
		entry.Avoided = Parried
	case "evaded":
		entry.Avoided = Evaded
	case "resisted":
		entry.Avoided = Resisted
	default:
		entry.Avoided = UnknownAvoid
	}
	return entry, nil
}

func pMiss(line string) (*LogEntry, error) {
	if match, err := regexp.Match("missed trying to use.*", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	miss := regexp.MustCompile(`(?P<player>\w+) missed trying to use (?P<skill>.*?) on (?P<target>.*).`)
	match := miss.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as avoid: <%v>", line)
	}
	entry.Skill = match[miss.SubexpIndex("skill")]
	entry.Target = match[miss.SubexpIndex("target")]
	entry.Source = match[miss.SubexpIndex("player")]
	entry.Avoided = Missed
	return entry, nil
}

func pTempMoraleLost(line string) (*LogEntry, error) {
	if match, err := regexp.Match("You have lost .* of temporary Morale!", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	re := regexp.MustCompile(`You have lost (?P<value>[\d,]+) points of temporary Morale!`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as temp morale lost: <%s>", msg)
	}
	val, err := strconv.Atoi(strings.Replace(match[re.SubexpIndex("value")], ",", "", -1))
	if err != nil {
		return nil, fmt.Errorf("value not convertable to int: %v <%v>", err, msg)
	}
	entry.Value = val

	return entry, nil
}

func pDefeat(line string) (*LogEntry, error) {
	if match, err := regexp.Match(".* defeated .*$", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp
	re := regexp.MustCompile(`(?P<victor>.*) defeated (?P<dead>.*)\.`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as defeat: <%s>", msg)
	}
	entry.Source = match[re.SubexpIndex("victor")]
	entry.Target = match[re.SubexpIndex("dead")]
	return entry, nil
}

func pIncapacitate(line string) (*LogEntry, error) {
	if match, err := regexp.Match(`( incapacitated you|You have been incapacitated by misadventure)\.$`, []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	if msg == "You have been incapacitated by misadventure." {
		entry.Target = selfplaceholder
		return entry, nil
	}
	re := regexp.MustCompile(`(?P<source>.*) incapacitated you\.`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as incapacitation: <%s>", msg)
	}
	entry.Source = match[re.SubexpIndex("source")]
	return entry, nil

}

func pRevive(line string) (*LogEntry, error) {
	if match, err := regexp.Match(".* been revived.$", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	if msg == "You have been revived." {
		entry.Target = selfplaceholder
		return entry, nil
	}

	re := regexp.MustCompile(`(?P<target>.*) has been revived\.`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as revive: <%s>", msg)
	}
	entry.Target = match[re.SubexpIndex("target")]
	return entry, nil
}

func pSuccumb(line string) (*LogEntry, error) {
	if match, err := regexp.Match("succumb.*wounds", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	if msg == "You succumb to your wounds." {
		entry.Target = selfplaceholder
		return entry, nil
	}

	re := regexp.MustCompile(`(?P<target>.*) has succumbed to .* wounds\.`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as succumbing to wounds: <%s>", msg)
	}
	entry.Target = match[re.SubexpIndex("target")]
	return entry, nil
}

func pCorrRemove(line string) (*LogEntry, error) {
	if match, err := regexp.Match("(have dispelled.*from.*|Nothing to dispel.)", []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	if msg == "Nothing to dispel." {
		entry.Source = selfplaceholder
		return entry, nil
	}

	re := regexp.MustCompile(`You have dispelled (?P<corruption>.*) from (?P<target>.*)\.$`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as corr removal: <%s>", msg)
	}
	entry.Target = match[re.SubexpIndex("target")]
	entry.Skill = match[re.SubexpIndex("corruption")]
	return entry, nil
}

func pCCBroken(line string) (*LogEntry, error) {
	if match, err := regexp.Match(` released .* from being immobilized!`, []byte(line)); err != nil || !match {
		return nil, &ParseNotMatchError{}
	}
	timestamp, msg, err := extractTimestamp(line)
	if err != nil {
		return nil, fmt.Errorf("error parsing timestamp: %w", err)
	}
	entry := &LogEntry{RawMessage: line}
	entry.Timestamp = timestamp

	re := regexp.MustCompile(`(?P<source>.*) (have|has) released (?P<target>.*) from being immobilized!$`)
	match := re.FindStringSubmatch(msg)
	if len(match) == 0 {
		return nil, fmt.Errorf("Failed to parse as cc break: <%s>", msg)
	}
	entry.Target = match[re.SubexpIndex("target")]
	if match[re.SubexpIndex("source")] == "You" {
		entry.Source = selfplaceholder
	} else {
		entry.Source = match[re.SubexpIndex("source")]
	}
	return entry, nil
}

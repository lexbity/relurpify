package services

import (
	"strconv"
	"strings"
	"time"
)

// matchesCron checks if the current time matches a cron expression.
// Supports: * (wildcard), ranges (1-5), lists (1,3,5), steps (*/2, 1-10/3).
func matchesCron(expr string, t time.Time) bool {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return false // invalid expression
	}

	minute, hour, day, month, weekday := t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())

	// Cron: minute hour day month weekday
	// Weekday in cron: 0 = Sunday, 7 = Sunday alias. Go's time gives
	// Sunday=0, so match the field directly and, for Sunday, also try the
	// 7 alias (covers `0`, `7`, `0,3`, and ranges over both spellings).
	weekdayMatch := matchCronField(fields[4], weekday, 0, 6)
	if !weekdayMatch && weekday == 0 {
		weekdayMatch = matchCronField(fields[4], 7, 0, 7)
	}

	return matchCronField(fields[0], minute, 0, 59) &&
		matchCronField(fields[1], hour, 0, 23) &&
		matchCronField(fields[2], day, 1, 31) &&
		matchCronField(fields[3], month, 1, 12) &&
		weekdayMatch
}

// matchCronField checks if a value matches a cron field expression.
func matchCronField(field string, value, lowerBound, upperBound int) bool {
	// Handle wildcards
	if field == "*" {
		return true
	}

	// Handle steps (e.g., */2, 1-10/3)
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}

		var start, end int
		switch {
		case parts[0] == "*":
			start, end = lowerBound, upperBound
		case strings.Contains(parts[0], "-"):
			rangeParts := strings.Split(parts[0], "-")
			if len(rangeParts) != 2 {
				return false
			}
			var err error
			start, err = strconv.Atoi(rangeParts[0])
			if err != nil {
				return false
			}
			end, err = strconv.Atoi(rangeParts[1])
			if err != nil {
				return false
			}
		default:
			var err error
			start, err = strconv.Atoi(parts[0])
			if err != nil {
				return false
			}
			end = upperBound
		}

		for i := start; i <= end; i += step {
			if i == value {
				return true
			}
		}
		return false
	}

	// Handle ranges (e.g., 1-5)
	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return false
		}
		start, err := strconv.Atoi(parts[0])
		if err != nil {
			return false
		}
		end, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
		return value >= start && value <= end
	}

	// Handle lists (e.g., 1,3,5)
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, p := range parts {
			if v, err := strconv.Atoi(p); err == nil && v == value {
				return true
			}
		}
		return false
	}

	// Handle single value
	if v, err := strconv.Atoi(field); err == nil {
		return v == value
	}

	return false
}

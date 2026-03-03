package util

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

var zoneStr = time.Now().Format("-0700")

// Time granularity constants
type TimeGranularity int

const (
	GranularityUnknown TimeGranularity = iota // unknown granularity
	GranularitySecond                         // precise to second
	GranularityMinute                         // precise to minute
	GranularityHour                           // precise to hour
	GranularityDay                            // precise to day
	GranularityMonth                          // precise to month
	GranularityQuarter                        // precise to quarter
	GranularityYear                           // precise to year
)

// timeOf internal function, parses various time point formats and returns time granularity
// Supported formats:
// 1. Timestamp (seconds): 1609459200 (GranularitySecond)
// 2. Standard date: 20060102, 2006-01-02 (GranularityDay)
// 3. Date with time: 20060102/15:04, 2006-01-02/15:04 (GranularityMinute)
// 4. Full datetime: 20060102150405 (GranularitySecond)
// 5. RFC3339: 2006-01-02T15:04:05Z07:00 (GranularitySecond)
// 6. Relative time: 5h-ago, 3d-ago, 1w-ago, 1m-ago, 1y-ago (granularity determined by unit)
// 7. Natural language: now (GranularitySecond), today, yesterday (GranularityDay)
// 8. Year: 2006 (GranularityYear)
// 9. Month: 200601, 2006-01 (GranularityMonth)
// 10. Quarter: 2006Q1, 2006Q2, 2006Q3, 2006Q4 (GranularityQuarter)
// 11. Year-month-day-hour-minute: 200601021504 (GranularityMinute)
func timeOf(str string) (t time.Time, g TimeGranularity, ok bool) {
	if str == "" {
		return time.Time{}, GranularityUnknown, false
	}

	str = strings.TrimSpace(str)

	// Handle natural language time
	switch strings.ToLower(str) {
	case "now":
		return time.Now(), GranularitySecond, true
	case "today":
		now := time.Now()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), GranularityDay, true
	case "yesterday":
		now := time.Now().AddDate(0, 0, -1)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), GranularityDay, true
	case "this-week":
		now := time.Now()
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		// This week's Monday
		monday := now.AddDate(0, 0, -(weekday - 1))
		return time.Date(monday.Year(), monday.Month(), monday.Day(), 0, 0, 0, 0, now.Location()), GranularityDay, true
	case "last-week":
		now := time.Now()
		weekday := int(now.Weekday())
		if weekday == 0 { // Sunday
			weekday = 7
		}
		// Last week's Monday
		lastMonday := now.AddDate(0, 0, -(weekday-1)-7)
		return time.Date(lastMonday.Year(), lastMonday.Month(), lastMonday.Day(), 0, 0, 0, 0, now.Location()), GranularityDay, true
	case "this-month":
		now := time.Now()
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), GranularityMonth, true
	case "last-month":
		now := time.Now()
		return time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, now.Location()), GranularityMonth, true
	case "this-year":
		now := time.Now()
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location()), GranularityYear, true
	case "last-year":
		now := time.Now()
		return time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, now.Location()), GranularityYear, true
	case "all":
		// Return zero time
		return time.Time{}, GranularityYear, true
	}

	// Handle relative time: 5h-ago, 3d-ago, 1w-ago, 1m-ago, 1y-ago
	if strings.HasSuffix(str, "-ago") {
		str = strings.TrimSuffix(str, "-ago")

		// Special handling: 0d-ago means start of today
		if str == "0d" {
			now := time.Now()
			return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()), GranularityDay, true
		}

		// Parse number and unit
		re := regexp.MustCompile(`^(\d+)([hdwmy])$`)
		matches := re.FindStringSubmatch(str)
		if len(matches) == 3 {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				return time.Time{}, GranularityUnknown, false
			}

			// Ensure number is positive
			if num <= 0 {
				// 0d-ago is already specially handled, other 0 or negative values are invalid
				if num == 0 && matches[2] != "d" {
					return time.Time{}, GranularityUnknown, false
				}
				return time.Time{}, GranularityUnknown, false
			}

			now := time.Now()
			var resultTime time.Time
			var granularity TimeGranularity

			switch matches[2] {
			case "h": // hour
				resultTime = now.Add(-time.Duration(num) * time.Hour)
				granularity = GranularityHour
			case "d": // day
				resultTime = now.AddDate(0, 0, -num)
				granularity = GranularityDay
			case "w": // week
				resultTime = now.AddDate(0, 0, -num*7)
				granularity = GranularityDay
			case "m": // month
				resultTime = now.AddDate(0, -num, 0)
				granularity = GranularityMonth
			case "y": // year
				resultTime = now.AddDate(-num, 0, 0)
				granularity = GranularityYear
			default:
				return time.Time{}, GranularityUnknown, false
			}

			return resultTime, granularity, true
		}

		// Try standard duration parsing
		dur, err := time.ParseDuration(str)
		if err == nil {
			// Determine granularity based on duration unit
			hours := dur.Hours()
			if hours < 1 {
				return time.Now().Add(-dur), GranularitySecond, true
			} else if hours < 24 {
				return time.Now().Add(-dur), GranularityHour, true
			} else {
				return time.Now().Add(-dur), GranularityDay, true
			}
		}

		return time.Time{}, GranularityUnknown, false
	}

	// Handle quarter: 2006Q1, 2006Q2, 2006Q3, 2006Q4
	if matched, _ := regexp.MatchString(`^\d{4}Q[1-4]$`, str); matched {
		re := regexp.MustCompile(`^(\d{4})Q([1-4])$`)
		matches := re.FindStringSubmatch(str)
		if len(matches) == 3 {
			year, _ := strconv.Atoi(matches[1])
			quarter, _ := strconv.Atoi(matches[2])

			// Validate year range
			if year < 1970 || year > 9999 {
				return time.Time{}, GranularityUnknown, false
			}

			// Calculate quarter start month
			startMonth := time.Month((quarter-1)*3 + 1)

			return time.Date(year, startMonth, 1, 0, 0, 0, 0, time.Local), GranularityQuarter, true
		}
	}

	// Handle year: 2006
	if len(str) == 4 && isDigitsOnly(str) {
		year, err := strconv.Atoi(str)
		if err == nil && year >= 1970 && year <= 9999 {
			return time.Date(year, 1, 1, 0, 0, 0, 0, time.Local), GranularityYear, true
		}
		return time.Time{}, GranularityUnknown, false
	}

	// Handle month: 200601 or 2006-01
	if (len(str) == 6 && isDigitsOnly(str)) || (len(str) == 7 && strings.Count(str, "-") == 1) {
		var year, month int
		var err error

		if len(str) == 6 && isDigitsOnly(str) {
			year, err = strconv.Atoi(str[0:4])
			if err != nil {
				return time.Time{}, GranularityUnknown, false
			}
			month, err = strconv.Atoi(str[4:6])
			if err != nil {
				return time.Time{}, GranularityUnknown, false
			}
		} else { // 2006-01
			parts := strings.Split(str, "-")
			if len(parts) != 2 {
				return time.Time{}, GranularityUnknown, false
			}
			year, err = strconv.Atoi(parts[0])
			if err != nil {
				return time.Time{}, GranularityUnknown, false
			}
			month, err = strconv.Atoi(parts[1])
			if err != nil {
				return time.Time{}, GranularityUnknown, false
			}
		}

		if year < 1970 || year > 9999 || month < 1 || month > 12 {
			return time.Time{}, GranularityUnknown, false
		}

		return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local), GranularityMonth, true
	}

	// Handle date format: 20060102 or 2006-01-02
	if len(str) == 8 && isDigitsOnly(str) {
		// Validate year, month, day
		year, _ := strconv.Atoi(str[0:4])
		month, _ := strconv.Atoi(str[4:6])
		day, _ := strconv.Atoi(str[6:8])

		if year < 1970 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 {
			return time.Time{}, GranularityUnknown, false
		}

		// Further validate date
		if !isValidDate(year, month, day) {
			return time.Time{}, GranularityUnknown, false
		}

		// Construct time directly
		result := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
		return result, GranularityDay, true
	} else if len(str) == 10 && strings.Count(str, "-") == 2 {
		// Validate year, month, day
		parts := strings.Split(str, "-")
		if len(parts) != 3 {
			return time.Time{}, GranularityUnknown, false
		}

		year, err1 := strconv.Atoi(parts[0])
		month, err2 := strconv.Atoi(parts[1])
		day, err3 := strconv.Atoi(parts[2])

		if err1 != nil || err2 != nil || err3 != nil {
			return time.Time{}, GranularityUnknown, false
		}

		if year < 1970 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 {
			return time.Time{}, GranularityUnknown, false
		}

		// Further validate date
		if !isValidDate(year, month, day) {
			return time.Time{}, GranularityUnknown, false
		}

		// Construct time directly
		result := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
		return result, GranularityDay, true
	}

	// Handle year-month-day-hour-minute: 200601021504
	if len(str) == 12 && isDigitsOnly(str) {
		year, _ := strconv.Atoi(str[0:4])
		month, _ := strconv.Atoi(str[4:6])
		day, _ := strconv.Atoi(str[6:8])
		hour, _ := strconv.Atoi(str[8:10])
		minute, _ := strconv.Atoi(str[10:12])

		if year < 1970 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 ||
			hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return time.Time{}, GranularityUnknown, false
		}

		// Further validate date
		if !isValidDate(year, month, day) {
			return time.Time{}, GranularityUnknown, false
		}

		// Construct time directly
		result := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
		return result, GranularityMinute, true
	}

	// Handle date with time: 20060102/15:04 or 2006-01-02/15:04
	if strings.Contains(str, "/") {
		parts := strings.Split(str, "/")
		if len(parts) != 2 {
			return time.Time{}, GranularityUnknown, false
		}

		datePart := parts[0]
		timePart := parts[1]

		// Validate date part
		var year, month, day int
		var err1, err2, err3 error

		if len(datePart) == 8 && isDigitsOnly(datePart) {
			year, err1 = strconv.Atoi(datePart[0:4])
			month, err2 = strconv.Atoi(datePart[4:6])
			day, err3 = strconv.Atoi(datePart[6:8])
		} else if len(datePart) == 10 && strings.Count(datePart, "-") == 2 {
			dateParts := strings.Split(datePart, "-")
			if len(dateParts) != 3 {
				return time.Time{}, GranularityUnknown, false
			}
			year, err1 = strconv.Atoi(dateParts[0])
			month, err2 = strconv.Atoi(dateParts[1])
			day, err3 = strconv.Atoi(dateParts[2])
		} else {
			return time.Time{}, GranularityUnknown, false
		}

		if err1 != nil || err2 != nil || err3 != nil {
			return time.Time{}, GranularityUnknown, false
		}

		if year < 1970 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 {
			return time.Time{}, GranularityUnknown, false
		}

		// Further validate date
		if !isValidDate(year, month, day) {
			return time.Time{}, GranularityUnknown, false
		}

		// Validate time part
		if !regexp.MustCompile(`^\d{2}:\d{2}$`).MatchString(timePart) {
			return time.Time{}, GranularityUnknown, false
		}

		timeParts := strings.Split(timePart, ":")
		hour, err1 := strconv.Atoi(timeParts[0])
		minute, err2 := strconv.Atoi(timeParts[1])

		if err1 != nil || err2 != nil {
			return time.Time{}, GranularityUnknown, false
		}

		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return time.Time{}, GranularityUnknown, false
		}

		// Construct time directly
		result := time.Date(year, time.Month(month), day, hour, minute, 0, 0, time.Local)
		return result, GranularityMinute, true
	}

	// Handle full datetime: 20060102150405
	if len(str) == 14 && isDigitsOnly(str) {
		year, _ := strconv.Atoi(str[0:4])
		month, _ := strconv.Atoi(str[4:6])
		day, _ := strconv.Atoi(str[6:8])
		hour, _ := strconv.Atoi(str[8:10])
		minute, _ := strconv.Atoi(str[10:12])
		second, _ := strconv.Atoi(str[12:14])

		if year < 1970 || year > 9999 || month < 1 || month > 12 || day < 1 || day > 31 ||
			hour < 0 || hour > 23 || minute < 0 || minute > 59 || second < 0 || second > 59 {
			return time.Time{}, GranularityUnknown, false
		}

		// Further validate date
		if !isValidDate(year, month, day) {
			return time.Time{}, GranularityUnknown, false
		}

		// Construct time directly
		result := time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
		return result, GranularitySecond, true
	}

	// Handle timestamp (seconds)
	if isDigitsOnly(str) {
		n, err := strconv.ParseInt(str, 10, 64)
		if err == nil {
			// Check if within reasonable timestamp range (2001 to 2286 in seconds)
			if n >= 1000000000 && n <= 253402300799 {
				return time.Unix(n, 0), GranularitySecond, true
			}
		}
		return time.Time{}, GranularityUnknown, false
	}

	// Handle RFC3339: 2006-01-02T15:04:05Z07:00
	if strings.Contains(str, "T") && (strings.Contains(str, "Z") || strings.Contains(str, "+") || strings.Contains(str, "-")) {
		t, err := time.Parse(time.RFC3339, str)
		if err != nil {
			// Try format without seconds
			t, err = time.Parse("2006-01-02T15:04Z07:00", str)
		}
		if err == nil {
			return t, GranularitySecond, true
		}
	}

	// Reject all other unsupported formats
	return time.Time{}, GranularityUnknown, false
}

// TimeOf parses various time point formats
// Supported formats:
// 1. Timestamp (seconds): 1609459200
// 2. Standard date: 20060102, 2006-01-02
// 3. Date with time: 20060102/15:04, 2006-01-02/15:04
// 4. Full datetime: 20060102150405
// 5. RFC3339: 2006-01-02T15:04:05Z07:00
// 6. Relative time: 5h-ago, 3d-ago, 1w-ago, 1m-ago, 1y-ago (hour, day, week, month, year)
// 7. Natural language: now, today, yesterday
// 8. Year: 2006
// 9. Month: 200601, 2006-01
// 10. Quarter: 2006Q1, 2006Q2, 2006Q3, 2006Q4
// 11. Year-month-day-hour-minute: 200601021504
func TimeOf(str string) (t time.Time, ok bool) {
	t, _, ok = timeOf(str)
	return
}

// TimeRangeOf parses various time range formats
// Supported formats:
// 1. Single time point: determine appropriate range based on time granularity
//   - Second/minute/hour precision: expand to full day range
//   - Day precision: 00:00:00 ~ 23:59:59 of that day
//   - Month precision: first ~ last day of month
//   - Quarter precision: first ~ last day of quarter
//   - Year precision: first ~ last day of year
//
// 2. Time range: 2006-01-01~2006-01-31, 2006-01-01,2006-01-31, 2006-01-01 to 2006-01-31
// 3. Relative time: last-7d, last-30d, last-3m, last-1y (last 7 days, 30 days, 3 months, 1 year)
// 4. Specific periods: today, yesterday, this-week, last-week, this-month, last-month, this-year, last-year
// 5. all: represents all time
func TimeRangeOf(str string) (start, end time.Time, ok bool) {
	if str == "" {
		return time.Time{}, time.Time{}, false
	}

	str = strings.TrimSpace(str)

	// Handle all special case
	if strings.ToLower(str) == "all" {
		start = time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
		end = time.Date(9999, 12, 31, 23, 59, 59, 999999999, time.UTC)
		return start, end, true
	}

	// Handle relative time range: last-7d, last-30d, last-3m, last-1y
	if matched, _ := regexp.MatchString(`^last-\d+[dwmy]$`, str); matched {
		re := regexp.MustCompile(`^last-(\d+)([dwmy])$`)
		matches := re.FindStringSubmatch(str)
		if len(matches) == 3 {
			num, err := strconv.Atoi(matches[1])
			if err != nil || num <= 0 {
				return time.Time{}, time.Time{}, false
			}

			now := time.Now()
			end = time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 999999999, now.Location())

			switch matches[2] {
			case "d": // day
				start = now.AddDate(0, 0, -num)
				start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
				return start, end, true
			case "w": // week
				start = now.AddDate(0, 0, -num*7)
				start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
				return start, end, true
			case "m": // month
				start = now.AddDate(0, -num, 0)
				start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
				return start, end, true
			case "y": // year
				start = now.AddDate(-num, 0, 0)
				start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
				return start, end, true
			}
		}
	}

	// Handle time range: 2006-01-01~2006-01-31, 2006-01-01,2006-01-31, 2006-01-01 to 2006-01-31
	separators := []string{"~", ",", " to "}
	for _, sep := range separators {
		if strings.Contains(str, sep) {
			parts := strings.Split(str, sep)
			if len(parts) == 2 {
				startTime, startGran, startOk := timeOf(strings.TrimSpace(parts[0]))
				endTime, endGran, endOk := timeOf(strings.TrimSpace(parts[1]))

				if startOk && endOk {
					// Adjust time range based on granularity
					start = adjustStartTime(startTime, startGran)
					end = adjustEndTime(endTime, endGran)

					// Ensure start time is before end time
					if start.After(end) {
						// Correctly swap start and end time
						start, end = adjustStartTime(endTime, endGran), adjustEndTime(startTime, startGran)
					}

					return start, end, true
				}
			}
		}
	}

	// Handle single time point, determine appropriate range based on granularity
	t, g, ok := timeOf(str)
	if ok {
		switch g {
		case GranularitySecond, GranularityMinute, GranularityHour:
			// Second/minute/hour precision: expand to full day range
			start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			end = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
		case GranularityDay:
			// Day precision
			start = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
			end = time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
		case GranularityMonth:
			// Month precision
			start = time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
			end = time.Date(t.Year(), t.Month()+1, 0, 23, 59, 59, 999999999, t.Location())
		case GranularityQuarter:
			// Quarter precision
			quarter := (t.Month()-1)/3 + 1
			startMonth := time.Month((int(quarter)-1)*3 + 1)
			endMonth := startMonth + 2
			start = time.Date(t.Year(), startMonth, 1, 0, 0, 0, 0, t.Location())
			end = time.Date(t.Year(), endMonth+1, 0, 23, 59, 59, 999999999, t.Location())
		case GranularityYear:
			// Year precision
			start = time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
			end = time.Date(t.Year(), 12, 31, 23, 59, 59, 999999999, t.Location())
		}
		return start, end, true
	}

	return time.Time{}, time.Time{}, false
}

// adjustStartTime adjusts start time based on time granularity
func adjustStartTime(t time.Time, g TimeGranularity) time.Time {
	switch g {
	case GranularitySecond, GranularityMinute, GranularityHour:
		// For second/minute/hour precision, keep as is
		return t
	case GranularityDay:
		// Day precision: set to start of day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case GranularityMonth:
		// Month precision: set to first day of month
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case GranularityQuarter:
		// Quarter precision: set to first day of quarter
		quarter := (t.Month()-1)/3 + 1
		startMonth := time.Month((int(quarter)-1)*3 + 1)
		return time.Date(t.Year(), startMonth, 1, 0, 0, 0, 0, t.Location())
	case GranularityYear:
		// Year precision: set to first day of year
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	default:
		// Unknown granularity, default to start of day
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	}
}

// adjustEndTime adjusts end time based on time granularity
func adjustEndTime(t time.Time, g TimeGranularity) time.Time {
	switch g {
	case GranularitySecond, GranularityMinute, GranularityHour:
		// For second/minute/hour precision, keep as is
		return t
	case GranularityDay:
		// Day precision: set to end of day
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
	case GranularityMonth:
		// Month precision: set to last day of month
		return time.Date(t.Year(), t.Month()+1, 0, 23, 59, 59, 999999999, t.Location())
	case GranularityQuarter:
		// Quarter precision: set to last day of quarter
		quarter := (t.Month()-1)/3 + 1
		startMonth := time.Month((int(quarter)-1)*3 + 1)
		endMonth := startMonth + 2
		return time.Date(t.Year(), endMonth+1, 0, 23, 59, 59, 999999999, t.Location())
	case GranularityYear:
		// Year precision: set to last day of year
		return time.Date(t.Year(), 12, 31, 23, 59, 59, 999999999, t.Location())
	default:
		// Unknown granularity, default to end of day
		return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
	}
}

// isDigitsOnly checks if string contains only digits
func isDigitsOnly(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(s) > 0
}

// isValidDate checks if date is valid
func isValidDate(year, month, day int) bool {
	// Check days in month
	daysInMonth := 31

	switch month {
	case 4, 6, 9, 11:
		daysInMonth = 30
	case 2:
		// Leap year check
		if (year%4 == 0 && year%100 != 0) || year%400 == 0 {
			daysInMonth = 29
		} else {
			daysInMonth = 28
		}
	}

	return day <= daysInMonth
}

func PerfectTimeFormat(start time.Time, end time.Time) string {
	endTime := end

	// If end time is exactly midnight (00:00:00), subtract 1 second to treat as end of previous day
	if endTime.Hour() == 0 && endTime.Minute() == 0 && endTime.Second() == 0 && endTime.Nanosecond() == 0 {
		endTime = endTime.Add(-time.Second)
	}

	// Check if spans multiple years
	if start.Year() != endTime.Year() {
		return "2006-01-02 15:04:05" // Full format with year-month-day hour:minute:second
	}

	// Check if spans multiple days (within same year)
	if start.YearDay() != endTime.YearDay() {
		return "01-02 15:04:05" // Month-day hour:minute:second format
	}

	// Within the same day
	return "15:04:05" // Hour:minute:second only
}

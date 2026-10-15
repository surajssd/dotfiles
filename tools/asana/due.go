package main

import (
	"fmt"
	"time"
)

func parseDue(value string, now time.Time) (string, error) {
	switch value {
	case "today":
		return now.Format(time.DateOnly), nil
	case "tomorrow":
		return now.AddDate(0, 0, 1).Format(time.DateOnly), nil
	default:
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return "", fmt.Errorf("invalid due date %q: use YYYY-MM-DD, today, or tomorrow", value)
		}
		return value, nil
	}
}

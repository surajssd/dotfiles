package asana

import (
	"fmt"
	"net/url"
	"strings"
)

func ParseProjectURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err == nil && u.Scheme == "https" && u.Host == "app.asana.com" && u.User == nil && u.Fragment == "" {
		parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(u.Path, "/"), "/"), "/")
		if len(parts) >= 2 && parts[0] == "0" && IsGID(parts[1]) {
			return parts[1], nil
		}
		if (len(parts) == 4 || len(parts) == 5 && (parts[4] == "list" || parts[4] == "board")) &&
			parts[0] == "1" && IsGID(parts[1]) && parts[2] == "project" && IsGID(parts[3]) {
			return parts[3], nil
		}
	}
	return "", fmt.Errorf("invalid Asana project URL %q", value)
}

func IsGID(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

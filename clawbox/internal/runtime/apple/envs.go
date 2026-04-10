package apple

import (
	"fmt"
	"os"
	"strings"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

// BuildEnvArgs builds the -e environment variable arguments for a container run command.
func BuildEnvArgs(cfg *config.SessionConfig) []string {
	var envs []string

	// Default environment variables.
	envs = append(envs, "HOME=/home/node")
	envs = append(envs, "TERM=xterm-256color")
	envs = append(envs, "NODE_OPTIONS=--max-old-space-size=3072")

	// System timezone.
	tz := detectTimezone()
	if tz != "" {
		envs = append(envs, fmt.Sprintf("TZ=%s", tz))
	}

	// Session-specific env vars from YAML (can override defaults like TZ).
	for k, v := range cfg.Env {
		envs = append(envs, fmt.Sprintf("%s=%s", k, v))
	}

	return envs
}

// detectTimezone reads the system timezone from /etc/localtime.
func detectTimezone() string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return ""
	}

	// Strip everything up to and including "zoneinfo/".
	const marker = "zoneinfo/"
	idx := strings.Index(target, marker)
	if idx < 0 {
		return ""
	}

	return target[idx+len(marker):]
}

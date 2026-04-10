package apple

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
)

// BuildMountArgs builds the --mount arguments for a container run command.
func BuildMountArgs(sessionName string, cfg *config.SessionConfig) ([]string, error) {
	var mounts []string

	homeVol := config.HomeVolumeName(sessionName)
	brewVol := config.LinuxbrewVolumeName(sessionName)
	configDir := filepath.Join(config.StateDir(), sessionName, "config")

	// Case-sensitive container volumes for home and linuxbrew.
	mounts = append(mounts, fmt.Sprintf("type=volume,source=%s,target=/home/node", homeVol))
	mounts = append(mounts, fmt.Sprintf("type=volume,source=%s,target=/home/linuxbrew", brewVol))

	// Bind-mount the config dir for host-side access to openclaw.json.
	mounts = append(mounts, fmt.Sprintf("type=bind,source=%s,target=/home/node/.openclaw", configDir))
	mounts = append(mounts, fmt.Sprintf("type=bind,source=%s,target=/home/node/.openclaw/workspace",
		filepath.Join(configDir, "workspace")))

	// Extra mounts from YAML.
	for i, m := range cfg.Mounts {
		if m.Source == "" || m.Target == "" {
			return nil, fmt.Errorf("mount at index %d for session '%s' is missing source or target", i, sessionName)
		}

		spec := fmt.Sprintf("type=bind,source=%s,target=%s", m.Source, m.Target)
		if m.ReadOnly {
			spec += ",readonly"
		}
		mounts = append(mounts, spec)
	}

	// Skills directories: each subdirectory is mounted individually as readonly.
	for _, skillsDir := range cfg.Skills {
		if skillsDir == "" {
			continue
		}

		info, err := os.Stat(skillsDir)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("skills directory '%s' does not exist", skillsDir)
		}

		entries, err := os.ReadDir(skillsDir)
		if err != nil {
			return nil, fmt.Errorf("reading skills directory '%s': %w", skillsDir, err)
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			skillName := entry.Name()
			skillPath := filepath.Join(skillsDir, skillName)
			target := fmt.Sprintf("/home/node/.openclaw/workspace/skills/%s", skillName)
			mounts = append(mounts, fmt.Sprintf("type=bind,source=%s,target=%s,readonly", skillPath, target))
		}
	}

	return mounts, nil
}

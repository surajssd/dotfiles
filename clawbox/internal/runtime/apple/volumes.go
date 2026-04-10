package apple

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// volumeLsJSON represents a volume entry from "container volume ls --format json".
type volumeLsJSON struct {
	Name string `json:"name"`
}

// EnsureVolume creates a container volume if it doesn't already exist.
func (r *Runtime) EnsureVolume(name string, size string) error {
	exists, err := r.volumeExists(name)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	cmd := exec.Command("container", "volume", "create", name, "-s", size)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("creating volume '%s': %s: %w", name, strings.TrimSpace(string(out)), err)
	}
	return nil
}

// volumeExists checks if a volume with the given name exists.
func (r *Runtime) volumeExists(name string) (bool, error) {
	out, err := exec.Command("container", "volume", "ls", "--format", "json").Output()
	if err != nil {
		// Fallback to text-based check.
		textOut, textErr := exec.Command("container", "volume", "ls").Output()
		if textErr != nil {
			return false, nil
		}
		return containsExactToken(string(textOut), name), nil
	}
	return parseVolumeExists(out, name), nil
}

// parseVolumeExists checks if a volume name exists in the JSON output.
func parseVolumeExists(data []byte, name string) bool {
	var volumes []volumeLsJSON
	if err := json.Unmarshal(data, &volumes); err != nil {
		return containsExactToken(string(data), name)
	}
	for _, v := range volumes {
		if v.Name == name {
			return true
		}
	}
	return false
}

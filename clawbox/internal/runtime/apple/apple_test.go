package apple

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
)

func TestBuildRunArgs(t *testing.T) {
	t.Run("minimal with image only", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "myimage:latest"})
		assertArgs(t, args, []string{"run", "myimage:latest"})
	})

	t.Run("all flags empty", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{})
		assertArgs(t, args, []string{"run"})
	})

	t.Run("detach", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", Detach: true})
		assertContains(t, args, "-d")
	})

	t.Run("remove", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", Remove: true})
		assertContains(t, args, "--rm")
	})

	t.Run("interactive", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", Interactive: true})
		assertContains(t, args, "-it")
	})

	t.Run("cpus and memory", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", CPUs: 4, Memory: "8g"})
		assertContainsSequence(t, args, "--cpus", "4")
		assertContainsSequence(t, args, "--memory", "8g")
	})

	t.Run("uid and gid", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", UID: "501", GID: "20"})
		assertContainsSequence(t, args, "--uid", "501")
		assertContainsSequence(t, args, "--gid", "20")
	})

	t.Run("ports", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{
			Image: "img",
			Ports: []runtime.PortMapping{{Host: 8080, Container: 80}, {Host: 443, Container: 443}},
		})
		assertContainsSequence(t, args, "-p", "8080:80")
		assertContainsSequence(t, args, "-p", "443:443")
	})

	t.Run("mounts", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{
			Image:  "img",
			Mounts: []string{"type=bind,source=/a,target=/b"},
		})
		assertContainsSequence(t, args, "--mount", "type=bind,source=/a,target=/b")
	})

	t.Run("env", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{
			Image: "img",
			Env:   []string{"FOO=bar", "BAZ=qux"},
		})
		assertContainsSequence(t, args, "-e", "FOO=bar")
		assertContainsSequence(t, args, "-e", "BAZ=qux")
	})

	t.Run("name", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{Image: "img", Name: "mycontainer"})
		assertContainsSequence(t, args, "--name", "mycontainer")
	})

	t.Run("command at end", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{
			Image:   "img",
			Command: []string{"openclaw", "gateway", "--bind", "lan"},
		})
		last4 := args[len(args)-4:]
		assertArgs(t, last4, []string{"openclaw", "gateway", "--bind", "lan"})
	})

	t.Run("full combination ordering", func(t *testing.T) {
		args := buildRunArgs(runtime.RunOpts{
			Name:    "test",
			Image:   "myimage:v1",
			CPUs:    2,
			Memory:  "4g",
			UID:     "1000",
			GID:     "1000",
			Detach:  true,
			Ports:   []runtime.PortMapping{{Host: 18789, Container: 18789}},
			Mounts:  []string{"type=volume,source=vol,target=/data"},
			Env:     []string{"HOME=/home/node"},
			Command: []string{"openclaw", "gateway"},
		})
		if args[0] != "run" {
			t.Errorf("first arg = %q, want %q", args[0], "run")
		}
		imgIdx := indexOf(args, "myimage:v1")
		cmdIdx := indexOf(args, "openclaw")
		if imgIdx < 0 || cmdIdx < 0 || imgIdx >= cmdIdx {
			t.Errorf("image should appear before command")
		}
	})
}

func TestSubnetToGateway(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"192.168.65.0/24", "192.168.65.1"},
		{"10.0.0.0/8", "10.0.0.1"},
		{"172.16.0.0/16", "172.16.0.1"},
		{"192.168.1.128/25", "192.168.1.1"},
		{"not-a-cidr", ""},
		{"", ""},
		{"::1/128", ""},         // IPv6 — no To4
		{"fe80::1%eth0/64", ""}, // IPv6 link-local
	}
	for _, tt := range tests {
		if got := subnetToGateway(tt.input); got != tt.want {
			t.Errorf("subnetToGateway(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseVMNetGatewayText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"typical output", "NAME      STATUS    SUBNET\ndefault   running   192.168.65.0/24\n", "192.168.65.1"},
		{"header only", "NAME      STATUS    SUBNET\n", ""},
		{"empty", "", ""},
		{"fewer than 3 fields", "NAME STATUS\ndefault running\n", ""},
		{"extra whitespace", "NAME    STATUS    SUBNET\n  default   running   10.0.0.0/24  \n", "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseVMNetGatewayText(tt.input); got != tt.want {
				t.Errorf("parseVMNetGatewayText() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNew(t *testing.T) {
	r := New()
	if r == nil {
		t.Error("New() returned nil")
	}
}

func TestBuildMountArgs(t *testing.T) {
	t.Run("basic mounts no extras", func(t *testing.T) {
		cfg := &config.SessionConfig{}
		mounts, err := BuildMountArgs("test", cfg)
		if err != nil {
			t.Fatalf("BuildMountArgs() error: %v", err)
		}
		// Should have at least 4 base mounts (home, linuxbrew, .openclaw, workspace).
		if len(mounts) < 4 {
			t.Errorf("expected at least 4 mounts, got %d", len(mounts))
		}
		if !strings.Contains(mounts[0], "openclaw-test-home") {
			t.Errorf("first mount = %q, want home volume", mounts[0])
		}
	})

	t.Run("extra mounts", func(t *testing.T) {
		cfg := &config.SessionConfig{
			Mounts: []config.MountConfig{
				{Source: "/tmp/src", Target: "/dst", ReadOnly: false},
				{Source: "/tmp/ro", Target: "/ro", ReadOnly: true},
			},
		}
		mounts, err := BuildMountArgs("test", cfg)
		if err != nil {
			t.Fatalf("BuildMountArgs() error: %v", err)
		}
		// 4 base + 2 extra = 6.
		if len(mounts) != 6 {
			t.Errorf("expected 6 mounts, got %d", len(mounts))
		}
		if strings.Contains(mounts[4], "readonly") {
			t.Errorf("non-readonly mount should not have readonly flag: %q", mounts[4])
		}
		if !strings.Contains(mounts[5], "readonly") {
			t.Errorf("readonly mount missing readonly flag: %q", mounts[5])
		}
	})

	t.Run("mount missing source", func(t *testing.T) {
		cfg := &config.SessionConfig{
			Mounts: []config.MountConfig{{Target: "/dst"}},
		}
		_, err := BuildMountArgs("test", cfg)
		if err == nil {
			t.Error("expected error for mount with missing source")
		}
	})

	t.Run("mount missing target", func(t *testing.T) {
		cfg := &config.SessionConfig{
			Mounts: []config.MountConfig{{Source: "/src"}},
		}
		_, err := BuildMountArgs("test", cfg)
		if err == nil {
			t.Error("expected error for mount with missing target")
		}
	})

	t.Run("skills directories", func(t *testing.T) {
		// Create a temp skills dir with two subdirectories and a file.
		skillsDir := t.TempDir()
		os.MkdirAll(filepath.Join(skillsDir, "skill-a"), 0o755)
		os.MkdirAll(filepath.Join(skillsDir, "skill-b"), 0o755)
		os.WriteFile(filepath.Join(skillsDir, "not-a-dir.txt"), []byte("hi"), 0o644)

		cfg := &config.SessionConfig{Skills: []string{skillsDir}}
		mounts, err := BuildMountArgs("test", cfg)
		if err != nil {
			t.Fatalf("BuildMountArgs() error: %v", err)
		}
		// 4 base + 2 skill subdirs = 6.
		if len(mounts) != 6 {
			t.Errorf("expected 6 mounts, got %d: %v", len(mounts), mounts)
		}
	})

	t.Run("skills empty string skipped", func(t *testing.T) {
		cfg := &config.SessionConfig{Skills: []string{""}}
		mounts, err := BuildMountArgs("test", cfg)
		if err != nil {
			t.Fatalf("BuildMountArgs() error: %v", err)
		}
		if len(mounts) != 4 {
			t.Errorf("expected 4 mounts (empty skills skipped), got %d", len(mounts))
		}
	})

	t.Run("skills dir does not exist", func(t *testing.T) {
		cfg := &config.SessionConfig{Skills: []string{"/nonexistent/path"}}
		_, err := BuildMountArgs("test", cfg)
		if err == nil {
			t.Error("expected error for nonexistent skills directory")
		}
	})
}

func TestBuildEnvArgs(t *testing.T) {
	t.Run("defaults included", func(t *testing.T) {
		cfg := &config.SessionConfig{}
		envs := BuildEnvArgs(cfg)
		assertContains(t, envs, "HOME=/home/node")
		assertContains(t, envs, "TERM=xterm-256color")
		assertContains(t, envs, "NODE_OPTIONS=--max-old-space-size=3072")
	})

	t.Run("timezone detected", func(t *testing.T) {
		cfg := &config.SessionConfig{}
		envs := BuildEnvArgs(cfg)
		// On macOS, TZ should be set.
		hasTZ := false
		for _, e := range envs {
			if strings.HasPrefix(e, "TZ=") {
				hasTZ = true
			}
		}
		if !hasTZ {
			t.Log("TZ not detected — may be expected in some CI environments")
		}
	})

	t.Run("custom env vars appended", func(t *testing.T) {
		cfg := &config.SessionConfig{
			Env: map[string]string{"CUSTOM": "value", "FOO": "bar"},
		}
		envs := BuildEnvArgs(cfg)
		found := 0
		for _, e := range envs {
			if e == "CUSTOM=value" || e == "FOO=bar" {
				found++
			}
		}
		if found != 2 {
			t.Errorf("expected 2 custom env vars, found %d in %v", found, envs)
		}
	})
}

func TestDetectTimezone(t *testing.T) {
	tz := detectTimezone()
	// On macOS, this should return something like "America/Los_Angeles".
	// In CI or unusual environments, it may return "".
	if tz != "" && !strings.Contains(tz, "/") {
		t.Errorf("detectTimezone() = %q, expected either empty or tz like 'Region/City'", tz)
	}
}

func TestParseContainerRunning(t *testing.T) {
	jsonData := `[{"configuration":{"id":"openclaw-work"},"status":"running"},{"configuration":{"id":"litellm"},"status":"running"}]`

	tests := []struct {
		name string
		want bool
	}{
		{"openclaw-work", true},
		{"litellm", true},
		{"nonexistent", false},
	}
	for _, tt := range tests {
		if got := parseContainerRunning([]byte(jsonData), tt.name); got != tt.want {
			t.Errorf("parseContainerRunning(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}

	// Invalid JSON falls back to exact-token text matching.
	if !parseContainerRunning([]byte("openclaw-work is running"), "openclaw-work") {
		t.Error("text fallback should match exact token")
	}
	if parseContainerRunning([]byte("litellm is running"), "openclaw-work") {
		t.Error("text fallback should not match different name")
	}
	// Regression: "foo" must NOT match "foobar" (prefix collision).
	if parseContainerRunning([]byte("openclaw-foobar running"), "openclaw-foo") {
		t.Error("text fallback must not match prefix: openclaw-foo vs openclaw-foobar")
	}
}

func TestParseContainerList(t *testing.T) {
	jsonData := `[{"configuration":{"id":"openclaw-work"},"status":"running"},{"configuration":{"id":"openclaw-dev"},"status":"stopped"},{"configuration":{"id":"litellm"},"status":"running"}]`

	result, err := parseContainerList([]byte(jsonData), "openclaw-")
	if err != nil {
		t.Fatalf("parseContainerList() error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	if result[0].Name != "openclaw-work" || result[0].Status != "running" {
		t.Errorf("result[0] = %+v", result[0])
	}
	if result[1].Name != "openclaw-dev" || result[1].Status != "stopped" {
		t.Errorf("result[1] = %+v", result[1])
	}

	// Invalid JSON.
	_, err = parseContainerList([]byte("not json"), "x")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	// Empty list.
	result, _ = parseContainerList([]byte("[]"), "openclaw-")
	if len(result) != 0 {
		t.Errorf("expected 0 results for empty list, got %d", len(result))
	}
}

func TestParseVMNetGateway(t *testing.T) {
	t.Run("gateway directly available", func(t *testing.T) {
		data := `[{"id":"default","state":"running","status":{"ipv4Subnet":"192.168.65.0/24","ipv4Gateway":"192.168.65.1"}}]`
		gw, err := parseVMNetGateway([]byte(data))
		if err != nil || gw != "192.168.65.1" {
			t.Errorf("parseVMNetGateway() = %q, %v", gw, err)
		}
	})

	t.Run("fallback to subnet", func(t *testing.T) {
		data := `[{"id":"default","state":"running","status":{"ipv4Subnet":"10.0.0.0/24","ipv4Gateway":""}}]`
		gw, err := parseVMNetGateway([]byte(data))
		if err != nil || gw != "10.0.0.1" {
			t.Errorf("parseVMNetGateway() = %q, %v", gw, err)
		}
	})

	t.Run("no networks", func(t *testing.T) {
		gw, err := parseVMNetGateway([]byte("[]"))
		if err != nil || gw != "" {
			t.Errorf("parseVMNetGateway() = %q, %v", gw, err)
		}
	})

	t.Run("no gateway or subnet", func(t *testing.T) {
		data := `[{"id":"default","state":"running","status":{}}]`
		gw, err := parseVMNetGateway([]byte(data))
		if err != nil || gw != "" {
			t.Errorf("parseVMNetGateway() = %q, %v", gw, err)
		}
	})

	t.Run("invalid JSON falls back to text", func(t *testing.T) {
		text := "NAME      STATUS    SUBNET\ndefault   running   172.16.0.0/24\n"
		gw, err := parseVMNetGateway([]byte(text))
		if err != nil || gw != "172.16.0.1" {
			t.Errorf("parseVMNetGateway() = %q, %v", gw, err)
		}
	})
}

func TestParseVolumeExists(t *testing.T) {
	jsonData := `[{"name":"openclaw-work-home"},{"name":"openclaw-work-linuxbrew"}]`

	if !parseVolumeExists([]byte(jsonData), "openclaw-work-home") {
		t.Error("should find openclaw-work-home")
	}
	if parseVolumeExists([]byte(jsonData), "nonexistent") {
		t.Error("should not find nonexistent")
	}

	// Invalid JSON falls back to exact-token text matching.
	if !parseVolumeExists([]byte("openclaw-work-home volume"), "openclaw-work-home") {
		t.Error("text fallback should match exact token")
	}
	if parseVolumeExists([]byte("other-volume"), "openclaw-work-home") {
		t.Error("text fallback should not match different name")
	}
	// Regression: "openclaw-work" must NOT match "openclaw-work-home" (prefix collision).
	if parseVolumeExists([]byte("openclaw-work-home linuxbrew"), "openclaw-work") {
		t.Error("text fallback must not match prefix: openclaw-work vs openclaw-work-home")
	}
}

func TestContainsExactToken(t *testing.T) {
	tests := []struct {
		text string
		name string
		want bool
	}{
		{"openclaw-foo running", "openclaw-foo", true},
		{"openclaw-foobar running", "openclaw-foo", false},
		{"name openclaw-foo status", "openclaw-foo", true},
		{"openclaw-foo", "openclaw-foo", true},
		{"", "openclaw-foo", false},
		{"openclaw-foo\nopenclaw-bar", "openclaw-bar", true},
		{"openclaw-foo\nopenclaw-bar", "openclaw-ba", false},
	}
	for _, tt := range tests {
		if got := containsExactToken(tt.text, tt.name); got != tt.want {
			t.Errorf("containsExactToken(%q, %q) = %v, want %v", tt.text, tt.name, got, tt.want)
		}
	}
}

func TestBuildMountArgsReadDirError(t *testing.T) {
	// Create a skills dir that is a file, not a directory.
	tmpDir := t.TempDir()
	fakeSkillsDir := filepath.Join(tmpDir, "not-a-dir")
	os.WriteFile(fakeSkillsDir, []byte("i am a file"), 0o644)

	cfg := &config.SessionConfig{Skills: []string{fakeSkillsDir}}
	_, err := BuildMountArgs("test", cfg)
	if err == nil {
		t.Error("expected error when skills path is a file not a directory")
	}
}

// Test helpers.

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("got %d args %v, want %d args %v", len(got), got, len(want), want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("arg[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func assertContains(t *testing.T, args []string, val string) {
	t.Helper()
	if indexOf(args, val) < 0 {
		t.Errorf("args %v should contain %q", args, val)
	}
}

func assertContainsSequence(t *testing.T, args []string, a, b string) {
	t.Helper()
	for i := 0; i < len(args)-1; i++ {
		if args[i] == a && args[i+1] == b {
			return
		}
	}
	t.Errorf("args %v should contain sequence [%q, %q]", args, a, b)
}

func indexOf(args []string, val string) int {
	for i, a := range args {
		if a == val {
			return i
		}
	}
	return -1
}

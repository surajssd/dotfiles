package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime/apple"
	"github.com/surajssd/dotfiles/clawbox/internal/state"
)

var setupCmd = &cobra.Command{
	Use:   "setup <session-name>",
	Short: "🔧 Run initial onboarding for a new session",
	Args:  cobra.ExactArgs(1),
	RunE:  runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	sessionName := args[0]
	cfg, err := validateAndGetSession(sessionName)
	if err != nil {
		return err
	}

	// Ensure state directories and volumes.
	if err := state.EnsureDirs(sessionName); err != nil {
		return err
	}
	if err := appState.Runtime.EnsureVolume(config.HomeVolumeName(sessionName), config.VolumeSize); err != nil {
		return err
	}
	if err := appState.Runtime.EnsureVolume(config.LinuxbrewVolumeName(sessionName), config.VolumeSize); err != nil {
		return err
	}

	// Build mount and env args.
	mounts, err := apple.BuildMountArgs(sessionName, cfg)
	if err != nil {
		return err
	}
	envs := apple.BuildEnvArgs(cfg)

	// Get host UID/GID.
	uid, gid, err := getHostIDs()
	if err != nil {
		return err
	}

	// Hardcoded 4g memory for setup (matching bash behavior).
	const setupMemory = "4g"

	// Run onboarding.
	fmt.Printf("🔧 Running onboarding for session '%s'...\n", sessionName)
	if err := appState.Runtime.RunInteractive(runtime.RunOpts{
		Image:       cfg.Image,
		CPUs:        cfg.Resources.CPUs,
		Memory:      setupMemory,
		UID:         uid,
		GID:         gid,
		Remove:      true,
		Interactive: true,
		Mounts:      mounts,
		Env:         envs,
		Command:     []string{"openclaw", "onboard", "--mode", "local", "--no-install-daemon"},
	}); err != nil {
		return fmt.Errorf("onboarding failed: %w", err)
	}

	// Set gateway mode to local.
	fmt.Println("⚙️  Setting gateway mode to local...")
	if err := appState.Runtime.Run(runtime.RunOpts{
		Image:   cfg.Image,
		CPUs:    cfg.Resources.CPUs,
		Memory:  setupMemory,
		UID:     uid,
		GID:     gid,
		Remove:  true,
		Mounts:  mounts,
		Env:     envs,
		Command: []string{"openclaw", "config", "set", "gateway.mode", "local"},
	}); err != nil {
		return fmt.Errorf("setting gateway mode: %w", err)
	}

	// Allow Control UI access.
	fmt.Println("⚙️  Allowing Control UI access...")
	if err := appState.Runtime.Run(runtime.RunOpts{
		Image:   cfg.Image,
		CPUs:    cfg.Resources.CPUs,
		Memory:  setupMemory,
		UID:     uid,
		GID:     gid,
		Remove:  true,
		Mounts:  mounts,
		Env:     envs,
		Command: []string{"openclaw", "config", "set", "gateway.controlUi.dangerouslyAllowHostHeaderOriginFallback", "true"},
	}); err != nil {
		return fmt.Errorf("setting Control UI access: %w", err)
	}

	fmt.Printf("✅ Setup complete for session '%s'.\n", sessionName)
	fmt.Println()

	// Auto-start the session.
	return runStart(cmd, args)
}

// getHostIDs returns the host UID and GID as strings.
func getHostIDs() (string, string, error) {
	uidOut, err := exec.Command("id", "-u").Output()
	if err != nil {
		return "", "", fmt.Errorf("getting host UID: %w", err)
	}
	gidOut, err := exec.Command("id", "-g").Output()
	if err != nil {
		return "", "", fmt.Errorf("getting host GID: %w", err)
	}

	uid := strings.TrimSpace(string(uidOut))
	gid := strings.TrimSpace(string(gidOut))
	return uid, gid, nil
}

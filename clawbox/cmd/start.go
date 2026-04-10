package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/clawbox/internal/config"
	"github.com/surajssd/dotfiles/clawbox/internal/health"
	"github.com/surajssd/dotfiles/clawbox/internal/proxy"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime"
	"github.com/surajssd/dotfiles/clawbox/internal/runtime/apple"
	"github.com/surajssd/dotfiles/clawbox/internal/state"
)

var startCmd = &cobra.Command{
	Use:   "start <session-name>",
	Short: "🚀 Start an OpenClaw session container",
	Args:  cobra.ExactArgs(1),
	RunE:  runStart,
}

func runStart(cmd *cobra.Command, args []string) error {
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

	// Verify setup has been completed.
	if !state.ConfigExists(sessionName) {
		return fmt.Errorf("session '%s' has not been set up yet. Run setup first:\n  clawbox setup %s", sessionName, sessionName)
	}

	cname := config.ContainerName(sessionName)

	// Three-way container check: running, stopped, or create new.
	running, err := appState.Runtime.IsRunning(cname)
	if err != nil {
		return err
	}
	if running {
		fmt.Printf("🟢 Container '%s' is already running.\n", cname)
		_ = appState.ProxyManager.Start(sessionName, cfg) // non-fatal
		appState.ProxyManager.InjectEnv(sessionName, cname, cfg)
		printConnectionInfo(sessionName, cfg.Ports.Gateway)
		return nil
	}

	exists, err := appState.Runtime.Exists(cname)
	if err != nil {
		return err
	}
	if exists {
		fmt.Printf("♻️  Restarting existing container '%s'\n", cname)
		if err := appState.Runtime.Start(cname); err != nil {
			return fmt.Errorf("starting container: %w", err)
		}
		health.WaitForHealthy(sessionName, cname, cfg.Ports.Gateway)
		_ = appState.ProxyManager.Start(sessionName, cfg) // non-fatal
		appState.ProxyManager.InjectEnv(sessionName, cname, cfg)
		printConnectionInfo(sessionName, cfg.Ports.Gateway)
		return nil
	}

	// Create new container.
	mounts, err := apple.BuildMountArgs(sessionName, cfg)
	if err != nil {
		return err
	}
	envs := apple.BuildEnvArgs(cfg)
	uid, gid, err := getHostIDs()
	if err != nil {
		return err
	}

	fmt.Printf("🚀 Starting OpenClaw session '%s' as container '%s'\n", sessionName, cname)
	if err := appState.Runtime.Run(runtime.RunOpts{
		Name:   cname,
		Image:  cfg.Image,
		CPUs:   cfg.Resources.CPUs,
		Memory: cfg.Resources.Memory,
		UID:    uid,
		GID:    gid,
		Detach: true,
		Ports: []runtime.PortMapping{
			{Host: cfg.Ports.Gateway, Container: config.ContainerGatewayPort},
			{Host: cfg.Ports.Bridge, Container: config.ContainerBridgePort},
		},
		Mounts:  mounts,
		Env:     envs,
		Command: []string{"openclaw", "gateway", "--bind", "lan", "--port", fmt.Sprintf("%d", config.ContainerGatewayPort)},
	}); err != nil {
		return fmt.Errorf("creating container: %w", err)
	}

	health.WaitForHealthy(sessionName, cname, cfg.Ports.Gateway)

	if proxy.IsEnabled(cfg) {
		_ = appState.ProxyManager.Start(sessionName, cfg) // non-fatal
		appState.ProxyManager.InjectEnv(sessionName, cname, cfg)
	}

	printConnectionInfo(sessionName, cfg.Ports.Gateway)
	return nil
}

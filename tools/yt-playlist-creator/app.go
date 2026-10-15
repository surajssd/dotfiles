package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

type dependencies struct {
	stdin            io.Reader
	stdout           io.Writer
	stderr           io.Writer
	httpClient       *http.Client
	apiBaseURL       string
	oauthEndpoint    oauth2.Endpoint
	listen           func(context.Context) (net.Listener, error)
	authorizationURL func(string) error
	platform         string
	authTimeout      time.Duration
	securityTimeout  time.Duration
	searchTimeout    time.Duration
}

func realDependencies() dependencies {
	return dependencies{
		stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiBaseURL: "https://www.googleapis.com/youtube/v3",
		oauthEndpoint: oauth2.Endpoint{
			AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token",
			AuthStyle: oauth2.AuthStyleInParams,
		},
		listen: func(ctx context.Context) (net.Listener, error) {
			var config net.ListenConfig
			return config.Listen(ctx, "tcp4", "127.0.0.1:0")
		},
		platform: runtime.GOOS, authTimeout: 5 * time.Minute,
		securityTimeout: 30 * time.Second, searchTimeout: 60 * time.Second,
	}
}

func newCommand(deps dependencies) *cobra.Command {
	root := &cobra.Command{
		Use: "yt-playlist-creator", Short: "Create private YouTube playlists for YouTube Music",
		SilenceErrors: true, SilenceUsage: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetIn(deps.stdin)
	root.SetOut(deps.stdout)
	root.SetErr(deps.stderr)
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		_, _ = fmt.Fprint(deps.stderr, cmd.UsageString())
		return err
	})
	auth := &cobra.Command{
		Use: "auth", Short: "Enroll credentials on macOS", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	var clientSecrets string
	login := &cobra.Command{
		Use: "login", Short: "Authorize a Desktop app and save its refresh token in Keychain", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return authLogin(cmd.Context(), deps, clientSecrets)
		},
	}
	login.Flags().StringVar(&clientSecrets, "client-secrets", "", "downloaded Google Desktop app OAuth client JSON")
	auth.AddCommand(login)
	root.AddCommand(auth, newCreateCommand(deps))
	return root
}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

const (
	authTestClient  = "synthetic-desktop.apps.googleusercontent.com"
	authTestSecret  = "synthetic-client-secret"
	authTestRefresh = "synthetic-refresh-token"
	authTestAccess  = "synthetic-access-token"
	authTestCode    = "synthetic-authorization-code"
)

type authRequest struct {
	method, path, authorization, contentType string
	form                                     url.Values
}

type authFixture struct {
	deps                          dependencies
	stdout, stderr                bytes.Buffer
	clientPath, binary, directory string
	callsPath, inputPath, stored  string
	server                        *httptest.Server
	requests                      chan authRequest
	tokenCalls, apiCalls          atomic.Int32
	authorizations                []string
	callbackValues                func(url.Values)
	tokenHandler                  http.HandlerFunc
	callbackAddress               string
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USER", "playlist-tester")
	t.Setenv(credentialService, "")
	f := &authFixture{
		directory: dir, clientPath: filepath.Join(dir, "desktop.json"),
		binary: filepath.Join(dir, "security"), callsPath: filepath.Join(dir, "calls"),
		inputPath: filepath.Join(dir, "input"), stored: filepath.Join(dir, "stored"),
		requests: make(chan authRequest, 32),
	}
	script, err := os.ReadFile("testdata/security.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.binary, script, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/bash", filepath.Join(dir, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("SECURITY_CALLS_FILE", f.callsPath)
	t.Setenv("SECURITY_INPUT_FILE", f.inputPath)
	t.Setenv("SECURITY_STORED_FILE", f.stored)
	for _, name := range []string{"SECURITY_DIAGNOSTIC", "SECURITY_BLOCK", "SECURITY_WRITE_EXIT", "SECURITY_SKIP_WRITE", "SECURITY_READ_EXIT"} {
		t.Setenv(name, "")
	}
	f.writeStored(t, authTestBundle("existing-refresh-token"))
	f.writeClient(t, `{"installed":{"client_id":"`+authTestClient+`","client_secret":"`+authTestSecret+`","auth_uri":"https://untrusted.example/auth","token_uri":"https://untrusted.example/token","redirect_uris":["http://localhost"]}}`)
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse request form: %v", err)
		}
		f.requests <- authRequest{r.Method, r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("Content-Type"), r.PostForm}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/token" {
			f.tokenCalls.Add(1)
			if f.tokenHandler != nil {
				f.tokenHandler(w, r)
				return
			}
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600,"refresh_token":%q}`, authTestAccess, authTestRefresh)
			return
		}
		f.apiCalls.Add(1)
		_, _ = io.WriteString(w, `{}`)
	}))
	t.Cleanup(f.server.Close)
	f.deps = realDependencies()
	f.deps.stdin = strings.NewReader("")
	f.deps.stdout, f.deps.stderr = &f.stdout, &f.stderr
	f.deps.httpClient = f.server.Client()
	f.deps.httpClient.Timeout = time.Second
	f.deps.apiBaseURL = f.server.URL + "/api"
	f.deps.oauthEndpoint = oauth2.Endpoint{AuthURL: f.server.URL + "/authorize", TokenURL: f.server.URL + "/token"}
	f.deps.platform = "darwin"
	f.deps.authTimeout, f.deps.securityTimeout = 2*time.Second, time.Second
	f.deps.authorizationURL = func(raw string) error {
		f.authorizations = append(f.authorizations, raw)
		return f.callback(raw)
	}
	return f
}

func authTestBundle(refresh string) credentials {
	return credentials{ClientID: authTestClient, ClientSecret: authTestSecret, RefreshToken: refresh}
}

func (f *authFixture) writeClient(t *testing.T, value string) {
	t.Helper()
	if err := os.WriteFile(f.clientPath, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *authFixture) writeStored(t *testing.T, bundle credentials) {
	t.Helper()
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.stored, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (f *authFixture) callback(raw string) error {
	authorization, err := url.Parse(raw)
	if err != nil {
		return err
	}
	redirect, err := url.Parse(authorization.Query().Get("redirect_uri"))
	if err != nil {
		return err
	}
	f.callbackAddress = redirect.Host
	query := url.Values{"state": {authorization.Query().Get("state")}, "code": {authTestCode}}
	if f.callbackValues != nil {
		f.callbackValues(query)
	}
	redirect.RawQuery = query.Encode()
	response, err := f.server.Client().Get(redirect.String())
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	_, err = io.Copy(io.Discard, response.Body)
	return err
}

func (f *authFixture) execute(ctx context.Context) int {
	return run(ctx, f.deps, []string{"auth", "login", "--client-secrets", f.clientPath})
}

func (f *authFixture) noExternalWork(t *testing.T) {
	t.Helper()
	if len(f.authorizations) != 0 || len(f.requests) != 0 {
		t.Errorf("authorization URLs = %d, requests = %d, want none", len(f.authorizations), len(f.requests))
	}
	if _, err := os.Stat(f.callsPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("security was invoked: %v", err)
	}
}

func (f *authFixture) failure(t *testing.T, code int, message string) {
	t.Helper()
	if code != 1 || f.stdout.Len() != 0 || !strings.Contains(f.stderr.String(), message) || strings.Contains(f.stderr.String(), "Credentials saved") {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q, want %q", code, &f.stdout, &f.stderr, message)
	}
	f.noLeakedSecrets(t)
}

func (f *authFixture) noLeakedSecrets(t *testing.T) {
	t.Helper()
	output := f.stdout.String() + f.stderr.String()
	for _, secret := range []string{authTestSecret, authTestRefresh, authTestAccess, authTestCode} {
		if strings.Contains(output, secret) {
			t.Errorf("output contains a synthetic secret")
		}
	}
}

func (f *authFixture) closedListener(t *testing.T) {
	t.Helper()
	if f.callbackAddress == "" {
		t.Fatal("callback listener address was not recorded")
	}
	connection, err := net.DialTimeout("tcp", f.callbackAddress, 100*time.Millisecond)
	if err == nil {
		_ = connection.Close()
		t.Error("callback listener is still open after login returned")
	}
}

func TestAuthLoginPKCEAndKeychain(t *testing.T) {
	f := newAuthFixture(t)
	var previousState, previousChallenge string
	for range 2 {
		f.stdout.Reset()
		f.stderr.Reset()
		if code := f.execute(context.Background()); code != 0 {
			t.Fatalf("exit = %d, stderr = %s", code, &f.stderr)
		}
		if f.stdout.Len() != 0 || !strings.Contains(f.stderr.String(), "Credentials saved in macOS Keychain.") {
			t.Fatalf("stdout = %q, stderr = %q", &f.stdout, &f.stderr)
		}
		f.noLeakedSecrets(t)
		f.closedListener(t)
		authorization, err := url.Parse(f.authorizations[len(f.authorizations)-1])
		if err != nil {
			t.Fatal(err)
		}
		query := authorization.Query()
		if authorization.Scheme+"://"+authorization.Host+authorization.Path != f.server.URL+"/authorize" {
			t.Fatalf("authorization endpoint came from the client file: %s", authorization)
		}
		for key, want := range map[string]string{"client_id": authTestClient, "response_type": "code", "scope": youtubeScope, "access_type": "offline", "prompt": "consent", "code_challenge_method": "S256"} {
			if query.Get(key) != want {
				t.Errorf("authorization %s = %q, want %q", key, query.Get(key), want)
			}
		}
		redirect, err := url.Parse(query.Get("redirect_uri"))
		if err != nil || redirect.Scheme != "http" || redirect.Hostname() != "127.0.0.1" || redirect.Port() == "" || redirect.Path != "/oauth2/callback" {
			t.Fatalf("redirect = %v, error = %v", redirect, err)
		}
		request := <-f.requests
		if request.method != http.MethodPost || request.path != "/token" || request.authorization != "" || request.contentType != "application/x-www-form-urlencoded" {
			t.Errorf("token request metadata = %#v", request)
		}
		for key, want := range map[string]string{"grant_type": "authorization_code", "client_id": authTestClient, "client_secret": authTestSecret, "code": authTestCode, "redirect_uri": redirect.String()} {
			if request.form.Get(key) != want {
				t.Errorf("token form %s does not match", key)
			}
		}
		verifier := request.form.Get("code_verifier")
		digest := sha256.Sum256([]byte(verifier))
		challenge := base64.RawURLEncoding.EncodeToString(digest[:])
		if len(verifier) < 43 || len(query.Get("state")) < 43 || query.Get("code_challenge") != challenge {
			t.Errorf("state or PKCE verification failed")
		}
		if previousState == query.Get("state") || previousChallenge == challenge {
			t.Error("authorization reused state or PKCE material")
		}
		previousState, previousChallenge = query.Get("state"), challenge
	}
	if f.tokenCalls.Load() != 2 || f.apiCalls.Load() != 0 || len(f.requests) != 0 {
		t.Errorf("token calls = %d, API calls = %d, remaining requests = %d", f.tokenCalls.Load(), f.apiCalls.Load(), len(f.requests))
	}
	calls, err := os.ReadFile(f.callsPath)
	if err != nil {
		t.Fatal(err)
	}
	wantCall := "CALL\n-i\nCALL\nfind-generic-password\n-a\nplaylist-tester\n-s\n" + credentialService + "\n-w\n"
	if string(calls) != strings.Repeat(wantCall, 2) {
		t.Errorf("security arguments = %q", calls)
	}
	stdin, err := os.ReadFile(f.inputPath)
	if err != nil {
		t.Fatal(err)
	}
	prefix := `add-generic-password -U -a "playlist-tester" -s YT_PLAYLIST_CREATOR_OAUTH -D "api key" -X `
	if !bytes.HasPrefix(stdin, []byte(prefix)) || bytes.Count(stdin, []byte("\n")) != 1 || len(stdin) >= 4095 {
		t.Fatalf("invalid security stdin command")
	}
	decoded, err := hex.DecodeString(strings.TrimSuffix(strings.TrimPrefix(string(stdin), prefix), "\n"))
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := parseCredentials(decoded)
	if err != nil || bundle != authTestBundle(authTestRefresh) {
		t.Errorf("saved bundle differs from the authorized credentials: %v", err)
	}
}

func TestAuthClientValidationBeforeConsent(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"empty", "", "installed Desktop app"},
		{"malformed", `{`, "installed Desktop app"},
		{"web client", `{"web":{"client_id":"id","client_secret":"secret"}}`, "installed Desktop app"},
		{"null", `{"installed":null}`, "installed Desktop app"},
		{"wrong structure", `[]`, "installed Desktop app"},
		{"missing ID", `{"installed":{"client_secret":"secret"}}`, "installed Desktop app"},
		{"missing secret", `{"installed":{"client_id":"id"}}`, "installed Desktop app"},
		{"blank ID", `{"installed":{"client_id":"  ","client_secret":"secret"}}`, "installed Desktop app"},
		{"wrong type", `{"installed":{"client_id":123,"client_secret":"secret"}}`, "installed Desktop app"},
		{"trailing JSON", `{"installed":{"client_id":"id","client_secret":"secret"}} {}`, "installed Desktop app"},
		{"invalid UTF-8", "{\"installed\":\"\xff\"}", "installed Desktop app"},
		{"size", strings.Repeat(" ", (1<<20)+1), "read client secrets"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.writeClient(t, tc.content)
			f.failure(t, f.execute(context.Background()), tc.want)
			f.noExternalWork(t)
		})
	}
}

func TestAuthLoginPrerequisites(t *testing.T) {
	for _, name := range []string{"missing flag", "empty flag", "missing file", "missing security", "missing account", "invalid account", "other platform"} {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(t)
			args := []string{"auth", "login", "--client-secrets", f.clientPath}
			want := "--client-secrets is required"
			switch name {
			case "missing flag":
				args = []string{"auth", "login"}
			case "empty flag":
				args[3] = ""
			case "missing file":
				args[3] += ".missing"
				want = "open client secrets"
			case "missing security":
				if err := os.Remove(f.binary); err != nil {
					t.Fatal(err)
				}
				want = "security was not found"
			case "missing account", "invalid account":
				account := ""
				if name == "invalid account" {
					account = "name\nsecond-command"
				}
				t.Setenv("USER", account)
				want = "USER must name"
			case "other platform":
				f.deps.platform = "linux"
				want = "requires macOS"
			}
			f.failure(t, run(context.Background(), f.deps, args), want)
			f.noExternalWork(t)
		})
	}
}

func TestAuthHelpWithoutCredentials(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"auth"}, {"auth", "--help"}, {"auth", "login", "--help"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			f := newAuthFixture(t)
			f.deps.platform = "linux"
			t.Setenv(credentialService, "invalid")
			if code := run(context.Background(), f.deps, args); code != 0 || !strings.Contains(f.stdout.String(), "Usage:") || f.stderr.Len() != 0 {
				t.Errorf("exit = %d, stdout = %q, stderr = %q", code, &f.stdout, &f.stderr)
			}
			f.noExternalWork(t)
		})
	}
}

func TestAuthCallbackFailures(t *testing.T) {
	cases := []struct {
		name, want string
		modify     func(url.Values)
	}{
		{"wrong state", "invalid state", func(q url.Values) { q.Set("state", "wrong-state") }},
		{"missing state", "invalid state", func(q url.Values) { q.Del("state") }},
		{"repeated state", "invalid state", func(q url.Values) { q.Add("state", q.Get("state")) }},
		{"denied", "denied", func(q url.Values) { q.Set("error", "access_denied"); q.Set("error_description", authTestSecret) }},
		{"missing code", "authorization code", func(q url.Values) { q.Del("code") }},
		{"empty code", "authorization code", func(q url.Values) { q.Set("code", " ") }},
		{"repeated code", "authorization code", func(q url.Values) { q.Add("code", "other-code") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.callbackValues = tc.modify
			f.failure(t, f.execute(context.Background()), tc.want)
			f.closedListener(t)
			if len(f.requests) != 0 {
				t.Error("invalid callback caused a token request")
			}
			if _, err := os.Stat(f.callsPath); !errors.Is(err, os.ErrNotExist) {
				t.Error("invalid callback invoked security")
			}
		})
	}
}

func TestAuthDeadlineAndCancellation(t *testing.T) {
	for _, name := range []string{"deadline", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f.deps.authTimeout = 50 * time.Millisecond
			f.deps.authorizationURL = func(raw string) error {
				authorization, _ := url.Parse(raw)
				redirect, _ := url.Parse(authorization.Query().Get("redirect_uri"))
				f.callbackAddress = redirect.Host
				if name == "cancellation" {
					cancel()
				}
				return nil
			}
			started := time.Now()
			f.failure(t, f.execute(ctx), "authorization did not finish")
			if time.Since(started) > time.Second {
				t.Error("authorization did not return promptly")
			}
			f.closedListener(t)
			f.noExternalWork(t)
		})
	}
}

func TestAuthTokenErrorsAreRedacted(t *testing.T) {
	cases := []struct {
		name, body, want string
		status           int
	}{
		{"revoked", `{"error":"invalid_grant","error_description":"` + authTestRefresh + `","error_uri":"https://example.invalid/` + authTestCode + `"}`, "expired or was revoked", 400},
		{"scope", `{"error":"invalid_scope","error_description":"` + authTestSecret + `"}`, "permission", 400},
		{"client", `{"error":"invalid_client","error_description":"` + authTestSecret + `"}`, "client was rejected", 401},
		{"raw response", authTestAccess + authTestSecret, "authorization failed", 500},
		{"malformed success", `{"access_token":`, "invalid token", 200},
		{"missing refresh", `{"access_token":"` + authTestAccess + `","token_type":"Bearer"}`, "no refresh token", 200},
		{"blank refresh", `{"access_token":"` + authTestAccess + `","refresh_token":" "}`, "no refresh token", 200},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}
			f.failure(t, f.execute(context.Background()), tc.want)
			f.closedListener(t)
			if f.tokenCalls.Load() != 1 || f.apiCalls.Load() != 0 {
				t.Errorf("token calls = %d, API calls = %d", f.tokenCalls.Load(), f.apiCalls.Load())
			}
			if _, err := os.Stat(f.callsPath); !errors.Is(err, os.ErrNotExist) {
				t.Error("failed token exchange invoked security")
			}
		})
	}
}

func TestAuthKeychainFailuresDoNotReportSuccess(t *testing.T) {
	for _, name := range []string{"write failure", "readback failure", "unchanged value", "oversize bundle"} {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(t)
			t.Setenv("SECURITY_DIAGNOSTIC", authTestSecret+authTestRefresh)
			want := "save Keychain credentials"
			switch name {
			case "write failure":
				t.Setenv("SECURITY_WRITE_EXIT", "1")
			case "readback failure":
				t.Setenv("SECURITY_READ_EXIT", "1")
				want = "could not verify"
			case "unchanged value":
				t.Setenv("SECURITY_SKIP_WRITE", "1")
				want = "could not verify"
			case "oversize bundle":
				f.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(w, `{"access_token":%q,"refresh_token":%q}`, authTestAccess, strings.Repeat("r", 2500))
				}
				want = "Keychain command limit"
			}
			f.failure(t, f.execute(context.Background()), want)
			if name != "readback failure" {
				stored, err := os.ReadFile(f.stored)
				if err != nil {
					t.Fatal(err)
				}
				bundle, err := parseCredentials(keychainValue(stored))
				if err != nil || bundle != authTestBundle("existing-refresh-token") {
					t.Errorf("a failed save changed existing credentials: %v", err)
				}
			}
			if name == "oversize bundle" {
				if _, err := os.Stat(f.callsPath); !errors.Is(err, os.ErrNotExist) {
					t.Error("oversize bundle invoked security")
				}
			}
		})
	}
}

func TestKeychainQuotingAndNonASCIIReadback(t *testing.T) {
	f := newAuthFixture(t)
	account := "name \"quotes\" \\ dollar$ semicolon;"
	bundle := authTestBundle("refresh-世界-\"-\\-\n")
	if err := saveCredentials(context.Background(), f.deps, f.binary, account, bundle); err != nil {
		t.Fatal(err)
	}
	command, err := os.ReadFile(f.inputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(command), `add-generic-password -U -a "name \"quotes\" \\ dollar$ semicolon;" -s `) {
		t.Error("Keychain account was not quoted for the security parser")
	}
	t.Setenv("USER", account)
	loaded, err := loadCredentials(context.Background(), f.deps)
	if err != nil || loaded != bundle {
		t.Errorf("non-ASCII credential readback failed: %v", err)
	}
	calls, err := os.ReadFile(f.callsPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(calls, []byte(bundle.ClientSecret)) || bytes.Contains(calls, []byte(bundle.RefreshToken)) || bytes.Contains(calls, []byte(hex.EncodeToString([]byte(bundle.RefreshToken)))) {
		t.Error("credentials were passed in process arguments")
	}
}

func TestCredentialSourcePrecedence(t *testing.T) {
	valid, err := json.Marshal(authTestBundle(authTestRefresh))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, environment, platform string
		want                        credentials
		wantError, usesKeychain     bool
	}{
		{name: "environment on macOS", environment: string(valid), platform: "darwin", want: authTestBundle(authTestRefresh)},
		{name: "environment on Linux", environment: string(valid), platform: "linux", want: authTestBundle(authTestRefresh)},
		{name: "empty environment", platform: "darwin", want: authTestBundle("existing-refresh-token"), usesKeychain: true},
		{name: "missing on Linux", platform: "linux", wantError: true},
		{name: "whitespace", environment: " \n\t", platform: "darwin", wantError: true},
		{name: "malformed", environment: "{", platform: "darwin", wantError: true},
		{name: "missing field", environment: `{"client_id":"id","client_secret":"secret"}`, platform: "darwin", wantError: true},
		{name: "unknown field", environment: strings.TrimSuffix(string(valid), "}") + `,"unknown":"value"}`, platform: "darwin", wantError: true},
		{name: "trailing JSON", environment: string(valid) + " {}", platform: "darwin", wantError: true},
		{name: "null", environment: "null", platform: "darwin", wantError: true},
		{name: "wrong type", environment: `{"client_id":true}`, platform: "darwin", wantError: true},
		{name: "invalid UTF-8", environment: string(valid) + "\xff", platform: "darwin", wantError: true},
		{name: "oversize", environment: strings.Repeat(" ", (64<<10)+1), platform: "darwin", wantError: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.deps.platform = tc.platform
			t.Setenv(credentialService, tc.environment)
			loaded, err := loadCredentials(context.Background(), f.deps)
			if (err != nil) != tc.wantError || !tc.wantError && loaded != tc.want {
				t.Errorf("credential source did not match: error = %v", err)
			}
			if err != nil && !strings.Contains(err.Error(), credentialService) {
				t.Errorf("error does not name the credential source: %v", err)
			}
			_, called := os.Stat(f.callsPath)
			if tc.usesKeychain && called != nil || !tc.usesKeychain && !errors.Is(called, os.ErrNotExist) {
				t.Errorf("Keychain called = %t, want %t", called == nil, tc.usesKeychain)
			}
			if len(f.requests) != 0 || len(f.authorizations) != 0 {
				t.Error("credential loading attempted login or an HTTP request")
			}
		})
	}
}

func TestSecurityFailuresAreBoundedAndRedacted(t *testing.T) {
	for _, name := range []string{"read failure", "read timeout", "write timeout", "oversize output", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(t)
			f.deps.securityTimeout = 50 * time.Millisecond
			t.Setenv("SECURITY_DIAGNOSTIC", authTestSecret+authTestRefresh)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "read failure":
				t.Setenv("SECURITY_READ_EXIT", "1")
			case "read timeout":
				t.Setenv("SECURITY_BLOCK", "read")
			case "write timeout":
				t.Setenv("SECURITY_BLOCK", "write")
			case "oversize output":
				f.deps.securityTimeout = time.Second
				if err := os.WriteFile(f.stored, []byte(strings.Repeat("x", (64<<10)+1)+"\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "cancelled":
				cancel()
			}
			started := time.Now()
			var err error
			switch name {
			case "write timeout":
				err = saveCredentials(ctx, f.deps, f.binary, "playlist-tester", authTestBundle(authTestRefresh))
			case "oversize output":
				_, err = runSecurity(ctx, f.deps, f.binary, nil, "find-generic-password")
			default:
				_, err = loadCredentials(ctx, f.deps)
			}
			if err == nil {
				t.Fatal("expected failure")
			}
			if strings.Contains(err.Error(), authTestSecret) || strings.Contains(err.Error(), authTestRefresh) {
				t.Error("security failure leaked credentials")
			}
			if time.Since(started) > 2*time.Second {
				t.Error("security failure did not return within the bounded wait")
			}
		})
	}
}

func TestOAuthRefreshReuseAndRotation(t *testing.T) {
	for _, expired := range []bool{false, true} {
		t.Run(fmt.Sprintf("expired=%t", expired), func(t *testing.T) {
			f := newAuthFixture(t)
			if expired {
				f.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
					_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":1,"refresh_token":"rotated-refresh"}`, authTestAccess)
				}
			}
			client := newYouTubeClient(context.Background(), f.deps, authTestBundle(authTestRefresh))
			for range 2 {
				var result map[string]any
				if err := client.request(context.Background(), http.MethodGet, "/check", nil, &result); err != nil {
					t.Fatal(err)
				}
			}
			wantTokens := int32(1)
			if expired {
				wantTokens = 2
			}
			if f.tokenCalls.Load() != wantTokens || f.apiCalls.Load() != 2 {
				t.Errorf("token calls = %d, API calls = %d", f.tokenCalls.Load(), f.apiCalls.Load())
			}
			tokenIndex := 0
			for len(f.requests) > 0 {
				request := <-f.requests
				if request.path == "/token" {
					refresh := authTestRefresh
					if tokenIndex > 0 {
						refresh = "rotated-refresh"
					}
					want := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {authTestClient}, "client_secret": {authTestSecret}}
					if !reflect.DeepEqual(request.form, want) || request.authorization != "" {
						t.Error("token refresh used the wrong form or authentication style")
					}
					tokenIndex++
				} else if request.authorization != "Bearer "+authTestAccess || request.method != http.MethodGet || request.path != "/api/check" {
					t.Errorf("API request metadata is incorrect")
				}
			}
			if _, err := os.Stat(f.callsPath); !errors.Is(err, os.ErrNotExist) {
				t.Error("in-memory token refresh accessed Keychain")
			}
		})
	}
}

func TestOAuthTokenRequestTimeoutAndCancellation(t *testing.T) {
	for _, name := range []string{"timeout", "cancellation"} {
		t.Run(name, func(t *testing.T) {
			f := newAuthFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			release := make(chan struct{})
			defer close(release)
			f.deps.httpClient.Timeout = 50 * time.Millisecond
			f.tokenHandler = func(_ http.ResponseWriter, r *http.Request) {
				if name == "cancellation" {
					cancel()
				}
				select {
				case <-r.Context().Done():
				case <-release:
				}
			}
			client := newYouTubeClient(ctx, f.deps, authTestBundle(authTestRefresh))
			var result map[string]any
			started := time.Now()
			err := client.request(ctx, http.MethodPost, "/check", map[string]string{"name": "test"}, &result)
			if err == nil || outcomeUnknown(err) {
				t.Fatalf("failed token request = %v, want a known pre-mutation failure", err)
			}
			if time.Since(started) > time.Second || f.tokenCalls.Load() != 1 || f.apiCalls.Load() != 0 {
				t.Errorf("bounded token request: duration = %s, token calls = %d, API calls = %d", time.Since(started), f.tokenCalls.Load(), f.apiCalls.Load())
			}
		})
	}
}

func TestOAuthFailureDoesNotRetryOrSendMutation(t *testing.T) {
	for _, status := range []int{301, 302, 307, 308, 400, 401, 403, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := newAuthFixture(t)
			f.tokenHandler = func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Location", f.server.URL+"/redirected")
				w.WriteHeader(status)
				_, _ = fmt.Fprintf(w, `{"error":"invalid_grant","error_description":%q}`, authTestSecret+authTestRefresh+authTestAccess)
			}
			client := newYouTubeClient(context.Background(), f.deps, authTestBundle(authTestRefresh))
			var result map[string]any
			err := client.request(context.Background(), http.MethodPost, "/check", map[string]string{"name": "test"}, &result)
			if err == nil || outcomeUnknown(err) || !strings.Contains(err.Error(), "auth login") {
				t.Fatalf("failed refresh = %v, want a known pre-mutation failure", err)
			}
			if f.tokenCalls.Load() != 1 || f.apiCalls.Load() != 0 {
				t.Errorf("token calls = %d, API calls = %d, want 1 and 0", f.tokenCalls.Load(), f.apiCalls.Load())
			}
			if strings.Contains(err.Error(), authTestSecret) || strings.Contains(err.Error(), authTestRefresh) || strings.Contains(err.Error(), authTestAccess) {
				t.Error("OAuth failure leaked credentials")
			}
		})
	}
}

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

func TestCommand(t *testing.T) {
	for _, tc := range []struct {
		args  []string
		words int
	}{
		{[]string{}, 6},
		{[]string{"-n", "7"}, 7},
		{[]string{"--num", "1"}, 1},
	} {
		var stdout, stderr bytes.Buffer
		cmd := newCommand(&stdout, &stderr)
		cmd.SetArgs(tc.args)
		if code := run(context.Background(), cmd); code != 0 || stderr.Len() != 0 {
			t.Fatalf("args %v: exit %d, stderr %q", tc.args, code, stderr.String())
		}
		password := strings.TrimSuffix(stdout.String(), "\n")
		count := 0
		if len(password) < 3 || password[len(password)-2] < '0' || password[len(password)-2] > '9' || !strings.ContainsRune("!@#$%&*_-", rune(password[len(password)-1])) {
			t.Fatal("expected a trailing digit and restricted symbol")
		}
		for _, char := range password[:len(password)-2] {
			if char >= 'A' && char <= 'Z' {
				count++
			} else if (char < 'a' || char > 'z') && char != '-' {
				t.Fatalf("args %v: unexpected character %q", tc.args, char)
			}
		}
		if count != tc.words || stdout.String() != password+"\n" {
			t.Fatalf("args %v: got %d words or incorrect newline", tc.args, count)
		}
	}
}

func TestCommandLength(t *testing.T) {
	for _, flag := range []string{"-l", "--length"} {
		for _, length := range []int{2, 10, 32} {
			var stdout, stderr bytes.Buffer
			cmd := newCommand(&stdout, &stderr)
			cmd.SetArgs([]string{flag, fmt.Sprint(length)})
			if code := run(context.Background(), cmd); code != 0 || stderr.Len() != 0 {
				t.Fatalf("%s %d: exit %d, stderr %q", flag, length, code, stderr.String())
			}
			password := strings.TrimSuffix(stdout.String(), "\n")
			if len(password) != length || !strings.HasSuffix(stdout.String(), "\n") || password[length-2] < '0' || password[length-2] > '9' || !strings.ContainsRune("!@#$%&*_-", rune(password[length-1])) {
				t.Fatalf("%s %d: wrong length or missing suffix", flag, length)
			}
		}
	}
}

func TestCommandCounts(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--length", "10", "--digits", "2", "--symbols", "2"}, "Abacus00!!"},
		{[]string{"-l", "10", "-d", "2", "-s", "2"}, "Abacus00!!"},
		{[]string{"-n", "7", "-d", "2", "-s", "2"}, strings.Repeat("Abacus", 7) + "00@@"},
	} {
		args := tc.args
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		child := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestCommandProcess$", "--"}, args...)...)
		child.Env = append(os.Environ(), "DICEWARE_PROCESS_TEST=flags")
		var stderr bytes.Buffer
		child.Stderr = &stderr
		stdout, err := child.Output()
		cancel()
		want := tc.want + "\n"
		if err != nil || string(stdout) != want || stderr.Len() != 0 {
			t.Fatalf("args %v: output %q, error %v, stderr %q", args, stdout, err, stderr.String())
		}
	}
}

func TestCommandRejectsArguments(t *testing.T) {
	for _, args := range [][]string{
		{"-n", "0"}, {"-n", "-1"}, {"-n", "abc"}, {"-n", "1.5"}, {"-n"},
		{"--specials", "1"}, {"--unknown"}, {"extra"},
		{"--digits", "0"}, {"--digits", "-1"}, {"--digits", "abc"}, {"--digits"},
		{"-s", "0"}, {"-s", "-1"}, {"-s", "abc"}, {"-s"}, {"--symbols", "0"}, {"--symbols", "-1"}, {"--symbols", "abc"}, {"--symbols"},
		{"-d"}, {"-d", "0"}, {"-l"}, {"-l", "-1"}, {"--max-length", "10"},
		{"-l", "0"}, {"--length", "0"}, {"--length", "-1"}, {"--length", "abc"}, {"--length"},
		{"-n", "6", "--length", "10"}, {"--length", "1"},
	} {
		var stdout, stderr bytes.Buffer
		cmd := newCommand(&stdout, &stderr)
		cmd.SetArgs(args)
		if code := run(context.Background(), cmd); code != 1 || stdout.Len() != 0 || stderr.Len() == 0 {
			t.Fatalf("args %v: exit %d, stdout %q, stderr %q", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestCommandHelpAndCancellation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cmd := newCommand(&stdout, &stderr)
	cmd.SetArgs([]string{"--help"})
	if code := run(context.Background(), cmd); code != 0 || stderr.Len() != 0 {
		t.Fatalf("help exit %d, stderr %q", code, stderr.String())
	}
	for _, text := range []string{"six", "one", "-d, --digits", "-s, --symbols", "-l, --length", "!@#$%&*_-", "--num"} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("help missing %q", text)
		}
	}
	for _, removed := range []string{"--specials", "replacement", "--max-length"} {
		if strings.Contains(stdout.String(), removed) {
			t.Errorf("help includes removed feature %q", removed)
		}
	}
	stdout.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd = newCommand(&stdout, &stderr)
	cmd.SetArgs([]string{})
	if code := run(ctx, cmd); code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("canceled exit %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
}

func TestCommandProcess(t *testing.T) {
	mode := os.Getenv("DICEWARE_PROCESS_TEST")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	cmd := newCommand(os.Stdout, os.Stderr)
	if mode == "flags" {
		rand.Reader = bytes.NewReader(append(append(make([]byte, 14), 0, 0, 1, 1), make([]byte, 20)...))
		cmd.SetArgs(os.Args[3:])
		os.Exit(run(context.Background(), cmd))
	}
	cmd.SetArgs([]string{"-n", "2147483647"})
	cmd.PreRun = func(_ *cobra.Command, _ []string) {
		ready := os.NewFile(3, "ready")
		_, _ = fmt.Fprintln(ready, "ready")
		_ = ready.Close()
	}
	os.Exit(run(context.Background(), cmd))
}

func TestCommandSignals(t *testing.T) {
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			ready, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = ready.Close() }()
			defer func() { _ = writer.Close() }()
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			child := exec.CommandContext(ctx, executable, "-test.run=^TestCommandProcess$")
			child.Env = append(os.Environ(), "DICEWARE_PROCESS_TEST=signal")
			child.ExtraFiles = []*os.File{writer}
			var stdout, stderr bytes.Buffer
			child.Stdout, child.Stderr = &stdout, &stderr
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = child.Process.Kill() })
			_ = writer.Close()
			_ = ready.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, err := io.ReadFull(ready, make([]byte, 6)); err != nil {
				_ = child.Process.Kill()
				_ = child.Wait()
				t.Fatal(err)
			}
			if err := child.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			if err := child.Wait(); err == nil || ctx.Err() != nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), "context canceled") {
				t.Fatalf("signal result: %v, stdout %q, stderr %q", err, stdout.String(), stderr.String())
			}
		})
	}
}

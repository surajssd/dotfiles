package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/surajssd/dotfiles/tools/diceware"
)

func main() {
	os.Exit(run(context.Background(), newCommand(os.Stdout, os.Stderr)))
}

func run(ctx context.Context, cmd *cobra.Command) int {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.ExecuteContext(ctx); err != nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "error:", err)
		return 1
	}
	return 0
}

func newCommand(stdout, stderr io.Writer) *cobra.Command {
	options := diceware.Options{}
	cmd := &cobra.Command{
		Use:   "diceware",
		Short: "Generate a password from EFF words",
		Long: `Generate six capitalized EFF words joined without a separator by default.
Use -l/--length for an exact total character length. Select whole words that fit,
then fill space too short for a word with random letters and digits.
Without --length, -n/--num selects the word count (default six).
The suffix defaults to one random digit and one random symbol; both counts
must be at least one. -d/--digits and -s/--symbols change the appended counts.
Symbols use only !@#$%&*_-.
The required digit and symbol counts are included within --length.`,
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if cmd.Flags().Changed("length") {
				if options.Length < 1 {
					return fmt.Errorf("length must be a positive whole number")
				}
				options.WordCount = 0
			}
			if options.Digits < 1 || options.Symbols < 1 {
				return fmt.Errorf("digit and symbol counts must be at least 1")
			}
			password, err := diceware.Generate(cmd.Context(), options)
			if err != nil {
				return err
			}
			if err := cmd.Context().Err(); err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), password)
			return err
		},
	}
	cmd.CompletionOptions.DisableDefaultCmd = true
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.Flags().IntVarP(&options.Length, "length", "l", 0, "total characters, including digits and symbols")
	cmd.Flags().IntVarP(&options.WordCount, "num", "n", 6, "number of words when --length is absent")
	cmd.MarkFlagsMutuallyExclusive("length", "num")
	cmd.Flags().IntVarP(&options.Digits, "digits", "d", 1, "random digits to append (at least 1)")
	cmd.Flags().IntVarP(&options.Symbols, "symbols", "s", 1, "random symbols to append from !@#$%&*_- (at least 1)")
	return cmd
}

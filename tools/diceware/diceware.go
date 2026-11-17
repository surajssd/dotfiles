package diceware

import (
	"context"
	"crypto/rand"
	_ "embed"
	"fmt"
	"io"
	"math/big"
	"strings"
)

//go:embed eff_large_wordlist.txt
var wordList string

var words = func() []string {
	fields := strings.Fields(wordList)
	list := make([]string, 0, len(fields)/2)
	for i := 1; i < len(fields); i += 2 {
		list = append(list, fields[i])
	}
	return list
}()

const (
	digitChars  = "0123456789"
	symbolChars = "!@#$%&*_-"
	fillerChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
)

// Options requires exactly one positive WordCount or Length and nonnegative
// remaining counts. Length includes the requested digit and symbol suffix.
type Options struct {
	WordCount int
	Digits    int
	Symbols   int
	Length    int
}

// Generate selects EFF words using cryptographic randomness,
// capitalizes their first letters, and joins them without a delimiter or newline.
// With Length set, it selects words that fit, then fills any space too short for
// a word with random letters and digits. It appends the requested digit and symbol
// suffix within that length. Cancellation is cooperative and cannot interrupt an
// entropy read. Errors return no password.
func Generate(ctx context.Context, options Options) (string, error) {
	return generate(ctx, rand.Reader, options)
}

func generate(ctx context.Context, entropy io.Reader, options Options) (string, error) {
	if options.WordCount < 0 || options.Length < 0 || (options.WordCount > 0) == (options.Length > 0) {
		return "", fmt.Errorf("specify either a positive word count or a positive length")
	}
	if options.Digits < 0 || options.Symbols < 0 {
		return "", fmt.Errorf("digit and symbol counts must be nonnegative whole numbers")
	}
	wordBudget := options.Length
	if options.Length > 0 {
		if options.Digits > wordBudget || options.Symbols > wordBudget-options.Digits {
			return "", fmt.Errorf("length %d cannot fit %d digits and %d symbols", options.Length, options.Digits, options.Symbols)
		}
		wordBudget -= options.Digits
		wordBudget -= options.Symbols
	}
	var joined strings.Builder
	for selected := 0; options.Length > 0 || selected < options.WordCount; selected++ {
		candidates := words
		if options.Length > 0 {
			candidates = nil
			for _, word := range words {
				if len(word) <= wordBudget-joined.Len() {
					candidates = append(candidates, word)
				}
			}
			if len(candidates) == 0 {
				break
			}
		}
		index, err := draw(ctx, entropy, len(candidates))
		if err != nil {
			return "", err
		}
		word := candidates[index]
		joined.WriteByte(word[0] - 'a' + 'A')
		joined.WriteString(word[1:])
	}
	for options.Length > 0 && joined.Len() < wordBudget {
		index, err := draw(ctx, entropy, len(fillerChars))
		if err != nil {
			return "", err
		}
		joined.WriteByte(fillerChars[index])
	}
	password := []byte(joined.String())
	for range options.Digits {
		index, err := draw(ctx, entropy, len(digitChars))
		if err != nil {
			return "", err
		}
		password = append(password, digitChars[index])
	}
	for range options.Symbols {
		index, err := draw(ctx, entropy, len(symbolChars))
		if err != nil {
			return "", err
		}
		password = append(password, symbolChars[index])
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return string(password), nil
}

func draw(ctx context.Context, entropy io.Reader, size int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	index, err := rand.Int(entropy, big.NewInt(int64(size)))
	if err != nil {
		return 0, fmt.Errorf("read random bytes: %w", err)
	}
	return int(index.Int64()), nil
}

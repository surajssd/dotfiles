package diceware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestEFFWordList(t *testing.T) {
	const checksum = "addd35536511597a02fa0a9ff1e5284677b8883b83e986e43f15a3db996b903e"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(wordList))); got != checksum {
		t.Fatalf("word list checksum = %s, want %s", got, checksum)
	}
	rows := strings.Split(strings.TrimSpace(wordList), "\n")
	if len(rows) != 7776 || len(words) != len(rows) {
		t.Fatalf("got %d rows and %d words, want 7776", len(rows), len(words))
	}
	codes, uniqueWords := map[string]bool{}, map[string]bool{}
	for i, row := range rows {
		fields := strings.Fields(row)
		if len(fields) != 2 || len(fields[0]) != 5 {
			t.Fatalf("invalid row %d: %q", i+1, row)
		}
		for _, digit := range fields[0] {
			if digit < '1' || digit > '6' {
				t.Fatalf("invalid dice code %q", fields[0])
			}
		}
		for _, char := range fields[1] {
			if (char < 'a' || char > 'z') && char != '-' {
				t.Fatalf("invalid word %q", fields[1])
			}
		}
		if codes[fields[0]] || uniqueWords[fields[1]] || words[i] != fields[1] {
			t.Fatalf("duplicate or mismatched row %q", row)
		}
		codes[fields[0]], uniqueWords[fields[1]] = true, true
	}
}

func TestGenerateSelections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		count   int
		entropy []byte
		want    string
	}{
		{"first", 1, []byte{0, 0}, "Abacus"},
		{"last", 1, []byte{0x1e, 0x5f}, "Zoom"},
		{"repeated", 2, []byte{0, 0, 0, 0}, "AbacusAbacus"},
		{"ordered", 2, []byte{0x1e, 0x5f, 0, 0}, "ZoomAbacus"},
		{"reject out of range", 1, []byte{0x1e, 0x60, 0, 0}, "Abacus"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generate(context.Background(), bytes.NewReader(tc.entropy), Options{WordCount: tc.count})
			if err != nil || got != tc.want {
				t.Fatalf("generate = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestGenerateInvalidCounts(t *testing.T) {
	for _, options := range []Options{
		{}, {WordCount: -1},
		{WordCount: 1, Digits: -1},
		{WordCount: 1, Symbols: -1},
		{Length: -1}, {WordCount: 1, Length: 10},
	} {
		reader := &observedReader{Reader: bytes.NewReader(nil)}
		got, err := generate(context.Background(), reader, options)
		if err == nil || got != "" || reader.reads != 0 {
			t.Fatalf("options %+v = %q, %v, reads %d", options, got, err, reader.reads)
		}
	}
}

func TestGenerateSuffix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options Options
		entropy []byte
		want    string
	}{
		{"digits", Options{WordCount: 1, Digits: 2}, []byte{0, 0, 0, 9}, "Abacus09"},
		{"symbols", Options{WordCount: 1, Symbols: 2}, []byte{0, 0, 0, 8}, "Abacus!-"},
		{"digits then symbols", Options{WordCount: 1, Digits: 1, Symbols: 1}, []byte{0, 0, 9, 8}, "Abacus9-"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generate(context.Background(), bytes.NewReader(tc.entropy), tc.options)
			if err != nil || got != tc.want {
				t.Fatalf("generate = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
}

func TestGenerateLength(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options Options
		entropy []byte
		want    string
	}{
		{"fitting words", Options{Length: 10, Digits: 1, Symbols: 1}, []byte{0, 2, 0, 7, 0}, "AbideAim7!"},
		{"filler", Options{Length: 10, Digits: 1, Symbols: 1}, []byte{0, 0, 0, 52, 7, 0}, "AbacusA07!"},
		{"short dictionary word", Options{Length: 5, Digits: 1, Symbols: 1}, []byte{0, 7, 0}, "Aim7!"},
		{"no word fits", Options{Length: 4, Digits: 1, Symbols: 1}, []byte{0, 52, 7, 0}, "A07!"},
		{"suffix only", Options{Length: 2, Digits: 1, Symbols: 1}, []byte{9, 8}, "9-"},
		{"exact word fit", Options{Length: 8, Digits: 1, Symbols: 1}, []byte{0, 0, 7, 0}, "Abacus7!"},
		{"library without suffix", Options{Length: 1}, []byte{0}, "A"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := generate(context.Background(), bytes.NewReader(tc.entropy), tc.options)
			if err != nil || got != tc.want || len(got) != tc.options.Length {
				t.Fatalf("generate = %q, %v, want %q", got, err, tc.want)
			}
		})
	}
	for _, options := range []Options{
		{Length: 1, Digits: 1, Symbols: 1},
		{Length: 1, Digits: int(^uint(0) >> 1), Symbols: int(^uint(0) >> 1)},
	} {
		reader := &observedReader{Reader: bytes.NewReader(nil)}
		got, err := generate(context.Background(), reader, options)
		if err == nil || got != "" || reader.reads != 0 || !strings.Contains(err.Error(), "cannot fit") {
			t.Fatalf("options %+v = %q, %v, reads %d", options, got, err, reader.reads)
		}
	}
}

func TestGenerateLengthFailureAndCancellation(t *testing.T) {
	options := Options{Length: 10, Digits: 1, Symbols: 1}
	for available := range 6 {
		got, err := generate(context.Background(), bytes.NewReader(make([]byte, available)), options)
		if got != "" || (!errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF)) {
			t.Fatalf("available %d = %q, %v", available, got, err)
		}
	}
	for after := 1; after <= 5; after++ {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &observedReader{Reader: bytes.NewReader(make([]byte, 20))}
		reader.afterRead = func() {
			if reader.reads == after {
				cancel()
			}
		}
		got, err := generate(ctx, reader, options)
		cancel()
		if got != "" || !errors.Is(err, context.Canceled) || reader.reads != after {
			t.Fatalf("cancel after %d = %q, %v, reads %d", after, got, err, reader.reads)
		}
	}
}

func TestGenerateSuffixFailureAndCancellation(t *testing.T) {
	for _, available := range []int{2, 3} {
		got, err := generate(context.Background(), bytes.NewReader(make([]byte, available)), Options{WordCount: 1, Digits: 1, Symbols: 1})
		if got != "" || !errors.Is(err, io.EOF) {
			t.Fatalf("available %d = %q, %v", available, got, err)
		}
	}
	for _, after := range []int{2, 3} {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &observedReader{Reader: bytes.NewReader(make([]byte, 10))}
		reader.afterRead = func() {
			if reader.reads == after {
				cancel()
			}
		}
		got, err := generate(ctx, reader, Options{WordCount: 1, Digits: 1, Symbols: 1})
		cancel()
		if got != "" || !errors.Is(err, context.Canceled) || reader.reads != after {
			t.Fatalf("cancel after %d = %q, %v, reads %d", after, got, err, reader.reads)
		}
	}
}

func TestGenerateEntropyFailure(t *testing.T) {
	for _, available := range []int{0, 2, 4, 5} {
		got, err := generate(context.Background(), bytes.NewReader(make([]byte, available)), Options{WordCount: 2, Digits: 1, Symbols: 1})
		if got != "" || !errors.Is(err, io.EOF) {
			t.Fatalf("available %d = %q, %v, want empty value and EOF", available, got, err)
		}
	}
}

type observedReader struct {
	io.Reader
	reads     int
	afterRead func()
}

func (r *observedReader) Read(p []byte) (int, error) {
	r.reads++
	n, err := r.Reader.Read(p)
	if r.afterRead != nil {
		r.afterRead()
	}
	return n, err
}

func TestGenerateCancellation(t *testing.T) {
	for _, cancelAfter := range []int{0, 1, 2, 3, 4} {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &observedReader{Reader: bytes.NewReader(make([]byte, 20))}
		reader.afterRead = func() {
			if reader.reads == cancelAfter {
				cancel()
			}
		}
		if cancelAfter == 0 {
			cancel()
		}
		got, err := generate(ctx, reader, Options{WordCount: 2, Digits: 1, Symbols: 1})
		cancel()
		if got != "" || !errors.Is(err, context.Canceled) || reader.reads != cancelAfter {
			t.Fatalf("cancel after %d = %q, %v, reads %d", cancelAfter, got, err, reader.reads)
		}
	}
}

func TestGenerate(t *testing.T) {
	got, err := Generate(context.Background(), Options{WordCount: 6})
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, char := range got {
		if char >= 'A' && char <= 'Z' {
			count++
		} else if (char < 'a' || char > 'z') && char != '-' {
			t.Fatalf("unexpected character %q", char)
		}
	}
	if count != 6 {
		t.Fatalf("got %d words, want 6", count)
	}
}

func ExampleGenerate() {
	password, err := Generate(context.Background(), Options{WordCount: 6})
	if err != nil {
		panic(err)
	}
	fmt.Println(password)
}

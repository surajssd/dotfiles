# Diceware

Generate passwords with an independent Go library or the `diceware` command. The module embeds the 7,776-entry EFF long English word list and needs no Python, external generator, configuration file, or network access at runtime.

## Install and use

From the public dotfiles checkout, run:

```bash
make install-diceware
diceware
diceware -l 10
diceware --length 32
diceware -l 20 -d 2 -s 2
diceware -n 7
```

The installer builds `./cmd/diceware` into `$(go env GOPATH)/bin`. Rebuild after changing Go code. `make install-all` includes this command and skips its build when Go is absent.

Without `--length`, the default is six independently selected EFF words, joined without a separator, followed by one random digit and one random symbol. Words are capitalized at their first letter and may repeat. `-n/--num` changes the word count in this mode. Word counts must be positive whole numbers.

`-l/--length N` produces exactly N total characters, including the digit and symbol suffix. It reserves space for the suffix, selects each whole word uniformly from dictionary entries that fit the remaining space, and continues while a word fits. When no word fits, random uppercase letters, lowercase letters, and digits fill the remainder. Words are never truncated. Selecting only fitting entries is equivalent to retrying oversized words, without an unbounded retry loop. For example, a ten-character result can be `AbideAim7!`.

Do not combine `--length` with `--num`. The length must be positive and must fit the requested suffix. With the default one digit and one symbol, the minimum is two characters. If no dictionary word fits, the result uses random letters and digits plus the suffix. A shorter total length reduces password strength.

`-d/--digits N` appends N random digits, then `-s/--symbols N` appends N random symbols from `!@#$%&*_-`. Both counts default to one and must be positive whole numbers. Zero is rejected so every command result includes a digit and a symbol. Filler can contain additional digits. Each random choice uses uniform cryptographic randomness.

Successful output contains only the password and one newline. Errors go to stderr and exit with status 1, with no password on stdout. Positional arguments are rejected.

SIGINT and SIGTERM cancel generation. Cancellation is cooperative: it is checked before random draws and before success, but cannot interrupt an operating-system entropy read already in progress. An observed cancellation returns no password.

## Library

Import `github.com/surajssd/dotfiles/tools/diceware` and choose either an explicit word count or an exact length:

```go
password, err := diceware.Generate(context.Background(), diceware.Options{
    Length: 10,
    Digits: 1,
    Symbols: 1,
})
if err != nil {
    return err
}
```

`Generate(ctx context.Context, options Options) (string, error)` returns no newline and performs no printing, clipboard access, or signal handling. `Options` contains `WordCount`, `Length`, `Digits`, and `Symbols`. Exactly one of `WordCount` and `Length` must be positive; the other must be zero. Digit and symbol counts must be nonnegative. The library accepts zero suffix counts; both commands require at least one of each. Invalid counts, a length too small for the suffix, entropy errors, and observed cancellation return an empty value and an error. Callers own command defaults and signal handling. The library exposes no alternative random source or mutable word list.

## Data and license

The original Go code uses the [MIT license](LICENSE). The embedded EFF data uses CC BY 4.0. See [NOTICE.md](NOTICE.md) for attribution, the source URL, and its SHA-256 checksum.

## Verify

```bash
gofmt -w *.go cmd/diceware/*.go
go vet ./...
golangci-lint run
go test ./...
go test -race ./...
```

Tests validate the data checksum, all 7,776 rows, unique words and dice codes, deterministic selections and suffixes, fitting-word selection, exact lengths, filler, entropy errors, command output, and cancellation. Signal tests run child processes and never access a clipboard.

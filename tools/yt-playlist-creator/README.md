# YouTube playlist creator

`yt-playlist-creator` creates one private YouTube playlist from an ordered list of video references or song queries. It returns a YouTube Music URL. An agent can extract songs from an article or a page and send the list as JSON; the CLI never fetches or parses source pages.

YouTube Music shows only music videos from playlists created in YouTube. The Data API has no documented Music eligibility check, so this tool cannot guarantee that every accepted video appears or plays in Music. It does not infer eligibility from category, title, or channel.

## Install

Run these commands from the repository root:

```bash
make install-yt-playlist-creator
make install-skills
brew install yt-dlp
```

The Go installer requires Go 1.26 or newer and installs the binary into `$(go env GOPATH)/bin`. It may download dependencies or a compatible toolchain. It skips when Go is absent and fails on build errors. `make install-all` includes both the binary and skill. Rebuild the binary after changing Go code.

`yt-dlp` is needed only when a track is a query. Exact video IDs and links do not need it. The Homebrew bootstrap package list includes it. The agent skill lives in `skill/SKILL.md` and is linked as `yt-playlist-creator` in both `~/.claude/skills/` and `~/.agents/skills/`. A private skill with that name takes precedence.

## Authorize an account

### Set up the Google client

1. Create or select a Google Cloud project and [enable YouTube Data API v3](https://developers.google.com/youtube/v3/getting-started).
2. Configure the Google Auth Platform consent screen and audience. If the app is in Testing, add the intended Google account as a test user.
3. Create an OAuth client with application type **Desktop app**. Download its client JSON outside this repository.
4. Ensure the selected account has a YouTube channel and can create playlists. An API key or service account does not replace user authorization.
5. Run the login command on a Mac:

```bash
yt-playlist-creator auth login --client-secrets "$HOME/Downloads/client_secret.json"
```

6. Open the URL printed on stderr in a browser that can reach the same machine. Select the intended YouTube account and grant consent. Successful enrollment prints `Credentials saved in macOS Keychain.` on stderr.

Login binds only `127.0.0.1` on an available port. It uses a random state, an authorization code, offline access, and PKCE with S256. It stops after five minutes. Endpoints come from the application, not arbitrary URLs in the downloaded client file. It does not use a code-copy flow or browser cookies. Normal `create` commands never start login or request interactive consent.

### Permissions and token lifetime

The requested scope is `https://www.googleapis.com/auth/youtube.force-ssl`. Google grants broader account permissions than private playlist creation alone. The playlist write endpoints do not list a playlist-create-only scope.

External OAuth apps in Testing normally receive refresh tokens that expire after seven days when YouTube scopes are requested. Publishing an app does not remove all verification requirements or account-policy constraints. If credentials expire or are revoked, run `auth login` again.

### Credential sources

A nonempty `YT_PLAYLIST_CREATOR_OAUTH` environment variable takes precedence. It contains one JSON object with nonblank string fields `client_id`, `client_secret`, and `refresh_token`. An invalid nonempty value fails instead of falling back to a different account.

Otherwise, the CLI reads the macOS Keychain entry with service `YT_PLAYLIST_CREATOR_OAUTH` and account `$USER`. Login updates that entry with kind `api key`. No credential bundle, authorization code, or raw OAuth response is printed. Access tokens refresh in memory. The CLI does not store plaintext token files or use `apikey.sh`.

Enrollment requires macOS. Another host can use an environment bundle provisioned by a person through its secret manager. Keep client files and bundles out of the repository, shell history, logs, and chat. A remote agent cannot complete normal loopback consent unless a browser can reach the machine running the CLI.

### Keychain stdin behavior

A pipe test on 2026-10-10 with a separate synthetic keychain confirmed that trailing `security add-generic-password ... -w` is not a safe raw-stdin interface. It prompts twice and truncates password input at 128 bytes on the tested system.

The CLI instead starts `security -i` directly, without a shell, and passes exactly one command line through stdin:

```text
add-generic-password -U -a "<escaped account>" -s YT_PLAYLIST_CREATOR_OAUTH -D "api key" -X <hex-encoded JSON bytes>
```

The complete line, including one final newline, must be fewer than 4095 bytes because the command reader has a fixed buffer. The CLI rejects larger bundles before writing. Hex data is secret and is never put in process arguments or logs. The CLI never uses `-v`, which would echo the command.

After the write, the CLI reads the entry back and checks its exact content before reporting success. `find-generic-password -w` can print the whole value as hex when it contains non-ASCII or nonprintable bytes; both representations are handled. Updates do not delete the entry first. A timeout or failed verification can occur after a write commits, so inspect Keychain before assuming that a failed enrollment left the old value unchanged.

## Create a playlist

### Flags

```bash
yt-playlist-creator create \
  --title "Evening music" \
  --description "An ordered listening list" \
  --track "https://music.youtube.com/watch?v=VIDEO_ID_01" \
  --track "VIDEO_ID_02" \
  --track "Song title Artist name"
```

`VIDEO_ID_01` and `VIDEO_ID_02` are placeholders, not track recommendations. Repeat `--track` to preserve order. Commas inside a track remain part of that track.

A successful command prints only the new playlist URL:

```text
https://music.youtube.com/playlist?list=<playlist-id>
```

Every invocation creates a new playlist. Titles are not idempotency keys.

### JSON file or stdin

```json
{
  "title": "Evening music",
  "description": "https://example.com/song-list",
  "tracks": ["VIDEO_ID_01", "Song title Artist name"]
}
```

```bash
yt-playlist-creator create --file playlist.json --output json
yt-playlist-creator create --file - --output json < playlist.json
```

The title must be nonblank. Supply between 1 and 100 tracks. Title and description are preserved; an omitted description becomes empty. Use either `--file` or the title/description/track flags, never both, including explicitly empty conflicting flags. Only `--file -` reads stdin. Missing input fails rather than waiting for stdin.

JSON must be exactly one object with the case-sensitive fields `title`, `description`, and `tracks`. Unknown or repeated fields, wrong types, null values, invalid UTF-8, and trailing JSON are rejected. File and stdin payloads are limited to 1 MiB. The same limit applies to the canonical JSON representation of flag input.

### Track classification

| Input | Interpretation |
| --- | --- |
| Exactly 11 letters, digits, underscores, or hyphens | Video ID. |
| HTTPS `/watch?v=<id>` on `music.youtube.com`, `youtube.com`, `www.youtube.com`, or `m.youtube.com` | Exact video. |
| `https://youtu.be/<id>` | Exact video. |
| Any other nonblank string that does not start with `http://` or `https://` | Song query. |

URLs with user information, ports, unsupported paths or hosts, invalid IDs, or missing/repeated watch `v` parameters are rejected. Tracking parameters and fragments are ignored. A watch URL with both `v` and `list` identifies only its video. Playlist-only links and source page URLs are invalid.

An 11-character word such as `Bad_Romance` is treated as an ID, not a query. If that ID does not exist, video preflight rejects it. Include the artist or other text to make it a query.

Duplicate normalized IDs are rejected, including two queries that resolve to the same video. Each duplicate entry names the other input position. No invalid, unresolved, duplicate, or unavailable track is silently dropped.

### Song search

Queries resolve sequentially before credential lookup, using the first anonymous result from the Songs section of YouTube Music. The CLI requires `yt-dlp` on PATH only for these runs and invokes it without cookies or user configuration:

```bash
yt-dlp --ignore-config --flat-playlist --playlist-items 1 --no-warnings --quiet \
  --print '%(id)s\t%(title)s' \
  'https://music.youtube.com/search?q=<url-encoded query>#songs'
```

The CLI supplies a literal tab in the print template. The Songs search command was verified with `yt-dlp` 2026.08.19 on 2026-10-10 through `uvx yt-dlp`. The `#songs` fragment selects the section in the `youtube:music:search_url` extractor; the CLI does not hardcode a search-parameter token. `--ignore-config` prevents local yt-dlp settings from injecting cookies or changing the search behavior.

Flat results provide `id`, `title`, and `url`, not artist, album, channel, or duration. The CLI does no artist matching. Final `title` and `channel` always come from `videos.list`, not yt-dlp output. Check these fields after creation; a wrong match can be corrected in YouTube Music.

Every query has a 60-second process deadline. The measured search cost was about 1.5 seconds per query, or 2.5 minutes for 100 queries. The timeout permits a worst case of about 100 minutes for 100 queries. Empty, malformed, failed, or timed-out results are reported together after all remaining queries run. Cancellation stops the run promptly. There is no fallback to plain YouTube search.

### Preview

```bash
yt-playlist-creator create --file playlist.json --dry-run
yt-playlist-creator create --file playlist.json --dry-run --output json
```

A dry run resolves queries, loads credentials, and retrieves video metadata. It consumes read quota but never writes a playlist. Human output shows each input, resolved ID, title, channel, and source. A validated preview does not guarantee a later insertion or Music playback.

## JSON result contract

`--output json` prints exactly one object for each exit after flag parsing, including failures. An agent does not need to parse stderr. Help remains help text. Flag-parsing errors print usage and the error on stderr, with no stdout.

| Status | Exit | Fields besides `status` |
| --- | --- | --- |
| `created` | 0 | `playlist_id`, `url`, `privacy`, `added`, `tracks` |
| `validated` | 0 | `title`, `description`, `privacy`, `tracks` |
| `rejected` | 1 | `error`, `tracks` |
| `error` | 1 | `error` |
| `partial` | 1 | `playlist_id`, `url`, `added`, `next_index`, `outcome_unknown`, `error`, `tracks` |
| `unknown` | 1 | `error`, `outcome_unknown: true` |

`created` is reported only after every insertion is confirmed. `validated` contains no playlist ID or URL. `rejected` means at least one track is invalid, unresolved, duplicated, or missing from preflight. Other failures before a possible write use `error`.

Each `tracks` array is in input order. Every entry has `input`, `video_id`, `title`, `channel`, and `source`. Source is `exact` or `search`. Failed entries also have `error`; successful entries have no `error` key.

| Rejection stage | `video_id` | `title` and `channel` |
| --- | --- | --- |
| Local track validation | Null on every entry. | Null on every entry. |
| Query resolution or duplicate detection | Resolved IDs; null for unresolved entries. | Null on every entry. |
| Video preflight | Normalized IDs. | Metadata for found IDs; null for missing IDs. |

On `rejected`, fix the entries with an `error`. Replace resolved queries with their `video_id` before running again so those tracks are not searched twice.

### Partial and unknown outcomes

The CLI creates exactly one private playlist, then inserts tracks sequentially without explicit positions. It stops at the first failed insertion. It never retries a mutation, deletes a partial playlist, changes visibility, skips an item, or continues with later tracks.

If the playlist ID is known, an incomplete run reports `partial`. `added` counts only confirmed insertions. `next_index` is the zero-based index of the first unconfirmed input track. `outcome_unknown` indicates that the failed request may have succeeded, for example after a timeout, connection loss, malformed success response, or server failure. The full track list permits manual recovery with exact IDs; there is no resume command.

If creation has an uncertain outcome and no reliable playlist ID, the result is `unknown`. A playlist may already exist. Inspect the account before retrying. If private visibility is not confirmed, the CLI stops and reports the known playlist for inspection; it never broadens or repairs visibility automatically.

In default `url` mode, operation errors leave stdout empty. Recovery details, including the URL, count, index, uncertainty, and track list when available, appear on stderr. In JSON mode they appear in the status object as well. Do not rerun `create` to resume a partial playlist; that makes a new one.

If stdout fails, the CLI exits 1 and reports the write error on stderr. It also reports any known playlist or uncertain creation outcome, even when the playlist was fully created. A failed output write can leave incomplete output, so inspect stderr before retrying. A closed stdout pipe does not suppress these recovery diagnostics.

## Requests and quota

All account reads and writes use YouTube Data API v3. HTTP requests have a 30-second timeout, reject redirects, and have no automatic retries. Keychain commands have a 30-second timeout. SIGINT and SIGTERM cancel pending input, search, authorization, and API work; handled failures exit 1 after rendering their result.

Preflight uses `videos.list?part=snippet` in batches of at most 50 IDs and maps metadata by ID. Missing videos prevent creation. The CLI distinguishes expired credentials, insufficient scope, a missing channel, unavailable videos, permission denial, quota exhaustion, and rate limits. A valid `Retry-After` value is reported but does not cause a write retry. API response bodies and subprocess diagnostics are not dumped into error messages.

Creation costs 50 quota units, each insertion costs 50, and each video batch costs one. For `N` distinct valid tracks, a successful run uses approximately `50 + 50*N + ceil(N/50)` units. For 100 tracks this is 5,052 units. OAuth and anonymous search are excluded; failed API requests also consume quota. The general default is 10,000 units per day, but project quotas and daily playlist-creation limits can differ.

## Verification

Run from this directory:

```bash
gofmt -w *.go
go vet ./...
GOTOOLCHAIN=go1.26.6 golangci-lint run
go test ./...
go test -race ./...
```

The toolchain override matches the repository's installed linter. Use a compatible version if your linter differs. Tests use fake credential and search executables, temporary homes, and local HTTP servers. They never access a real account or create real playlists. Run `shellfmt.sh` on changed shell scripts, including test fixtures.

The live Music compatibility test remains pending. After explicit authorization, create one private playlist with known music tracks and at least one query. Confirm the intended account, private visibility, displayed tracks, and order in YouTube Music. If compatibility fails, revisit the backend rather than switching silently to an unofficial API. Do not delete the test playlist unless the user requests cleanup.

## Outside this version

The CLI does not fetch source pages, rank candidates, perform artist matching, use signed-in search, support duplicate tracks, edit or resume existing playlists, roll back writes, create public or unlisted playlists, download media, or provide shell completion.

## References

- [YouTube Music playlist behavior](https://support.google.com/youtubemusic/answer/7205933?hl=en)
- [Playlist creation](https://developers.google.com/youtube/v3/docs/playlists/insert)
- [Playlist item insertion](https://developers.google.com/youtube/v3/docs/playlistItems/insert)
- [Video metadata](https://developers.google.com/youtube/v3/docs/videos/list)
- [Installed-app OAuth](https://developers.google.com/identity/protocols/oauth2/native-app)
- [OAuth token expiration](https://developers.google.com/identity/protocols/oauth2)
- [Apple security interactive parser](https://github.com/apple-oss-distributions/Security/blob/db15acbe6a7f257a859ad9a3bb86097bfe0679d9/SecurityTool/macOS/security.c)

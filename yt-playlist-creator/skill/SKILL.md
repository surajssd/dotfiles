---
name: yt-playlist-creator
description: Create a private YouTube Music playlist from an ordered list of songs, for example a list found on a web page. Use when asked to make a YouTube or YouTube Music playlist from a list, an article, or a page.
allowed-tools: Bash, WebFetch
---

# Create a YouTube Music playlist

1. Extract the songs from the source in their original order. Write each track as `<title> <artist>`. Keep exact YouTube or YouTube Music links as they are.
2. Use the title of the source as the playlist title and put the source URL in the description.
3. Run `yt-playlist-creator create --file - --output json` with a JSON object of `title`, `description`, and `tracks` on stdin. Create directly when requested. Use `--dry-run` only when the user asks for a preview. Do not fetch source pages through the CLI.
4. On `created`, report the URL and each track's `input`, `title`, and `channel`. On `rejected`, fix only the entries with an `error`, replace each resolved query with its `video_id`, and run again. On `partial` or `unknown`, report the object and stop; do not run again. On `error`, report the error; credentials must be provisioned by a person.

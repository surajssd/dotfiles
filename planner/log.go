package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const logSection = "Progress log"

const logHelp = `Append a dated entry to the Progress log section of a plan's body and set
StatusChecked to today.

The entry is a heading, ### <today>: <title>, followed by the text piped on
stdin, if any. It goes at the end of the "## Progress log" section, before
the next heading of level one or two; a plan without that section gets one at
the end of the file. Headings inside fenced code blocks are ignored. The
words of <title> are joined with spaces.

--note replaces StatusNote in the same write, so the entry can carry the
detail while the note stays short. A status change stays with planner set
status.

<plan> is a path under the root, a [[wikilink]], a basename, the short name
shown by planner get, or a URL the plan lists. On success the new heading is
printed.`

type logOptions struct {
	note    string
	noteSet bool
}

func runLog(deps dependencies, root string, args []string, opts logOptions) error {
	title := strings.Join(strings.Fields(strings.Join(args[1:], " ")), " ")
	if title == "" {
		return errors.New("provide a nonempty entry title")
	}
	body := ""
	if !deps.stdinIsTerminal() {
		input, err := io.ReadAll(deps.stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		body = string(input)
	}
	c, err := loadCorpus(root)
	if err != nil {
		return err
	}
	p, err := findPlan(c, args[0])
	if err != nil {
		return err
	}
	switch {
	case !p.hasFront:
		return fmt.Errorf("%s: the plan has no front matter; give it a status and --note with planner set status first", p.relPath)
	case p.frontErr != nil:
		return fmt.Errorf("%s: front matter does not decode (%v); fix it before logging to it", p.relPath, p.frontErr)
	}
	data, err := os.ReadFile(p.path)
	if err != nil {
		return err
	}
	today := deps.now().Format(dateLayout)
	updated, err := updateFrontMatter(data, statusEdits("", opts.note, opts.noteSet, today))
	if err != nil {
		return fmt.Errorf("%s: %w", p.relPath, err)
	}
	heading := fmt.Sprintf("### %s: %s", today, title)
	if err := writePlanFile(p.path, appendLogEntry(updated, heading, body)); err != nil {
		return err
	}
	return writeOutput(deps.stdout, heading+"\n")
}

var sectionHeadingRE = regexp.MustCompile(`^(#{1,2})[ \t]+(.+?)[ \t]*$`)

// appendLogEntry adds a heading and body to the end of the Progress log
// section of a file that starts with front matter, creating the section at
// the end of the file when it is absent. Everything outside the insertion
// point is kept byte for byte, and the entry uses the file's line endings.
func appendLogEntry(data []byte, heading, body string) []byte {
	lines := strings.SplitAfter(string(data), "\n")
	eol := lineEnding(lines)
	closing := frontMatterEnd(lines)
	sectionStart, sectionEnd := findLogSection(lines, closing)
	body = strings.Trim(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if strings.TrimSpace(body) == "" {
		body = ""
	}
	entry := eol + heading + eol
	if body != "" {
		entry += eol + strings.ReplaceAll(body, "\n", eol) + eol
	}
	var out strings.Builder
	if sectionStart < 0 {
		writeLines(&out, lines[:lastText(lines, len(lines), closing)], eol)
		out.WriteString(eol + "## " + logSection + eol + entry)
		return []byte(out.String())
	}
	writeLines(&out, lines[:lastText(lines, sectionEnd, sectionStart+1)], eol)
	out.WriteString(entry)
	if sectionEnd < len(lines) {
		out.WriteString(eol)
		out.WriteString(strings.Join(lines[sectionEnd:], ""))
	}
	return []byte(out.String())
}

// frontMatterEnd returns the index of the first body line, after the closing
// --- of a block the caller has already validated.
func frontMatterEnd(lines []string) int {
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == "---" {
			return i + 1
		}
	}
	return len(lines)
}

// findLogSection returns the index of the ## Progress log heading, or -1,
// and the index of the level one or two heading that ends the section, or
// len(lines). Headings inside fenced code blocks do not count.
func findLogSection(lines []string, from int) (start, end int) {
	start, end = -1, len(lines)
	inFence, fence := false, ""
	for i := from; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], "\r\n")
		if m := fenceRE.FindStringSubmatch(line); m != nil {
			if !inFence {
				inFence, fence = true, m[1]
				continue
			}
			if m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				inFence = false
				continue
			}
		}
		if inFence {
			continue
		}
		m := sectionHeadingRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if start >= 0 {
			return start, i
		}
		if m[1] == "##" && strings.EqualFold(m[2], logSection) {
			start = i
		}
	}
	return start, end
}

// lastText returns the index after the last line before end that holds
// text, never going below floor, so trailing blank lines are dropped.
func lastText(lines []string, end, floor int) int {
	for end > floor && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
}

// writeLines copies lines and makes sure the last one ends with a newline.
func writeLines(out *strings.Builder, lines []string, eol string) {
	text := strings.Join(lines, "")
	out.WriteString(text)
	if text != "" && !strings.HasSuffix(text, "\n") {
		out.WriteString(eol)
	}
}

package ingest

import "regexp"

// The HTML-to-text pieces Provider adapters share; each adapter composes them with its own cleanup.
var (
	Tag    = regexp.MustCompile(`<[^>]*>`)
	Br     = regexp.MustCompile(`<br\s*/?>`)
	Spaces = regexp.MustCompile(`[ \t\r\f]+`)
)

// StripTags replaces every break matched by brk with a newline and drops all other tags.
func StripTags(h string, brk *regexp.Regexp) string {
	return Tag.ReplaceAllString(brk.ReplaceAllString(h, "\n"), "")
}

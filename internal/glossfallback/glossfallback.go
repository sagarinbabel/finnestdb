package glossfallback

import "strings"

// ETPrefix marks an Estonian definition fallback in a gloss field when no
// English translation is available for the row.
const ETPrefix = "[ET] "

func HasETPrefix(gloss string) bool {
	return strings.HasPrefix(strings.TrimSpace(gloss), strings.TrimSpace(ETPrefix))
}

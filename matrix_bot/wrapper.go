package matrix_bot

import "strings"

// convertToMatrixHTML converts plain text (with \n) into Matrix-style HTML
// This matches what the official client does for multi-line captions
func convertToMatrixHTML(text string) string {
	// Replace newlines with <br />
	lines := strings.Split(text, "\n")
	var paragraphs []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			paragraphs = append(paragraphs, "<p></p>")
			continue
		}
		paragraphs = append(paragraphs, "<p>"+strings.ReplaceAll(trimmed, "\n", "<br />")+"</p>")
	}

	return strings.Join(paragraphs, "\n")
}

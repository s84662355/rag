package chunker

import "strings"

func SplitText(text string, maxChars, overlap int) []string {
	text = strings.TrimSpace(normalize(text))
	if text == "" {
		return nil
	}
	if maxChars <= 0 {
		maxChars = 900
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= maxChars {
		overlap = maxChars / 4
	}

	runes := []rune(text)
	if len(runes) <= maxChars {
		return []string{text}
	}

	var chunks []string
	start := 0
	for start < len(runes) {
		end := min(start+maxChars, len(runes))
		if end < len(runes) {
			limit := start + maxChars/2
			for i := end; i > limit; i-- {
				switch runes[i-1] {
				case '\n', '.', '。', '!', '！', '?', '？', ';', '；':
					end = i
					i = limit
				}
			}
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		if end == len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = end
		}
		start = next
	}
	return chunks
}

func EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	r := []rune(text)
	return max(1, len(r)/2)
}

func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

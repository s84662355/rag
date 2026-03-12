package parser

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	pdf "rsc.io/pdf"
)

type ParsedDocument struct {
	Title string
	Text  string
}

var (
	codeBlockPattern = regexp.MustCompile("(?s)```.*?```")
	inlineCode       = regexp.MustCompile("`([^`]*)`")
	linkPattern      = regexp.MustCompile(`\[(.*?)\]\((.*?)\)`)
	imagePattern     = regexp.MustCompile(`!\[(.*?)\]\((.*?)\)`)
	headingPattern   = regexp.MustCompile(`(?m)^#{1,6}\s*`)
	listPattern      = regexp.MustCompile(`(?m)^\s*[-*+]\s+`)
	numListPattern   = regexp.MustCompile(`(?m)^\s*\d+\.\s+`)
	mdQuotePattern   = regexp.MustCompile(`(?m)^\s*>\s*`)
)

func ParseByFilename(filename string, data []byte) (_ *ParsedDocument, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse document panic: %v", r)
		}
	}()

	ext := strings.ToLower(filepath.Ext(filename))
	title := strings.TrimSuffix(filepath.Base(filename), ext)
	switch ext {
	case ".md", ".markdown":
		text := parseMarkdown(string(data))
		return &ParsedDocument{Title: title, Text: text}, nil
	case ".html", ".htm":
		text, htmlTitle, err := parseHTML(data)
		if err != nil {
			return nil, err
		}
		if htmlTitle != "" {
			title = htmlTitle
		}
		return &ParsedDocument{Title: title, Text: text}, nil
	case ".pdf":
		text, err := parsePDF(data)
		if err != nil {
			return nil, err
		}
		return &ParsedDocument{Title: title, Text: text}, nil
	case ".txt":
		return &ParsedDocument{Title: title, Text: normalizeText(string(data))}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

func ParseWebPage(url string) (_ *ParsedDocument, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("parse webpage panic: %v", r)
		}
	}()

	client := &http.Client{Timeout: 25 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "rag-bot/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch webpage: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch webpage status=%d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read webpage body: %w", err)
	}
	text, title, err := parseHTML(body)
	if err != nil {
		return nil, err
	}
	if title == "" {
		title = url
	}
	return &ParsedDocument{
		Title: title,
		Text:  text,
	}, nil
}

func parseMarkdown(input string) string {
	s := normalizeText(input)
	s = codeBlockPattern.ReplaceAllString(s, "\n")
	s = imagePattern.ReplaceAllString(s, "$1 ")
	s = linkPattern.ReplaceAllString(s, "$1")
	s = inlineCode.ReplaceAllString(s, "$1")
	s = headingPattern.ReplaceAllString(s, "")
	s = listPattern.ReplaceAllString(s, "")
	s = numListPattern.ReplaceAllString(s, "")
	s = mdQuotePattern.ReplaceAllString(s, "")
	return normalizeText(s)
}

func parseHTML(data []byte) (text string, title string, err error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", "", fmt.Errorf("parse html: %w", err)
	}

	var walker func(*html.Node, bool)
	var b strings.Builder
	walker = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			name := strings.ToLower(n.Data)
			if name == "script" || name == "style" || name == "noscript" {
				skip = true
			}
			if name == "title" && n.FirstChild != nil && title == "" {
				title = strings.TrimSpace(n.FirstChild.Data)
			}
		}
		if n.Type == html.TextNode && !skip {
			v := strings.TrimSpace(n.Data)
			if v != "" {
				b.WriteString(v)
				b.WriteRune('\n')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walker(c, skip)
		}
	}
	walker(doc, false)

	return normalizeText(b.String()), title, nil
}

func parsePDF(data []byte) (text string, err error) {
	if !looksLikePDF(data) {
		return "", fmt.Errorf("invalid pdf file")
	}

	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("malformed pdf file: %v", r)
		}
	}()

	tmp, err := os.CreateTemp("", "rag_*.pdf")
	if err != nil {
		return "", fmt.Errorf("create temp pdf: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		return "", fmt.Errorf("write temp pdf: %w", err)
	}

	reader, err := pdf.Open(tmpPath)
	if err != nil {
		return "", fmt.Errorf("open pdf: %w", err)
	}

	var b strings.Builder
	pageCount := reader.NumPage()
	for i := 1; i <= pageCount; i++ {
		p := reader.Page(i)
		if p.V.IsNull() {
			continue
		}
		content := p.Content()
		for _, t := range content.Text {
			b.WriteString(t.S)
			b.WriteRune(' ')
		}
		b.WriteRune('\n')
	}
	return normalizeText(b.String()), nil
}

func looksLikePDF(data []byte) bool {
	if len(data) < 5 {
		return false
	}
	max := len(data)
	if max > 1024 {
		max = 1024
	}
	return bytes.Contains(data[:max], []byte("%PDF-"))
}

func normalizeText(input string) string {
	s := strings.ReplaceAll(input, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	empty := false
	for _, line := range lines {
		v := strings.TrimSpace(line)
		if v == "" {
			if !empty {
				out = append(out, "")
			}
			empty = true
			continue
		}
		empty = false
		out = append(out, v)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

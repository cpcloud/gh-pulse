// SPDX-FileCopyrightText: 2026 Phillip Cloud
//
// SPDX-License-Identifier: Apache-2.0

package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

var entryUTCTimestampPattern = regexp.MustCompile(`\b(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+([0-9]{1,2}),\s+([0-9]{1,2}:[0-9]{2})\s+(UTC)\b`)
var entryUpdatePattern = regexp.MustCompile(`^(Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)\s+([0-9]{1,2}),\s+([0-9]{1,2}:[0-9]{2})\s+UTC\s+([[:alpha:]][[:alpha:] ]*?)\s+-\s+(.+)$`)

var entryHTMLPolicy = func() *bluemonday.Policy {
	policy := bluemonday.NewPolicy()
	policy.AllowElements(
		"a", "b", "blockquote", "br", "code", "div", "em", "h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "i", "kbd", "li", "ol", "p", "pre", "samp", "strong", "tt", "ul", "var",
	)
	policy.AllowAttrs("href").OnElements("a")
	policy.AllowURLSchemes("http", "https")
	policy.RequireParseableURLs(true)
	return policy
}()

func entryMarkdown(input string, reference time.Time, location *time.Location) (string, error) {
	safeHTML := entryHTMLPolicy.Sanitize(input)
	document, err := html.Parse(strings.NewReader(safeHTML))
	if err != nil {
		return "", fmt.Errorf("parse sanitized entry HTML: %w", err)
	}
	localizeEntryTimestamps(document, reference, location)
	insertParagraphSeparators(document)
	markdownBytes, err := htmltomarkdown.ConvertNode(document)
	if err != nil {
		return "", fmt.Errorf("convert entry HTML: %w", err)
	}
	return stripUnsafeTerminalText(string(markdownBytes)), nil
}

type entryUpdate struct {
	when    time.Time
	status  string
	details string
}

func parseEntryUpdates(input string, reference time.Time, location *time.Location) ([]entryUpdate, bool) {
	document, err := html.Parse(strings.NewReader(entryHTMLPolicy.Sanitize(input)))
	if err != nil {
		return nil, false
	}
	paragraphs, ok := entryParagraphs(document)
	if !ok || len(paragraphs) == 0 {
		return nil, false
	}

	updates := make([]entryUpdate, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		match := entryUpdatePattern.FindStringSubmatch(strings.Join(strings.Fields(paragraph), " "))
		if match == nil {
			return nil, false
		}
		when, valid := localizedEntryTimestamp(match[1], match[2], match[3], reference, location)
		if !valid {
			return nil, false
		}
		updates = append(updates, entryUpdate{
			when:    when,
			status:  stripUnsafeTerminalLine(strings.TrimSpace(match[4])),
			details: stripUnsafeTerminalLine(strings.TrimSpace(match[5])),
		})
	}
	return updates, true
}

func entryParagraphs(document *html.Node) ([]string, bool) {
	var paragraphs []string
	valid := true
	var visit func(*html.Node, bool)
	visit = func(node *html.Node, insideParagraph bool) {
		if !valid {
			return
		}
		if node.Type == html.TextNode {
			if !insideParagraph && strings.TrimSpace(node.Data) != "" {
				valid = false
			}
			return
		}
		if node.Type == html.ElementNode && node.Data == "p" {
			var text strings.Builder
			collectEntryText(node, &text)
			paragraphs = append(paragraphs, text.String())
			return
		}
		if node.Type == html.ElementNode {
			switch node.Data {
			case "html", "head", "body", "div":
			default:
				valid = false
				return
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child, insideParagraph)
		}
	}
	visit(document, false)
	return paragraphs, valid
}

func collectEntryText(node *html.Node, text *strings.Builder) {
	if node.Type == html.ElementNode && node.Data == "br" {
		text.WriteByte(' ')
		return
	}
	if node.Type == html.TextNode {
		text.WriteString(node.Data)
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		collectEntryText(child, text)
	}
}

type entryTextSegment struct {
	node       *html.Node
	start, end int
}

func localizeEntryTimestamps(node *html.Node, reference time.Time, location *time.Location) {
	if node.Type == html.ElementNode && node.Data == "p" {
		localizeParagraphTimestamps(node, reference, location)
		return
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		localizeEntryTimestamps(child, reference, location)
	}
}

func localizeParagraphTimestamps(paragraph *html.Node, reference time.Time, location *time.Location) {
	var text strings.Builder
	var segments []entryTextSegment
	var collect func(*html.Node)
	collect = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "br" {
			text.WriteByte(' ')
			return
		}
		if node.Type == html.TextNode {
			start := text.Len()
			text.WriteString(node.Data)
			segments = append(segments, entryTextSegment{node: node, start: start, end: text.Len()})
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			collect(child)
		}
	}
	collect(paragraph)

	value := text.String()
	matches := entryUTCTimestampPattern.FindAllStringSubmatchIndex(value, -1)
	for matchIndex := len(matches) - 1; matchIndex >= 0; matchIndex-- {
		match := matches[matchIndex]
		localized, ok := localizedEntryTimestamp(
			value[match[2]:match[3]],
			value[match[4]:match[5]],
			value[match[6]:match[7]],
			reference,
			location,
		)
		if !ok {
			continue
		}
		replaceEntryTextRange(segments, match[0], match[1], localized.Format("Jan 2, 15:04 MST"))
	}
	removeEmptyVarElements(paragraph)
}

func localizedEntryTimestamp(monthName, dayText, clockText string, reference time.Time, location *time.Location) (time.Time, bool) {
	monthValue, err := time.Parse("Jan", monthName)
	if err != nil {
		return time.Time{}, false
	}
	day, err := strconv.Atoi(dayText)
	if err != nil {
		return time.Time{}, false
	}
	clock, err := time.Parse("15:04", clockText)
	if err != nil {
		return time.Time{}, false
	}
	reference = reference.UTC()
	for year := reference.Year(); year >= reference.Year()-1; year-- {
		candidate := time.Date(year, monthValue.Month(), day, clock.Hour(), clock.Minute(), 0, 0, time.UTC)
		if candidate.Month() == monthValue.Month() && candidate.Day() == day && !candidate.After(reference) {
			return candidate.In(location), true
		}
	}
	return time.Time{}, false
}

func replaceEntryTextRange(segments []entryTextSegment, start, end int, replacement string) {
	inserted := false
	for _, segment := range segments {
		if start >= segment.end || end <= segment.start {
			continue
		}
		localStart := max(start, segment.start) - segment.start
		localEnd := min(end, segment.end) - segment.start
		value := ""
		if !inserted {
			value = replacement
			inserted = true
		}
		segment.node.Data = segment.node.Data[:localStart] + value + segment.node.Data[localEnd:]
	}
}

func removeEmptyVarElements(node *html.Node) {
	for child := node.FirstChild; child != nil; {
		next := child.NextSibling
		removeEmptyVarElements(child)
		if child.Type == html.ElementNode && child.Data == "var" && !hasVisibleText(child) {
			node.RemoveChild(child)
		}
		child = next
	}
}

func hasVisibleText(node *html.Node) bool {
	if node.Type == html.TextNode && strings.TrimSpace(node.Data) != "" {
		return true
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if hasVisibleText(child) {
			return true
		}
	}
	return false
}

func insertParagraphSeparators(parent *html.Node) {
	previousParagraph := false
	for child := parent.FirstChild; child != nil; {
		next := child.NextSibling
		paragraph := child.Type == html.ElementNode && child.Data == "p"
		if paragraph && previousParagraph {
			parent.InsertBefore(&html.Node{Type: html.ElementNode, Data: "hr"}, child)
		}
		if child.Type != html.TextNode || strings.TrimSpace(child.Data) != "" {
			previousParagraph = paragraph
		}
		insertParagraphSeparators(child)
		child = next
	}
}

func stripUnsafeTerminalText(value string) string {
	return stripUnsafeTerminal(value, true)
}

func stripUnsafeTerminalLine(value string) string {
	return stripUnsafeTerminal(value, false)
}

func stripUnsafeTerminal(value string, multiline bool) string {
	return strings.Map(func(value rune) rune {
		if unicode.Is(unicode.Bidi_Control, value) {
			return -1
		}
		if unicode.IsControl(value) && (!multiline || value != '\n' && value != '\t') {
			return -1
		}
		return value
	}, value)
}

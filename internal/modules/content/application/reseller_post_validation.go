package application

import (
	"strings"
	"unicode/utf8"

	"github.com/dujiao-next/internal/modules/content/contract"
	"golang.org/x/net/html"
)

// validateResellerPost applies limits to every supplied translation on create and update.
func validateResellerPost(input CreatePostInput) error {
	hasTitle := false
	for _, raw := range input.TitleJSON {
		value, ok := raw.(string)
		if !ok || utf8.RuneCountInString(value) > 120 {
			return contract.ErrResellerPostTitle
		}
		hasTitle = hasTitle || strings.TrimSpace(value) != ""
	}
	if !hasTitle {
		return contract.ErrResellerPostTitle
	}
	for _, raw := range input.SummaryJSON {
		value, ok := raw.(string)
		if !ok || utf8.RuneCountInString(value) > 300 {
			return contract.ErrResellerPostSummary
		}
	}
	for _, raw := range input.ContentJSON {
		value, ok := raw.(string)
		if !ok || resellerPostTextLength(value) > 10000 {
			return contract.ErrResellerPostContent
		}
	}
	return nil
}

// Count Unicode code points in decoded text, excluding rich-text markup.
func resellerPostTextLength(value string) int {
	tokenizer := html.NewTokenizer(strings.NewReader(value))
	length := 0
	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			return length
		case html.TextToken:
			length += utf8.RuneCount(tokenizer.Text())
		}
	}
}

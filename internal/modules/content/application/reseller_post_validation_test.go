package application

import (
	"errors"
	"strings"
	"testing"

	"github.com/dujiao-next/internal/modules/content/contract"
)

func TestResellerPostLimits(t *testing.T) {
	for _, typ := range []string{"notice", "blog"} {
		t.Run(typ, func(t *testing.T) {
			input := CreatePostInput{Type: typ, TitleJSON: map[string]interface{}{"zh-CN": strings.Repeat("文", 120)}, SummaryJSON: map[string]interface{}{"zh-CN": strings.Repeat("😀", 300)}, ContentJSON: map[string]interface{}{"zh-CN": "<p>" + strings.Repeat("&#x1F600;", 10000) + "</p>"}}
			if err := validateResellerPost(input); err != nil {
				t.Fatal(err)
			}
			input.TitleJSON["en-US"] = strings.Repeat("a", 121)
			if err := validateResellerPost(input); !errors.Is(err, contract.ErrResellerPostTitle) {
				t.Fatal(err)
			}
			delete(input.TitleJSON, "en-US")
			input.SummaryJSON["zh-TW"] = strings.Repeat("字", 301)
			if err := validateResellerPost(input); !errors.Is(err, contract.ErrResellerPostSummary) {
				t.Fatal(err)
			}
			delete(input.SummaryJSON, "zh-TW")
			input.ContentJSON["zh-CN"] = "<p>" + strings.Repeat("字", 10001) + "</p>"
			if err := validateResellerPost(input); !errors.Is(err, contract.ErrResellerPostContent) {
				t.Fatal(err)
			}
			input.SummaryJSON = nil
			input.ContentJSON = nil
			if err := validateResellerPost(input); err != nil {
				t.Fatalf("optional fields: %v", err)
			}
			input.TitleJSON = map[string]interface{}{"zh-CN": "  "}
			if err := validateResellerPost(input); !errors.Is(err, contract.ErrResellerPostTitle) {
				t.Fatal(err)
			}
			input.TitleJSON = map[string]interface{}{"zh-CN": 123}
			if err := validateResellerPost(input); !errors.Is(err, contract.ErrResellerPostTitle) {
				t.Fatal(err)
			}
		})
	}
}

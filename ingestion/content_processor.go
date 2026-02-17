package ingestion

import (
	"fmt"
	"log"
	"net/url"
	"strings"

	"github.com/go-shiori/go-readability"
	"github.com/microcosm-cc/bluemonday"
)

// Holds the results of content processing.
type ProcessedContent struct {
	MainHTML       string // The main article HTML, cleaned and extracted.
	MainText       string // The plain text version of the main article content.
	ExtractedTitle string // The title extracted by the Readability library.
}

// Handles HTML cleaning and main content extraction.
type ContentProcessor struct {
	htmlPolicy      *bluemonday.Policy
	stripTagsPolicy *bluemonday.Policy
}

func NewContentProcessor() *ContentProcessor {
	return &ContentProcessor{
		htmlPolicy:      bluemonday.UGCPolicy(),       // For cleaning HTML for Readability
		stripTagsPolicy: bluemonday.StripTagsPolicy(), // For getting plain text from HTML
	}
}

// Cleans the raw HTML and extracts the main article content.
// baseURL is used by Readability to resolve relative links if any; can be a placeholder like "http://localhost".
func (cp *ContentProcessor) Process(rawHTML string, baseURL *url.URL) (*ProcessedContent, error) {
	if rawHTML == "" {
		return nil, fmt.Errorf("raw HTML content is empty")
	}

	cleanedHTML := cp.htmlPolicy.Sanitize(rawHTML)
	if cleanedHTML == "" && rawHTML != "" { // If policy stripped everything from non-empty input
		log.Printf("WARN: Bluemonday UGCPolicy sanitized non-empty raw HTML to an empty string.")
		// Depending on requirements, we might want to try a more lenient policy or return error.
		// For now, if cleanedHTML is empty, Readability will likely fail or produce nothing.
	}

	article, err := readability.FromReader(strings.NewReader(cleanedHTML), baseURL)

	result := &ProcessedContent{}

	if err == nil && article.Content != "" {
		result.MainHTML = article.Content // Readability already performs some cleaning.
		result.MainText = article.TextContent
		result.ExtractedTitle = article.Title
		log.Printf("INFO: ContentProcessor successfully extracted main content. Extracted title: '%s'", result.ExtractedTitle)
	} else {
		if err != nil {
			log.Printf("WARN: ContentProcessor: Readability extraction failed: %v. Using cleaned HTML as fallback.", err)
		} else { // article.Content was empty
			log.Printf("WARN: ContentProcessor: Readability returned empty article.Content. Using cleaned HTML as fallback.")
		}
		// Fallback to using the initially cleaned HTML if Readability fails or yields no content.
		// We still want to store *something* if the email had HTML.
		result.MainHTML = cleanedHTML
		result.MainText = cp.stripTagsPolicy.Sanitize(cleanedHTML) // Get plain text from the cleaned HTML
		result.ExtractedTitle = ""                                 // No title extracted in this case
	}

	if result.MainHTML == "" {
		// This means both rawHTML was empty initially, or cleanedHTML became empty and Readability also failed/returned empty.
		return nil, fmt.Errorf("processed content (MainHTML) is empty after cleaning and attempting extraction")
	}

	return result, nil
}

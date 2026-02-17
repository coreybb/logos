package ingestion

import (
	"context"
	"log"
	"net/url"
	"path/filepath" // For placeholderBaseURL construction

	"github.com/coreybb/logos/conversion"
	"github.com/coreybb/logos/models"
)

// Defines the input for the content processing pipeline.
type ContentInput struct {
	Bytes            []byte
	OriginalFormat   models.ReadingFormat
	OriginalFileName string // Optional: for context, e.g., base URL for HTML processing
	// Optional context for logging within pipeline, if needed in future
	// UserID           string
	// MessageIDFromMIME string
}

// Holds the result of processing through the ContentPipelineService.
type PipelineOutput struct {
	FinalContentBytes []byte               // The actual bytes to be stored (e.g., cleaned HTML or original PDF)
	FinalFormat       models.ReadingFormat // The format of FinalContentBytes
	ProcessedData     *ProcessedContent    // Result from ContentProcessor if HTML was processed; nil otherwise.
}

// Orchestrates the conversion and HTML processing of raw content.
type ContentPipelineService struct {
	Converter        *conversion.Converter
	ContentProcessor *ContentProcessor // From ingestion/content_processor.go
}

func NewContentPipelineService(
	converter *conversion.Converter,
	contentProcessor *ContentProcessor,
) *ContentPipelineService {
	return &ContentPipelineService{
		Converter:        converter,
		ContentProcessor: contentProcessor,
	}
}

// processHTMLWithContentProcessor takes HTML bytes and processes it with ContentProcessor.
// It handles errors and fallbacks, returning the (potentially modified) HTML and ProcessedContent.
func (ps *ContentPipelineService) processHTMLWithContentProcessor(
	htmlBytes []byte,
	originalFileNameForBaseURL string, // Used to create a placeholder base URL for ContentProcessor
	originalFormatHint models.ReadingFormat, // For logging context
) (processedHTMLContentBytes []byte, processedData *ProcessedContent, err error) { // err for unexpected errors
	if len(htmlBytes) == 0 {
		log.Printf("WARN (ContentPipelineService.processHTML): Input HTML bytes are empty for %s. Skipping ContentProcessor.", originalFileNameForBaseURL)
		return htmlBytes, &ProcessedContent{}, nil
	}

	log.Printf("INFO (ContentPipelineService.processHTML): Processing HTML content (from %s, original format %s) with ContentProcessor.", originalFileNameForBaseURL, originalFormatHint)
	// Create a placeholder base URL for resolving relative links if any, specific to attachment context
	var placeholderBaseURL *url.URL
	if originalFileNameForBaseURL != "" {
		// Using file path for attachments; for email bodies, originalFileNameForBaseURL might be generic like "email_body.html"
		// or this function might not even be called if it's an email body and ProcessContent handles it directly.
		// For now, assume it's for attachment-originated HTML.
		placeholderBaseURL, _ = url.Parse("file://" + filepath.ToSlash(originalFileNameForBaseURL))
	}

	extractedData, procErr := ps.ContentProcessor.Process(string(htmlBytes), placeholderBaseURL)

	if procErr != nil {
		log.Printf("WARN (ContentPipelineService.processHTML): ContentProcessor failed for HTML (from %s, original format %s): %v. Using HTML content pre-ContentProcessor.",
			originalFileNameForBaseURL, originalFormatHint, procErr)
		return htmlBytes, &ProcessedContent{MainHTML: string(htmlBytes), MainText: string(htmlBytes)}, nil
	}

	if extractedData == nil {
		log.Printf("WARN (ContentPipelineService.processHTML): ContentProcessor returned nil ProcessedContent for %s. Using HTML content pre-ContentProcessor.", originalFileNameForBaseURL)
		return htmlBytes, &ProcessedContent{MainHTML: string(htmlBytes), MainText: string(htmlBytes)}, nil
	}

	if extractedData.MainHTML == "" {
		log.Printf("WARN (ContentPipelineService.processHTML): ContentProcessor yielded no main content for HTML from %s. Using HTML content pre-ContentProcessor.", originalFileNameForBaseURL)
		return htmlBytes, extractedData, nil
	}

	log.Printf("INFO (ContentPipelineService.processHTML): Successfully processed HTML from %s with ContentProcessor. Extracted Title: '%s'", originalFileNameForBaseURL, extractedData.ExtractedTitle)
	return []byte(extractedData.MainHTML), extractedData, nil
}

// ProcessContent takes raw content bytes and its original format,
// attempts conversion to HTML if applicable, and then processes
// the HTML to extract the main article content.
func (ps *ContentPipelineService) ProcessContent(
	ctx context.Context,
	input ContentInput,
) (PipelineOutput, error) {
	needsHTMLConversion := false
	attemptConverter := false

	switch input.OriginalFormat {
	case models.ReadingFormatDOCX, models.ReadingFormatRTF, models.ReadingFormatMD:
		needsHTMLConversion = true
		attemptConverter = true
	case models.ReadingFormatTXT:
		attemptConverter = true
	}

	htmlBytesToProcess := input.Bytes
	currentFormat := input.OriginalFormat

	if attemptConverter {
		log.Printf("INFO (ContentPipelineService): Format %s attempting conversion via Converter for '%s'.", input.OriginalFormat, input.OriginalFileName)
		convertedBytes, newFmt, convErr := ps.Converter.ToHTML(input.Bytes, input.OriginalFormat)

		if convErr != nil {
			log.Printf("WARN (ContentPipelineService): Converter.ToHTML failed for '%s' (OriginalFormat: %s): %v. Proceeding with original content.",
				input.OriginalFileName, input.OriginalFormat, convErr)
			// Keep original content and format if conversion fails; currentFormat and htmlBytesToProcess remain original
		} else {
			// Conversion succeeded (or was a pass-through), update current state
			htmlBytesToProcess = convertedBytes
			currentFormat = newFmt // This could be HTML (for DOCX, MD, RTF, TXT->pre) or original (if PDF pass-through in ToHTML)
			if newFmt == input.OriginalFormat && string(convertedBytes) == string(input.Bytes) {
				log.Printf("INFO (ContentPipelineService): Converter.ToHTML resulted in no change for '%s' (Format: %s).", input.OriginalFileName, input.OriginalFormat)
			} else {
				log.Printf("INFO (ContentPipelineService): Converter.ToHTML processed '%s'. OriginalFormat: %s, NewFormat: %s.", input.OriginalFileName, input.OriginalFormat, newFmt)
			}
		}
	}

	if currentFormat == models.ReadingFormatHTML {
		// Use originalFormatHint for logging clarity if it was converted
		originalFormatHintForLog := input.OriginalFormat
		if needsHTMLConversion && currentFormat == models.ReadingFormatHTML {
			// It means it was converted, so originalFormatHintForLog is correct.
		} else if !needsHTMLConversion && currentFormat == models.ReadingFormatHTML {
			// It was already HTML, so originalFormatHintForLog is also correct.
		}

		processedHTMLBytes, procData, procErr := ps.processHTMLWithContentProcessor(htmlBytesToProcess, input.OriginalFileName, originalFormatHintForLog)
		if procErr != nil {
			// processHTMLWithContentProcessor handles its internal fallbacks and logs.
			// An error here would be for unexpected issues not handled by fallbacks.
			log.Printf("ERROR (ContentPipelineService): Unexpected error from processHTMLWithContentProcessor for '%s': %v. Returning pre-ContentProcessor HTML.", input.OriginalFileName, procErr)
			return PipelineOutput{
				FinalContentBytes: htmlBytesToProcess, // The HTML that went into the failing processor
				FinalFormat:       models.ReadingFormatHTML,
				ProcessedData:     procData, // Might be partially filled from fallback in helper
			}, procErr // Propagate the unexpected error
		}
		return PipelineOutput{
			FinalContentBytes: processedHTMLBytes,
			FinalFormat:       models.ReadingFormatHTML,
			ProcessedData:     procData,
		}, nil
	}

	// If not HTML (either originally or after conversion attempts), return the content as is.
	// This path is taken for PDF, EPUB, MOBI directly, or for TXT/DOCX/MD/RTF if their conversion
	// via ps.Converter.ToHTML failed or didn't result in HTML.
	log.Printf("INFO (ContentPipelineService): Content '%s' (Format: %s) is not HTML or did not convert to HTML. Returning as is.", input.OriginalFileName, currentFormat)
	return PipelineOutput{
		FinalContentBytes: htmlBytesToProcess, // This will be original input.Bytes if conversion failed or wasn't HTML
		FinalFormat:       currentFormat,      // This will be original input.OriginalFormat or as changed by converter
		ProcessedData:     nil,                // No HTML-specific processing done by ContentProcessor
	}, nil
}

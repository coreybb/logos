package ingestion

import (
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/coreybb/logos/webutil"
	"github.com/google/uuid"
	"github.com/jhillyerd/enmime"
)

// Responsible for constructing a models.Reading object.
type ReadingBuilder struct {
	sourceRepo *datastore.SourceRepository
}

// Creates a new ReadingBuilder.
func NewReadingBuilder(sourceRepo *datastore.SourceRepository) *ReadingBuilder {
	return &ReadingBuilder{sourceRepo: sourceRepo}
}

// Constructs a models.Reading from processed HTML content (e.g., from an email body).
func (rb *ReadingBuilder) BuildFromHTML(
	ctx context.Context,
	actualSenderEmail string,
	webhookSubject string,
	env *enmime.Envelope, // Parsed MIME message, for headers like From, Date, Subject (fallback)
	processedContent *ProcessedContent, // Output from ContentProcessor
	messageIDFromMIME string, // Original Message-ID for logging
) (models.Reading, error) {

	var reading models.Reading

	if processedContent == nil {
		return reading, fmt.Errorf("processedContent cannot be nil for BuildFromHTML")
	}

	contentHash, err := webutil.GenerateHash(processedContent.MainHTML)
	if err != nil {
		log.Printf("ERROR (ReadingBuilder - HTML): Failed to hash content for Message-ID '%s': %v", messageIDFromMIME, err)
		return reading, fmt.Errorf("failed to generate content hash: %w", err)
	}

	readingSourceID, _ := rb.determineSourceIDFromSenderEmail(ctx, actualSenderEmail, messageIDFromMIME)

	readingTitle := processedContent.ExtractedTitle
	if readingTitle == "" { // Fallback to email subject if Readability didn't find a title
		if env != nil {
			readingTitle = env.GetHeader("Subject")
		}
	}
	if readingTitle == "" { // Further fallback to webhook subject
		readingTitle = webhookSubject
	}
	if readingTitle == "" {
		log.Printf("WARN (ReadingBuilder - HTML): Email (Message-ID: '%s') has no discernible subject or extracted title. Defaulting title.", messageIDFromMIME)
		readingTitle = "Untitled Reading"
	}

	readingID := uuid.NewString()

	reading = models.Reading{
		ID:          readingID,
		SourceID:    readingSourceID,
		Author:      extractAuthorFromEnv(env),
		CreatedAt:   time.Now().UTC(),
		ContentHash: contentHash,
		Excerpt:     generateExcerptFromText(processedContent.MainText),
		PublishedAt: extractPublishedDateFromEnv(env),
		Title:       readingTitle,
		Format:      models.ReadingFormatHTML, // Use prefixed constant
		// StoragePath will be set by the caller after successful storage.
	}
	return reading, nil
}

// Constructs a models.Reading from a direct file attachment.
func (rb *ReadingBuilder) BuildFromFile(
	ctx context.Context,
	actualSenderEmail string,
	webhookSubject string,
	env *enmime.Envelope, // Parsed MIME message, for headers like From, Date, Subject. Can be nil if not from email.
	fileBytes []byte,
	originalFormat models.ReadingFormat, // e.g., models.ReadingFormatPDF, models.ReadingFormatDOCX
	originalFileName string,
	messageIDFromMIME string, // Original Message-ID for logging, can be empty if not from email
) (models.Reading, error) {

	var reading models.Reading

	if len(fileBytes) == 0 {
		return reading, fmt.Errorf("file bytes are empty for Message-ID '%s', Filename: '%s'", messageIDFromMIME, originalFileName)
	}

	contentHash, err := webutil.GenerateHash(string(fileBytes)) // Hash the raw file bytes
	if err != nil {
		log.Printf("ERROR (ReadingBuilder - File): Failed to hash file content for Message-ID '%s', Filename: '%s': %v", messageIDFromMIME, originalFileName, err)
		return reading, fmt.Errorf("failed to generate content hash for file: %w", err)
	}

	readingSourceID, _ := rb.determineSourceIDFromSenderEmail(ctx, actualSenderEmail, messageIDFromMIME)

	readingTitle := strings.TrimSuffix(originalFileName, filepath.Ext(originalFileName))
	if readingTitle == "" { // Fallback to email subject if filename is weird or empty
		if env != nil {
			readingTitle = env.GetHeader("Subject")
		}
	}
	if readingTitle == "" { // Further fallback to webhook subject
		readingTitle = webhookSubject
	}
	if readingTitle == "" {
		log.Printf("WARN (ReadingBuilder - File): Email (Message-ID: '%s') attachment '%s' has no discernible title. Defaulting title.", messageIDFromMIME, originalFileName)
		readingTitle = "Untitled Attachment"
	}

	readingID := uuid.NewString()
	var excerpt string

	// Attempt to generate excerpt for text-based formats
	switch originalFormat {
	case models.ReadingFormatTXT, models.ReadingFormatMD: // Use prefixed constants
		excerpt = generateExcerptFromText(string(fileBytes))
	default:
		// For binary formats (pdf, docx, epub, mobi, rtf), generating a meaningful excerpt is complex.
		emailBodyText := ""
		if env != nil {
			emailBodyText = env.Text
		}
		if emailBodyText != "" {
			excerpt = generateExcerptFromText(emailBodyText)
			if len(excerpt) > 150 {
				excerpt = "Attached " + strings.ToUpper(string(originalFormat)) + " document."
			}
		} else {
			excerpt = "Attached " + strings.ToUpper(string(originalFormat)) + " document."
		}
	}

	reading = models.Reading{
		ID:          readingID,
		SourceID:    readingSourceID,
		Author:      extractAuthorFromEnv(env),
		CreatedAt:   time.Now().UTC(),
		ContentHash: contentHash,
		Excerpt:     excerpt,
		PublishedAt: extractPublishedDateFromEnv(env),
		Title:       readingTitle,
		Format:      originalFormat,
	}

	return reading, nil
}

// Looks up a reading source by the sender's email.
func (rb *ReadingBuilder) determineSourceIDFromSenderEmail(ctx context.Context, senderEmail string, messageIDFromMIME string) (string, error) {
	if rb.sourceRepo == nil {
		log.Printf("WARN (ReadingBuilder): SourceRepository not available. Cannot determine source ID for sender '%s' (Message-ID: '%s')", senderEmail, messageIDFromMIME)
		return uuid.Nil.String(), fmt.Errorf("sourcerepository not initialized")
	}
	if senderEmail == "" { // No sender email to look up
		return uuid.Nil.String(), nil
	}

	source, err := rb.sourceRepo.GetSourceByIdentifierAndType(ctx, senderEmail, "email")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			log.Printf("INFO (ReadingBuilder): No 'email' type ReadingSource found for sender '%s'. Auto-creating. (Message-ID: '%s')", senderEmail, messageIDFromMIME)
			newSource := models.ReadingSource{
				ID:         uuid.NewString(),
				CreatedAt:  time.Now().UTC(),
				Name:       senderEmail,
				Type:       "email",
				Identifier: senderEmail,
			}
			if createErr := rb.sourceRepo.CreateReadingSource(ctx, &newSource); createErr != nil {
				log.Printf("ERROR (ReadingBuilder): Failed to auto-create ReadingSource for sender '%s' (Message-ID: '%s'): %v", senderEmail, messageIDFromMIME, createErr)
				return uuid.Nil.String(), fmt.Errorf("failed to auto-create source: %w", createErr)
			}
			log.Printf("INFO (ReadingBuilder): Auto-created ReadingSource ID '%s' for sender '%s' (Message-ID: '%s')", newSource.ID, senderEmail, messageIDFromMIME)
			return newSource.ID, nil
		}
		log.Printf("ERROR (ReadingBuilder): Failed to query ReadingSource for sender '%s' (Message-ID: '%s'): %v", senderEmail, messageIDFromMIME, err)
		return uuid.Nil.String(), fmt.Errorf("failed to query source: %w", err)
	}
	log.Printf("INFO (ReadingBuilder): Determined ReadingSource ID '%s' for sender '%s' (Message-ID: '%s')", source.ID, senderEmail, messageIDFromMIME)
	return source.ID, nil
}

// Extracts the author's name or address from the "From" header.
// Gracefully handles nil env.
func extractAuthorFromEnv(env *enmime.Envelope) string {
	if env == nil {
		return ""
	}
	fromHeader := env.GetHeader("From")
	if fromHeader == "" {
		return ""
	}
	addrs, err := enmime.ParseAddressList(fromHeader)
	if err == nil && len(addrs) > 0 {
		if addrs[0].Name != "" {
			return addrs[0].Name
		}
		return addrs[0].Address
	}
	log.Printf("WARN (ReadingBuilder): Could not parse 'From' header ('%s'). Using raw header for author.", fromHeader)
	return fromHeader
}

// Creates a short summary from plain text content.
func generateExcerptFromText(plainTextContent string) string {
	maxLength := 250
	trimmedContent := strings.TrimSpace(plainTextContent)

	if len(trimmedContent) == 0 {
		return ""
	}
	if len(trimmedContent) <= maxLength {
		return trimmedContent
	}

	runes := []rune(trimmedContent)
	if len(runes) <= maxLength {
		return trimmedContent
	}

	excerptRunes := runes[:maxLength]
	tempExcerpt := string(excerptRunes)

	lastPeriod := strings.LastIndex(tempExcerpt, ". ")
	if lastPeriod > 0 && lastPeriod > maxLength-75 {
		return string(runes[:lastPeriod+1]) + "..."
	}

	lastSpace := strings.LastIndex(tempExcerpt, " ")
	if lastSpace > 0 && lastSpace > maxLength-100 {
		return string(runes[:lastSpace]) + "..."
	}

	return string(excerptRunes) + "..."
}

// Parses the "Date" header to get the email's publication time.
func extractPublishedDateFromEnv(env *enmime.Envelope) *time.Time {
	if env == nil {
		return nil
	}
	dateStr := env.GetHeader("Date")
	if dateStr == "" {
		return nil
	}
	formats := []string{
		time.RFC1123Z, time.RFC1123, "Mon, 2 Jan 2006 15:04:05 -0700 (MST)",
		time.RFC822Z, time.RFC822, "02 Jan 2006 15:04:05 -0700",
		time.RFC3339, time.RFC3339Nano,
		"Mon, January 2, 2006 3:04:05 PM MST", "Mon, Jan 2, 2006 3:04 PM",
	}
	for _, format := range formats {
		parsedTime, err := time.Parse(format, dateStr)
		if err == nil {
			utcTime := parsedTime.UTC()
			return &utcTime
		}
	}
	log.Printf("WARN (ReadingBuilder): Could not parse Date header '%s' with common formats.", dateStr)
	return nil
}

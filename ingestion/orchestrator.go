package ingestion

import (
	"context"
	"fmt"
	"log"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/models"
	"github.com/jhillyerd/enmime"
)

// Coordinates the various steps of processing inbound content.
type IngestionOrchestrator struct {
	ReadingRepo    *datastore.ReadingRepository
	SourceRepo     *datastore.SourceRepository
	Pipeline       *ContentPipelineService
	ReadingBuilder *ReadingBuilder
}

// Creates a new IngestionOrchestrator.
func NewIngestionOrchestrator(
	readingRepo *datastore.ReadingRepository,
	sourceRepo *datastore.SourceRepository,
	pipeline *ContentPipelineService,
	readingBuilder *ReadingBuilder,
) *IngestionOrchestrator {
	return &IngestionOrchestrator{
		ReadingRepo:    readingRepo,
		SourceRepo:     sourceRepo,
		Pipeline:       pipeline,
		ReadingBuilder: readingBuilder,
	}
}

// Identifies content, converts, builds models, stores, and links to user.
// The http.ResponseWriter is passed through for now for error handling consistency,
// but ideally, this layer shouldn't directly write HTTP responses.
func (io *IngestionOrchestrator) ProcessInboundEmail(
	w http.ResponseWriter, ctx context.Context,
	userID, actualSenderEmail, webhookSubject string,
	env *enmime.Envelope, messageIDFromMIME string,
) error {
	rawContentBytes, originalIdentifiedFormat, originalFileName, isAttachment, err := io.identifyPrimaryContent(env)
	if err != nil {
		// Error already logged by identifyPrimaryContent if it's from there directly.
		// The caller (HandleInbound) will use this error to call handleProcessingError.
		return fmt.Errorf("no usable primary content found for UserID %s (Message-ID: %s): %w", userID, messageIDFromMIME, err)
	}

	var reading models.Reading
	var finalContentToStore []byte
	var finalFormatForReading models.ReadingFormat
	var processedHTMLDataForBuilder *ProcessedContent

	if isAttachment {
		finalContentToStore, finalFormatForReading, processedHTMLDataForBuilder, err = io.processAttachedFile(
			rawContentBytes, originalIdentifiedFormat, originalFileName, userID, messageIDFromMIME,
		)
	} else {
		finalContentToStore, finalFormatForReading, processedHTMLDataForBuilder, err = io.processEmailBody(
			rawContentBytes, originalIdentifiedFormat, userID, messageIDFromMIME,
		)
	}

	if err != nil {
		log.Printf("ERROR (IngestionOrchestrator): Failed to process primary content (format: %s, isAttachment: %t) for UserID %s (Message-ID: %s): %v", originalIdentifiedFormat, isAttachment, userID, messageIDFromMIME, err)
		return fmt.Errorf("failed to process email content: %w", err)
	}

	if finalFormatForReading == models.ReadingFormatHTML && processedHTMLDataForBuilder != nil {
		reading, err = io.ReadingBuilder.BuildFromHTML(ctx, actualSenderEmail, webhookSubject, env, processedHTMLDataForBuilder, messageIDFromMIME)
	} else {
		reading, err = io.ReadingBuilder.BuildFromFile(ctx, actualSenderEmail, webhookSubject, env, finalContentToStore, finalFormatForReading, originalFileName, messageIDFromMIME)
	}
	if err != nil { // This is the error from ReadingBuilder
		log.Printf("ERROR (IngestionOrchestrator): Failed to build Reading model for UserID %s (Message-ID: %s): %v", userID, messageIDFromMIME, err)
		return fmt.Errorf("failed to build reading model: %w", err)
	}

	// Process persistence (deduplication, storing, DB record creation)
	err = io.processReadingPersistenceAndDeduplication(ctx, &reading, finalContentToStore, finalFormatForReading, userID, messageIDFromMIME)
	if err != nil {
		// Error is already logged by the helper method
		return err // Propagate the error
	}

	// Link User to Reading
	receivedAt := time.Now().UTC()
	if errLink := io.ReadingRepo.AddUserReading(ctx, userID, reading.ID, receivedAt); errLink != nil {
		log.Printf("ERROR (IngestionOrchestrator): Failed to link Reading %s to User %s (Message-ID %s): %v", reading.ID, userID, messageIDFromMIME, errLink)
		// This is not returned as a fatal error for the whole ingestion.
	} else {
		log.Printf("INFO (IngestionOrchestrator): Linked Reading %s to User %s (Message-ID %s)", reading.ID, userID, messageIDFromMIME)
	}
	return nil
}

// Examines the email envelope and determines the main content to process.
func (io *IngestionOrchestrator) identifyPrimaryContent(env *enmime.Envelope) (rawContentBytes []byte, format models.ReadingFormat, originalFileName string, isAttachment bool, err error) {
	log.Printf("INFO (identifyPrimaryContent): Identifying primary content. HTML available: %t, Text available: %t, Inline parts: %d, Attachment parts: %d", env.HTML != "", env.Text != "", len(env.Inlines), len(env.Attachments))

	// Try to find a suitable attachment first
	attachmentBytes, attachmentFormat, attachmentFileName, found := io.findPriorityAttachment(env)
	if found {
		return attachmentBytes, attachmentFormat, attachmentFileName, true, nil
	}

	// If no suitable attachment, check HTML body
	if env.HTML != "" {
		log.Printf("INFO (identifyPrimaryContent): Using HTML body as primary content. Size: %d bytes.", len(env.HTML))
		return []byte(env.HTML), models.ReadingFormatHTML, "email_body.html", false, nil
	}

	// If no HTML body, check plain text body
	if env.Text != "" {
		log.Printf("INFO (identifyPrimaryContent): No suitable attachments or HTML body. Using TEXT body as primary content. Size: %d bytes.", len(env.Text))
		return []byte(env.Text), models.ReadingFormatTXT, "email_body.txt", false, nil
	}

	return nil, "", "", false, fmt.Errorf("no processable content (attachments, HTML body, or Text body) found in the email")
}

// Loops through attachments and selects the best one based on predefined priorities.
func (io *IngestionOrchestrator) findPriorityAttachment(env *enmime.Envelope) (contentBytes []byte, format models.ReadingFormat, fileName string, found bool) {
	priorityOrder := []PrioritizedFormat{
		{".pdf", models.ReadingFormatPDF},
		{".epub", models.ReadingFormatEPUB},
		{".mobi", models.ReadingFormatMOBI},
		{".docx", models.ReadingFormatDOCX},
		{".rtf", models.ReadingFormatRTF},
		{".md", models.ReadingFormatMD},
		{".txt", models.ReadingFormatTXT},
		{".text", models.ReadingFormatTXT},
	}

	const minAttachmentSizeBytes = 100

	for _, pFormat := range priorityOrder {
		for _, attachment := range env.Attachments {
			content, fmtName, fName, foundAttachment := io.checkAttachmentAgainstPriority(attachment, pFormat, minAttachmentSizeBytes)
			if foundAttachment {
				return content, fmtName, fName, true
			}
		}
	}
	return nil, "", "", false
}

// Evaluates a single attachment against a specific prioritized format.
func (io *IngestionOrchestrator) checkAttachmentAgainstPriority(
	attachment *enmime.Part,
	pFormat PrioritizedFormat,
	minAttachmentSizeBytes int,
) (contentBytes []byte, format models.ReadingFormat, fileName string, found bool) {
	log.Printf("DEBUG (checkAttachmentAgainstPriority): Checking attachment: Name='%s', ContentType='%s', Size=%d, PriorityFormat: %s",
		attachment.FileName, attachment.ContentType, len(attachment.Content), pFormat.Format)

	isTooSmall := len(attachment.Content) < minAttachmentSizeBytes
	extensionDoesNotMatchPriority := !AttachmentExtensionMatches(attachment.FileName, pFormat.Extension)

	if isTooSmall && extensionDoesNotMatchPriority {
		return nil, "", "", false
	}

	if AttachmentExtensionMatches(attachment.FileName, pFormat.Extension) {
		log.Printf("INFO (checkAttachmentAgainstPriority): Found priority attachment by extension: Name='%s', Format='%s'", attachment.FileName, pFormat.Format)
		return attachment.Content, pFormat.Format, attachment.FileName, true
	}

	// Fallback to content type matching
	contentTypeBase, _, _ := mime.ParseMediaType(attachment.ContentType)
	return MatchAttachmentByContentType(attachment, pFormat, contentTypeBase)
}

// Handles an identified attachment, attempting conversion and processing.
func (io *IngestionOrchestrator) processAttachedFile(
	attachmentBytes []byte, originalFormat models.ReadingFormat, originalFileName, userID, messageIDFromMIME string,
) ([]byte, models.ReadingFormat, *ProcessedContent, error) {
	log.Printf("INFO (processAttachedFile): Processing attachment: Name='%s', Format='%s', Size=%d bytes, UserID=%s, Message-ID=%s",
		originalFileName, originalFormat, len(attachmentBytes), userID, messageIDFromMIME)

	ctx := context.TODO() // Or pass appropriate context down

	if IsDirectReadingFormat(originalFormat) { // Handles PDF, EPUB, MOBI
		return io.handleDirectFormatAttachment(attachmentBytes, originalFormat, originalFileName)
	}

	// For ALL other types (TXT, DOCX, RTF, MD, HTML, etc.), use the ContentPipelineService
	contentIn := ContentInput{
		Bytes:            attachmentBytes,
		OriginalFormat:   originalFormat,
		OriginalFileName: originalFileName,
	}
	pipelineOutput, err := io.Pipeline.ProcessContent(ctx, contentIn)
	if err != nil {
		// This error from ProcessContent is for unexpected/fatal errors in the pipeline itself.
		log.Printf("ERROR (processAttachedFile): ContentPipelineService failed for %s: %v. Using original attachment.", originalFileName, err)
		return attachmentBytes, originalFormat, nil, err // Return original and the error
	}

	return pipelineOutput.FinalContentBytes, pipelineOutput.FinalFormat, pipelineOutput.ProcessedData, nil
}

func (io *IngestionOrchestrator) handleDirectFormatAttachment(
	attachmentBytes []byte,
	originalFormat models.ReadingFormat,
	originalFileName string,
) (contentBytes []byte, format models.ReadingFormat, processedData *ProcessedContent, err error) {
	log.Printf("INFO (handleDirectFormatAttachment): Attachment %s is a direct reading format (%s), using as is.", originalFileName, originalFormat)
	return attachmentBytes, originalFormat, nil, nil
}

// Handles an email body, which could be HTML or plain text.
func (io *IngestionOrchestrator) processEmailBody(
	bodyBytes []byte, originalFormat models.ReadingFormat, userID, messageIDFromMIME string,
) ([]byte, models.ReadingFormat, *ProcessedContent, error) {
	log.Printf("INFO (processEmailBody): Processing email body (Original Format: %s), UserID %s (Message-ID: %s)", originalFormat, userID, messageIDFromMIME)
	ctx := context.TODO() // Or pass appropriate context down

	contentIn := ContentInput{
		Bytes:          bodyBytes,
		OriginalFormat: originalFormat,
		// OriginalFileName for email body could be a generic name like "email_body.html" or "email_body.txt"
		// This is mainly for ContentProcessor's base URL context if it were used directly on non-attachment HTML.
		OriginalFileName: "email_body." + strings.ToLower(string(originalFormat)),
	}

	pipelineOutput, err := io.Pipeline.ProcessContent(ctx, contentIn)
	if err != nil {
		log.Printf("ERROR (processEmailBody): ContentPipelineService failed for email body (Original Format: %s): %v. Using original body.", originalFormat, err)
		return bodyBytes, originalFormat, nil, err
	}

	return pipelineOutput.FinalContentBytes, pipelineOutput.FinalFormat, pipelineOutput.ProcessedData, nil
}

// Handles checking for existing content by hash,
// storing the content if new, creating/updating the reading DB record, and updating the
// reading pointer with the correct ID and StoragePath.
func (io *IngestionOrchestrator) processReadingPersistenceAndDeduplication(
	ctx context.Context,
	reading *models.Reading, // Will be modified
	contentToStore []byte,
	formatForStorage models.ReadingFormat,
	userID string,
	messageIDFromMIME string,
) error {
	existingReading, errDb := io.ReadingRepo.GetReadingByContentHash(ctx, reading.ContentHash)
	if errDb != nil {
		log.Printf("ERROR (IngestionOrchestrator): Failed to check for existing reading by hash %s for UserID %s (Message-ID: %s): %v", reading.ContentHash, userID, messageIDFromMIME, errDb)
		return fmt.Errorf("failed to check for duplicate content: %w", errDb)
	}

	isNewReading := existingReading == nil

	if isNewReading {
		err := io.persistNewReading(ctx, reading, contentToStore, formatForStorage, userID, messageIDFromMIME)
		if err != nil {
			return err // Error is already logged by persistNewReading
		}
	} else {
		log.Printf("INFO (IngestionOrchestrator): Using EXISTING Reading DB record: ID=%s (Format: %s) for UserID=%s (Message-ID %s) based on ContentHash=%s", existingReading.ID, existingReading.Format, userID, messageIDFromMIME, reading.ContentHash)
		reading.ID = existingReading.ID
		reading.StoragePath = existingReading.StoragePath
		// Note: Other fields of the 'reading' object (Title, Author, etc.) retain the values
		// from the current processing pass, which might be slightly different or more up-to-date
		// than the originally stored existingReading. This is by design for now.
	}
	return nil
}

// Handles storing the content and creating the database record for a new reading.
// It modifies the reading pointer with the new StoragePath.
func (io *IngestionOrchestrator) persistNewReading(
	ctx context.Context,
	reading *models.Reading, // Pointer to be modified
	contentToStore []byte,
	formatForStorage models.ReadingFormat,
	userID string,
	messageIDFromMIME string,
) error {
	log.Printf("INFO (IngestionOrchestrator): Content hash %s not found. Processing as new reading. (UserID %s, Message-ID %s)", reading.ContentHash, userID, messageIDFromMIME)

	// Store content body in the reading for DB persistence
	reading.ContentBody = string(contentToStore)
	reading.StoragePath = fmt.Sprintf("readings/%s/%s.%s", userID, reading.ID, string(formatForStorage))

	if errDbCreate := io.ReadingRepo.CreateReading(ctx, reading); errDbCreate != nil {
		log.Printf("ERROR (IngestionOrchestrator): Failed to create NEW Reading DB record for ReadingID %s, UserID %s (Message-ID: %s): %v", reading.ID, userID, messageIDFromMIME, errDbCreate)
		// Consider if the stored file should be deleted here if DB creation fails.
		return fmt.Errorf("failed to save reading record to database: %w", errDbCreate)
	}
	log.Printf("INFO (IngestionOrchestrator): Created NEW Reading DB record: ID=%s, Format=%s, UserID=%s (Message-ID %s)", reading.ID, reading.Format, userID, messageIDFromMIME)
	return nil
}

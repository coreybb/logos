package webhooks

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/coreybb/logos/conversion"
	"github.com/coreybb/logos/datastore"
	"github.com/coreybb/logos/ingestion"
	"github.com/coreybb/logos/storage"
	"github.com/coreybb/logos/webutil"
	"github.com/jhillyerd/enmime"
)

const (
	inboundEmailDomain = "parse.lakonic.dev"

	formFieldEmail   = "email"
	formFieldTo      = "to"
	formFieldFrom    = "from"
	formFieldSubject = "subject"
)

type InboundEmailHandler struct {
	Orchestrator *ingestion.IngestionOrchestrator
}

func NewInboundEmailHandler(readingRepo *datastore.ReadingRepository, sourceRepo *datastore.SourceRepository) *InboundEmailHandler {
	contentProc := ingestion.NewContentProcessor()
	converterInst, errConv := conversion.NewConverter()
	if errConv != nil {
		// NewConverter currently logs a warning and returns (converter, nil) even if pandoc is not found.
		// If it were to return a critical error, this handler should probably propagate it.
		log.Printf("WARN: Error initializing converter (this is unexpected as NewConverter usually doesn't error out): %v", errConv)
		// Depending on future policy, might panic or return an error from NewInboundEmailHandler
	}

	pipelineService := ingestion.NewContentPipelineService(converterInst, contentProc)
	readingBuild := ingestion.NewReadingBuilder(sourceRepo)
	contentStore := storage.NewLocalFileStorer("")
	orch := ingestion.NewIngestionOrchestrator(
		readingRepo,
		sourceRepo,
		pipelineService,
		readingBuild,
		contentStore,
	)

	return &InboundEmailHandler{
		Orchestrator: orch,
	}
}

func (h *InboundEmailHandler) HandleInbound(w http.ResponseWriter, r *http.Request) {
	log.Printf("InboundEmailHandler: HandleInbound called. Method: %s, Path: %s, Content-Type: %s", r.Method, r.URL.Path, r.Header.Get("Content-Type"))

	webhookData, err := parseWebhookRequest(r)
	if err != nil {
		if !webutil.HasResponseWriterSentHeader(w) {
			webutil.RespondWithError(w, http.StatusBadRequest, err.Error())
		}
		return
	}

	userID, err := extractUserIDFromRecipient(webhookData.Recipient)
	if err != nil {
		handleProcessingError(w, fmt.Sprintf("Could not extract UserID from recipient '%s'", webhookData.Recipient), err, true)
		return
	}

	// Parse env first as it's needed for reliable sender extraction and Message-ID
	env, err := parseMimeMessage(webhookData.RawMIME)
	if err != nil {
		handleProcessingError(w, "Failed to parse raw MIME with enmime", err, true)
		return
	}
	// Get Message-ID early for logging, even if sender extraction fails
	messageIDFromMIME := env.GetHeader("Message-ID")

	var actualSenderEmail string

	// Try "Sender" header first using AddressList
	senderList, errSender := env.AddressList("Sender")
	if errSender == nil && len(senderList) > 0 && senderList[0].Address != "" {
		actualSenderEmail = strings.ToLower(senderList[0].Address)
	}

	// If not found in "Sender", try "From" header using AddressList
	if actualSenderEmail == "" {
		fromList, errFrom := env.AddressList("From")
		if errFrom == nil && len(fromList) > 0 && fromList[0].Address != "" {
			actualSenderEmail = strings.ToLower(fromList[0].Address)
		}
	}

	// Fallback to raw webhookData.Sender if enmime parsing didn't yield an address
	if actualSenderEmail == "" {
		rawSenderInput := strings.TrimSpace(webhookData.Sender)
		if rawSenderInput != "" {
			// Simple extraction from "Name <email@example.com>" format
			if strings.Contains(rawSenderInput, "<") && strings.Contains(rawSenderInput, ">") {
				start := strings.LastIndex(rawSenderInput, "<")
				end := strings.LastIndex(rawSenderInput, ">")
				if start != -1 && end != -1 && start < end {
					extracted := strings.TrimSpace(rawSenderInput[start+1 : end])
					if extracted != "" { // Ensure something was actually extracted
						actualSenderEmail = strings.ToLower(extracted)
					}
				}
			}
			// If parsing "Name <email>" failed or wasn't the format, and still no email,
			// use the rawSenderInput directly if it looks somewhat like an email (contains @).
			// This is a very basic check.
			if actualSenderEmail == "" && strings.Contains(rawSenderInput, "@") {
				actualSenderEmail = strings.ToLower(rawSenderInput)
			}
		}
	}

	if actualSenderEmail == "" {
		errMsg := fmt.Sprintf("Could not determine actual sender email from parsed headers or raw input. Raw Sender Field: '%s', Message-ID: '%s'",
			webhookData.Sender, messageIDFromMIME)
		handleProcessingError(w, errMsg, nil, true)
		return
	}

	log.Printf("INFO: Processing email for UserID: %s, Sender: %s, Subject: '%s', Message-ID: '%s'",
		userID, actualSenderEmail, webhookData.Subject, messageIDFromMIME)

	err = h.Orchestrator.ProcessInboundEmail(w, r.Context(), userID, actualSenderEmail, webhookData.Subject, env, messageIDFromMIME)
	if err != nil {
		log.Printf("ERROR (HandleInbound): Error from IngestionOrchestrator for UserID %s (Message-ID: %s): %v", userID, messageIDFromMIME, err)
		if !webutil.HasResponseWriterSentHeader(w) {
			webutil.RespondWithError(w, http.StatusInternalServerError, "Internal server error processing email")
		}
		return
	}

	logAttachments(userID, messageIDFromMIME, env)

	if !webutil.HasResponseWriterSentHeader(w) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK (Email processed successfully)"))
	}
}

type webhookInputData struct {
	RawMIME   string
	Recipient string
	Sender    string
	Subject   string
}

func parseWebhookRequest(r *http.Request) (webhookInputData, error) {
	var data webhookInputData
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		if err := r.ParseForm(); err != nil {
			log.Printf("ERROR: Failed to parse form data: %v", err)
			return data, fmt.Errorf("failed to parse form data: %w", err)
		}
	}
	data.RawMIME = r.FormValue(formFieldEmail)
	data.Recipient = r.FormValue(formFieldTo)
	data.Sender = r.FormValue(formFieldFrom)
	data.Subject = r.FormValue(formFieldSubject)

	if data.RawMIME == "" {
		log.Printf("WARN: Raw MIME field ('%s') is empty in webhook.", formFieldEmail)
		logPreamble := "Available form fields:"
		if r.Form != nil && len(r.Form) > 0 {
			for key, values := range r.Form {
				log.Printf("%s Key: %s, Values: %v", logPreamble, key, values)
				logPreamble = "                 "
			}
		} else {
			log.Printf("%s None found or r.Form is nil.", logPreamble)
		}
		return data, fmt.Errorf("missing raw email content in webhook payload")
	}
	if data.Recipient == "" {
		log.Printf("WARN: Recipient field ('%s') is empty in webhook.", formFieldTo)
		return data, fmt.Errorf("missing recipient information in webhook")
	}
	return data, nil
}

func parseMimeMessage(rawMimeString string) (*enmime.Envelope, error) {
	log.Printf("INFO: Received raw MIME. Length: %d bytes. Now parsing with enmime.", len(rawMimeString))
	reader := strings.NewReader(rawMimeString)
	env, err := enmime.ReadEnvelope(reader)
	if err != nil {
		return nil, fmt.Errorf("enmime.ReadEnvelope failed: %w", err)
	}
	return env, nil
}

func handleProcessingError(
	w http.ResponseWriter, logMessage string, originalErr error, acknowledgeOnly bool) {
	if originalErr != nil {
		log.Printf("WARN: %s: %v", logMessage, originalErr)
	} else {
		log.Printf("WARN: %s", logMessage)
	}

	if webutil.HasResponseWriterSentHeader(w) {
		log.Println("WARN: Headers already written, skipping response in handleProcessingError.")
		return
	}

	if acknowledgeOnly {
		w.Header().Set(webutil.HeaderContentType, webutil.ContentTypeTextPlainUTF8)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("OK (%s)", logMessage)))
	} else {
		webutil.RespondWithError(w, http.StatusInternalServerError, "Error processing email")
	}
}

func logAttachments(userID string, messageIDFromMIME string, env *enmime.Envelope) {
	if len(env.Attachments) == 0 && len(env.Inlines) == 0 {
		return
	}
	log.Printf("INFO: Other Attachments/Inline parts for UserID %s (Message-ID %s):", userID, messageIDFromMIME)
	for _, att := range env.Attachments {
		log.Printf("  Attachment: Name: %s, Type: %s, Size: %d bytes", att.FileName, att.ContentType, len(att.Content))
	}
	for _, inline := range env.Inlines {
		log.Printf("  Inline: Name: %s, Type: %s, Size: %d bytes", inline.FileName, inline.ContentType, len(inline.Content))
	}
}

func extractUserIDFromRecipient(recipient string) (string, error) {
	emailPart := recipient
	if strings.Contains(recipient, "<") {
		parts := strings.SplitN(recipient, "<", 2)
		if len(parts) == 2 && strings.Contains(parts[1], ">") {
			emailPart = strings.TrimSuffix(parts[1], ">")
		}
	}
	emailPart = strings.ToLower(emailPart)
	expectedSuffix := "@" + inboundEmailDomain
	if !strings.HasSuffix(emailPart, expectedSuffix) {
		return "", fmt.Errorf("recipient domain does not match expected inbound domain '%s'", inboundEmailDomain)
	}
	localPart := strings.TrimSuffix(emailPart, expectedSuffix)
	expectedPrefix := "inbox+"
	if !strings.HasPrefix(localPart, expectedPrefix) {
		return "", fmt.Errorf("recipient local part does not start with '%s'", expectedPrefix)
	}
	userID := localPart[len(expectedPrefix):]
	if userID == "" {
		return "", fmt.Errorf("extracted UserID is empty")
	}
	return userID, nil
}

package ingestion

import (
	"log"
	"path/filepath"
	"strings"

	"github.com/coreybb/logos/models"
	"github.com/jhillyerd/enmime"
)

// PrioritizedFormat helps in selecting the best attachment by defining a file extension
// and its corresponding models.ReadingFormat.
type PrioritizedFormat struct {
	Extension string
	Format    models.ReadingFormat
}

// AttachmentExtensionMatches checks if the attachment's filename extension matches the expected extension (case-insensitive).
func AttachmentExtensionMatches(fileName, expectedExtension string) bool {
	return strings.EqualFold(filepath.Ext(fileName), expectedExtension)
}

// MatchAttachmentByContentType attempts to identify an attachment based on its MIME content type
// as a fallback if extension matching fails. It also uses AttachmentExtensionMatches for TXT
// due to the generic nature of "text/plain".
func MatchAttachmentByContentType(attachment *enmime.Part, pFormat PrioritizedFormat, contentTypeBase string) ([]byte, models.ReadingFormat, string, bool) {
	switch pFormat.Format {
	case models.ReadingFormatPDF:
		if contentTypeBase == "application/pdf" {
			log.Printf("INFO (MatchAttachmentByContentType): Found PDF attachment by content type: Name='%s'", attachment.FileName)
			return attachment.Content, models.ReadingFormatPDF, attachment.FileName, true
		}
	case models.ReadingFormatDOCX:
		if contentTypeBase == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
			log.Printf("INFO (MatchAttachmentByContentType): Found DOCX attachment by content type: Name='%s'", attachment.FileName)
			return attachment.Content, models.ReadingFormatDOCX, attachment.FileName, true
		}
	case models.ReadingFormatRTF:
		if contentTypeBase == "application/rtf" || contentTypeBase == "text/rtf" {
			log.Printf("INFO (MatchAttachmentByContentType): Found RTF attachment by content type: Name='%s'", attachment.FileName)
			return attachment.Content, models.ReadingFormatRTF, attachment.FileName, true
		}
	case models.ReadingFormatMD:
		if contentTypeBase == "text/markdown" {
			log.Printf("INFO (MatchAttachmentByContentType): Found MD attachment by content type: Name='%s'", attachment.FileName)
			return attachment.Content, models.ReadingFormatMD, attachment.FileName, true
		}
	case models.ReadingFormatTXT:
		// For TXT, also ensure the extension is .txt or .text as text/plain is very generic.
		if contentTypeBase == "text/plain" && (AttachmentExtensionMatches(attachment.FileName, ".txt") || AttachmentExtensionMatches(attachment.FileName, ".text")) {
			log.Printf("INFO (MatchAttachmentByContentType): Found TXT attachment by content type (text/plain) and extension: Name='%s'", attachment.FileName)
			return attachment.Content, models.ReadingFormatTXT, attachment.FileName, true
		}
	}
	return nil, "", "", false
}

// Checks if the format is one that's typically used directly
// without conversion to HTML, and not typically processed by ContentProcessor.
func IsDirectReadingFormat(format models.ReadingFormat) bool {
	switch format {
	case models.ReadingFormatPDF, models.ReadingFormatEPUB, models.ReadingFormatMOBI:
		return true
	default:
		return false
	}
}

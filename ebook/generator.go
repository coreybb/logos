package ebook

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/coreybb/logos/models"
)

const (
	defaultEbookConvertTimeout = 60 * time.Second
	ebookConvertCommand        = "ebook-convert"
)

// EditionGenerator handles the generation of ebooks using Calibre's ebook-convert tool.
type EditionGenerator struct {
	calibreToolPath string
	defaultTimeout  time.Duration
}

// Attempts to find the ebook-convert CLI tool in the system PATH.
func NewEditionGenerator() *EditionGenerator {
	path, err := exec.LookPath(ebookConvertCommand)
	if err != nil {
		log.Printf("WARN (EditionGenerator): '%s' command not found in PATH. Ebook generation will fail unless a specific path is configured or the command becomes available.", ebookConvertCommand)
		// Return a generator that will fail at runtime if calibreToolPath is empty and generation is attempted.
		// This allows the application to start even if Calibre isn't immediately available.
	} else {
		log.Printf("INFO (EditionGenerator): Found '%s' command at: %s", ebookConvertCommand, path)
	}
	return &EditionGenerator{
		calibreToolPath: path, // Will be empty if not found
		defaultTimeout:  defaultEbookConvertTimeout,
	}
}

// GenerateEdition converts an input HTML file to the specified ebook format.
// outputDir is the base directory where the edition file will be saved.
// The final filename will be <editionID>.<format_extension> within outputDir.
func (eg *EditionGenerator) GenerateEdition(
	ctx context.Context,
	inputHTMLPath string, // Absolute path to the source HTML file
	metadata models.EditionMetadata, // Includes Title, Author, etc.
	outputFormat models.EditionFormat,
	outputDir string, // Base directory for output
	editionID string, // Used for filename
) (generatedFilePath string, fileSize int64, err error) {

	if eg.calibreToolPath == "" {
		return "", 0, fmt.Errorf("'%s' command path not configured or found, cannot generate ebook", ebookConvertCommand)
	}
	if inputHTMLPath == "" {
		return "", 0, fmt.Errorf("input HTML path cannot be empty")
	}
	if outputFormat == "" {
		return "", 0, fmt.Errorf("output format cannot be empty")
	}
	if outputDir == "" {
		return "", 0, fmt.Errorf("output directory cannot be empty")
	}
	if editionID == "" {
		return "", 0, fmt.Errorf("edition ID cannot be empty")
	}

	formatExtension := strings.ToLower(string(outputFormat))
	outputFileName := editionID + "." + formatExtension
	fullOutputFilePath := filepath.Join(outputDir, outputFileName)

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", 0, fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
	}

	// Base arguments: input file, output file
	args := []string{inputHTMLPath, fullOutputFilePath}

	// Add metadata arguments
	if metadata.Title != "" {
		args = append(args, "--title", metadata.Title)
	}
	if metadata.Author != "" {
		args = append(args, "--authors", metadata.Author)
	}
	if metadata.Publisher != "" {
		args = append(args, "--publisher", metadata.Publisher)
	}
	if len(metadata.Tags) > 0 {
		args = append(args, "--tags", strings.Join(metadata.Tags, ","))
	}
	if metadata.Language != "" { // Expects ISO639 language code e.g. "eng", "fra"
		args = append(args, "--language", metadata.Language)
	}
	if metadata.CoverImageBytes != nil && len(metadata.CoverImageBytes) > 0 {
		// ebook-convert needs a file path for the cover.
		// We need to write the bytes to a temporary file.
		tmpCoverFile, err := os.CreateTemp("", "cover-*.jpg") // Assume JPEG for now, or detect from bytes
		if err != nil {
			log.Printf("WARN (EditionGenerator): Failed to create temporary cover file: %v. Proceeding without cover.", err)
		} else {
			defer os.Remove(tmpCoverFile.Name()) // Clean up
			if _, err := tmpCoverFile.Write(metadata.CoverImageBytes); err != nil {
				log.Printf("WARN (EditionGenerator): Failed to write to temporary cover file: %v. Proceeding without cover.", err)
			} else {
				if err := tmpCoverFile.Close(); err != nil {
					log.Printf("WARN (EditionGenerator): Failed to close temporary cover file: %v. Proceeding without cover.", err)
				} else {
					args = append(args, "--cover", tmpCoverFile.Name())
				}
			}
		}
	}

	// Add output profile based on format
	switch outputFormat {
	case models.EditionFormatMOBI:
		args = append(args, "--output-profile", "kindle")
	case models.EditionFormatEPUB:
		// EPUB3 by default is usually good. No specific profile needed unless issues arise.
		// args = append(args, "--epub-version", "3") // if needed
	case models.EditionFormatPDF:
		// PDF output might benefit from a profile like "tablet" or specific page setup options.
		// args = append(args, "--pdf-page-size", "letter") // Example
	}

	// Add some common options for better Kindle experience
	args = append(args, "--preserve-cover-aspect-ratio")
	args = append(args, "--change-justification", "justify") // Or "left"
	args = append(args, "--embed-all-fonts")
	args = append(args, "--expand-css")

	// Debug: Log the command
	log.Printf("INFO (EditionGenerator): Executing ebook-convert: %s %v", eg.calibreToolPath, args)

	ctxWithTimeout, cancel := context.WithTimeout(ctx, eg.defaultTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctxWithTimeout, eg.calibreToolPath, args...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	startTime := time.Now()
	err = cmd.Run()
	duration := time.Since(startTime)

	if ctxWithTimeout.Err() == context.DeadlineExceeded {
		log.Printf("ERROR (EditionGenerator): ebook-convert command timed out after %s for edition %s. Stderr: %s", duration, editionID, stderrBuf.String())
		return "", 0, fmt.Errorf("ebook-convert command timed out: %w. Stderr: %s", ctxWithTimeout.Err(), stderrBuf.String())
	}
	if err != nil {
		log.Printf("ERROR (EditionGenerator): ebook-convert command failed for edition %s (took %s). Exit error: %v. Stdout: %s, Stderr: %s", editionID, duration, err, stdoutBuf.String(), stderrBuf.String())
		return "", 0, fmt.Errorf("ebook-convert failed: %w. Stderr: %s", err, stderrBuf.String())
	}

	// Check if output file was actually created
	stat, err := os.Stat(fullOutputFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("ERROR (EditionGenerator): ebook-convert command for edition %s seemed to succeed (exit 0) but output file '%s' not found. Stdout: %s, Stderr: %s", editionID, fullOutputFilePath, stdoutBuf.String(), stderrBuf.String())
			return "", 0, fmt.Errorf("output file '%s' not found after successful conversion command", fullOutputFilePath)
		}
		log.Printf("ERROR (EditionGenerator): Failed to stat output file '%s' for edition %s: %v", fullOutputFilePath, editionID, err)
		return "", 0, fmt.Errorf("failed to stat output file '%s': %w", fullOutputFilePath, err)
	}

	fileSize = stat.Size()
	if fileSize == 0 {
		log.Printf("WARN (EditionGenerator): ebook-convert command for edition %s produced an empty file: %s. Stdout: %s, Stderr: %s", editionID, fullOutputFilePath, stdoutBuf.String(), stderrBuf.String())
		// Optionally, treat as error: return "", 0, fmt.Errorf("generated ebook file '%s' is empty", fullOutputFilePath)
	}

	log.Printf("INFO (EditionGenerator): Successfully generated ebook for edition %s: %s (Size: %d bytes, Took: %s)", editionID, fullOutputFilePath, fileSize, duration)
	if stdoutBuf.Len() > 0 {
		log.Printf("DEBUG (EditionGenerator): ebook-convert stdout for %s: %s", editionID, stdoutBuf.String())
	}
	if stderrBuf.Len() > 0 { // Calibre often outputs non-fatal warnings to stderr
		log.Printf("INFO (EditionGenerator): ebook-convert stderr (warnings/info) for %s: %s", editionID, stderrBuf.String())
	}

	return fullOutputFilePath, fileSize, nil
}

// EditionMetadata contains metadata for generating an ebook.
type EditionMetadata struct {
	Title           string
	Author          string
	Publisher       string   // Optional
	Tags            []string // Optional
	Language        string   // Optional, ISO639 code
	CoverImageBytes []byte   // Optional, raw bytes of the cover image
	// Add other Calibre metadata fields as needed:
	// ISBN, Series, SeriesIndex, Comments, etc.
}

// Helper to get a string representation for logging or other uses.
func (em EditionMetadata) String() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Title: '%s'", em.Title))
	sb.WriteString(fmt.Sprintf(", Author: '%s'", em.Author))
	if em.Publisher != "" {
		sb.WriteString(fmt.Sprintf(", Publisher: '%s'", em.Publisher))
	}
	if len(em.Tags) > 0 {
		sb.WriteString(fmt.Sprintf(", Tags: [%s]", strings.Join(em.Tags, ", ")))
	}
	if em.Language != "" {
		sb.WriteString(fmt.Sprintf(", Language: '%s'", em.Language))
	}
	if len(em.CoverImageBytes) > 0 {
		sb.WriteString(fmt.Sprintf(", CoverImage: %d bytes", len(em.CoverImageBytes)))
	} else {
		sb.WriteString(", CoverImage: None")
	}
	return sb.String()
}

// --- Example of more advanced options if needed later ---

// CalibreOptions holds various settings for ebook-convert.
// This is a more structured way to pass many options if the simple ones aren't enough.
type CalibreOptions struct {
	// Input/Output
	InputFile  string
	OutputFile string

	// Metadata (subset)
	Title   string
	Authors string // Comma-separated if multiple

	// Look & Feel
	BaseFontSize           int    // e.g., 0 (default)
	ExtraCSS               string // CSS to add
	ChangeJustification    string // "left", "justify", "original"
	DisableFontRescaling   bool
	EmbedAllFonts          bool
	ExpandCSS              bool
	InsertBlankLine        bool
	LineHeight             float64 // e.g., 0 (default), 12.0
	MarginBottom           float64 // default 5.0
	MarginLeft             float64 // default 5.0
	MarginRight            float64 // default 5.0
	MarginTop              float64 // default 5.0
	PreserveCoverRatio     bool
	RemoveParagraphSpacing bool

	// Output Profile (e.g., "kindle", "kindle_dx", "kindle_fire", "kobo", "nook", "tablet")
	OutputProfile string

	// MOBI specific
	MobiTocAtStart bool

	// EPUB specific
	EpubVersion string // "2" or "3"

	// PDF specific
	PdfPageSize          string // e.g., "letter", "a4"
	PdfDefaultFontSize   int    // e.g., 12
	PdfMonospaceFontSize int    // e.g., 10

	// Debug
	Verbose bool
}

// ToArgs converts CalibreOptions into a slice of string arguments for exec.Command.
func (opts *CalibreOptions) ToArgs() []string {
	var args []string
	args = append(args, opts.InputFile, opts.OutputFile)

	if opts.Title != "" {
		args = append(args, "--title", opts.Title)
	}
	if opts.Authors != "" {
		args = append(args, "--authors", opts.Authors)
	}
	if opts.BaseFontSize > 0 {
		args = append(args, "--base-font-size", strconv.Itoa(opts.BaseFontSize))
	}
	if opts.ExtraCSS != "" {
		args = append(args, "--extra-css", opts.ExtraCSS)
	}
	if opts.ChangeJustification != "" {
		args = append(args, "--change-justification", opts.ChangeJustification)
	}
	if opts.DisableFontRescaling {
		args = append(args, "--disable-font-rescaling")
	}
	if opts.EmbedAllFonts {
		args = append(args, "--embed-all-fonts")
	}
	if opts.ExpandCSS {
		args = append(args, "--expand-css")
	}
	// ... add all other options ...
	if opts.OutputProfile != "" {
		args = append(args, "--output-profile", opts.OutputProfile)
	}
	if opts.Verbose {
		args = append(args, "-vv") // Calibre verbose flag
	}
	return args
}

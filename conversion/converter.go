package conversion

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os/exec"
	"time"

	"github.com/coreybb/logos/models" // Import models package
)

// Converter provides methods to convert various document formats to HTML.
type Converter struct {
	pandocPath string // Path to pandoc executable, detected on creation
	timeout    time.Duration
}

// NewConverter creates a new Converter, attempting to find pandoc.
func NewConverter() (*Converter, error) {
	path, err := exec.LookPath("pandoc")
	if err != nil {
		log.Printf("WARN (Converter): pandoc executable not found in PATH. Conversions requiring pandoc will fail.")
		// Return a converter that can still handle non-pandoc formats
		// but log the issue. Erroring out might be too harsh if only
		// HTML/TXT/PDF etc are expected initially.
		// Alternatively, return error:
		// return nil, fmt.Errorf("pandoc executable not found in PATH: %w", err)
	} else {
		log.Printf("INFO (Converter): Found pandoc executable at: %s", path)
	}
	return &Converter{
		pandocPath: path,             // Store the path, even if empty
		timeout:    30 * time.Second, // Default timeout for pandoc execution
	}, nil
}

// runPandoc executes the pandoc command to convert from one format to another.
func (c *Converter) runPandoc(ctx context.Context, fromFormat string, inputBytes []byte) ([]byte, error) {
	if c.pandocPath == "" {
		return nil, fmt.Errorf("pandoc executable not found, cannot perform conversion from %s", fromFormat)
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.pandocPath, "-f", fromFormat, "-t", "html")

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe for pandoc: %w", err)
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("failed to start pandoc command: %w", err)
	}

	// Write input bytes to stdin in a separate goroutine to avoid blocking
	go func() {
		defer stdinPipe.Close() // Close stdin pipe once writing is done
		_, err := io.Copy(stdinPipe, bytes.NewReader(inputBytes))
		if err != nil {
			// Log error, but the main error handling will be on cmd.Wait()
			log.Printf("ERROR (Converter): Failed writing to pandoc stdin: %v", err)
		}
	}()

	err = cmd.Wait()
	if err != nil {
		stderrOutput := stderrBuf.String()
		// Check if context timeout was exceeded
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("pandoc execution timed out after %v: %w. Stderr: %s", c.timeout, ctx.Err(), stderrOutput)
		}
		// Otherwise, it's likely a pandoc execution error
		return nil, fmt.Errorf("pandoc execution failed: %w. Stderr: %s", err, stderrOutput)
	}

	// Even if exit code is 0, check stderr for warnings
	stderrOutput := stderrBuf.String()
	if stderrOutput != "" {
		log.Printf("WARN (Converter): pandoc stderr output during %s to html conversion:\n%s", fromFormat, stderrOutput)
	}

	return stdoutBuf.Bytes(), nil
}

// ToHTML attempts to convert the given content bytes in the specified originalFormat to HTML.
// For formats that are already HTML or directly usable as HTML, it might pass them through.
// It returns the (potentially converted) content bytes, the new format (usually models.ReadingFormatHTML if converted), and an error.
func (c *Converter) ToHTML(contentBytes []byte, originalFormat models.ReadingFormat) ([]byte, models.ReadingFormat, error) {
	ctx := context.Background() // Base context for pandoc execution

	var pandocInputFormat string
	performPandocConversion := false

	switch originalFormat {
	case models.ReadingFormatMD:
		log.Printf("INFO (Converter): Attempting to convert Markdown to HTML.")
		pandocInputFormat = "markdown"
		performPandocConversion = true
	case models.ReadingFormatDOCX:
		log.Printf("INFO (Converter): Attempting to convert DOCX to HTML.")
		pandocInputFormat = "docx"
		performPandocConversion = true
	case models.ReadingFormatRTF:
		log.Printf("INFO (Converter): Attempting to convert RTF to HTML.")
		pandocInputFormat = "rtf"
		performPandocConversion = true

	case models.ReadingFormatHTML: // If it's already identified as HTML
		log.Printf("INFO (Converter): Content is already HTML, passing through.")
		return contentBytes, models.ReadingFormatHTML, nil
	case models.ReadingFormatTXT:
		log.Printf("INFO (Converter): Converting TXT to basic HTML (wrapping in <pre>).")
		// Basic conversion for TXT - wrap in <pre> tags.
		// Alternatively, could use pandoc: pandocInputFormat = "plain", performPandocConversion = true
		htmlBytes := []byte("<pre>" + string(contentBytes) + "</pre>")
		return htmlBytes, models.ReadingFormatHTML, nil
	case models.ReadingFormatPDF, models.ReadingFormatEPUB, models.ReadingFormatMOBI:
		// These formats are not converted to HTML during this ingestion step.
		log.Printf("INFO (Converter): Format '%s' is a direct reading format, no HTML conversion performed by ToHTML.", originalFormat)
		return contentBytes, originalFormat, nil // Return original bytes and format
	default:
		log.Printf("WARN (Converter): Unknown or unsupported format for ToHTML conversion: %s", originalFormat)
		// Return original bytes and format, with an error indicating no conversion happened.
		return contentBytes, originalFormat, fmt.Errorf("unsupported format for ToHTML conversion: %s", originalFormat)
	}

	if performPandocConversion {
		if c.pandocPath == "" {
			err := fmt.Errorf("pandoc conversion required for %s, but pandoc executable was not found", originalFormat)
			log.Printf("ERROR (Converter): %v", err)
			// Return original content because conversion cannot proceed
			return contentBytes, originalFormat, err
		}
		htmlBytes, err := c.runPandoc(ctx, pandocInputFormat, contentBytes)
		if err != nil {
			log.Printf("ERROR (Converter): Failed to convert %s to HTML using pandoc: %v", originalFormat, err)
			// Return original content on conversion failure
			return contentBytes, originalFormat, fmt.Errorf("pandoc conversion from %s failed: %w", originalFormat, err)
		}
		log.Printf("INFO (Converter): Successfully converted %s to HTML using pandoc.", originalFormat)
		return htmlBytes, models.ReadingFormatHTML, nil
	}

	// Should not be reached due to the switch statement logic, but acts as a fallback.
	log.Printf("WARN (Converter): Reached unexpected end of ToHTML function for format: %s", originalFormat)
	return contentBytes, originalFormat, fmt.Errorf("unexpected state in ToHTML for format %s", originalFormat)
}

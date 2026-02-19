package ebook

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/coreybb/logos/models"
	epub "github.com/go-shiori/go-epub"
)

// EditionGenerator handles the generation of EPUB ebooks.
type EditionGenerator struct{}

func NewEditionGenerator() *EditionGenerator {
	log.Println("INFO (EditionGenerator): Using go-epub for EPUB generation")
	return &EditionGenerator{}
}

// GenerateEdition converts an input HTML file to an EPUB.
// outputDir is the base directory where the edition file will be saved.
// The final filename will be <editionID>.epub within outputDir.
func (eg *EditionGenerator) GenerateEdition(
	ctx context.Context,
	inputHTMLPath string,
	metadata models.EditionMetadata,
	outputFormat models.EditionFormat,
	outputDir string,
	editionID string,
) (generatedFilePath string, fileSize int64, err error) {

	if inputHTMLPath == "" {
		return "", 0, fmt.Errorf("input HTML path cannot be empty")
	}
	if outputDir == "" {
		return "", 0, fmt.Errorf("output directory cannot be empty")
	}
	if editionID == "" {
		return "", 0, fmt.Errorf("edition ID cannot be empty")
	}

	startTime := time.Now()

	title := metadata.Title
	if title == "" {
		title = "Logos Edition"
	}
	author := metadata.Author
	if author == "" {
		author = "Logos"
	}

	e, err := epub.NewEpub(title)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create epub: %w", err)
	}
	e.SetAuthor(author)
	if metadata.Language != "" {
		e.SetLang(metadata.Language)
	} else {
		e.SetLang("en")
	}

	// Read the combined HTML content
	htmlBytes, err := os.ReadFile(inputHTMLPath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to read input HTML: %w", err)
	}

	// Add the content as a section
	_, err = e.AddSection(string(htmlBytes), title, "", "")
	if err != nil {
		return "", 0, fmt.Errorf("failed to add section to epub: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", 0, fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
	}

	// Always output as .epub regardless of the requested format
	outputFileName := editionID + ".epub"
	fullOutputFilePath := filepath.Join(outputDir, outputFileName)

	err = e.Write(fullOutputFilePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to write epub file: %w", err)
	}

	stat, err := os.Stat(fullOutputFilePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to stat output file '%s': %w", fullOutputFilePath, err)
	}

	duration := time.Since(startTime)
	log.Printf("INFO (EditionGenerator): Successfully generated EPUB for edition %s: %s (Size: %d bytes, Took: %s)",
		editionID, fullOutputFilePath, stat.Size(), duration)

	return fullOutputFilePath, stat.Size(), nil
}

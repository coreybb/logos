package ebook

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/coreybb/logos/models"
	epub "github.com/go-shiori/go-epub"
)

var imgSrcRegex = regexp.MustCompile(`<img([^>]*)\ssrc=["']([^"']+)["']([^>]*)>`)

// EditionGenerator handles the generation of EPUB ebooks.
type EditionGenerator struct{}

func NewEditionGenerator() *EditionGenerator {
	log.Println("INFO (EditionGenerator): Using go-epub for EPUB generation")
	return &EditionGenerator{}
}

// GenerateEdition creates an EPUB from a set of readings with a title page,
// table of contents, and individual chapters per article.
func (eg *EditionGenerator) GenerateEdition(
	ctx context.Context,
	readings []models.Reading,
	metadata models.EditionMetadata,
	outputFormat models.EditionFormat,
	outputDir string,
	editionID string,
	colorImages bool,
) (generatedFilePath string, fileSize int64, err error) {

	if len(readings) == 0 {
		return "", 0, fmt.Errorf("no readings provided")
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

	// Title page
	titlePageHTML := buildTitlePage(title, author, metadata.Date)
	_, err = e.AddSection(titlePageHTML, title, "titlepage", "")
	if err != nil {
		return "", 0, fmt.Errorf("failed to add title page: %w", err)
	}

	// Each reading as its own chapter
	for i, reading := range readings {
		if reading.Format != models.ReadingFormatHTML || reading.ContentBody == "" {
			continue
		}

		articleHTML := buildArticleSection(reading)
		articleHTML = embedImages(e, articleHTML, colorImages)

		sectionID := fmt.Sprintf("article-%d", i+1)
		_, err = e.AddSection(articleHTML, reading.Title, sectionID, "")
		if err != nil {
			log.Printf("WARN (EditionGenerator): Failed to add section for reading %s: %v", reading.ID, err)
			continue
		}
	}

	if err := os.MkdirAll(outputDir, os.ModePerm); err != nil {
		return "", 0, fmt.Errorf("failed to create output directory '%s': %w", outputDir, err)
	}

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
	log.Printf("INFO (EditionGenerator): Successfully generated EPUB for edition %s: %s (%d articles, %d bytes, %s)",
		editionID, fullOutputFilePath, len(readings), stat.Size(), duration)

	return fullOutputFilePath, stat.Size(), nil
}

func buildTitlePage(title, author, date string) string {
	if date == "" {
		date = time.Now().Format("January 2, 2006")
	}

	return fmt.Sprintf(`<div style="text-align: center; padding-top: 40%%;">
	<h1 style="font-size: 2em; margin-bottom: 0.5em;">%s</h1>
	<p style="font-size: 1.2em; color: #666;">%s</p>
	<p style="font-size: 1em; color: #999; margin-top: 2em;">%s</p>
</div>`, title, date, author)
}

func buildArticleSection(reading models.Reading) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(`<h1>%s</h1>`, reading.Title))
	if reading.Author != "" {
		sb.WriteString(fmt.Sprintf(`<p style="color: #666; font-style: italic; margin-bottom: 2em;">%s</p>`, reading.Author))
	}
	sb.WriteString(reading.ContentBody)
	return sb.String()
}

// embedImages finds all <img> tags with external URLs, downloads and embeds
// them in the EPUB, optionally converting to grayscale.
func embedImages(e *epub.Epub, html string, colorImages bool) string {
	imageCount := 0

	result := imgSrcRegex.ReplaceAllStringFunc(html, func(match string) string {
		submatches := imgSrcRegex.FindStringSubmatch(match)
		if len(submatches) < 4 {
			return match
		}

		srcURL := submatches[2]

		if strings.HasPrefix(srcURL, "data:") {
			return match
		}
		if !strings.HasPrefix(srcURL, "http://") && !strings.HasPrefix(srcURL, "https://") {
			return match
		}

		imageCount++
		internalName := fmt.Sprintf("image-%03d", imageCount)

		var embeddedPath string
		var err error

		if colorImages {
			embeddedPath, err = e.AddImage(srcURL, internalName)
		} else {
			embeddedPath, err = addGrayscaleImage(e, srcURL, internalName)
		}

		if err != nil {
			log.Printf("WARN (EditionGenerator): Failed to embed image %s: %v", srcURL, err)
			return match
		}

		return fmt.Sprintf(`<img%s src="%s"%s>`, submatches[1], embeddedPath, submatches[3])
	})

	if imageCount > 0 {
		log.Printf("INFO (EditionGenerator): Embedded %d images in EPUB (color: %t)", imageCount, colorImages)
	}

	return result
}

// addGrayscaleImage downloads an image, converts it to grayscale, and adds it to the EPUB.
func addGrayscaleImage(e *epub.Epub, srcURL string, internalName string) (string, error) {
	resp, err := http.Get(srcURL)
	if err != nil {
		return "", fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("image download returned status %d", resp.StatusCode)
	}

	src, format, err := image.Decode(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to decode image: %w", err)
	}

	bounds := src.Bounds()
	gray := image.NewGray(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, color.GrayModel.Convert(src.At(x, y)))
		}
	}

	tmpFile, err := os.CreateTemp("", "logos-img-*."+format)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file for image: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	switch format {
	case "jpeg":
		err = jpeg.Encode(tmpFile, gray, &jpeg.Options{Quality: 85})
	default:
		err = png.Encode(tmpFile, gray)
	}
	tmpFile.Close()
	if err != nil {
		return "", fmt.Errorf("failed to encode grayscale image: %w", err)
	}

	return e.AddImage(tmpFile.Name(), internalName)
}

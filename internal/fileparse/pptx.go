package fileparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// extractPPTX extracts plain text from a PPTX file.
// It directly parses the PPTX (ZIP) file without external dependencies.
func extractPPTX(filePath string) (string, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	var text strings.Builder
	slideNum := 0

	// Process slides in order (slide1.xml, slide2.xml, etc.)
	for _, f := range zr.File {
		// Check if this is a slide file
		if !strings.HasPrefix(f.Name, "ppt/slides/slide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}

		slideNum++
		text.WriteString(fmt.Sprintf("## Slide %d\n", slideNum))

		rc, err := f.Open()
		if err != nil {
			continue
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		// Extract text from slide
		slideText := extractTextFromSlideXML(data)
		if slideText != "" {
			text.WriteString(slideText)
			text.WriteString("\n\n")
		}
	}

	// Also extract notes if present
	notesText := extractPPTXNotes(zr)
	if notesText != "" {
		text.WriteString("## Notes\n")
		text.WriteString(notesText)
	}

	return strings.TrimSpace(text.String()), nil
}

// extractTextFromSlideXML extracts text from slide XML content.
// PPTX uses <a:t> tags within DrawingML for text content.
func extractTextFromSlideXML(data []byte) string {
	// Parse as XML to handle nested structures properly
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	inTextElement := false

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			// <a:t> is the text element in DrawingML
			if se.Name.Local == "t" {
				inTextElement = true
			}
		case xml.EndElement:
			if se.Name.Local == "t" {
				inTextElement = false
			}
			// Add line break after paragraph or text body
			if se.Name.Local == "p" || se.Name.Local == "txBody" {
				text.WriteString("\n")
			}
		case xml.CharData:
			if inTextElement {
				text.WriteString(string(se))
			}
		}
	}

	return strings.TrimSpace(text.String())
}

// extractPPTXNotes extracts speaker notes from PPTX files.
func extractPPTXNotes(zr *zip.ReadCloser) string {
	var text strings.Builder

	for _, f := range zr.File {
		if !strings.HasPrefix(f.Name, "ppt/notesSlides/notesSlide") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			continue
		}

		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}

		notesText := extractTextFromSlideXML(data)
		if notesText != "" {
			text.WriteString(notesText)
			text.WriteString("\n")
		}
	}

	return strings.TrimSpace(text.String())
}

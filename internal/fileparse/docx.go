package fileparse

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

// extractDOCX extracts plain text from a DOCX file.
// It directly parses the DOCX (ZIP) file without external dependencies.
func extractDOCX(filePath string) (string, error) {
	zr, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer zr.Close()

	var documentXML *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			documentXML = f
			break
		}
	}

	if documentXML == nil {
		return "", fmt.Errorf("word/document.xml not found in DOCX")
	}

	rc, err := documentXML.Open()
	if err != nil {
		return "", fmt.Errorf("open document.xml: %w", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("read document.xml: %w", err)
	}

	// Extract text from XML using proper XML parsing
	text := extractTextFromWordXML(data)
	return strings.TrimSpace(text), nil
}

// extractTextFromWordXML extracts text content from WordprocessingML XML.
// WordprocessingML uses <w:t> tags for text content within <w:p> paragraphs.
func extractTextFromWordXML(data []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var text strings.Builder
	var currentPara strings.Builder
	inTextElement := false

	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch se := token.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "t":
				// Text element
				inTextElement = true
			case "br":
				// Line break within paragraph
				currentPara.WriteString("\n")
			case "tab":
				// Tab character
				currentPara.WriteString("\t")
			}
		case xml.EndElement:
			switch se.Name.Local {
			case "t":
				inTextElement = false
			case "p":
				// End of paragraph - flush and add double newline
				paraText := strings.TrimSpace(currentPara.String())
				if paraText != "" {
					if text.Len() > 0 {
						text.WriteString("\n\n")
					}
					text.WriteString(paraText)
				}
				currentPara.Reset()
			}
		case xml.CharData:
			if inTextElement {
				currentPara.WriteString(string(se))
			}
		}
	}

	// Handle any remaining text (document without proper paragraph structure)
	remaining := strings.TrimSpace(currentPara.String())
	if remaining != "" {
		if text.Len() > 0 {
			text.WriteString("\n\n")
		}
		text.WriteString(remaining)
	}

	return text.String()
}

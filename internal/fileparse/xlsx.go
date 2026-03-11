package fileparse

import (
	"fmt"
	"os"
	"strings"

	"github.com/xuri/excelize/v2"
)

// extractXLSX extracts cell text from all sheets of an XLSX/XLS file.
// Supports both modern .xlsx (Open XML) and legacy .xls (Binary Interchange File Format).
func extractXLSX(filePath string) (string, error) {
	// Check if file is legacy XLS format
	isXLS, err := isLegacyXLS(filePath)
	if err != nil {
		return "", fmt.Errorf("check file format: %w", err)
	}

	if isXLS {
		return extractLegacyXLS(filePath)
	}

	return extractModernXLSX(filePath)
}

// isLegacyXLS checks if the file is in legacy Excel 97-2003 binary format.
// XLS files start with specific OLE compound document signatures.
func isLegacyXLS(filePath string) (bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return false, err
	}
	defer f.Close()

	// Read first 8 bytes to check for OLE compound document signature
	// XLS files are OLE compound documents starting with:
	// D0 CF 11 E0 A1 B1 1A E1 (which spells "DOCFILE" in a way)
	header := make([]byte, 8)
	_, err = f.Read(header)
	if err != nil {
		return false, err
	}

	// OLE compound document signature
	if len(header) >= 8 &&
		header[0] == 0xD0 && header[1] == 0xCF &&
		header[2] == 0x11 && header[3] == 0xE0 &&
		header[4] == 0xA1 && header[5] == 0xB1 &&
		header[6] == 0x1A && header[7] == 0xE1 {
		return true, nil
	}

	return false, nil
}

// extractModernXLSX extracts text from modern XLSX format (Excel 2007+).
func extractModernXLSX(filePath string) (string, error) {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return "", fmt.Errorf("open xlsx: %w", err)
	}
	defer f.Close()

	return extractSheets(f)
}

// extractLegacyXLS attempts to extract text from legacy XLS format.
// excelize v2 has limited support for XLS through its internal conversion.
func extractLegacyXLS(filePath string) (string, error) {
	// Try to open with excelize - it has some XLS support
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		// Return empty string without error - let the AI use skills to analyze the file
		// The file path is passed separately to the agent via attachments
		return "", nil
	}
	defer f.Close()

	return extractSheets(f)
}

// extractSheets extracts text from all sheets in an excelize file.
func extractSheets(f *excelize.File) (string, error) {
	var buf strings.Builder
	sheets := f.GetSheetList()

	if len(sheets) == 0 {
		return "", nil
	}

	for i, sheet := range sheets {
		if i > 0 {
			buf.WriteString("\n")
		}
		buf.WriteString(fmt.Sprintf("## Sheet: %s\n", sheet))

		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}

		for _, row := range rows {
			// Filter out empty cells and join with tabs
			nonEmptyCells := make([]string, 0, len(row))
			for _, cell := range row {
				trimmed := strings.TrimSpace(cell)
				if trimmed != "" {
					nonEmptyCells = append(nonEmptyCells, trimmed)
				}
			}
			if len(nonEmptyCells) > 0 {
				buf.WriteString(strings.Join(nonEmptyCells, "\t"))
				buf.WriteString("\n")
			}
		}
	}

	return buf.String(), nil
}

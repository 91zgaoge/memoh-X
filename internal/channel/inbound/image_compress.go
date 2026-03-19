package inbound

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"log/slog"
)

// Image compression constants
// ADJUSTED: Less aggressive compression to preserve image quality for LLM recognition
const (
	// Size thresholds - INCREASED to reduce unnecessary compression
	smallImageThreshold  = 800 * 1024  // 800KB - no compression needed (was 100KB)
	mediumImageThreshold = 2 * 1024 * 1024 // 2MB - moderate compression (was 1MB)

	// Target dimensions - INCREASED for better clarity
	maxSmallDimension  = 1536 // for medium images (was 1024)
	maxLargeDimension  = 1024 // for large images (was 512)

	// JPEG quality - INCREASED for better quality
	jpegQualityMedium = 90 // was 85
	jpegQualityHigh   = 85 // was 70
)

// compressImageIfNeeded compresses image data if it exceeds size thresholds
// Returns compressed data, mime type, and whether compression was applied
func compressImageIfNeeded(data []byte, mimeType string, logger *slog.Logger) ([]byte, string, bool) {
	originalSize := len(data)

	// Skip compression for small images
	if originalSize < smallImageThreshold {
		return data, mimeType, false
	}

	// Decode image
	img, format, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		logger.Warn("failed to decode image for compression, using original",
			slog.String("error", err.Error()),
			slog.Int("originalSize", originalSize))
		return data, mimeType, false
	}

	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Determine target dimensions and quality based on original size
	var targetMaxDim, quality int
	if originalSize < mediumImageThreshold {
		targetMaxDim = maxSmallDimension
		quality = jpegQualityMedium
		logger.Info("compressing medium image",
			slog.Int("originalSize", originalSize),
			slog.String("format", format),
			slog.Int("width", width),
			slog.Int("height", height),
			slog.Int("targetMaxDim", targetMaxDim))
	} else {
		targetMaxDim = maxLargeDimension
		quality = jpegQualityHigh
		logger.Info("compressing large image",
			slog.Int("originalSize", originalSize),
			slog.String("format", format),
			slog.Int("width", width),
			slog.Int("height", height),
			slog.Int("targetMaxDim", targetMaxDim))
	}

	// Calculate new dimensions maintaining aspect ratio
	newWidth, newHeight := calculateDimensions(width, height, targetMaxDim)

	// Resize image
	resized := resizeImage(img, newWidth, newHeight)

	// Encode to JPEG with specified quality
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: quality})
	if err != nil {
		logger.Warn("failed to encode compressed image, using original",
			slog.String("error", err.Error()),
			slog.Int("originalSize", originalSize))
		return data, mimeType, false
	}

	compressed := buf.Bytes()
	compressedSize := len(compressed)

	// Only use compressed if it's actually smaller
	if compressedSize >= originalSize {
		logger.Info("compressed image not smaller, using original",
			slog.Int("originalSize", originalSize),
			slog.Int("compressedSize", compressedSize))
		return data, mimeType, false
	}

	logger.Info("image compression successful",
		slog.Int("originalSize", originalSize),
		slog.Int("compressedSize", compressedSize),
		slog.Float64("ratio", float64(compressedSize)/float64(originalSize)),
		slog.Int("width", newWidth),
		slog.Int("height", newHeight))

	return compressed, "image/jpeg", true
}

// calculateDimensions calculates new dimensions maintaining aspect ratio
func calculateDimensions(width, height, maxDim int) (newWidth, newHeight int) {
	if width <= maxDim && height <= maxDim {
		return width, height
	}

	aspectRatio := float64(width) / float64(height)

	if width > height {
		newWidth = maxDim
		newHeight = int(float64(maxDim) / aspectRatio)
	} else {
		newHeight = maxDim
		newWidth = int(float64(maxDim) * aspectRatio)
	}

	// Ensure minimum dimension of 1
	if newWidth < 1 {
		newWidth = 1
	}
	if newHeight < 1 {
		newHeight = 1
	}

	return newWidth, newHeight
}

// resizeImage resizes an image to the specified dimensions using simple scaling.
// NOTE: This is a basic implementation. For better quality, consider using
// golang.org/x/image/draw with Catmull-Rom or BiLinear interpolation.
func resizeImage(src image.Image, width, height int) image.Image {
	// Create new image with target dimensions
	dst := image.NewRGBA(image.Rect(0, 0, width, height))

	// Simple scaling
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Map destination coordinates to source coordinates
			srcX := x * srcWidth / width
			srcY := y * srcHeight / height
			dst.Set(x, y, src.At(bounds.Min.X+srcX, bounds.Min.Y+srcY))
		}
	}

	return dst
}

// getImageMimeType returns the mime type for an image based on format name
func getImageMimeType(format string) string {
	switch format {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	default:
		return "image/jpeg"
	}
}

// formatBytes formats byte count to human readable string
func formatBytes(bytes int) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)

	switch {
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

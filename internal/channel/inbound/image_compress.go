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
// With 524K context window, we can afford larger images
// Target: ~100KB compressed (100K tokens) per image, max 3 images = 300K tokens
// Leaves ~200K for system prompt (~50K) + response (~100K) + text messages (~50K)
const (
	// Size thresholds - RELAXED for 524K context
	smallImageThreshold  = 500 * 1024  // 500KB - no compression (was 200KB)
	largeImageThreshold  = 2 * 1024 * 1024  // 2MB - large image threshold

	// Target dimensions - INCREASED for better quality
	// 1024x1024 at quality 85 produces ~100-200KB JPEG
	maxDimension = 1024  // Max 1024px (was 768px)

	// JPEG quality - INCREASED for better recognition
	jpegQuality = 85  // was 80
)

// compressImageIfNeeded compresses image data to ensure it fits within token limits
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

	// Always compress to maxDimension for consistency
	logger.Info("compressing image for token limit compliance",
		slog.Int("originalSize", originalSize),
		slog.String("format", format),
		slog.Int("width", width),
		slog.Int("height", height),
		slog.Int("targetMaxDim", maxDimension),
		slog.Int("quality", jpegQuality))

	// Calculate new dimensions maintaining aspect ratio
	newWidth, newHeight := calculateDimensions(width, height, maxDimension)

	// Resize image
	resized := resizeImage(img, newWidth, newHeight)

	// Encode to JPEG with specified quality
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: jpegQuality})
	if err != nil {
		logger.Warn("failed to encode compressed image, using original",
			slog.String("error", err.Error()),
			slog.Int("originalSize", originalSize))
		return data, mimeType, false
	}

	compressed := buf.Bytes()
	compressedSize := len(compressed)

	// For very large images, apply additional compression if needed
	if compressedSize > largeImageThreshold {
		logger.Info("image still too large after compression, applying stronger compression",
			slog.Int("compressedSize", compressedSize),
			slog.Int("targetSize", largeImageThreshold))

		// Try with lower quality
		buf.Reset()
		err = jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 60})
		if err == nil {
			recompressed := buf.Bytes()
			if len(recompressed) < compressedSize {
				compressed = recompressed
				compressedSize = len(compressed)
			}
		}
	}

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
		slog.Int("height", newHeight),
		slog.Int("estimatedTokens", compressedSize)) // 1 char ≈ 1 token for base64

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

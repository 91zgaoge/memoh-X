package media

import (
	"context"
	"io"
)

// Asset represents a media asset
type Asset struct {
	ContentHash string
	ContentType string
	Size        int64
	Name        string
}

// Service provides media operations
type Service struct{}

// NewService creates a new media service
func NewService() *Service {
	return &Service{}
}

// Open opens a media asset by content hash
func (s *Service) Open(ctx context.Context, botID, contentHash string) (io.ReadCloser, Asset, error) {
	// Minimal implementation - return empty asset
	return nil, Asset{}, nil
}

package wecom

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

const (
	// DefaultChunkSize is the default chunk size for media uploads (512 KB)
	DefaultChunkSize = 512 * 1024

	// MaxChunkCount is the maximum number of chunks allowed
	MaxChunkCount = 100

	// MaxUploadRetries is the maximum number of retries for a single chunk
	MaxUploadRetries = 2
)

// UploadManager handles media file uploads via the WeCom WebSocket protocol.
type UploadManager struct {
	wsClient *WebSocketClient
	logger   *slog.Logger
}

// NewUploadManager creates a new UploadManager.
func NewUploadManager(wsClient *WebSocketClient, logger *slog.Logger) *UploadManager {
	return &UploadManager{
		wsClient: wsClient,
		logger:   logger,
	}
}

// UploadMedia uploads a file using the three-step protocol (init → chunks → finish)
// and returns the media_id assigned by WeCom.
func (u *UploadManager) UploadMedia(ctx context.Context, data []byte, filename, mediaType string) (string, error) {
	fileMD5 := calculateFileMD5(data)
	chunkSize := DefaultChunkSize
	chunks := splitIntoChunks(data, chunkSize)
	if len(chunks) == 0 {
		return "", fmt.Errorf("file is empty")
	}
	if len(chunks) > MaxChunkCount {
		return "", fmt.Errorf("file too large: %d chunks exceeds max %d", len(chunks), MaxChunkCount)
	}

	u.logger.Info("starting media upload",
		slog.String("filename", filename),
		slog.String("media_type", mediaType),
		slog.Int("file_size", len(data)),
		slog.Int("chunk_count", len(chunks)),
		slog.String("md5", fileMD5))

	uploadID, err := u.uploadInit(ctx, filename, len(data), mediaType, len(chunks), fileMD5)
	if err != nil {
		return "", fmt.Errorf("upload init: %w", err)
	}

	if err := u.uploadChunks(ctx, uploadID, chunks); err != nil {
		return "", fmt.Errorf("upload chunks: %w", err)
	}

	mediaID, err := u.uploadFinish(ctx, uploadID, fileMD5)
	if err != nil {
		return "", fmt.Errorf("upload finish: %w", err)
	}

	u.logger.Info("media upload complete",
		slog.String("filename", filename),
		slog.String("media_id", mediaID))

	return mediaID, nil
}

// uploadInit sends the init command and returns the upload_id.
func (u *UploadManager) uploadInit(ctx context.Context, filename string, fileSize int, mediaType string, chunkNum int, md5sum string) (string, error) {
	reqID := generateReqID(CmdUploadMediaInit)
	body := UploadMediaInitBody{
		Filename:  filename,
		FileSize:  fileSize,
		MediaType: mediaType,
		ChunkNum:  chunkNum,
		MD5:       md5sum,
	}

	frame, err := u.wsClient.SendAndWait(ctx, reqID, body, CmdUploadMediaInit, UploadAckTimeout)
	if err != nil {
		return "", err
	}

	var result UploadMediaInitResult
	if err := json.Unmarshal(frame.Body, &result); err != nil {
		return "", fmt.Errorf("parse init response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("init failed (code %d): %s", result.ErrCode, result.ErrMsg)
	}
	if result.UploadID == "" {
		return "", fmt.Errorf("init response missing upload_id")
	}

	u.logger.Debug("upload init success",
		slog.String("upload_id", result.UploadID),
		slog.Int("server_chunk_size", result.ChunkSize))

	return result.UploadID, nil
}

// uploadChunks uploads all chunks with concurrency control.
func (u *UploadManager) uploadChunks(ctx context.Context, uploadID string, chunks [][]byte) error {
	concurrency := chunkConcurrency(len(chunks))
	sem := make(chan struct{}, concurrency)
	errs := make([]error, len(chunks))
	var wg sync.WaitGroup

	for i, chunk := range chunks {
		wg.Add(1)
		sem <- struct{}{}

		go func(idx int, data []byte) {
			defer wg.Done()
			defer func() { <-sem }()

			errs[idx] = u.uploadChunkWithRetry(ctx, uploadID, idx, data)
		}(i, chunk)
	}

	wg.Wait()

	for i, err := range errs {
		if err != nil {
			return fmt.Errorf("chunk %d: %w", i, err)
		}
	}
	return nil
}

// uploadChunkWithRetry uploads a single chunk, retrying on failure.
func (u *UploadManager) uploadChunkWithRetry(ctx context.Context, uploadID string, index int, data []byte) error {
	var lastErr error
	for attempt := 0; attempt <= MaxUploadRetries; attempt++ {
		if attempt > 0 {
			u.logger.Warn("retrying chunk upload",
				slog.String("upload_id", uploadID),
				slog.Int("chunk", index),
				slog.Int("attempt", attempt))
		}
		if err := u.uploadChunk(ctx, uploadID, index, data); err != nil {
			lastErr = err
			// Do not retry on context cancellation
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		return nil
	}
	return lastErr
}

// uploadChunk uploads a single chunk.
func (u *UploadManager) uploadChunk(ctx context.Context, uploadID string, index int, data []byte) error {
	reqID := generateReqID(fmt.Sprintf("%s_c%d", CmdUploadMediaChunk, index))
	body := UploadMediaChunkBody{
		UploadID:   uploadID,
		ChunkIndex: index,
		ChunkData:  base64.StdEncoding.EncodeToString(data),
	}

	frame, err := u.wsClient.SendAndWait(ctx, reqID, body, CmdUploadMediaChunk, UploadAckTimeout)
	if err != nil {
		return err
	}

	// Parse optional error from body
	if len(frame.Body) > 0 {
		var resp ResponseBody
		if err := json.Unmarshal(frame.Body, &resp); err == nil && resp.ErrCode != 0 {
			return fmt.Errorf("chunk %d failed (code %d): %s", index, resp.ErrCode, resp.ErrMsg)
		}
	}

	u.logger.Debug("chunk uploaded",
		slog.String("upload_id", uploadID),
		slog.Int("chunk", index))

	return nil
}

// uploadFinish sends the finish command and returns the media_id.
func (u *UploadManager) uploadFinish(ctx context.Context, uploadID, fileMD5 string) (string, error) {
	reqID := generateReqID(CmdUploadMediaFinish)
	body := UploadMediaFinishBody{
		UploadID: uploadID,
		MD5:      fileMD5,
	}

	frame, err := u.wsClient.SendAndWait(ctx, reqID, body, CmdUploadMediaFinish, UploadAckTimeout)
	if err != nil {
		return "", err
	}

	var result UploadMediaFinishResult
	if err := json.Unmarshal(frame.Body, &result); err != nil {
		return "", fmt.Errorf("parse finish response: %w", err)
	}
	if result.ErrCode != 0 {
		return "", fmt.Errorf("finish failed (code %d): %s", result.ErrCode, result.ErrMsg)
	}
	if result.MediaID == "" {
		return "", fmt.Errorf("finish response missing media_id")
	}

	return result.MediaID, nil
}

// calculateFileMD5 returns the hex-encoded MD5 hash of data.
func calculateFileMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

// splitIntoChunks splits data into chunks of at most chunkSize bytes.
func splitIntoChunks(data []byte, chunkSize int) [][]byte {
	if len(data) == 0 {
		return nil
	}
	var chunks [][]byte
	for len(data) > 0 {
		size := chunkSize
		if size > len(data) {
			size = len(data)
		}
		chunks = append(chunks, data[:size])
		data = data[size:]
	}
	return chunks
}

// chunkConcurrency returns the upload concurrency for the given chunk count.
func chunkConcurrency(n int) int {
	switch {
	case n <= 4:
		return n
	case n <= 10:
		return 3
	default:
		return 2
	}
}

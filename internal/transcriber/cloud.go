package transcriber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	appaudio "github.com/sawy3r/voxsteer/internal/audio"
)

// Cloud calls an OpenAI-compatible multipart audio transcription endpoint.
type Cloud struct {
	Endpoint string
	APIKey   string
	Model    string
	Language string
	Client   *http.Client
}

// EndpointError preserves provider diagnostics for callers while exposing a
// separate summary that is safe to write to operational logs.
type EndpointError struct {
	StatusCode int
	Status     string
	Details    string
}

func (e *EndpointError) Error() string {
	if e.Details == "" {
		return fmt.Sprintf("transcription endpoint returned %s", e.Status)
	}
	return fmt.Sprintf("transcription endpoint returned %s: %s", e.Status, e.Details)
}

func (e *EndpointError) SafeSummary() string {
	return fmt.Sprintf("endpoint returned HTTP %d", e.StatusCode)
}

func (c *Cloud) Transcribe(ctx context.Context, samples []float32) (string, error) {
	wav, err := appaudio.EncodeWAV(samples)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", "dictation.wav")
	if err != nil {
		return "", fmt.Errorf("create audio form: %w", err)
	}
	if _, err := file.Write(wav); err != nil {
		return "", fmt.Errorf("write audio form: %w", err)
	}
	if err := form.WriteField("model", defaultString(c.Model, "whisper-1")); err != nil {
		return "", err
	}
	if c.Language != "" {
		if err := form.WriteField("language", c.Language); err != nil {
			return "", err
		}
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	client := c.Client
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("transcription request: %w", err)
	}
	defer resp.Body.Close()

	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("read transcription response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &EndpointError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Details:    strings.TrimSpace(string(responseBody)),
		}
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return "", fmt.Errorf("decode transcription response: %w", err)
	}
	return strings.TrimSpace(result.Text), nil
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

package backend

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kataras/golog"
	"github.com/tmc/langchaingo/llms"
)

// QwenImageClient is a client for Alibaba Bailian Qwen image generation.
type QwenImageClient struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// NewQwenImageClient creates a new Qwen image client.
func NewQwenImageClient(apiKey, baseURL string) *QwenImageClient {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1/images/generations"
	}
	return &QwenImageClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
			Transport: &http.Transport{
				DisableKeepAlives: false,
				MaxIdleConns:      100,
				IdleConnTimeout:   5 * time.Minute,
			},
		},
	}
}

// GenerateImage generates an image using Qwen image API and stores it locally.
func (q *QwenImageClient) GenerateImage(ctx context.Context, model, prompt string, userID, imageType string) (string, error) {
	if q.apiKey == "" {
		golog.Errorf("qwen_api_key is not set")
		return "", fmt.Errorf("qwen_api_key is not set")
	}

	if strings.TrimSpace(model) == "" {
		model = "qwen-image"
	}

	requestBody := map[string]interface{}{
		"model":  model,
		"prompt": prompt,
		"size":   "1280*1280",
	}
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	golog.Infof("generating image with Qwen model %s...", model)

	req, err := http.NewRequestWithContext(ctx, "POST", q.baseURL, bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+q.apiKey)

	resp, err := q.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}

	var result struct {
		Data []struct {
			URL    string `json:"url"`
			B64    string `json:"b64_json"`
			Base64 string `json:"base64"`
		} `json:"data"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if resp.StatusCode >= 400 {
		msg := result.Message
		if msg == "" {
			msg = result.Error.Message
		}
		if msg == "" {
			msg = string(respBytes)
		}
		return "", fmt.Errorf("qwen api error (status %d): %s", resp.StatusCode, msg)
	}

	if result.Code != "" && result.Code != "200" {
		msg := result.Message
		if msg == "" {
			msg = result.Error.Message
		}
		return "", fmt.Errorf("qwen api error (%s): %s", result.Code, msg)
	}

	if len(result.Data) == 0 {
		return "", fmt.Errorf("no image data in response")
	}

	var imageData []byte
	if result.Data[0].URL != "" {
		downloadReq, err := http.NewRequestWithContext(ctx, "GET", result.Data[0].URL, nil)
		if err != nil {
			return "", fmt.Errorf("failed to create image download request: %w", err)
		}
		downloadResp, err := q.httpClient.Do(downloadReq)
		if err != nil {
			return "", fmt.Errorf("failed to download image: %w", err)
		}
		defer downloadResp.Body.Close()
		imageData, err = io.ReadAll(downloadResp.Body)
		if err != nil {
			return "", fmt.Errorf("failed to read downloaded image: %w", err)
		}
		if downloadResp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("failed to download image, status: %d", downloadResp.StatusCode)
		}
	} else {
		rawB64 := result.Data[0].B64
		if rawB64 == "" {
			rawB64 = result.Data[0].Base64
		}
		if rawB64 == "" {
			return "", fmt.Errorf("neither image url nor base64 image returned")
		}
		imageData, err = base64.StdEncoding.DecodeString(rawB64)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 image: %w", err)
		}
	}

	fileName := fmt.Sprintf("%s_%d.png", imageType, time.Now().UnixNano())
	uploadDir := "./data/uploads"
	if userID != "" {
		uploadDir = filepath.Join(uploadDir, userID)
	}
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create upload directory: %w", err)
	}
	filePath := filepath.Join(uploadDir, fileName)
	if err := os.WriteFile(filePath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	golog.Infof("%s saved to %s", imageType, filePath)
	return filePath, nil
}

// GenerateTextWithModel is not supported for qwen image client.
func (q *QwenImageClient) GenerateTextWithModel(ctx context.Context, prompt string, model string) (string, error) {
	return "", fmt.Errorf("qwen image client does not support text generation")
}

// GenerateFromSinglePrompt is not supported for qwen image client.
func (q *QwenImageClient) GenerateFromSinglePrompt(ctx context.Context, llm llms.Model, prompt string, options ...llms.CallOption) (string, error) {
	return "", fmt.Errorf("qwen image client does not support text generation")
}

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
	llm        llms.Model
}

const defaultQwenImageModel = "qwen-image-2.0-pro"
const defaultQwenImageSize = "1280*1280"

// NewQwenImageClient creates a new Qwen image client.
func NewQwenImageClient(apiKey, baseURL string, llm llms.Model) *QwenImageClient {
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1"
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
		llm: llm,
	}
}

// qwenSiteRoot returns DashScope site root from configured baseURL.
func (q *QwenImageClient) qwenSiteRoot() string {
	root := strings.TrimSuffix(strings.TrimSpace(q.baseURL), "/")
	markers := []string{"/api/v1", "/compatible-mode", "/services"}
	for _, marker := range markers {
		if idx := strings.Index(root, marker); idx >= 0 {
			return root[:idx]
		}
	}
	return root
}

// qwenAPIRoot returns DashScope API root for native multimodal endpoints.
func (q *QwenImageClient) qwenAPIRoot() string {
	root := strings.TrimSuffix(strings.TrimSpace(q.baseURL), "/")
	if idx := strings.Index(root, "/api/v1"); idx >= 0 {
		return root[:idx+len("/api/v1")]
	}
	siteRoot := q.qwenSiteRoot()
	return siteRoot + "/api/v1"
}

// qwenMultimodalURL returns native multimodal image generation endpoint.
func (q *QwenImageClient) qwenMultimodalURL() string {
	return q.qwenAPIRoot() + "/services/aigc/multimodal-generation/generation"
}

// qwenCompatibleURL returns OpenAI-compatible image generation endpoint.
func (q *QwenImageClient) qwenCompatibleURL() string {
	return q.qwenSiteRoot() + "/compatible-mode/v1/images/generations"
}

// qwenErrorMessage extracts the most useful error message from response payload.
func qwenErrorMessage(respBytes []byte) string {
	var result struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(respBytes, &result); err == nil {
		if strings.TrimSpace(result.Message) != "" {
			return result.Message
		}
		if strings.TrimSpace(result.Error.Message) != "" {
			return result.Error.Message
		}
	}
	return string(respBytes)
}

// downloadImageFromURL downloads image bytes from remote URL.
func (q *QwenImageClient) downloadImageFromURL(ctx context.Context, imageURL string) ([]byte, error) {
	downloadReq, err := http.NewRequestWithContext(ctx, "GET", imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create image download request: %w", err)
	}
	downloadResp, err := q.httpClient.Do(downloadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer downloadResp.Body.Close()

	imageData, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read downloaded image: %w", err)
	}
	if downloadResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download image, status: %d", downloadResp.StatusCode)
	}
	return imageData, nil
}

// GenerateImage generates an image using Qwen image API and stores it locally.
func (q *QwenImageClient) GenerateImage(ctx context.Context, model, prompt string, userID, imageType string) (string, error) {
	if q.apiKey == "" {
		golog.Errorf("qwen_api_key is not set")
		return "", fmt.Errorf("qwen_api_key is not set")
	}

	if strings.TrimSpace(model) == "" {
		model = defaultQwenImageModel
	}

	golog.Infof("generating image with Qwen model %s...", model)
	tryMultimodalFirst := true

	multiRequestBody := struct {
		Model string `json:"model"`
		Input struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text,omitempty"`
				} `json:"content"`
			} `json:"messages"`
		} `json:"input"`
		Parameters struct {
			NegativePrompt string `json:"negative_prompt,omitempty"`
			PromptExtend   bool   `json:"prompt_extend"`
			Watermark      bool   `json:"watermark"`
			Size           string `json:"size,omitempty"`
		} `json:"parameters,omitempty"`
	}{
		Model: model,
	}
	multiRequestBody.Input.Messages = []struct {
		Role    string `json:"role"`
		Content []struct {
			Text string `json:"text,omitempty"`
		} `json:"content"`
	}{
		{
			Role: "user",
			Content: []struct {
				Text string `json:"text,omitempty"`
			}{
				{Text: prompt},
			},
		},
	}
	multiRequestBody.Parameters.NegativePrompt = "低分辨率，低画质，肢体畸形，手指畸形，画面过饱和，蜡像感，人脸无细节，过度光滑，画面具有AI感，构图混乱，文字模糊，扭曲"
	multiRequestBody.Parameters.PromptExtend = true
	multiRequestBody.Parameters.Watermark = false
	multiRequestBody.Parameters.Size = defaultQwenImageSize

	var imageData []byte
	multiStatus := 0
	multiRespBytes := []byte("multimodal skipped")
	if tryMultimodalFirst {
		multiJSONBody, err := json.Marshal(multiRequestBody)
		if err != nil {
			return "", fmt.Errorf("failed to marshal multimodal request body: %w", err)
		}

		multiReq, err := http.NewRequestWithContext(ctx, "POST", q.qwenMultimodalURL(), bytes.NewReader(multiJSONBody))
		if err != nil {
			return "", fmt.Errorf("failed to create multimodal request: %w", err)
		}
		multiReq.Header.Set("Content-Type", "application/json")
		multiReq.Header.Set("Authorization", "Bearer "+q.apiKey)

		multiResp, err := q.httpClient.Do(multiReq)
		if err != nil {
			return "", fmt.Errorf("failed to send multimodal request: %w", err)
		}
		multiStatus = multiResp.StatusCode
		multiRespBytes, err = io.ReadAll(multiResp.Body)
		multiResp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read multimodal response body: %w", err)
		}
		if multiStatus < 400 {
			var multiResult struct {
				Output struct {
					Choices []struct {
						Message struct {
							Content []struct {
								Image string `json:"image,omitempty"`
							} `json:"content"`
						} `json:"message"`
					} `json:"choices"`
					Results []struct {
						URL string `json:"url"`
					} `json:"results"`
				} `json:"output"`
			}
			if err := json.Unmarshal(multiRespBytes, &multiResult); err == nil {
				for _, choice := range multiResult.Output.Choices {
					for _, content := range choice.Message.Content {
						if strings.TrimSpace(content.Image) != "" {
							imageData, err = q.downloadImageFromURL(ctx, content.Image)
							if err != nil {
								return "", err
							}
							break
						}
					}
				}
				if len(imageData) == 0 && len(multiResult.Output.Results) > 0 && strings.TrimSpace(multiResult.Output.Results[0].URL) != "" {
					imageData, err = q.downloadImageFromURL(ctx, multiResult.Output.Results[0].URL)
					if err != nil {
						return "", err
					}
				}
			}
		} else {
			golog.Warnf("multimodal image request failed, status=%d, request_id=%s, msg=%s", multiStatus, multiResp.Header.Get("x-request-id"), qwenErrorMessage(multiRespBytes))
		}
	} else {
		golog.Infof("skip multimodal for model %s, use compatible endpoint first", model)
	}

	if len(imageData) == 0 {
		compatibleRequestBody := map[string]interface{}{
			"model":  model,
			"prompt": prompt,
			"size":   "1280x1280",
		}
		compatibleJSONBody, err := json.Marshal(compatibleRequestBody)
		if err != nil {
			return "", fmt.Errorf("failed to marshal compatible request body: %w", err)
		}

		compatibleReq, err := http.NewRequestWithContext(ctx, "POST", q.qwenCompatibleURL(), bytes.NewReader(compatibleJSONBody))
		if err != nil {
			return "", fmt.Errorf("failed to create compatible request: %w", err)
		}
		compatibleReq.Header.Set("Content-Type", "application/json")
		compatibleReq.Header.Set("Authorization", "Bearer "+q.apiKey)

		compatibleResp, err := q.httpClient.Do(compatibleReq)
		if err != nil {
			return "", fmt.Errorf("failed to send compatible request: %w", err)
		}
		compatibleRespBytes, err := io.ReadAll(compatibleResp.Body)
		compatibleResp.Body.Close()
		if err != nil {
			return "", fmt.Errorf("failed to read compatible response body: %w", err)
		}
		if compatibleResp.StatusCode >= 400 {
			return "", fmt.Errorf(
				"qwen image request failed, multimodal(status=%d,msg=%s), compatible(status=%d,msg=%s)",
				multiStatus,
				qwenErrorMessage(multiRespBytes),
				compatibleResp.StatusCode,
				qwenErrorMessage(compatibleRespBytes),
			)
		}

		var compatibleResult struct {
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
		if err := json.Unmarshal(compatibleRespBytes, &compatibleResult); err != nil {
			return "", fmt.Errorf("failed to decode compatible response: %w", err)
		}
		if compatibleResult.Code != "" && compatibleResult.Code != "200" {
			msg := compatibleResult.Message
			if msg == "" {
				msg = compatibleResult.Error.Message
			}
			return "", fmt.Errorf("qwen compatible api error (%s): %s", compatibleResult.Code, msg)
		}
		if len(compatibleResult.Data) == 0 {
			return "", fmt.Errorf("no image data in compatible response")
		}
		if strings.TrimSpace(compatibleResult.Data[0].URL) != "" {
			imageData, err = q.downloadImageFromURL(ctx, compatibleResult.Data[0].URL)
			if err != nil {
				return "", err
			}
		} else {
			rawB64 := compatibleResult.Data[0].B64
			if rawB64 == "" {
				rawB64 = compatibleResult.Data[0].Base64
			}
			if rawB64 == "" {
				return "", fmt.Errorf("neither image url nor base64 image returned")
			}
			imageData, err = base64.StdEncoding.DecodeString(rawB64)
			if err != nil {
				return "", fmt.Errorf("failed to decode base64 image: %w", err)
			}
		}
	} else {
		golog.Infof("qwen multimodal image generation succeeded via %s", q.qwenMultimodalURL())
	}

	if len(imageData) == 0 {
		return "", fmt.Errorf("image generation succeeded but got empty image data")
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

// GenerateTextWithModel generates text using injected LLM for compatibility.
func (q *QwenImageClient) GenerateTextWithModel(ctx context.Context, prompt string, model string) (string, error) {
	if q.llm == nil {
		return "", fmt.Errorf("llm is not configured for text generation")
	}
	return llms.GenerateFromSinglePrompt(ctx, q.llm, prompt)
}

// GenerateFromSinglePrompt generates text using injected LLM.
func (q *QwenImageClient) GenerateFromSinglePrompt(ctx context.Context, llm llms.Model, prompt string, options ...llms.CallOption) (string, error) {
	if q.llm == nil {
		return "", fmt.Errorf("llm is not configured for text generation")
	}
	return llms.GenerateFromSinglePrompt(ctx, q.llm, prompt, options...)
}

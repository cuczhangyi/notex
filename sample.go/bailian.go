package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// BailianAdapter 阿里百炼适配器
type BailianAdapter struct {
	config       *ModelConfig
	baseURL      string
	apiKey       string
	modelName    string
	client       *http.Client
	streamClient *http.Client
}

type bailianEndpoint struct {
	useSiteRoot bool
	path        string
}

type bailianModelRoute struct {
	chatCompletions bailianEndpoint
	imagePrimary    bailianEndpoint
	imageFallback   bailianEndpoint
}

// NewBailianAdapter 创建阿里百炼适配器
func NewBailianAdapter(config *ModelConfig) (*BailianAdapter, error) {
	baseURL := config.BaseURL
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/api/v1"
	}

	adapter := &BailianAdapter{
		config:    config,
		baseURL:   baseURL,
		apiKey:    config.APIKey,
		modelName: config.ModelName,
		client:    GetHTTPClient().GetClient(),
		streamClient: &http.Client{
			Transport: GetHTTPClient().GetClient().Transport,
		},
	}

	return adapter, nil
}

func (a *BailianAdapter) GetName() string {
	return "bailian"
}

func (a *BailianAdapter) GetModelType() ModelType {
	return a.config.ModelType
}

func (a *BailianAdapter) apiRoot() string {
	return strings.TrimSuffix(a.baseURL, "/")
}

func (a *BailianAdapter) siteRoot() string {
	root := a.apiRoot()
	if strings.HasSuffix(root, "/api/v1") {
		return strings.TrimSuffix(root, "/api/v1")
	}
	return root
}

func (a *BailianAdapter) endpointURL(endpoint bailianEndpoint) string {
	root := a.apiRoot()
	if endpoint.useSiteRoot {
		root = a.siteRoot()
	}
	return fmt.Sprintf("%s%s", root, endpoint.path)
}

func (a *BailianAdapter) modelRoute() bailianModelRoute {
	modelName := strings.ToLower(a.modelName)
	switch {
	case strings.Contains(modelName, "qwen-image"):
		return bailianModelRoute{
			chatCompletions: bailianEndpoint{useSiteRoot: true, path: "/compatible-mode/v1/chat/completions"},
			imagePrimary:    bailianEndpoint{useSiteRoot: false, path: "/services/aigc/multimodal-generation/generation"},
			imageFallback:   bailianEndpoint{useSiteRoot: true, path: "/compatible-mode/v1/images/generations"},
		}
	default:
		return bailianModelRoute{
			chatCompletions: bailianEndpoint{useSiteRoot: true, path: "/compatible-mode/v1/chat/completions"},
			imagePrimary:    bailianEndpoint{useSiteRoot: false, path: "/services/aigc/multimodal-generation/generation"},
			imageFallback:   bailianEndpoint{useSiteRoot: true, path: "/compatible-mode/v1/images/generations"},
		}
	}
}

// BailianChatRequest 阿里百炼聊天请求
type BailianChatRequest struct {
	Model string `json:"model"`
	Input struct {
		Messages []BailianMessage `json:"messages"`
	} `json:"input"`
	Parameters struct {
		Temperature    float64 `json:"temperature,omitempty"`
		MaxTokens      int     `json:"max_tokens,omitempty"`
		EnableThinking *bool   `json:"enable_thinking,omitempty"`
		ResultFormat   string  `json:"result_format,omitempty"`
	} `json:"parameters"`
}

// BailianMessage 阿里百炼消息
type BailianMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// BailianChatResponse 阿里百炼聊天响应
type BailianChatResponse struct {
	Output struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	} `json:"output"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

// ChatCompletion 同步调用
func (a *BailianAdapter) ChatCompletion(ctx context.Context, messages []Message, params map[string]interface{}) (string, error) {
	chatMessages := make([]BailianMessage, len(messages))
	for i, msg := range messages {
		chatMessages[i] = BailianMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	disableThinking := false
	req := struct {
		Model          string           `json:"model"`
		Messages       []BailianMessage `json:"messages"`
		Temperature    float64          `json:"temperature,omitempty"`
		MaxTokens      int              `json:"max_tokens,omitempty"`
		EnableThinking *bool            `json:"enable_thinking,omitempty"`
	}{
		Model:          a.modelName,
		Messages:       chatMessages,
		EnableThinking: &disableThinking,
	}

	if temp, ok := params["temperature"].(float64); ok {
		req.Temperature = temp
	} else if temp, ok := params["temperature"].(int); ok {
		req.Temperature = float64(temp)
	}
	if maxTokens, ok := params["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// 创建请求
	route := a.modelRoute()
	url := a.endpointURL(route.chatCompletions)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))

	// 发送请求
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", buildBailianHTTPError(resp, body)
	}

	// 解析响应
	var chatResp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}

	return chatResp.Choices[0].Message.Content, nil
}

// BailianStreamChunk 阿里百炼流式chunk
type BailianStreamChunk struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Object  string `json:"object"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ChatStream 流式调用
func (a *BailianAdapter) ChatStream(ctx context.Context, messages []Message, params map[string]interface{}) (<-chan StreamChunk, error) {
	chatMessages := make([]BailianMessage, len(messages))
	for i, msg := range messages {
		chatMessages[i] = BailianMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
	}
	disableThinking := false
	req := struct {
		Model          string           `json:"model"`
		Messages       []BailianMessage `json:"messages"`
		Stream         bool             `json:"stream"`
		Temperature    float64          `json:"temperature,omitempty"`
		MaxTokens      int              `json:"max_tokens,omitempty"`
		EnableThinking *bool            `json:"enable_thinking,omitempty"`
	}{
		Model:          a.modelName,
		Messages:       chatMessages,
		Stream:         true,
		EnableThinking: &disableThinking,
	}

	if temp, ok := params["temperature"].(float64); ok {
		req.Temperature = temp
	} else if temp, ok := params["temperature"].(int); ok {
		req.Temperature = float64(temp)
	}
	if maxTokens, ok := params["max_tokens"].(int); ok {
		req.MaxTokens = maxTokens
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	route := a.modelRoute()
	url := a.endpointURL(route.chatCompletions)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))

	// 发送请求
	resp, err := a.streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, buildBailianHTTPError(resp, body)
	}

	ch := make(chan StreamChunk)

	go func() {
		defer func() {
			resp.Body.Close()
			close(ch)
		}()

		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				ch <- StreamChunk{Err: ctx.Err()}
				return
			default:
			}

			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, ":") {
				continue
			}
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if payload == "" || payload == "[DONE]" {
				continue
			}

			var chunk BailianStreamChunk
			if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
				ch <- StreamChunk{Err: fmt.Errorf("failed to decode chunk: %w", err)}
				return
			}
			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				streamChunk := StreamChunk{
					Delta:        choice.Delta.Content,
					FinishReason: choice.FinishReason,
				}
				if streamChunk.FinishReason != "" {
					ch <- streamChunk
					return
				}
				if streamChunk.Delta != "" {
					ch <- streamChunk
				}
			}
		}
		if err := scanner.Err(); err != nil {
			ch <- StreamChunk{Err: fmt.Errorf("failed to read stream response: %w", err)}
		}
	}()

	return ch, nil
}

func buildBailianHTTPError(resp *http.Response, body []byte) error {
	requestID := resp.Header.Get("X-Request-Id")
	if requestID == "" {
		requestID = resp.Header.Get("x-request-id")
	}

	var dashscopeErr struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if err := json.Unmarshal(body, &dashscopeErr); err == nil {
		if dashscopeErr.RequestID != "" {
			requestID = dashscopeErr.RequestID
		}
		if dashscopeErr.Code != "" || dashscopeErr.Message != "" {
			return fmt.Errorf("dashscope request failed, status=%d, code=%s, message=%s, request_id=%s", resp.StatusCode, dashscopeErr.Code, dashscopeErr.Message, requestID)
		}
	}

	return fmt.Errorf("dashscope request failed, status=%d, request_id=%s, body=%s", resp.StatusCode, requestID, string(body))
}

// GenerateImage 文生图（百炼可能不支持）
func (a *BailianAdapter) GenerateImage(ctx context.Context, prompt string, params map[string]interface{}) (*ImageResult, error) {
	compatibleSize := "1792x1024"
	if raw, ok := params["size"].(string); ok && raw != "" {
		compatibleSize = raw
	}
	nativeSize := strings.ReplaceAll(compatibleSize, "x", "*")
	disableThinking := false
	multiReq := struct {
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
			Size           string `json:"size,omitempty"`
			EnableThinking *bool  `json:"enable_thinking,omitempty"`
		} `json:"parameters,omitempty"`
	}{Model: a.modelName}
	multiReq.Input.Messages = []struct {
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
	multiReq.Parameters.Size = nativeSize
	multiReq.Parameters.EnableThinking = &disableThinking

	multiBody, err := json.Marshal(multiReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal multimodal image request: %w", err)
	}

	route := a.modelRoute()
	multiURL := a.endpointURL(route.imagePrimary)
	multiHTTPReq, err := http.NewRequestWithContext(ctx, "POST", multiURL, bytes.NewReader(multiBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create multimodal image request: %w", err)
	}
	multiHTTPReq.Header.Set("Content-Type", "application/json")
	multiHTTPReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))

	multiResp, err := a.client.Do(multiHTTPReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send multimodal image request: %w", err)
	}
	multiRespBody, readErr := io.ReadAll(multiResp.Body)
	multiResp.Body.Close()
	if readErr != nil {
		return nil, fmt.Errorf("failed to read multimodal image response: %w", readErr)
	}
	if multiResp.StatusCode == http.StatusOK {
		var multiParsed struct {
			Output struct {
				Choices []struct {
					Message struct {
						Content []struct {
							Image string `json:"image,omitempty"`
						} `json:"content"`
					} `json:"message"`
				} `json:"choices"`
			} `json:"output"`
		}
		if err := json.Unmarshal(multiRespBody, &multiParsed); err == nil {
			for _, choice := range multiParsed.Output.Choices {
				for _, content := range choice.Message.Content {
					if content.Image != "" {
						return &ImageResult{
							URL:           content.Image,
							RevisedPrompt: prompt,
						}, nil
					}
				}
			}
		}
	}
	multiErr := fmt.Sprintf("multimodal image response parse failed: %s", string(multiRespBody))
	if multiResp.StatusCode != http.StatusOK {
		multiErr = buildBailianHTTPError(multiResp, multiRespBody).Error()
	}

	compatibleReq := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
		N      int    `json:"n,omitempty"`
		Size   string `json:"size,omitempty"`
	}{
		Model:  a.modelName,
		Prompt: prompt,
		N:      1,
		Size:   compatibleSize,
	}

	reqBody, err := json.Marshal(compatibleReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	compatibleURL := a.endpointURL(route.imageFallback)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", compatibleURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", a.apiKey))

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		compatibleErr := fmt.Sprintf("compatible request failed with status %d: %s", resp.StatusCode, string(body))
		return nil, fmt.Errorf("%s; %s", multiErr, compatibleErr)
	}

	var compatibleResp struct {
		Data []struct {
			URL     string `json:"url"`
			Revised string `json:"revised_prompt"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &compatibleResp); err == nil && len(compatibleResp.Data) > 0 && compatibleResp.Data[0].URL != "" {
		return &ImageResult{
			URL:           compatibleResp.Data[0].URL,
			RevisedPrompt: compatibleResp.Data[0].Revised,
		}, nil
	}

	var nativeParsed struct {
		Output struct {
			Results []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"output"`
	}
	if err := json.Unmarshal(body, &nativeParsed); err == nil && len(nativeParsed.Output.Results) > 0 && nativeParsed.Output.Results[0].URL != "" {
		return &ImageResult{
			URL:           nativeParsed.Output.Results[0].URL,
			RevisedPrompt: prompt,
		}, nil
	}

	return nil, fmt.Errorf("%s; failed to parse image response: %s", multiErr, string(body))
}

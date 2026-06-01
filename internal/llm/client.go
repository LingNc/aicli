package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/log"
	"github.com/lingnc/aicli/internal/prompt"
)

// Client 是 OpenAI 兼容 API 客户端
type Client struct {
	cfg  *config.Config
	http *http.Client
}

// New 创建 LLM 客户端
func New(cfg *config.Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 60 * time.Second},
	}
}

// chatRequest 是 API 请求结构体
type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse 是 SSE 中的 JSON 结构
type chatResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// StreamResult 是流式回调的结果
type StreamResult struct {
	FullContent string
	Duration    time.Duration
}

// StreamChat 发送流式请求，对每个 chunk 调用 callback
func (c *Client) StreamChat(userInput string, callback func(chunk string)) (*StreamResult, error) {
	start := time.Now()

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/chat/completions"

	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []message{
			{Role: "system", Content: fmt.Sprintf(prompt.System, prompt.BuildSystemInfo())},
			{Role: "user", Content: fmt.Sprintf("[时间: %s]\n%s", time.Now().Format("2006-01-02 15:04:05"), userInput)},
		},
		Stream:      true,
		Temperature: 0.1,
		MaxTokens:   1024,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	log.Debug("请求 URL: %s", url)
	log.Debug("请求体: %s", string(body))

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API 请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误 (HTTP %d): %s", resp.StatusCode, string(errBody))
	}

	var fullContent strings.Builder
	scanner := bufio.NewScanner(resp.Body)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var cr chatResponse
		if err := json.Unmarshal([]byte(data), &cr); err != nil {
			log.Debug("解析 SSE 失败: %v (data: %s)", err, data)
			continue
		}

		if len(cr.Choices) > 0 && cr.Choices[0].Delta.Content != "" {
			chunk := cr.Choices[0].Delta.Content
			fullContent.WriteString(chunk)
			callback(chunk)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取流式响应失败: %w", err)
	}

	result := &StreamResult{
		FullContent: fullContent.String(),
		Duration:    time.Since(start),
	}

	log.Debug("完整响应: %s", result.FullContent)
	log.Debug("耗时: %v", result.Duration)

	return result, nil
}

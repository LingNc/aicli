package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/lingnc/aicli/internal/config"
	"github.com/lingnc/aicli/internal/log"
	"github.com/lingnc/aicli/internal/prompt"
)

// Client 是 OpenAI 兼容 API 客户端
type Client struct {
	cfg           *config.Config
	http          *http.Client
	apiTimeout    time.Duration
	streamTimeout time.Duration
}

// New 创建 LLM 客户端
func New(cfg *config.Config) *Client {
	apiTimeout := 300
	if cfg.APITimeout > 0 {
		apiTimeout = cfg.APITimeout
	}
	streamTimeout := 30
	if cfg.StreamTimeout > 0 {
		streamTimeout = cfg.StreamTimeout
	}
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
	}
	return &Client{
		cfg:           cfg,
		http:          &http.Client{Transport: transport},
		apiTimeout:    time.Duration(apiTimeout) * time.Second,
		streamTimeout: time.Duration(streamTimeout) * time.Second,
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
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content,omitempty"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

// StreamResult 是流式回调的结果
type StreamResult struct {
	FullContent string
	Duration    time.Duration
}

// StreamChat 发送流式请求，对每个 chunk 调用 callback，对每个 reasoning chunk 调用 reasoningCallback
func (c *Client) StreamChat(userInput string, think bool, callback func(chunk string), reasoningCallback func(chunk string)) (*StreamResult, error) {
	start := time.Now()

	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/chat/completions"

	temp := 0.1
	if v, ok := c.cfg.RequestBody["temperature"]; ok {
		switch n := v.(type) {
		case int:
			temp = float64(n)
		case float64:
			temp = n
		}
	}
	maxTok := 1024
	if v, ok := c.cfg.RequestBody["max_tokens"]; ok {
		switch n := v.(type) {
		case int:
			maxTok = n
		case float64:
			maxTok = int(n)
		}
	}

	reqBody := chatRequest{
		Model: c.cfg.Model,
		Messages: []message{
			{Role: "system", Content: prompt.System},
			{Role: "user", Content: fmt.Sprintf("#( %s #)", prompt.BuildSystemInfo())},
			{Role: "user", Content: fmt.Sprintf("#( %s #)\n%s", time.Now().Format("2006-01-02 15:04:05"), userInput)},
		},
		Stream:      true,
		Temperature: temp,
		MaxTokens:   maxTok,
	}

	// reqBody 是纯值类型，Marshal 必定成功
	bodyBytes, _ := json.Marshal(reqBody)
	var bodyMap map[string]any
	_ = json.Unmarshal(bodyBytes, &bodyMap)

	// 合并 request_body 中的额外参数（extra_body 子 map 展平合并）
	for k, v := range c.cfg.RequestBody {
		if k == "temperature" || k == "max_tokens" || k == "extra_body" {
			continue // 已处理或单独处理
		}
		bodyMap[k] = v
	}
	if eb, ok := c.cfg.RequestBody["extra_body"].(map[string]any); ok {
		maps.Copy(bodyMap, eb)
	}

	// 合并 thinking_body（启用思考模式时）
	if think && len(c.cfg.ThinkingBody) > 0 {
		for k, v := range c.cfg.ThinkingBody {
			if k == "extra_body" {
				continue
			}
			bodyMap[k] = v
		}
		if eb, ok := c.cfg.ThinkingBody["extra_body"].(map[string]any); ok {
			maps.Copy(bodyMap, eb)
		}
	}

	body, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	log.Debug("请求 URL: %s", url)
	if log.IsDebug() {
		var pretty bytes.Buffer
		if json.Indent(&pretty, body, "", "  ") == nil {
			log.Debug("请求体:\n%s", pretty.String())
		} else {
			log.Debug("请求体: %s", string(body))
		}
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	// 首字节超时：通过 request context 控制，ctx 超时只影响响应头返回，
	// 一旦服务端返回响应头，body 流式读取不再受 ctx 限制。
	ctx, cancel := context.WithTimeout(req.Context(), c.apiTimeout)
	defer cancel()
	req = req.WithContext(ctx)

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
	var contentBuf strings.Builder
	var reasoningBuf strings.Builder
	var lastRaw json.RawMessage
	scanner := bufio.NewScanner(resp.Body)

	type streamChunk struct {
		line string
		err  error
	}
	chunkCh := make(chan streamChunk, 1)
	stopCh := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for scanner.Scan() {
			select {
			case chunkCh <- streamChunk{line: scanner.Text()}:
			case <-stopCh:
				return
			}
		}
		select {
		case chunkCh <- streamChunk{err: scanner.Err()}:
		case <-stopCh:
		}
	}()

streaming:
	for {
		select {
		case chunk, ok := <-chunkCh:
			if !ok {
				break streaming
			}
			if chunk.err != nil {
				return nil, fmt.Errorf("读取流式响应失败: %w", chunk.err)
			}
			line := chunk.line
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				break streaming
			}

			var cr chatResponse
			if err := json.Unmarshal([]byte(data), &cr); err != nil {
				log.Debug("解析 SSE 失败: %v (data: %s)", err, data)
				continue
			}

			if len(cr.Choices) > 0 && log.IsDebug() {
				delta := cr.Choices[0].Delta
				if delta.Content != "" {
					contentBuf.WriteString(delta.Content)
				}
				if delta.ReasoningContent != "" {
					reasoningBuf.WriteString(delta.ReasoningContent)
				}
			}

			if len(cr.Choices) > 0 && cr.Choices[0].Delta.Content != "" {
				chunkText := cr.Choices[0].Delta.Content
				fullContent.WriteString(chunkText)
				callback(chunkText)
			}
			if len(cr.Choices) > 0 && cr.Choices[0].Delta.ReasoningContent != "" {
				if reasoningCallback != nil {
					reasoningCallback(cr.Choices[0].Delta.ReasoningContent)
				}
			}
			if log.IsDebug() {
				lastRaw = json.RawMessage(data)
			}
		case <-time.After(c.streamTimeout):
			close(stopCh)
			return nil, fmt.Errorf("流式响应超时（%v 无数据）", c.streamTimeout)
		}
	}
	close(stopCh)
	<-done

	result := &StreamResult{
		FullContent: fullContent.String(),
		Duration:    time.Since(start),
	}

	log.Debug("完整响应: %q", result.FullContent)
	log.Debug("耗时: %v", result.Duration)

	// 输出完整原始响应体：以最后一个 chunk 为基础，替换 delta 为合并后的完整 message
	if log.IsDebug() && len(lastRaw) > 0 {
		var lastObj map[string]any
		if json.Unmarshal(lastRaw, &lastObj) == nil {
			if choices, ok := lastObj["choices"].([]any); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]any); ok {
					choice["message"] = map[string]any{
						"role":              "assistant",
						"content":           contentBuf.String(),
						"reasoning_content": reasoningBuf.String(),
					}
					delete(choice, "delta")
				}
			}
			if b, err := json.MarshalIndent(lastObj, "", "  "); err == nil {
				log.Debug("原始响应体: %s", string(b))
			}
		}
	}

	return result, nil
}

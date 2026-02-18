package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	openai "github.com/sashabaranov/go-openai"
	"google.golang.org/genai"
)

type providerClient interface {
	Stream(ctx context.Context, prompt string, out chan<- string) error
}

type openAIProvider struct {
	client *openai.Client
	model  string
}

func newOpenAIProvider(apiKey, model string) (providerClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("openai api key is empty")
	}
	if model == "" {
		model = "gpt-4.1-mini"
	}
	return &openAIProvider{
		client: openai.NewClient(apiKey),
		model:  model,
	}, nil
}

func (p *openAIProvider) Stream(ctx context.Context, prompt string, out chan<- string) error {
	req := openai.ChatCompletionRequest{
		Model: p.model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleUser,
				Content: prompt,
			},
		},
		Temperature: 0.2,
		Stream:      true,
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		resp, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		for _, choice := range resp.Choices {
			if choice.Delta.Content != "" {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case out <- choice.Delta.Content:
				}
			}
		}
	}
}

type ollamaProvider struct {
	client   *http.Client
	endpoint string
	model    string
}

func newOllamaProvider(endpoint, model string) providerClient {
	if endpoint == "" {
		endpoint = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3"
	}
	return &ollamaProvider{
		client: &http.Client{
			Timeout: 0, // stream contínuo
		},
		endpoint: strings.TrimRight(endpoint, "/"),
		model:    model,
	}
}

func (p *ollamaProvider) Stream(ctx context.Context, prompt string, out chan<- string) error {
	payload := map[string]interface{}{
		"model":  p.model,
		"prompt": prompt,
		"stream": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
		return fmt.Errorf("ollama returned %d: %s", resp.StatusCode, string(raw))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)

	for scanner.Scan() {
		var item struct {
			Response string `json:"response"`
			Done     bool   `json:"done"`
			Error    string `json:"error"`
		}

		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			continue
		}
		if item.Error != "" {
			return fmt.Errorf("ollama error: %s", item.Error)
		}
		if item.Response != "" {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- item.Response:
			}
		}
		if item.Done {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

type geminiProvider struct {
	client *genai.Client
	model  string
}

func newGeminiProvider(apiKey, model string) (providerClient, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("gemini api key is empty")
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}

	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey:  apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, err
	}

	return &geminiProvider{
		client: client,
		model:  model,
	}, nil
}

func (p *geminiProvider) Stream(ctx context.Context, prompt string, out chan<- string) error {
	if p.client == nil {
		return fmt.Errorf("gemini client is nil")
	}

	config := &genai.GenerateContentConfig{
		Temperature: genai.Ptr[float32](0.2),
	}

	// lastText é usado para extrair apenas o novo conteúdo caso o SDK retorne o texto acumulado.
	lastLen := 0

	for result, err := range p.client.Models.GenerateContentStream(ctx, p.model, genai.Text(prompt), config) {
		if err != nil {
			return err
		}
		if result == nil {
			continue
		}

		fullText := result.Text()
		if len(fullText) > lastLen {
			chunk := fullText[lastLen:]
			lastLen = len(fullText)
			
			select {
			case <-ctx.Done():
				return ctx.Err()
			case out <- chunk:
			}
		}
	}

	return nil
}

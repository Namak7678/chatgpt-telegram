package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type StreamEvent struct {
	Text string
	Err  error
}

type Client struct {
	endpoint string
	model    string
	http     *http.Client
}

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
	Done  bool   `json:"done"`
	Error string `json:"error"`
}

func New(baseURL, model string) (*Client, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid OLLAMA_URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("OLLAMA_URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("OLLAMA_URL must not contain credentials, a query, or a fragment")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "host.docker.internal" {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return nil, fmt.Errorf("OLLAMA_URL must point to this host or Docker Desktop's host.docker.internal")
		}
	}
	if model == "" {
		return nil, fmt.Errorf("OLLAMA_MODEL is required")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{
		endpoint: strings.TrimRight(baseURL, "/") + "/api/chat",
		model:    model,
		http:     &http.Client{Transport: transport},
	}, nil
}

func (c *Client) Stream(ctx context.Context, messages []Message) (<-chan StreamEvent, error) {
	body, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	})
	if err != nil {
		return nil, fmt.Errorf("encode Ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Ollama: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		defer resp.Body.Close()
		errorBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if len(errorBody) == 0 {
			return nil, fmt.Errorf("Ollama returned %s", resp.Status)
		}
		return nil, fmt.Errorf("Ollama returned %s: %s", resp.Status, strings.TrimSpace(string(errorBody)))
	}

	events := make(chan StreamEvent, 8)
	go func() {
		defer close(events)
		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)
		for {
			var chunk chatResponse
			if err := decoder.Decode(&chunk); err != nil {
				if err != io.EOF {
					if !sendEvent(ctx, events, StreamEvent{Err: fmt.Errorf("decode Ollama response: %w", err)}) {
						return
					}
				}
				return
			}
			if chunk.Error != "" {
				sendEvent(ctx, events, StreamEvent{Err: fmt.Errorf("Ollama: %s", chunk.Error)})
				return
			}
			if chunk.Message.Content != "" {
				if !sendEvent(ctx, events, StreamEvent{Text: chunk.Message.Content}) {
					return
				}
			}
			if chunk.Done {
				return
			}
		}
	}()

	return events, nil
}

func sendEvent(ctx context.Context, events chan<- StreamEvent, event StreamEvent) bool {
	select {
	case events <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

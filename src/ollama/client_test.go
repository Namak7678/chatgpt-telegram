package ollama

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamReadsOllamaChunks(t *testing.T) {
	requests := make(chan chatRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.Error(w, "unexpected endpoint", http.StatusBadRequest)
			return
		}
		var request chatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte("{\"message\":{\"content\":\"hello\"},\"done\":false}\n"))
		_, _ = w.Write([]byte("{\"message\":{\"content\":\" world\"},\"done\":true}\n"))
	}))
	defer server.Close()

	client, err := New(server.URL, "test-model")
	require.NoError(t, err)
	events, err := client.Stream(context.Background(), []Message{{Role: "user", Content: "hello"}})
	require.NoError(t, err)

	var answer string
	for event := range events {
		require.NoError(t, event.Err)
		answer += event.Text
	}
	require.Equal(t, "hello world", answer)
	request := <-requests
	require.Equal(t, "test-model", request.Model)
	require.True(t, request.Stream)
	require.Equal(t, []Message{{Role: "user", Content: "hello"}}, request.Messages)
}

func TestNewRejectsInvalidConfiguration(t *testing.T) {
	_, err := New("localhost:11434", "test-model")
	require.ErrorContains(t, err, "absolute HTTP or HTTPS")
	_, err = New("http://localhost:11434", "")
	require.ErrorContains(t, err, "OLLAMA_MODEL")
	_, err = New("https://example.com", "test-model")
	require.ErrorContains(t, err, "must point to this host")
}

package ollama

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClient_ChatWithTools_NonNativeReturnsGuardError(t *testing.T) {
	client := NewClient("http://localhost:11434", "test-model", "")
	client.SetNativeToolCalling(false)

	resp, err := client.ChatWithTools(
		context.Background(),
		[]Message{{Role: "user", Content: "ping"}},
		[]LLMToolSpec{{Name: "echo"}},
		nil,
	)

	require.ErrorIs(t, err, ErrNativeToolCallingRequired)
	require.Nil(t, resp)
}

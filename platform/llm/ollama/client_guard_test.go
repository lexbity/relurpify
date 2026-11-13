package ollama

import (
	"context"
	"testing"

	"codeburg.org/lexbit/relurpify/model"
	"github.com/stretchr/testify/require"
)

func TestClient_ChatWithTools_NonNativeReturnsGuardError(t *testing.T) {
	client := NewClient("http://localhost:11434", "test-model", "")
	client.SetNativeToolCalling(false)

	resp, err := client.ChatWithTools(
		context.Background(),
		[]model.Message{{Role: "user", Content: "ping"}},
		[]model.LLMToolSpec{{Name: "echo"}},
		nil,
	)

	require.ErrorIs(t, err, ErrNativeToolCallingRequired)
	require.Nil(t, resp)
}

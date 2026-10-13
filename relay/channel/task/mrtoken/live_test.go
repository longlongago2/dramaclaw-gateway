package mrtoken

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Explicit opt-in: this test submits one paid, one-second H3 video task.
func TestMrTokenLiveVideo(t *testing.T) {
	keyFile := os.Getenv("MRTOKEN_LIVE_KEY_FILE")
	if keyFile == "" {
		t.Skip("set MRTOKEN_LIVE_KEY_FILE to opt in to a paid one-second video")
	}
	secret, err := os.ReadFile(keyFile)
	require.NoError(t, err)
	var credential struct {
		Key string `json:"key"`
	}
	require.NoError(t, common.Unmarshal(secret, &credential))
	require.True(t, credential.Key != "", "platform credential is missing")
	service.InitHttpClient()
	client, err := platformClient("")
	require.NoError(t, err)
	request, err := http.NewRequest(http.MethodGet, "https://api.mrtoken.cn/v1/models", nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+credential.Key)
	response, err := client.Do(request)
	require.NoError(t, err)
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	require.NoError(t, err)
	require.Equal(t, 200, response.StatusCode)
	var catalog struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(data, &catalog))
	found := false
	for _, entry := range catalog.Data {
		if entry.ID == "minimax-h3-t2v" {
			found = true
		}
	}
	require.True(t, found, "H3 T2V is unavailable")
	a, c, info, _ := videoContext(t, `{"model":"H3-local","prompt":"A blue paper boat floating slowly on still water, fixed camera, simple scene.","duration":1,"width":512,"height":512,"metadata":{"steps":8}}`, "minimax-h3-t2v")
	info.PublicTaskID = model.GenerateTaskID()
	info.ApiKey = credential.Key
	a.Init(info)
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	response, err = a.DoRequest(c, info, body)
	require.NoError(t, err)
	require.Contains(t, []int{200, 201, 202}, response.StatusCode)
	taskID, _, taskErr := a.DoResponse(c, response, info)
	require.Nil(t, taskErr)
	require.True(t, taskID != "")
	t.Log("H3 video accepted; polling original task")
	deadline := time.Now().Add(5 * time.Minute)
	completed := false
	for time.Now().Before(deadline) {
		response, err = a.FetchTask("https://api.mrtoken.cn", credential.Key, map[string]any{"task_id": taskID, "model": "minimax-h3-t2v"}, "")
		require.NoError(t, err)
		data, err = io.ReadAll(response.Body)
		response.Body.Close()
		require.NoError(t, err)
		result, err := a.ParseTaskResult(data)
		require.NoError(t, err)
		require.NotEqual(t, model.TaskStatusFailure, result.Status, "upstream generation failed")
		if result.Status == model.TaskStatusSuccess {
			completed = true
			break
		}
		time.Sleep(3 * time.Second)
	}
	require.True(t, completed, "video did not complete before timeout; do not resubmit automatically")
	request, err = http.NewRequest(http.MethodGet, fmt.Sprintf("https://api.mrtoken.cn/v1/videos/%s/content?model=minimax-h3-t2v", taskID), nil)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+credential.Key)
	response, err = client.Do(request)
	require.NoError(t, err)
	defer response.Body.Close()
	require.Equal(t, 200, response.StatusCode)
	assert.True(t, strings.HasPrefix(response.Header.Get("Content-Type"), "video/"))
	data, err = io.ReadAll(io.LimitReader(response.Body, 20<<20))
	require.NoError(t, err)
	require.Greater(t, len(data), 12)
	assert.Equal(t, "ftyp", string(data[4:8]))
	t.Logf("H3 video completed and MP4 downloaded: %d bytes", len(data))
}

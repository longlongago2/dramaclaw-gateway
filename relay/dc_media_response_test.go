package relay

import (
	"net/url"
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDCMediaTaskResponseIncludesCanonicalFields(t *testing.T) {
	task := &model.Task{
		TaskID:   "task_public",
		Status:   model.TaskStatusSuccess,
		Progress: "100%",
		Properties: model.Properties{
			OriginModelName: "example-video-model",
		},
		PrivateData: model.TaskPrivateData{ResultURL: "https://example.com/result.mp4"},
	}
	response := buildDCMediaTaskResponse(task)
	assert.Equal(t, task.TaskID, response["id"])
	assert.Equal(t, task.TaskID, response["task_id"])
	assert.Equal(t, "succeeded", response["status"])
	assert.Equal(t, 100, response["progress"])
	require.Len(t, response["results"], 1)
	assert.Equal(t, "https://example.com/result.mp4", response["result_url"])
	assert.NotContains(t, response, "code")
	assert.NotContains(t, response, "data")
}

func TestBuildDCMediaTaskResponseUsesStableTerminalStates(t *testing.T) {
	assert.Equal(t, "cancelled", mapTaskStatusToDCMedia(model.TaskStatusCancelled))
	assert.Equal(t, "failed", mapTaskStatusToDCMedia(model.TaskStatusFailure))
	assert.Equal(t, "running", mapTaskStatusToDCMedia(model.TaskStatusInProgress))
	assert.Equal(t, 100, taskProgressPercent("125%"))
}

func TestMrTokenCompletedResponseAllowsSignedDownload(t *testing.T) {
	task := &model.Task{
		TaskID: "task_public", Platform: constant.TaskPlatformMrToken,
		Status:      model.TaskStatusSuccess,
		PrivateData: model.TaskPrivateData{ResultURL: "/v1/videos/task_public/content"},
	}
	response := buildDCMediaTaskResponse(task)
	resultURL, ok := response["result_url"].(string)
	require.True(t, ok)
	parsed, err := url.Parse(resultURL)
	require.NoError(t, err)
	assert.Equal(t, "/v1/public/videos/task_public/content", parsed.Path)
	expires, err := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	require.NoError(t, err)
	signature := parsed.Query().Get("signature")
	assert.True(t, taskcommon.ValidatePublicProxySignature(task.TaskID, expires, signature))
	assert.False(t, taskcommon.ValidatePublicProxySignature("another_task", expires, signature))
}

func TestMrTokenSeedanceResponsePreservesNativeMediaURL(t *testing.T) {
	task := &model.Task{TaskID: "task_seedance", Platform: constant.TaskPlatformMrToken, Status: model.TaskStatusSuccess}
	task.Properties.UpstreamModelName = "doubao-seedance-2.0-mini"
	task.PrivateData.ResultURL = "https://example.test/seedance.mp4"
	response := buildDCMediaTaskResponse(task)
	assert.Equal(t, task.PrivateData.ResultURL, response["result_url"])
}

package mrtoken

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHappyHorseCanonicalRequests(t *testing.T) {
	for _, version := range []string{"1.0", "1.1"} {
		for _, mode := range []string{"t2v", "i2v", "r2v"} {
			t.Run(version+mode, func(t *testing.T) {
				extra := ""
				if mode == "i2v" {
					extra = `,"image":"https://example.test/first.png"`
				}
				refs := ""
				if mode == "r2v" {
					refs = `,"reference_images":["data:image/png;base64,aGVsbG8="]`
				}
				a, c, info, _ := videoContext(t, `{"model":"local-horse","prompt":"boat","duration":3`+extra+`,"metadata":{"ratio":"16:9","resolution":"720p","seed":0,"watermark":false,"generate_audio":false,"return_last_frame":false`+refs+`}}`, "happyhorse-"+version+"-"+mode)
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				endpoint, err := a.BuildRequestURL(info)
				require.NoError(t, err)
				assert.Equal(t, "https://api.mrtoken.cn/v1/video-generation/video-synthesis", endpoint)
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				body, err := io.ReadAll(reader)
				require.NoError(t, err)
				assert.NotContains(t, string(body), "generate_audio")
				assert.NotContains(t, string(body), "return_last_frame")
				var payload happyHorseRequest
				require.NoError(t, common.Unmarshal(body, &payload))
				assert.Equal(t, info.UpstreamModelName, payload.Model)
				assert.Equal(t, "boat", payload.Input.Prompt)
				require.NotNil(t, payload.Parameters.Seed)
				assert.EqualValues(t, 0, *payload.Parameters.Seed)
				require.NotNil(t, payload.Parameters.Watermark)
				assert.False(t, *payload.Parameters.Watermark)
				assert.Equal(t, "720P", *payload.Parameters.Resolution)
				assert.Equal(t, map[string]float64{"seconds": 3}, a.EstimateBilling(c, info))
				if mode == "t2v" {
					assert.Empty(t, payload.Input.Media)
				} else {
					require.Len(t, payload.Input.Media, 1)
					if mode == "i2v" {
						assert.Equal(t, "first_frame", payload.Input.Media[0].Type)
						assert.Nil(t, payload.Parameters.Ratio)
					} else {
						assert.Equal(t, "reference_image", payload.Input.Media[0].Type)
					}
				}
			})
		}
	}
}

func TestHappyHorseRejectsInvalidInputsBeforeBilling(t *testing.T) {
	for _, test := range []struct{ name, mode, extra string }{
		{"short duration", "t2v", `,"duration":2`},
		{"long duration", "t2v", `,"duration":16`},
		{"overflow duration", "t2v", `,"duration":9223372036854775807`},
		{"image on text", "t2v", `,"image":"https://example.test/ref.png"`},
		{"missing image", "i2v", ``},
		{"missing references", "r2v", ``},
		{"excess references", "r2v", `,"metadata":{"reference_images":["a","b","c","d","e","f","g","h","i","j"]}`},
		{"video unsupported", "t2v", `,"metadata":{"reference_videos":["https://example.test/ref.mp4"]}`},
		{"audio enabled", "t2v", `,"metadata":{"generate_audio":true}`},
		{"tail output enabled", "t2v", `,"metadata":{"return_last_frame":true}`},
		{"supplier duration bypass", "t2v", `,"metadata":{"parameters":{"duration":99999}}`},
		{"negative seed", "t2v", `,"metadata":{"seed":-1}`},
		{"invalid resolution", "t2v", `,"metadata":{"resolution":"4k"}`},
		{"tail frame", "i2v", `,"image":"https://example.test/ref.png","metadata":{"last_frame_image":"https://example.test/tail.png"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, info, _ := videoContext(t, `{"model":"local-horse","prompt":"boat"`+test.extra+`}`, "happyhorse-1.0-"+test.mode)
			taskErr := a.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Nil(t, a.EstimateBilling(c, info))
		})
	}
}

func TestHappyHorseSubmissionPollingAndDelivery(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/video-generation/tasks/horse-upstream", r.URL.Path)
		assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
		_, err := io.WriteString(w, `{"output":{"task_id":"horse-upstream","task_status":"SUCCEEDED","video_url":"https://example.test/result.mp4"}}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	a, c, info, _ := videoContext(t, `{"model":"local-horse","prompt":"boat"}`, "happyhorse-1.0-t2v")
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	id, _, taskErr := a.DoResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"horse-upstream","task_status":"PENDING"}}`))}, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "horse-upstream", id)
	resp, err := a.FetchTask(server.URL+"/v1", "test-platform-key", map[string]any{"task_id": id, "model": "happyhorse-1.0-t2v"}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	result, err := a.ParseTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://example.test/result.mp4", result.Url)
	_, err = a.ParseTaskResult([]byte(`{"output":{"task_status":"SUCCEEDED"}}`))
	require.Error(t, err)
	failure, err := a.ParseTaskResult([]byte(`{"output":{"task_status":"FAILED","message":"bad test-platform-key"}}`))
	require.NoError(t, err)
	assert.NotContains(t, failure.Reason, "test-platform-key")
	assert.Contains(t, failure.Reason, "[REDACTED]")
	_, err = a.ParseTaskResult([]byte(`{"output":{"task_status":"UNKNOWN"}}`))
	require.Error(t, err)
	video, err := a.ConvertToOpenAIVideo(&model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Properties: model.Properties{UpstreamModelName: "happyhorse-1.0-t2v"}, PrivateData: model.TaskPrivateData{ResultURL: "https://example.test/result.mp4"}})
	require.NoError(t, err)
	assert.True(t, bytes.Contains(video, []byte("https://example.test/result.mp4")))
}

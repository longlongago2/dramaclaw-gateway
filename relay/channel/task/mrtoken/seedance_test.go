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

func TestSeedancePlatformRequestAndLifecycle(t *testing.T) {
	service.InitHttpClient()
	for _, name := range []string{"doubao-seedance-2.0", "doubao-seedance-2.0-fast", "doubao-seedance-2.0-mini"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
				if r.Method == http.MethodPost {
					assert.Equal(t, "/v1/video-generation/seedance", r.URL.Path)
					assert.Equal(t, "task_public", r.Header.Get("Idempotency-Key"))
					var payload map[string]any
					require.NoError(t, common.DecodeJson(r.Body, &payload))
					assert.Equal(t, name, payload["model"])
					assert.Equal(t, float64(4), payload["duration"])
					assert.Equal(t, "480p", payload["resolution"])
					assert.Equal(t, "16:9", payload["ratio"])
					assert.Equal(t, float64(0), payload["seed"])
					assert.Equal(t, false, payload["generate_audio"])
					assert.Equal(t, false, payload["return_last_frame"])
					assert.Equal(t, false, payload["watermark"])
					assert.NotContains(t, payload, "width")
					content, ok := payload["content"].([]any)
					require.True(t, ok)
					require.Len(t, content, 1)
					assert.Equal(t, map[string]any{"type": "text", "text": "boat"}, content[0])
					w.WriteHeader(http.StatusAccepted)
					_, err := io.WriteString(w, `{"data":{"taskId":"cgt_upstream"}}`)
					require.NoError(t, err)
					return
				}
				assert.Equal(t, "/v1/video-generation/seedance/tasks/cgt_upstream", r.URL.Path)
				assert.Empty(t, r.URL.RawQuery)
				_, err := io.WriteString(w, `{"id":"cgt_upstream","status":"SUCCEEDED","content":{"video_url":"https://example.test/seedance.mp4"}}`)
				require.NoError(t, err)
			}))
			defer server.Close()
			a, c, info, recorder := videoContext(t, `{"model":"Seedance-local","prompt":"boat","duration":4,"seed":0,"metadata":{"resolution":"480p","ratio":"16:9","generate_audio":false,"return_last_frame":false,"watermark":false,"content":[{"type":"text","text":"untrusted override"}]}}`, name)
			a.baseURL = server.URL
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			assert.Equal(t, map[string]float64{"seconds": 4}, a.EstimateBilling(c, info))
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			resp, err := a.DoRequest(c, info, body)
			require.NoError(t, err)
			id, _, taskErr := a.DoResponse(c, resp, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "cgt_upstream", id)
			assert.Contains(t, recorder.Body.String(), `"id":"task_public"`)
			assert.NotContains(t, recorder.Body.String(), id)
			assert.True(t, c.GetBool("task_submit_no_retry"))
			resp, err = a.FetchTask(server.URL+"/v1", a.apiKey, map[string]any{"task_id": id, "model": name}, "")
			require.NoError(t, err)
			defer resp.Body.Close()
			data, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			result, err := a.ParseTaskResult(data)
			require.NoError(t, err)
			assert.Equal(t, model.TaskStatusSuccess, result.Status)
			assert.Equal(t, "https://example.test/seedance.mp4", result.Url)
			task := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess}
			task.Properties.UpstreamModelName = name
			task.PrivateData.ResultURL = result.Url
			converted, err := a.ConvertToOpenAIVideo(task)
			require.NoError(t, err)
			assert.Contains(t, string(converted), result.Url)
		})
	}
}

func TestSeedanceCanonicalReferenceRoles(t *testing.T) {
	for _, test := range []struct {
		extra string
		roles []string
	}{
		{`,"image":"https://example.test/first.png","metadata":{"last_frame_image":"https://example.test/last.png","ratio":"auto"}`, []string{"first_frame", "last_frame"}},
		{`,"metadata":{"reference_images":["https://example.test/ref.png"],"reference_videos":["https://example.test/ref.mp4"],"reference_audios":["https://example.test/ref.mp3"]}`, []string{"reference_image", "reference_video", "reference_audio"}},
	} {
		a, c, info, _ := videoContext(t, `{"model":"local","prompt":"boat","duration":15`+test.extra+`}`, "doubao-seedance-2.0-mini")
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		body, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		var payload seedanceRequest
		require.NoError(t, common.DecodeJson(body, &payload))
		require.Len(t, payload.Content, len(test.roles)+1)
		for i, role := range test.roles {
			assert.Equal(t, role, payload.Content[i].Role)
		}
		if strings.Contains(test.extra, `"auto"`) {
			assert.Equal(t, "adaptive", *payload.Ratio)
		}
	}
}

func TestSeedancePollingRejectsWrongTaskAndMissingDelivery(t *testing.T) {
	service.InitHttpClient()
	for _, body := range []string{
		`{"id":"cgt_other","status":"running"}`,
		`{"id":"cgt_upstream","status":"succeeded","content":{}}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := io.WriteString(w, body)
			require.NoError(t, err)
		}))
		a := &TaskAdaptor{}
		_, err := a.FetchTask(server.URL, "test-platform-key", map[string]any{"task_id": "cgt_upstream", "model": "doubao-seedance-2.0-mini"}, "")
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "test-platform-key")
		server.Close()
	}
}

func TestSeedanceRejectsInvalidRequestsBeforeBilling(t *testing.T) {
	for _, body := range []string{
		`{"model":"local","prompt":"boat","duration":3}`,
		`{"model":"local","prompt":"boat","duration":16}`,
		`{"model":"local","prompt":"boat","duration":0}`,
		`{"model":"local","prompt":"boat","metadata":{"duration":999999999}}`,
		`{"model":"local","prompt":"boat","metadata":{"frames":100}}`,
		`{"model":"local","prompt":"boat","metadata":{"resolution":"1080p"}}`,
		`{"model":"local","prompt":"boat","metadata":{"seed":2147483648}}`,
		`{"model":"local","prompt":"boat","metadata":{"reference_audios":["https://example.test/ref.mp3"]}}`,
		`{"model":"local","prompt":"boat","metadata":{"reference_images":["file:///local.png"]}}`,
		`{"model":"local","prompt":"boat","metadata":{"reference_images":["https://user:password@example.test/ref.png"]}}`,
		`{"model":"local","prompt":"boat","metadata":{"reference_images":["https://example.test/a.png","https://example.test/b.png","https://example.test/c.png","https://example.test/d.png","https://example.test/e.png","https://example.test/f.png","https://example.test/g.png","https://example.test/h.png","https://example.test/i.png","https://example.test/j.png"]}}`,
	} {
		a, c, info, _ := videoContext(t, body, "doubao-seedance-2.0-mini")
		require.NotNil(t, a.ValidateRequestAndSetAction(c, info), body)
		assert.Empty(t, a.EstimateBilling(c, info), body)
	}
}

func TestH3R2VReferenceCountAndDefaultSize(t *testing.T) {
	for _, count := range []int{0, 1, 4, 5} {
		references := make([]string, count)
		for i := range references {
			references[i] = "file_12345678901234567890123456789012"
		}
		data, err := common.Marshal(map[string]any{"model": "local", "prompt": "boat", "duration": 30, "metadata": map[string]any{"reference_images": references}})
		require.NoError(t, err)
		a, c, info, _ := videoContext(t, string(data), "minimax-h3-r2v")
		if count == 0 || count == 5 {
			require.NotNil(t, a.ValidateRequestAndSetAction(c, info))
			continue
		}
		require.Nil(t, a.ValidateRequestAndSetAction(c, info))
		body, err := a.BuildRequestBody(c, info)
		require.NoError(t, err)
		var payload videoRequest
		require.NoError(t, common.DecodeJson(body, &payload))
		assert.Equal(t, "1280x736", payload.Size)
		assert.Equal(t, 30, *payload.Seconds)
		assert.Len(t, payload.ReferenceImages, count)
	}
	// Standard local 16:9 720p must preserve the provider's aligned height.
	a, c, info, _ := videoContext(t, `{"model":"local","prompt":"boat","duration":1,"width":1280,"height":720,"metadata":{"reference_images":["file_12345678901234567890123456789012"]}}`, "minimax-h3-r2v")
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.True(t, bytes.Contains(data, []byte(`"size":"1280x736"`)))
}

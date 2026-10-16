package mrtoken

import (
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

func TestWan27CanonicalRequests(t *testing.T) {
	for _, test := range []struct {
		name, upstream, body string
		mediaTypes           []string
	}{
		{"text and audio", "wan2.7-t2v", `{"model":"local-wan","prompt":"paper boat","duration":2,"metadata":{"resolution":"720p","ratio":"9:16","reference_audios":["https://example.test/sound.mp3"],"seed":0,"watermark":false,"prompt_extend":false,"generate_audio":false}}`, nil},
		{"first and tail with audio", "wan2.7-i2v", `{"model":"local-wan","prompt":"paper boat","image":"https://example.test/first.png","duration":5,"metadata":{"last_frame_image":"https://example.test/last.png","reference_audios":["https://example.test/sound.mp3"]}}`, []string{"first_frame", "last_frame", "driving_audio"}},
		{"clip and tail", "wan2.7-i2v", `{"model":"local-wan","prompt":"paper boat","duration":5,"metadata":{"reference_videos":["https://example.test/clip.mp4"],"last_frame_image":"https://example.test/last.png"}}`, []string{"first_clip", "last_frame"}},
		{"ordered mixed references", "wan2.7-r2v", `{"model":"local-wan","prompt":"图1走过视频1","image":"https://example.test/first.png","duration":10,"metadata":{"wan_media":[{"type":"reference_video","url":"https://example.test/clip.mp4","reference_voice":"https://example.test/voice.mp3"},{"type":"reference_image","url":"https://example.test/actor.png"}],"resolution":"720p"}}`, []string{"first_frame", "reference_video", "reference_image"}},
		{"first plus grouped references", "wan2.7-r2v", `{"model":"local-wan","prompt":"图1在湖边","image":"https://example.test/first.png","duration":5,"metadata":{"reference_images":["https://example.test/actor.png"]}}`, []string{"first_frame", "reference_image"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, info, _ := videoContext(t, test.body, test.upstream)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			endpoint, err := a.BuildRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, "https://api.mrtoken.cn/v1/video-generation/video-synthesis", endpoint)
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			var payload wan27Request
			require.NoError(t, common.Unmarshal(body, &payload))
			assert.Equal(t, test.upstream, payload.Model)
			assert.NotEmpty(t, payload.Input.Prompt)
			assert.Equal(t, map[string]float64{"seconds": float64(*payload.Parameters.Duration)}, a.EstimateBilling(c, info))
			types := make([]string, 0, len(payload.Input.Media))
			for _, item := range payload.Input.Media {
				types = append(types, item.Type)
			}
			assert.Equal(t, append([]string{}, test.mediaTypes...), types)
			if test.name == "text and audio" {
				assert.Equal(t, "https://example.test/sound.mp3", payload.Input.AudioURL)
				assert.Equal(t, "9:16", *payload.Parameters.Ratio)
				assert.EqualValues(t, 0, *payload.Parameters.Seed)
				assert.False(t, *payload.Parameters.Watermark)
				assert.False(t, *payload.Parameters.PromptExtend)
			}
			if test.name == "ordered mixed references" {
				assert.Equal(t, "https://example.test/voice.mp3", payload.Input.Media[1].ReferenceVoice)
				assert.Nil(t, payload.Parameters.Ratio)
			}
		})
	}
}

func TestWan27RejectsInvalidRequestsBeforeBilling(t *testing.T) {
	for _, test := range []struct{ name, upstream, body string }{
		{"duration under minimum", "wan2.7-t2v", `{"model":"wan","prompt":"boat","duration":1}`},
		{"duration over maximum", "wan2.7-t2v", `{"model":"wan","prompt":"boat","duration":16}`},
		{"oversized billing duration", "wan2.7-t2v", `{"model":"wan","prompt":"boat","duration":2147483647}`},
		{"text with image", "wan2.7-t2v", `{"model":"wan","prompt":"boat","image":"https://example.test/a.png"}`},
		{"i2v missing source", "wan2.7-i2v", `{"model":"wan","prompt":"boat"}`},
		{"i2v double source", "wan2.7-i2v", `{"model":"wan","prompt":"boat","image":"https://example.test/a.png","metadata":{"reference_videos":["https://example.test/a.mp4"]}}`},
		{"r2v no references", "wan2.7-r2v", `{"model":"wan","prompt":"boat"}`},
		{"r2v video too long", "wan2.7-r2v", `{"model":"wan","prompt":"boat","duration":11,"metadata":{"reference_videos":["https://example.test/a.mp4"]}}`},
		{"r2v voice alone", "wan2.7-r2v", `{"model":"wan","prompt":"boat","metadata":{"reference_images":["https://example.test/a.png"],"reference_audios":["https://example.test/a.mp3"]}}`},
		{"r2v untrusted media type", "wan2.7-r2v", `{"model":"wan","prompt":"boat","metadata":{"wan_media":[{"type":"first_frame","url":"https://example.test/a.png"}]}}`},
		{"supplier parameter bypass", "wan2.7-t2v", `{"model":"wan","prompt":"boat","metadata":{"parameters":{"duration":100000}}}`},
		{"negative seed", "wan2.7-t2v", `{"model":"wan","prompt":"boat","metadata":{"seed":-1}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, info, _ := videoContext(t, test.body, test.upstream)
			taskErr := a.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Nil(t, a.EstimateBilling(c, info))
		})
	}
}

func TestWanAcceptsDramaClawDerivedDimensions(t *testing.T) {
	for _, modelName := range []string{"wan2.7-t2v", "wan2.7-i2v", "wan2.7-r2v", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(modelName, func(t *testing.T) {
			payload := map[string]any{
				"model": "local-wan", "prompt": "paper boat", "duration": 2,
				"width": 1280, "height": 720,
				"metadata": map[string]any{"resolution": "720p", "ratio": "16:9", "generate_audio": false, "watermark": false},
			}
			if modelName == "wan2.7-i2v" {
				payload["image"] = "https://example.test/first.png"
			}
			if modelName == "wan2.7-r2v" {
				payload["metadata"].(map[string]any)["reference_images"] = []string{"https://example.test/ref.png"}
			}
			body, err := common.Marshal(payload)
			require.NoError(t, err)
			a, c, info, _ := videoContext(t, string(body), modelName)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			var native map[string]any
			require.NoError(t, common.DecodeJson(reader, &native))
			assert.Equal(t, modelName, native["model"])
			assert.NotContains(t, native, "width")
			assert.NotContains(t, native, "height")
			assert.Equal(t, "720P", native["parameters"].(map[string]any)["resolution"])
			assert.Equal(t, map[string]float64{"seconds": 2}, a.EstimateBilling(c, info))

			payload["height"] = 1280
			body, err = common.Marshal(payload)
			require.NoError(t, err)
			a, c, info, _ = videoContext(t, string(body), modelName)
			require.NotNil(t, a.ValidateRequestAndSetAction(c, info), "conflicting ratio must fail before billing")
			assert.Nil(t, a.EstimateBilling(c, info))
		})
	}
}

func TestWan27TaskQueryAndDelivery(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, r.URL.Path == "/v1/video-generation/tasks/wan-upstream" || r.URL.Path == "/v1/video-generation/tasks/another-task")
		assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
		_, err := io.WriteString(w, `{"output":{"task_id":"wan-upstream","task_status":"SUCCEEDED","video_url":"https://example.test/result.mp4"}}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	a, c, info, _ := videoContext(t, `{"model":"wan","prompt":"boat"}`, "wan2.7-t2v")
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	id, _, taskErr := a.DoResponse(c, &http.Response{Body: io.NopCloser(strings.NewReader(`{"output":{"task_id":"wan-upstream","task_status":"PENDING"}}`))}, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "wan-upstream", id)
	resp, err := a.FetchTask(server.URL+"/v1", "test-platform-key", map[string]any{"task_id": id, "model": "wan2.7-t2v"}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	result, err := a.ParseTaskResult(body)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Equal(t, "https://example.test/result.mp4", result.Url)
	_, err = a.FetchTask(server.URL+"/v1", "test-platform-key", map[string]any{"task_id": "another-task", "model": "wan2.7-t2v"}, "")
	require.ErrorContains(t, err, "invalid MrToken Wan task query response")
	_, err = a.ParseTaskResult([]byte(`{"output":{"task_status":"SUCCEEDED"}}`))
	require.Error(t, err)
	failed, err := a.ParseTaskResult([]byte(`{"output":{"task_status":"FAILED","message":"bad test-platform-key"}}`))
	require.NoError(t, err)
	assert.NotContains(t, failed.Reason, "test-platform-key")

	completedTask := &model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess}
	completedTask.Properties.UpstreamModelName = "wan2.7-t2v"
	completedTask.PrivateData.ResultURL = "https://provider.example.test/signed.mp4?secret=hidden"
	response, err := a.ConvertToOpenAIVideo(completedTask)
	require.NoError(t, err)
	var video struct {
		Metadata map[string]any `json:"metadata"`
	}
	require.NoError(t, common.Unmarshal(response, &video))
	assert.Equal(t, completedTask.GetResultURL(), video.Metadata["url"])
}

func TestWanNativeSubmission(t *testing.T) {
	service.InitHttpClient()
	for _, modelName := range []string{"wan2.7-t2v", "wan3.0-video", "wan3.0-video-prime"} {
		t.Run(modelName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/v1/video-generation/video-synthesis", r.URL.Path)
				assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
				assert.Equal(t, "task_business", r.Header.Get("Idempotency-Key"))
				assert.Equal(t, "task_business", r.Header.Get("X-Request-Id"))
				var body map[string]any
				require.NoError(t, common.DecodeJson(r.Body, &body))
				assert.Equal(t, modelName, body["model"])
				assert.Equal(t, "paper boat", body["input"].(map[string]any)["prompt"])
				assert.EqualValues(t, 2, body["parameters"].(map[string]any)["duration"])
				w.WriteHeader(http.StatusAccepted)
				_, err := io.WriteString(w, `{"output":{"task_id":"wan-native","task_status":"PENDING"}}`)
				require.NoError(t, err)
			}))
			defer server.Close()
			a, c, info, _ := videoContext(t, `{"model":"local-wan","prompt":"paper boat","duration":2}`, modelName)
			info.PublicTaskID = "task_business"
			a.baseURL = server.URL
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			body, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			response, err := a.DoRequest(c, info, body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusAccepted, response.StatusCode)
			assert.True(t, c.GetBool("task_submit_no_retry"))
			id, _, taskErr := a.DoResponse(c, response, info)
			require.Nil(t, taskErr)
			assert.Equal(t, "wan-native", id)
		})
	}
}

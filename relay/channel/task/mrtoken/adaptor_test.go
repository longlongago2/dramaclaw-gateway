package mrtoken

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func videoContext(t *testing.T, payload string, upstreamModel string) (*TaskAdaptor, *gin.Context, *relaycommon.RelayInfo, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/video/generations", strings.NewReader(payload))
	c.Request.Header.Set("Content-Type", "application/json")
	t.Cleanup(func() { common.CleanupBodyStorage(c) })
	info := &relaycommon.RelayInfo{
		OriginModelName: "H3-local",
		ChannelMeta:     &relaycommon.ChannelMeta{ChannelBaseUrl: "https://api.mrtoken.cn", ApiKey: "test-platform-key", UpstreamModelName: upstreamModel},
		TaskRelayInfo:   &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	a := &TaskAdaptor{}
	a.Init(info)
	return a, c, info, recorder
}

func TestVideoRequestTranslatesGeometryAndPreservesExplicitZero(t *testing.T) {
	a, c, info, _ := videoContext(t, `{"model":"H3-local","prompt":"paper boat","duration":1,"width":1280,"height":720,"seed":0,"metadata":{"steps":8,"parameters":{"turbo":false,"lora_strength":0}}}`, "minimax-h3-t2v")
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	var request map[string]any
	require.NoError(t, common.Unmarshal(data, &request))
	assert.Equal(t, "minimax-h3-t2v", request["model"])
	assert.Equal(t, "1280x736", request["size"])
	assert.EqualValues(t, 1, request["seconds"])
	assert.EqualValues(t, 0, request["seed"])
	assert.EqualValues(t, 8, request["steps"])
	assert.Equal(t, map[string]any{"turbo": false, "lora_strength": float64(0)}, request["parameters"])
	for _, field := range []string{"duration", "width", "height", "image", "metadata", "n", "response_format"} {
		assert.NotContains(t, request, field)
	}
	assert.Equal(t, map[string]float64{"seconds": 1}, a.EstimateBilling(c, info))
}

func TestInvalidVideoInputsFailBeforeUploadsAndBilling(t *testing.T) {
	for _, test := range []struct{ name, model, extra string }{
		{"duration too long", "minimax-h3-t2v", `,"duration":31`},
		{"zero duration", "minimax-h3-t2v", `,"duration":0`},
		{"overflow duration", "minimax-h3-t2v", `,"duration":9223372036854775807`},
		{"oversized dimensions", "minimax-h3-t2v", `,"width":2147483647,"height":2147483647`},
		{"negative seed", "minimax-h3-t2v", `,"metadata":{"seed":-1}`},
		{"missing max first frame", "minimax-h3-max-i2v", ``},
		{"image on text model", "minimax-h3-t2v", `,"image":"https://example.test/first.png"`},
		{"missing r2v references", "minimax-h3-r2v", ``},
		{"video reference", "minimax-h3-i2v", `,"metadata":{"reference_videos":["https://example.test/ref.mp4"]}`},
		{"last frame without first", "minimax-h3-i2v", `,"metadata":{"last_frame_image":"https://example.test/last.png"}`},
		{"unknown model", "other-video-model", ``},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, info, _ := videoContext(t, `{"model":"H3-local","prompt":"boat"`+test.extra+`}`, test.model)
			err := a.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, err)
			assert.Equal(t, http.StatusBadRequest, err.StatusCode)
			assert.Nil(t, a.EstimateBilling(c, info))
		})
	}
}

func TestFirstLastFrameUploadsPinVendorRoute(t *testing.T) {
	service.InitHttpClient()
	var pngBytes bytes.Buffer
	require.NoError(t, png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, 32, 32))))
	imageData := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes.Bytes())
	uploads := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
		if r.URL.Path == "/v1/files" {
			assert.Equal(t, "minimax-h3-i2v", r.URL.Query().Get("model"))
			if uploads == 0 {
				assert.Empty(t, r.URL.Query().Get("vendorKeyId"))
			} else {
				assert.Equal(t, "vendor-7", r.URL.Query().Get("vendorKeyId"))
			}
			uploads++
			require.NoError(t, r.ParseMultipartForm(1<<20))
			defer r.MultipartForm.RemoveAll()
			file, _, err := r.FormFile("file")
			require.NoError(t, err)
			defer file.Close()
			data, err := io.ReadAll(file)
			require.NoError(t, err)
			assert.Equal(t, pngBytes.Bytes(), data)
			w.Header().Set("X-OpenImgVideo-Vendor-Key-Id", "vendor-7")
			_, err = io.WriteString(w, `{"id":"file_12345678901234567890123456789012"}`)
			require.NoError(t, err)
			return
		}
		assert.Equal(t, "/v1/videos", r.URL.Path)
		assert.Equal(t, "vendor-7", r.URL.Query().Get("vendorKeyId"))
		assert.Equal(t, "task_public", r.Header.Get("Idempotency-Key"))
		assert.Equal(t, "task_public", r.Header.Get("X-Request-Id"))
		w.WriteHeader(http.StatusAccepted)
		_, err := io.WriteString(w, `{"id":"video_upstream","status":"queued"}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	payload, err := common.Marshal(map[string]any{"model": "H3-local", "prompt": "boat", "duration": 1, "image": imageData, "metadata": map[string]any{"last_frame_image": imageData}})
	require.NoError(t, err)
	a, c, info, recorder := videoContext(t, string(payload), "minimax-h3-i2v")
	a.baseURL = server.URL
	require.Nil(t, a.ValidateRequestAndSetAction(c, info))
	body, err := a.BuildRequestBody(c, info)
	require.NoError(t, err)
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, 2, uploads)
	assert.Contains(t, string(data), `"input_reference":{"file_id":"file_12345678901234567890123456789012"}`)
	assert.Contains(t, string(data), `"last_frame":{"file_id":"file_12345678901234567890123456789012"}`)
	assert.NotContains(t, string(data), imageData)
	resp, err := a.DoRequest(c, info, bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	id, _, taskErr := a.DoResponse(c, resp, info)
	require.Nil(t, taskErr)
	assert.Equal(t, "video_upstream", id)
	assert.Contains(t, recorder.Body.String(), `"id":"task_public"`)
	assert.NotContains(t, recorder.Body.String(), "video_upstream")
	assert.True(t, c.GetBool("task_submit_no_retry"))
}

func TestImageModeAndR2VReferences(t *testing.T) {
	for _, test := range []struct {
		model, extra   string
		referenceCount int
		first          bool
	}{
		{"minimax-h3-i2v", `,"metadata":{"reference_images":["file_12345678901234567890123456789012"]}`, 0, true},
		{"minimax-h3-r2v", `,"metadata":{"reference_images":["file_12345678901234567890123456789012","file_22345678901234567890123456789012"]}`, 2, false},
		{"minimax-h3-max-t2v", ``, 0, false},
		{"minimax-h3-max-i2v", `,"image":"file_12345678901234567890123456789012"`, 0, true},
	} {
		t.Run(test.model, func(t *testing.T) {
			a, c, info, _ := videoContext(t, `{"model":"H3-local","prompt":"boat","duration":1`+test.extra+`}`, test.model)
			require.Nil(t, a.ValidateRequestAndSetAction(c, info))
			reader, err := a.BuildRequestBody(c, info)
			require.NoError(t, err)
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			var payload videoRequest
			require.NoError(t, common.Unmarshal(data, &payload))
			assert.Len(t, payload.ReferenceImages, test.referenceCount)
			assert.Equal(t, test.first, payload.InputReference != nil)
		})
	}
}

func TestPollingUsesOriginalModelAndKeepsResultBehindProxy(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/video_upstream", r.URL.Path)
		assert.Equal(t, "minimax-h3-t2v", r.URL.Query().Get("model"))
		assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
		_, err := io.WriteString(w, `{"id":"video_upstream","status":"completed","url":"https://example.test/private-result"}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	a := &TaskAdaptor{}
	resp, err := a.FetchTask(server.URL+"/v1", "test-platform-key", map[string]any{"task_id": "video_upstream", "model": "minimax-h3-t2v"}, "")
	require.NoError(t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	result, err := a.ParseTaskResult(data)
	require.NoError(t, err)
	assert.Equal(t, model.TaskStatusSuccess, result.Status)
	assert.Empty(t, result.Url)
	converted, err := a.ConvertToOpenAIVideo(&model.Task{TaskID: "task_public", Status: model.TaskStatusSuccess, Data: data})
	require.NoError(t, err)
	assert.NotContains(t, string(converted), "private-result")
	assert.Contains(t, string(converted), "/v1/public/videos/task_public/content?")
}

func TestTaskStateMappingAndSecretFreeFailures(t *testing.T) {
	a := &TaskAdaptor{apiKey: "test-platform-key"}
	for _, test := range []struct{ status, want string }{
		{"queued", model.TaskStatusQueued}, {"in_progress", model.TaskStatusInProgress},
		{"completed", model.TaskStatusSuccess}, {"failed", model.TaskStatusFailure}, {"cancelled", model.TaskStatusFailure},
	} {
		data, err := common.Marshal(map[string]any{"status": test.status, "error": map[string]string{"message": "invalid test-platform-key"}})
		require.NoError(t, err)
		result, err := a.ParseTaskResult(data)
		require.NoError(t, err)
		assert.Equal(t, test.want, result.Status)
		assert.NotContains(t, result.Reason, "test-platform-key")
	}
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, err := io.WriteString(w, `{"detail":{"reasons":["insufficient balance","invalid test-platform-key"]}}`)
		require.NoError(t, err)
	}))
	defer server.Close()
	_, err := a.FetchTask(server.URL, a.apiKey, map[string]any{"task_id": "video_id", "model": "minimax-h3-t2v"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insufficient balance")
	assert.NotContains(t, err.Error(), a.apiKey)
}

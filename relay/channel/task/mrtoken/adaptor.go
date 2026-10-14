package mrtoken

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	videodto "github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskAdaptor struct {
	taskcommon.BaseBilling
	baseURL string
	apiKey  string
}

// IsPlatformURL selects the protocol from the configured channel, never the model name.
func IsPlatformURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && strings.EqualFold(parsed.Host, "api.mrtoken.cn") &&
		parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		(strings.TrimRight(parsed.Path, "/") == "" || strings.TrimRight(parsed.Path, "/") == "/v1")
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.baseURL = strings.TrimSuffix(strings.TrimRight(info.ChannelBaseUrl, "/"), "/v1")
	a.apiKey = info.ApiKey
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if taskErr := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate); taskErr != nil {
		return taskErr
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	var original struct {
		Seed *int64 `json:"seed,omitempty"`
	}
	if err := common.UnmarshalBodyReusable(c, &original); err != nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("invalid MrToken video seed"), "invalid_request", http.StatusBadRequest)
	}
	if original.Seed != nil {
		if _, exists := req.Metadata["seed"]; !exists {
			req.Metadata["seed"] = *original.Seed
		}
	}
	if IsHappyHorseModel(info.UpstreamModelName) {
		planned, err := planHappyHorseRequest(req, info.UpstreamModelName)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		c.Set("mrtoken_happyhorse_request", planned)
		if len(planned.Input.Media) > 0 {
			info.Action = constant.TaskActionGenerate
		}
		return nil
	}
	if IsSeedanceModel(info.UpstreamModelName) {
		planned, err := planSeedanceRequest(req, info.UpstreamModelName)
		if err != nil {
			return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		}
		c.Set("mrtoken_seedance_request", planned)
		return nil
	}
	planned, err := planVideoRequest(req, info.UpstreamModelName)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if planned.firstFrame != "" || len(planned.references) > 0 {
		info.Action = constant.TaskActionGenerate
	}
	c.Set("mrtoken_video_request", planned)
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	if value, exists := c.Get("mrtoken_happyhorse_request"); exists {
		if planned, ok := value.(*happyHorseRequest); ok && planned.Parameters.Duration != nil {
			return map[string]float64{"seconds": float64(*planned.Parameters.Duration)}
		}
	}
	if value, exists := c.Get("mrtoken_seedance_request"); exists {
		if planned, ok := value.(*seedanceRequest); ok && planned.Duration != nil {
			return map[string]float64{"seconds": float64(*planned.Duration)}
		}
	}
	value, _ := c.Get("mrtoken_video_request")
	if planned, ok := value.(*videoRequest); ok && planned.Seconds != nil {
		return map[string]float64{"seconds": float64(*planned.Seconds)}
	}
	return nil
}

func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if IsHappyHorseModel(info.UpstreamModelName) {
		return a.baseURL + "/v1/video-generation/video-synthesis", nil
	}
	if IsSeedanceModel(info.UpstreamModelName) {
		return a.baseURL + "/v1/video-generation/seedance", nil
	}
	return a.baseURL + "/v1/videos", nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", info.PublicTaskID)
	req.Header.Set("X-Request-Id", info.PublicTaskID)
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	if value, exists := c.Get("mrtoken_happyhorse_request"); exists {
		data, err := common.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("cannot encode MrToken HappyHorse request")
		}
		return bytes.NewReader(data), nil
	}
	if value, exists := c.Get("mrtoken_seedance_request"); exists {
		data, err := common.Marshal(value)
		if err != nil {
			return nil, fmt.Errorf("cannot encode MrToken Seedance request")
		}
		return bytes.NewReader(data), nil
	}
	value, _ := c.Get("mrtoken_video_request")
	planned, ok := value.(*videoRequest)
	if !ok {
		return nil, fmt.Errorf("MrToken video request has not been validated")
	}
	request := *planned
	vendorID := ""
	if request.firstFrame != "" {
		fileID, err := a.uploadImage(c.Request.Context(), request.Model, request.firstFrame, &vendorID, info.ChannelSetting.Proxy)
		if err != nil {
			return nil, err
		}
		request.InputReference = &fileReference{FileID: fileID}
	}
	if request.lastFrame != "" {
		fileID, err := a.uploadImage(c.Request.Context(), request.Model, request.lastFrame, &vendorID, info.ChannelSetting.Proxy)
		if err != nil {
			return nil, err
		}
		request.LastFrame = &fileReference{FileID: fileID}
	}
	for _, source := range request.references {
		fileID, err := a.uploadImage(c.Request.Context(), request.Model, source, &vendorID, info.ChannelSetting.Proxy)
		if err != nil {
			return nil, err
		}
		request.ReferenceImages = append(request.ReferenceImages, fileReference{FileID: fileID})
	}
	c.Set("mrtoken_vendor_key_id", vendorID)
	data, err := common.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("cannot encode MrToken H3 request")
	}
	return bytes.NewReader(data), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, body io.Reader) (*http.Response, error) {
	client, err := platformClient(info.ChannelSetting.Proxy)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	endpoint, _ := a.BuildRequestURL(info)
	if vendorID := c.GetString("mrtoken_vendor_key_id"); vendorID != "" {
		endpoint += "?vendorKeyId=" + url.QueryEscape(vendorID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("invalid MrToken platform URL")
	}
	if err := a.BuildRequestHeader(c, req, info); err != nil {
		return nil, err
	}
	// Do not resubmit a paid creation after an uncertain transport failure.
	c.Set("task_submit_no_retry", true)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MrToken video submission failed; do not automatically resubmit")
	}
	return sanitizedResponse(resp, a.apiKey)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var data struct {
		ID          string `json:"id"`
		TaskID      string `json:"task_id"`
		CamelTaskID string `json:"taskId"`
		Output      struct {
			TaskID      string `json:"task_id"`
			CamelTaskID string `json:"taskId"`
		} `json:"output"`
		Data struct {
			ID          string `json:"id"`
			TaskID      string `json:"task_id"`
			CamelTaskID string `json:"taskId"`
		} `json:"data"`
	}
	if err != nil || common.Unmarshal(body, &data) != nil {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid MrToken video submission response"), "invalid_response", http.StatusBadGateway)
	}
	upstreamID := data.ID
	for _, candidate := range []string{data.TaskID, data.CamelTaskID, data.Data.ID, data.Data.TaskID, data.Data.CamelTaskID, data.Output.TaskID, data.Output.CamelTaskID} {
		if upstreamID == "" {
			upstreamID = candidate
		}
	}
	if upstreamID == "" || strings.Contains(upstreamID, a.apiKey) && a.apiKey != "" {
		return "", nil, service.TaskErrorWrapperLocal(fmt.Errorf("MrToken video response is missing a task ID"), "invalid_response", http.StatusBadGateway)
	}
	video := videodto.NewOpenAIVideo()
	video.ID, video.TaskID, video.Model = info.PublicTaskID, info.PublicTaskID, info.OriginModelName
	c.JSON(http.StatusOK, video)
	return upstreamID, body, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, _ := body["task_id"].(string)
	modelName, _ := body["model"].(string)
	if taskID == "" || !supportedModel(modelName) {
		return nil, fmt.Errorf("MrToken polling requires the original task ID and model")
	}
	client, err := platformClient(proxy)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1") + "/v1/videos/" + url.PathEscape(taskID) + "?model=" + url.QueryEscape(modelName)
	if IsSeedanceModel(modelName) {
		endpoint = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1") + "/v1/video-generation/seedance/tasks/" + url.PathEscape(taskID)
	} else if IsHappyHorseModel(modelName) {
		endpoint = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1") + "/v1/video-generation/tasks/" + url.PathEscape(taskID)
	}
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("invalid MrToken polling URL")
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("MrToken task query failed")
	}
	resp, err = sanitizedResponse(resp, key)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MrToken task query failed: HTTP %d (%s)", resp.StatusCode, errorMessage(data, key))
	}
	if IsSeedanceModel(modelName) {
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		var task struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Content struct {
				VideoURL string `json:"video_url"`
			} `json:"content"`
		}
		if readErr != nil || common.Unmarshal(data, &task) != nil || task.ID != "" && task.ID != taskID {
			return nil, fmt.Errorf("invalid MrToken Seedance task query response")
		}
		status := strings.ToLower(task.Status)
		if (status == "succeeded" || status == "completed" || status == "success") && strings.TrimSpace(task.Content.VideoURL) == "" {
			return nil, fmt.Errorf("MrToken Seedance completed task is missing video content")
		}
		resp.Body = io.NopCloser(bytes.NewReader(data))
	}
	return resp, nil
}

func (a *TaskAdaptor) ParseTaskResult(data []byte) (*relaycommon.TaskInfo, error) {
	var task struct {
		Status   string   `json:"status"`
		Progress *float64 `json:"progress"`
		Content  struct {
			VideoURL string `json:"video_url"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if common.Unmarshal(data, &task) != nil {
		return nil, fmt.Errorf("invalid MrToken task response")
	}
	var native struct {
		Output *struct {
			Status   string `json:"task_status"`
			VideoURL string `json:"video_url"`
			Message  string `json:"message"`
		} `json:"output"`
	}
	if common.Unmarshal(data, &native) == nil && native.Output != nil {
		task.Status, task.Content.VideoURL = native.Output.Status, native.Output.VideoURL
		if native.Output.Message != "" {
			task.Error = &struct {
				Message string `json:"message"`
			}{Message: native.Output.Message}
		}
		status := strings.ToLower(task.Status)
		if (status == "succeeded" || status == "success" || status == "completed") && strings.TrimSpace(task.Content.VideoURL) == "" {
			return nil, fmt.Errorf("MrToken completed task is missing video content")
		}
	}
	result := &relaycommon.TaskInfo{}
	if task.Progress != nil && *task.Progress > 0 && *task.Progress < 100 {
		result.Progress = fmt.Sprintf("%.0f%%", *task.Progress)
	}
	switch strings.ToLower(task.Status) {
	case "queued", "pending":
		result.Status = model.TaskStatusQueued
	case "in_progress", "processing", "running":
		result.Status = model.TaskStatusInProgress
	case "completed", "succeeded", "success":
		result.Status, result.Progress = model.TaskStatusSuccess, "100%"
		result.Url = task.Content.VideoURL
	case "failed", "error", "cancelled", "canceled":
		result.Status, result.Progress, result.Reason = model.TaskStatusFailure, "100%", "MrToken video generation failed"
		if task.Error != nil {
			result.Reason = task.Error.Message
			if a.apiKey != "" {
				result.Reason = strings.ReplaceAll(result.Reason, a.apiKey, "[REDACTED]")
			}
		}
	default:
		return nil, fmt.Errorf("unknown MrToken video task status")
	}
	return result, nil
}

func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	video := task.ToOpenAIVideo()
	if task.Status == model.TaskStatusSuccess && (IsSeedanceModel(task.Properties.UpstreamModelName) || IsHappyHorseModel(task.Properties.UpstreamModelName)) {
		video.SetMetadata("url", task.GetResultURL())
	} else if task.Status == model.TaskStatusSuccess {
		video.SetMetadata("url", taskcommon.BuildPublicProxyURL(task.TaskID))
	}
	return common.Marshal(video)
}

func (a *TaskAdaptor) GetModelList() []string { return append([]string(nil), modelList...) }
func (a *TaskAdaptor) GetChannelName() string { return "mrtoken" }

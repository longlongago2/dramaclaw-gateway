package controller

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMrTokenVideoProxyUsesOriginalModelAndForwardsRange(t *testing.T) {
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMemoryCache, originalRedis := common.MemoryCacheEnabled, common.RedisEnabled
	originalFetch := *system_setting.GetFetchSetting()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = originalMemoryCache, originalRedis
		*system_setting.GetFetchSetting() = originalFetch
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	common.MemoryCacheEnabled = false
	// Operator-managed provider targets work even when arbitrary media URL
	// fetching is protected (e.g. local proxies or DNS fake-IP deployments).
	system_setting.GetFetchSetting().EnableSSRFProtection = true
	system_setting.GetFetchSetting().AllowPrivateIp = false
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/videos/video_upstream/content", r.URL.Path)
		assert.Equal(t, "minimax-h3-i2v", r.URL.Query().Get("model"))
		assert.Equal(t, "Bearer test-platform-key", r.Header.Get("Authorization"))
		assert.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Range", "bytes 0-3/10")
		w.WriteHeader(http.StatusPartialContent)
		_, err := io.WriteString(w, "test")
		require.NoError(t, err)
	}))
	defer server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "test-platform-key", BaseURL: common.GetPointer(server.URL + "/v1")}
	require.NoError(t, db.Create(channel).Error)
	task := &model.Task{TaskID: "task_public", Platform: constant.TaskPlatformMrToken, UserId: 42, ChannelId: channel.Id, Status: model.TaskStatusSuccess}
	task.Properties.UpstreamModelName = "minimax-h3-i2v"
	task.PrivateData.UpstreamTaskID = "video_upstream"
	task.PrivateData.UpstreamResultURL = "http://untrusted.invalid/private-result"
	require.NoError(t, db.Create(task).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_public/content", nil)
	c.Request.Header.Set("Range", "bytes=0-3")
	c.Set("id", 42)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	VideoProxy(c)
	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "bytes 0-3/10", recorder.Header().Get("Content-Range"))
	assert.Equal(t, "test", recorder.Body.String())
	assert.NotContains(t, recorder.Body.String(), "test-platform-key")

	// The generated signed URL must work for media downloaders without a user token.
	publicRecorder := httptest.NewRecorder()
	publicContext, _ := gin.CreateTestContext(publicRecorder)
	publicContext.Request = httptest.NewRequest(http.MethodGet, taskcommon.BuildPublicProxyURL(task.TaskID), nil)
	publicContext.Request.Header.Set("Range", "bytes=0-3")
	publicContext.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	PublicVideoProxy(publicContext)
	assert.Equal(t, http.StatusPartialContent, publicRecorder.Code)
	assert.Equal(t, "test", publicRecorder.Body.String())
	assert.NotContains(t, publicRecorder.Body.String(), channel.Key)

	invalidRecorder := httptest.NewRecorder()
	invalidContext, _ := gin.CreateTestContext(invalidRecorder)
	invalidContext.Request = httptest.NewRequest(http.MethodGet, "/v1/public/videos/task_public/content?expires=1&signature=invalid", nil)
	invalidContext.Params = publicContext.Params
	PublicVideoProxy(invalidContext)
	assert.Equal(t, http.StatusUnauthorized, invalidRecorder.Code)
}

func TestMrTokenSeedanceVideoProxyDoesNotSendPlatformKeyToCDN(t *testing.T) {
	originalDB, originalLogDB := model.DB, model.LOG_DB
	originalMemoryCache, originalRedis := common.MemoryCacheEnabled, common.RedisEnabled
	originalFetch := *system_setting.GetFetchSetting()
	t.Cleanup(func() {
		model.DB, model.LOG_DB = originalDB, originalLogDB
		common.MemoryCacheEnabled, common.RedisEnabled = originalMemoryCache, originalRedis
		*system_setting.GetFetchSetting() = originalFetch
	})
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Task{}))
	common.MemoryCacheEnabled = false
	system_setting.GetFetchSetting().EnableSSRFProtection = false
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/seedance.mp4", r.URL.Path)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.URL.RawQuery)
		assert.Equal(t, "bytes=0-3", r.Header.Get("Range"))
		w.WriteHeader(http.StatusPartialContent)
		_, err := io.WriteString(w, "test")
		require.NoError(t, err)
	}))
	defer server.Close()
	channel := &model.Channel{Type: constant.ChannelTypeOpenAI, Key: "test-platform-key", BaseURL: common.GetPointer("https://api.mrtoken.cn")}
	require.NoError(t, db.Create(channel).Error)
	task := &model.Task{TaskID: "task_seedance", Platform: constant.TaskPlatformMrToken, UserId: 42, ChannelId: channel.Id, Status: model.TaskStatusSuccess}
	task.Properties.UpstreamModelName = "doubao-seedance-2.0-mini"
	task.PrivateData.ResultURL = server.URL + "/seedance.mp4"
	require.NoError(t, db.Create(task).Error)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task_seedance/content", nil)
	c.Request.Header.Set("Range", "bytes=0-3")
	c.Set("id", 42)
	c.Params = gin.Params{{Key: "task_id", Value: task.TaskID}}
	VideoProxy(c)
	assert.Equal(t, http.StatusPartialContent, recorder.Code)
	assert.Equal(t, "test", recorder.Body.String())
}

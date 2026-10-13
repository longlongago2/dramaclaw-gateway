package relay

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
)

func TestMrTokenVideoRoutingPreservesOpenAIChannels(t *testing.T) {
	for _, test := range []struct {
		base string
		want constant.TaskPlatform
	}{
		{"https://api.mrtoken.cn", constant.TaskPlatformMrToken},
		{"https://api.mrtoken.cn/v1/", constant.TaskPlatformMrToken},
		{"https://api.openai.com", "1"},
		{"https://api.mrtoken.cn.evil.test", "1"},
		{"https://user:password@api.mrtoken.cn", "1"},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("channel_type", constant.ChannelTypeOpenAI)
		c.Set("base_url", test.base)
		assert.Equal(t, test.want, GetTaskPlatform(c))
		require.NotNil(t, GetTaskAdaptor(test.want))
	}
}

func TestGetTaskAdaptorSupportsComfyUIChannelIDs(t *testing.T) {
	require.Equal(t, 63, constant.ChannelTypeComfyUI)
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeComfyUI))))
}

func TestGetAdaptorsSupportFalChannel(t *testing.T) {
	require.Equal(t, 61, constant.ChannelTypeFal)
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeFal)
	require.True(t, ok)
	require.Equal(t, constant.APITypeFal, apiType)
	require.NotNil(t, GetAdaptor(constant.APITypeFal))
	require.NotNil(t, GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(constant.ChannelTypeFal))))
}

func TestGetAdaptorSupportsDoubaoAudioChannel(t *testing.T) {
	require.Equal(t, 62, constant.ChannelTypeDoubaoAudio)
	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeDoubaoAudio)
	require.True(t, ok)
	require.Equal(t, constant.APITypeDoubaoAudio, apiType)
	require.NotNil(t, GetAdaptor(apiType))
}

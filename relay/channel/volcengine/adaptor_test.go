package volcengine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertAudioRequestBuildsVolcengineBasicSpeech(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	speed := 1.25

	reader, err := (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode:       constant.RelayModeAudioSpeech,
		OriginModelName: "dramaclaw-tts-model",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "app-id|access-token",
			UpstreamModelName: "volcano-tts-model",
		},
	}, dto.AudioRequest{
		Model:          "volcano-tts-model",
		Input:          "hello",
		Voice:          "custom-voice",
		ResponseFormat: "mp3",
		Speed:          &speed,
	})
	require.NoError(t, err)

	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	var request VolcengineTTSRequest
	require.NoError(t, common.Unmarshal(body, &request))
	assert.Equal(t, "app-id", request.App.AppID)
	assert.Equal(t, "access-token", request.App.Token)
	assert.Equal(t, "hello", request.Request.Text)
	assert.Equal(t, "volcano-tts-model", request.Request.Model)
	assert.Equal(t, "custom-voice", request.Audio.VoiceType)
	assert.Equal(t, speed, request.Audio.SpeedRatio)
}

func TestSeedAudioGenerationProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{
		RelayMode:       constant.RelayModeAudioSpeech,
		RequestId:       "request-test",
		OriginModelName: "local-audio-alias",
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "test-speech-key", UpstreamModelName: "seed-audio-1.0",
			ChannelBaseUrl: channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine],
		},
	}
	adaptor := &Adaptor{}
	reader, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model: "local-audio-alias", Input: "environment sound", ResponseFormat: "wav",
	})
	require.NoError(t, err)
	var body map[string]any
	require.NoError(t, common.DecodeJson(reader, &body))
	assert.Equal(t, "seed-audio-1.0", body["model"])
	assert.Equal(t, "environment sound", body["text_prompt"])
	assert.False(t, info.IsStream)
	requestURL, err := adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://openspeech.bytedance.com/api/v3/tts/create", requestURL)
	assert.Equal(t, channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeVolcEngine], info.ChannelBaseUrl)
	info.ChannelBaseUrl = "https://speech.example.com"
	requestURL, err = adaptor.GetRequestURL(info)
	require.NoError(t, err)
	assert.Equal(t, "https://speech.example.com/api/v3/tts/create", requestURL)
	header := make(http.Header)
	require.NoError(t, adaptor.SetupRequestHeader(c, &header, info))
	assert.Equal(t, "test-speech-key", header.Get("X-Api-Key"))
	assert.Equal(t, "request-test", header.Get("X-Api-Request-Id"))
	assert.Empty(t, header.Get("Authorization"))
	response := &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
		`{"audio":"YXVkaW8=","original_duration":5.1,"duration":4.2}`,
	))}
	usage, relayErr := adaptor.DoResponse(c, response, info)
	require.Nil(t, relayErr)
	assert.Equal(t, "audio", recorder.Body.String())
	assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, 6, usage.(*dto.Usage).CompletionTokenDetails.AudioTokens)
}

func TestSeedAudioGenerationPreservesProviderErrors(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{RelayMode: constant.RelayModeAudioSpeech, ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"}}
	response := &http.Response{StatusCode: 403, Header: http.Header{"X-Tt-Logid": []string{"trace-test"}}, Body: io.NopCloser(strings.NewReader(`{"code":10074,"message":"resource not granted"}`))}
	relayErr := (&Adaptor{}).DoErrorResponse(c, response, info)
	require.NotNil(t, relayErr)
	assert.Contains(t, relayErr.Error(), "10074")
	assert.Equal(t, "trace-test", c.GetString(common.UpstreamRequestIdKey))
}

func TestConvertAudioRequestRejectsDCMediaExtensionsForVolcengine(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	metadata, err := common.Marshal(map[string]any{
		"audio_url": "https://example.com/reference.wav",
	})
	require.NoError(t, err)

	_, err = (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode: constant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "app-id|access-token",
		},
	}, dto.AudioRequest{
		Model:    "volcano-tts-model",
		Input:    "hello",
		Metadata: metadata,
	})

	require.ErrorContains(t, err, "supports only basic speech requests")
}

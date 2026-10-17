package doubao_audio

import (
	"encoding/base64"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertAudioRequestUsesConfiguredUpstreamModel(t *testing.T) {
	c := doubaoAudioTestContext()
	speed := 1.25
	reader, err := (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "channel-configured-model",
		},
	}, dto.AudioRequest{
		Model:          "dramaclaw-model-alias",
		Input:          "生成一段城市环境音",
		ResponseFormat: "opus",
		Speed:          &speed,
	})
	require.NoError(t, err)

	var request audioGenerationRequest
	require.NoError(t, decodeDoubaoAudioRequest(reader, &request))
	assert.Equal(t, "channel-configured-model", request.Model)
	assert.Equal(t, "生成一段城市环境音", request.TextPrompt)
	assert.Equal(t, "ogg_opus", request.AudioConfig.Format)
	require.NotNil(t, request.AudioConfig.SpeechRate)
	assert.Equal(t, 25, *request.AudioConfig.SpeechRate)
	assert.Empty(t, request.References)
}

func TestRoboticVoicePromptIsPassedUnchangedWithoutSpeakerReference(t *testing.T) {
	prompt := "机械合成音，音调偏高，带有清脆的电子质感，语速轻快急促，断句简短，偶尔带机械提示音\n朗读：你好，准备出发！"
	reader, err := (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"},
	}, dto.AudioRequest{
		Model: "index-tts-2", Input: prompt, ResponseFormat: "mp3",
		Metadata: []byte(`{"audio_config":{"sample_rate":48000,"speech_rate":0,"pitch_rate":0,"loudness_rate":0}}`),
	})
	require.NoError(t, err)
	var request audioGenerationRequest
	require.NoError(t, common.DecodeJson(reader, &request))
	assert.Equal(t, "seed-audio-1.0", request.Model)
	assert.Equal(t, prompt, request.TextPrompt)
	assert.Empty(t, request.References)
	config, err := common.Marshal(request.AudioConfig)
	require.NoError(t, err)
	assert.JSONEq(t, `{"format":"mp3","sample_rate":48000,"speech_rate":0,"pitch_rate":0,"loudness_rate":0}`, string(config))
}

func TestGetRequestURLAcceptsBaseAndFullEndpoint(t *testing.T) {
	adaptor := &Adaptor{}
	for _, suffix := range []string{"", "/", "/api/v3", "/api/v3/", "/api/v3/tts", "/api/v3/tts/create/"} {
		t.Run(suffix, func(t *testing.T) {
			url, err := adaptor.GetRequestURL(&relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{
				ChannelBaseUrl: "https://speech.example.com" + suffix,
			}})
			require.NoError(t, err)
			assert.Equal(t, "https://speech.example.com/api/v3/tts/create", url)
		})
	}
}

func TestSpeechControlPlaneCredentials(t *testing.T) {
	for _, test := range []struct {
		name, key, apiKey, appID, accessKey string
		invalid                             bool
	}{
		{name: "new console API key", key: " speech-key ", apiKey: "speech-key"},
		{name: "legacy console", key: " app-id | access-token ", appID: "app-id", accessKey: "access-token"},
		{name: "missing token", key: "app-id|", invalid: true},
		{name: "empty key", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			header := make(http.Header)
			err := (&Adaptor{}).SetupRequestHeader(doubaoAudioTestContext(), &header, &relaycommon.RelayInfo{
				RequestId: "test-request", ChannelMeta: &relaycommon.ChannelMeta{ApiKey: test.key},
			})
			if test.invalid {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.apiKey, header.Get("X-Api-Key"))
			assert.Equal(t, test.appID, header.Get("X-Api-App-Id"))
			assert.Equal(t, test.accessKey, header.Get("X-Api-Access-Key"))
			assert.Equal(t, "test-request", header.Get("X-Api-Request-Id"))
			assert.Empty(t, header.Get("Authorization"))
		})
	}
}

func TestAudioGenerationPreservesExplicitSubtitleAndWatermarkFlags(t *testing.T) {
	reader, err := (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
	}, dto.AudioRequest{Model: "seed-audio-1.0", Input: "你好", Voice: "speaker-id", ResponseFormat: "wav",
		Metadata: []byte(`{"audio_config":{"sample_rate":40000,"enable_subtitle":false},"watermark":{"aigc_watermark":false,"aigc_metadata":{"enable":false,"content_producer":"test"}}}`),
	})
	require.NoError(t, err)
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.JSONEq(t, `{"model":"seed-audio-1.0","text_prompt":"你好","references":[{"speaker":"speaker-id"}],"audio_config":{"format":"wav","sample_rate":40000,"enable_subtitle":false},"watermark":{"aigc_watermark":false,"aigc_metadata":{"enable":false,"content_producer":"test"}}}`, string(body))
}

func TestAudioGenerationReturnsRequestedSubtitlesWithAudio(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	adaptor := &Adaptor{}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeAudioSpeech}
	_, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model: "seed-audio-1.0", Input: "你好", ResponseFormat: "mp3",
		Metadata: []byte(`{"audio_config":{"enable_subtitle":true}}`),
	})
	require.NoError(t, err)
	response := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(
		`{"audio":"YXVkaW8=","url":"https://example.com/audio.mp3","duration":4.2,"original_duration":5.1,"subtitle":{"text":"你好","sentences":[{"start_time":0,"end_time":4200,"text":"你好"}]}}`,
	))}
	usage, relayErr := adaptor.DoResponse(c, response, info)
	require.Nil(t, relayErr)
	require.NotNil(t, usage)
	assert.JSONEq(t, `{"audio":{"data":"YXVkaW8=","url":"https://example.com/audio.mp3","format":"mp3"},"duration":4.2,"original_duration":5.1,"subtitle":{"text":"你好","sentences":[{"start_time":0,"end_time":4200,"text":"你好"}]}}`, recorder.Body.String())
}

func TestAudioGenerationHTTPRoundTrip(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v3/tts/create", r.URL.Path)
		assert.Equal(t, "speech-test-key", r.Header.Get("X-Api-Key"))
		assert.Empty(t, r.Header.Get("Authorization"))
		var request audioGenerationRequest
		if !assert.NoError(t, common.DecodeJson(r.Body, &request)) {
			return
		}
		assert.Equal(t, "seed-audio-1.0", request.Model)
		assert.Equal(t, "你好", request.TextPrompt)
		assert.Equal(t, "wav", request.AudioConfig.Format)
		if assert.NotNil(t, request.Watermark) {
			assert.Equal(t, false, *request.Watermark.AIGCWatermark)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tt-Logid", "speech-roundtrip-id")
		_, err := w.Write([]byte(`{"audio":"YXVkaW8=","duration":1,"original_duration":1}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	info := &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey: "speech-test-key", ChannelBaseUrl: server.URL,
			UpstreamModelName: "seed-audio-1.0",
		},
	}
	adaptor := &Adaptor{}
	reader, err := adaptor.ConvertAudioRequest(c, info, dto.AudioRequest{
		Model: "local-alias", Input: "你好", ResponseFormat: "wav",
		Metadata: []byte(`{"watermark":{"aigc_watermark":false}}`),
	})
	require.NoError(t, err)
	response, err := adaptor.DoRequest(c, info, reader)
	require.NoError(t, err)
	require.IsType(t, &http.Response{}, response)
	_, relayErr := adaptor.DoResponse(c, response.(*http.Response), info)
	require.Nil(t, relayErr)
	assert.Equal(t, "audio", recorder.Body.String())
	assert.Equal(t, "audio/wav", recorder.Header().Get("Content-Type"))
	assert.Equal(t, "speech-roundtrip-id", c.GetString(common.UpstreamRequestIdKey))
}

func TestAudioGenerationRejectsEmptyPromptsAndNonFiniteSpeed(t *testing.T) {
	for _, speed := range []float64{math.NaN(), math.Inf(1)} {
		_, err := doubaoSpeechRate(&speed)
		require.Error(t, err)
	}
	_, err := (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
	}, dto.AudioRequest{Model: "seed-audio-1.0", Input: "  ", ResponseFormat: "mp3"})
	require.ErrorContains(t, err, "input is required")
}

func TestAudioGenerationNativeImageAndConfig(t *testing.T) {
	metadata := []byte(`{"references":[{"image_url":"https://example.com/scene.png"}],"audio_config":{"sample_rate":48000,"speech_rate":0,"loudness_rate":0,"pitch_rate":0}}`)
	reader, err := (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"},
	}, dto.AudioRequest{Model: "local-audio", Input: "sound of this scene", ResponseFormat: "wav", Metadata: metadata})
	require.NoError(t, err)
	var request audioGenerationRequest
	require.NoError(t, decodeDoubaoAudioRequest(reader, &request))
	require.Len(t, request.References, 1)
	assert.Equal(t, "https://example.com/scene.png", request.References[0].ImageURL)
	require.NotNil(t, request.AudioConfig.SpeechRate)
	assert.Equal(t, 0, *request.AudioConfig.SpeechRate)
	require.NotNil(t, request.AudioConfig.LoudnessRate)
	assert.Equal(t, 0, *request.AudioConfig.LoudnessRate)
	require.NotNil(t, request.AudioConfig.PitchRate)
	assert.Equal(t, 0, *request.AudioConfig.PitchRate)
	assert.Equal(t, "wav", request.AudioConfig.Format)
}

func TestAudioGenerationRejectsInvalidNativeOptions(t *testing.T) {
	for name, metadata := range map[string]string{
		"mixed resources":     `{"references":[{"image_url":"https://example.com/a.png"},{"speaker":"voice"}]}`,
		"conflicting fields":  `{"references":[{"audio_url":"https://example.com/a.wav","speaker":"voice"}]}`,
		"too many images":     `{"references":[{"image_url":"https://example.com/a.png"},{"image_url":"https://example.com/b.png"}]}`,
		"invalid data":        `{"references":[{"audio_data":"***"}]}`,
		"invalid rate":        `{"audio_config":{"pitch_rate":13}}`,
		"invalid sample rate": `{"audio_config":{"sample_rate":40000}}`,
		"format conflict":     `{"audio_config":{"format":"wav"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
				RelayMode:   relayconstant.RelayModeAudioSpeech,
				ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "seed-audio-1.0"},
			}, dto.AudioRequest{Model: "local-audio", Input: "sound", ResponseFormat: "mp3", Metadata: []byte(metadata)})
			require.Error(t, err)
		})
	}
}

func TestConvertAudioRequestMapsCurrentReferenceAudioProfile(t *testing.T) {
	c := doubaoAudioTestContext()
	useEmotion := false
	metadata, err := common.Marshal(map[string]any{
		"audio_url":                     "data:audio/wav;base64,UklGRg==",
		"should_use_prompt_for_emotion": useEmotion,
		"emotion_prompt":                "紧张、急促",
	})
	require.NoError(t, err)

	reader, err := (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "seed-audio-compatible-model",
		},
	}, dto.AudioRequest{
		Model:          "local-audio-model",
		Input:          "你终于来了。",
		ResponseFormat: "mp3",
		Metadata:       metadata,
	})
	require.NoError(t, err)

	var request audioGenerationRequest
	require.NoError(t, decodeDoubaoAudioRequest(reader, &request))
	require.Len(t, request.References, 1)
	assert.Equal(t, "UklGRg==", request.References[0].AudioData)
	assert.Empty(t, request.References[0].AudioURL)
	assert.Contains(t, request.TextPrompt, "@音频1")
	assert.NotContains(t, request.TextPrompt, "紧张、急促")
}

func TestConvertAudioRequestMapsCurrentMusicProfile(t *testing.T) {
	c := doubaoAudioTestContext()
	metadata, err := common.Marshal(map[string]any{
		"music_length_ms":            30_000,
		"force_instrumental":         true,
		"respect_sections_durations": true,
		"output_format":              "mp3_44100_128",
	})
	require.NoError(t, err)

	reader, err := (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "configured-audio-generation-model",
		},
	}, dto.AudioRequest{
		Model:          "local-music-model",
		Input:          "克制的悬疑配乐",
		ResponseFormat: "mp3",
		Metadata:       metadata,
	})
	require.NoError(t, err)

	var request audioGenerationRequest
	require.NoError(t, decodeDoubaoAudioRequest(reader, &request))
	assert.Contains(t, request.TextPrompt, "30 秒")
	assert.Contains(t, request.TextPrompt, "纯音乐")
	assert.Contains(t, request.TextPrompt, "段落的时长安排")
	assert.Equal(t, "mp3", request.AudioConfig.Format)
}

func TestConvertAudioRequestRejectsMusicLongerThanProviderLimit(t *testing.T) {
	metadata, err := common.Marshal(map[string]any{
		"music_length_ms": maxOutputDurationMS + 1,
	})
	require.NoError(t, err)

	_, err = (&Adaptor{}).ConvertAudioRequest(doubaoAudioTestContext(), &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "configured-audio-generation-model",
		},
	}, dto.AudioRequest{
		Model:          "local-music-model",
		Input:          "生成一段配乐",
		ResponseFormat: "mp3",
		Metadata:       metadata,
	})
	require.ErrorContains(t, err, "must not exceed 120000")
}

func TestConvertAudioRequestRejectsConflictingVoiceAndReferenceAudio(t *testing.T) {
	c := doubaoAudioTestContext()
	metadata, err := common.Marshal(map[string]any{
		"audio_url": "https://example.com/reference.mp3",
	})
	require.NoError(t, err)

	_, err = (&Adaptor{}).ConvertAudioRequest(c, &relaycommon.RelayInfo{
		RelayMode: relayconstant.RelayModeAudioSpeech,
	}, dto.AudioRequest{
		Model:          "configured-model",
		Input:          "测试",
		Voice:          "speaker-id",
		ResponseFormat: "mp3",
		Metadata:       metadata,
	})
	require.ErrorContains(t, err, "voice and metadata.audio_url cannot be combined")
}

func TestDoubaoAudioReferenceRejectsOversizedDataURL(t *testing.T) {
	payload := base64.StdEncoding.EncodeToString(make([]byte, maxReferenceAudioBytes+1))
	_, err := doubaoAudioReference("data:audio/wav;base64," + payload)
	require.ErrorContains(t, err, "exceeds maximum allowed size")
}

func TestDoubaoAudioReferenceRejectsInvalidBase64(t *testing.T) {
	_, err := doubaoAudioReference("data:audio/wav;base64,not-valid-***")
	require.ErrorContains(t, err, "invalid doubao audio base64 data")
}

func TestDoResponseReturnsAudioBinaryAndTracksDuration(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	response := `{"code":0,"message":"success","audio":"` + base64.StdEncoding.EncodeToString([]byte("audio-data")) + `","duration":4.2,"original_duration":5.1}`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Tt-Logid": []string{"upstream-log-id"}},
		Body:       io.NopCloser(strings.NewReader(response)),
	}

	usage, relayErr := (&Adaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{})
	require.Nil(t, relayErr)
	assert.Equal(t, "audio-data", recorder.Body.String())
	assert.Equal(t, "upstream-log-id", c.GetString(common.UpstreamRequestIdKey))
	require.IsType(t, &dto.Usage{}, usage)
	assert.Equal(t, 6, usage.(*dto.Usage).CompletionTokens)
}

func TestDoResponseRejectsOverflowingDurationAndRecordsClamp(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"code":0,"audio":"` + base64.StdEncoding.EncodeToString([]byte("audio-data")) + `","original_duration":1e300}`,
		)),
	}
	info := &relaycommon.RelayInfo{}

	usage, relayErr := (&Adaptor{}).DoResponse(c, resp, info)

	require.Nil(t, usage)
	require.NotNil(t, relayErr)
	assert.Contains(t, relayErr.Error(), "invalid original_duration")
	require.NotNil(t, info.QuotaClamp)
	assert.Equal(t, common.QuotaClampOverflow, info.QuotaClamp.Kind)
	assert.Equal(t, common.MaxQuota, info.QuotaClamp.Clamped)
	assert.Empty(t, recorder.Body.String())
}

func TestDoResponseReturnsCanonicalURL(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"code":0,"url":"https://example.com/result.mp3"}`)),
	}

	_, relayErr := (&Adaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{})
	require.Nil(t, relayErr)
	assert.JSONEq(t, `{"audio":{"url":"https://example.com/result.mp3"}}`, recorder.Body.String())
}

func TestDoErrorResponsePreservesProviderCodeAndLogID(t *testing.T) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"X-Tt-Logid": []string{"provider-trace-id"}},
		Body:       io.NopCloser(strings.NewReader(`{"code":10074,"message":"requested resource not granted"}`)),
	}

	relayErr := (&Adaptor{}).DoErrorResponse(c, resp, &relaycommon.RelayInfo{})
	require.NotNil(t, relayErr)
	assert.Equal(t, "provider-trace-id", c.GetString(common.UpstreamRequestIdKey))
	assert.Contains(t, relayErr.Error(), "10074")
	assert.Contains(t, relayErr.Error(), "requested resource not granted")
}

func doubaoAudioTestContext() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/audio/speech", nil)
	return c
}

func decodeDoubaoAudioRequest(reader io.Reader, target *audioGenerationRequest) error {
	body, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return common.Unmarshal(body, target)
}

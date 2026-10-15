package mrtoken

import (
	"io"
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWan3CanonicalGenerationRequests(t *testing.T) {
	for _, modelName := range []string{"wan3.0-video", "wan3.0-video-prime"} {
		for _, test := range []struct {
			name, body string
			types      []string
		}{
			{"text", `{"model":"local-wan","prompt":"paper boat","duration":30,"metadata":{"resolution":"480p","ratio":"21:9","generate_audio":false,"seed":-1,"prompt_extend":false,"watermark":false}}`, nil},
			{"first and last", `{"model":"local-wan","prompt":"paper boat","image":"https://example.test/first.png","duration":5,"metadata":{"last_frame_image":"https://example.test/last.png"}}`, []string{"first_frame", "last_frame"}},
			{"ordered image and audio references", `{"model":"local-wan","prompt":"图1听音频1","duration":5,"metadata":{"wan_media":[{"type":"reference_audio","url":"https://example.test/sound.mp3"},{"type":"reference_image","url":"https://example.test/actor.png"}]}}`, []string{"reference_audio", "reference_image"}},
		} {
			t.Run(modelName+"/"+test.name, func(t *testing.T) {
				a, c, info, _ := videoContext(t, test.body, modelName)
				require.Nil(t, a.ValidateRequestAndSetAction(c, info))
				endpoint, err := a.BuildRequestURL(info)
				require.NoError(t, err)
				assert.Equal(t, "https://api.mrtoken.cn/v1/video-generation/video-synthesis", endpoint)
				reader, err := a.BuildRequestBody(c, info)
				require.NoError(t, err)
				body, err := io.ReadAll(reader)
				require.NoError(t, err)
				var payload wan3Request
				require.NoError(t, common.Unmarshal(body, &payload))
				assert.Equal(t, modelName, payload.Model)
				assert.Equal(t, map[string]float64{"seconds": float64(*payload.Parameters.Duration)}, a.EstimateBilling(c, info))
				types := make([]string, 0, len(payload.Input.Media))
				for _, item := range payload.Input.Media {
					types = append(types, item.Type)
				}
				assert.Equal(t, append([]string{}, test.types...), types)
				if test.name == "text" {
					assert.Equal(t, "480P", *payload.Parameters.Resolution)
					assert.Equal(t, "21:9", *payload.Parameters.Ratio)
					assert.EqualValues(t, -1, *payload.Parameters.Seed)
					assert.False(t, *payload.Parameters.Audio)
					assert.False(t, *payload.Parameters.PromptExtend)
					assert.False(t, *payload.Parameters.Watermark)
				}
			})
		}
	}
}

func TestWan3RejectsUnsupportedOrUnbillableMedia(t *testing.T) {
	for _, test := range []struct{ name, body string }{
		{"too long", `{"model":"wan","prompt":"boat","duration":31}`},
		{"video reference", `{"model":"wan","prompt":"boat","metadata":{"reference_videos":["https://example.test/a.mp4"]}}`},
		{"ordered video reference", `{"model":"wan","prompt":"boat","metadata":{"wan_media":[{"type":"reference_video","url":"https://example.test/a.mp4"}]}}`},
		{"frame mixed with reference", `{"model":"wan","prompt":"boat","image":"https://example.test/a.png","metadata":{"reference_images":["https://example.test/b.png"]}}`},
		{"unsupported file", `{"model":"wan","prompt":"boat","metadata":{"reference_file":"https://example.test/a.pdf"}}`},
		{"negative multiplier", `{"model":"wan","prompt":"boat","duration":-1}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			a, c, info, _ := videoContext(t, test.body, "wan3.0-video")
			taskErr := a.ValidateRequestAndSetAction(c, info)
			require.NotNil(t, taskErr)
			assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
			assert.Nil(t, a.EstimateBilling(c, info))
		})
	}
}

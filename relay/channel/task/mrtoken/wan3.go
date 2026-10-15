package mrtoken

import (
	"fmt"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type wan3Request struct {
	Model string `json:"model"`
	Input struct {
		Prompt string       `json:"prompt"`
		Media  []wan27Media `json:"media,omitempty"`
	} `json:"input"`
	Parameters struct {
		Duration     *int    `json:"duration,omitempty"`
		Resolution   *string `json:"resolution,omitempty"`
		Ratio        *string `json:"ratio,omitempty"`
		Audio        *bool   `json:"audio,omitempty"`
		Seed         *int64  `json:"seed,omitempty"`
		PromptExtend *bool   `json:"prompt_extend,omitempty"`
		Watermark    *bool   `json:"watermark,omitempty"`
	} `json:"parameters"`
}

func IsWan3GenerationModel(name string) bool {
	return name == "wan3.0-video" || name == "wan3.0-video-prime"
}

func IsWanNativeGenerationModel(name string) bool {
	return IsWan27GenerationModel(name) || IsWan3GenerationModel(name)
}

func planWan3Request(req relaycommon.TaskSubmitReq, modelName string) (*wan3Request, error) {
	if !IsWan3GenerationModel(modelName) {
		return nil, fmt.Errorf("unsupported MrToken Wan 3.0 generation model")
	}
	if _, err := relaycommon.ValidateDCMediaTaskRequest(&req); err != nil {
		return nil, err
	}
	if req.DurationAuto {
		return nil, fmt.Errorf("MrToken Wan 3.0 requires a fixed duration")
	}
	options := wan27Options{}
	if err := req.UnmarshalMetadata(&options); err != nil {
		return nil, fmt.Errorf("invalid MrToken Wan 3.0 metadata")
	}
	for _, field := range []string{"duration", "frames", "input", "parameters", "media", "audio"} {
		if _, exists := req.Metadata[field]; exists {
			return nil, fmt.Errorf("use canonical fields for MrToken Wan 3.0 %s", field)
		}
	}
	if options.ReferenceFile != "" || options.ReferenceLink != "" || len(options.ReferenceVideos) > 0 || options.LastFrameImage != "" && req.Image == "" || options.ReturnLastFrame != nil && *options.ReturnLastFrame {
		return nil, fmt.Errorf("MrToken Wan 3.0 video/file/link references require separate billing or media support")
	}
	if req.Fps != 0 || options.NegativePrompt != "" {
		return nil, fmt.Errorf("unsupported MrToken Wan 3.0 fps or negative prompt")
	}
	if req.Width != 0 && (options.Resolution == "" || options.Ratio == "") {
		return nil, fmt.Errorf("MrToken Wan 3.0 dimensions require metadata.resolution and metadata.ratio")
	}
	if strings.TrimSpace(req.Prompt) == "" || len([]rune(req.Prompt)) > 20000 {
		return nil, fmt.Errorf("MrToken Wan 3.0 prompt must contain 1 to 20000 characters")
	}
	if options.Seed != nil && (*options.Seed < -1 || *options.Seed > 2147483647) {
		return nil, fmt.Errorf("MrToken Wan 3.0 seed must be -1 or between 0 and 2147483647")
	}
	seconds := req.Duration
	if !req.DurationSet && seconds == 0 {
		seconds = 5
	}
	if seconds < 2 || seconds > 30 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("MrToken Wan 3.0 duration must be between 2 and 30 seconds")
	}
	resolution := strings.ToUpper(strings.TrimSpace(options.Resolution))
	if resolution == "" {
		resolution = "1080P"
	}
	if resolution != "480P" && resolution != "720P" && resolution != "1080P" {
		return nil, fmt.Errorf("unsupported MrToken Wan 3.0 resolution")
	}
	ratio := options.Ratio
	if ratio == "" || ratio == "auto" {
		ratio = "adaptive"
	}
	switch ratio {
	case "adaptive", "21:9", "16:9", "4:3", "1:1", "3:4", "9:16":
	default:
		return nil, fmt.Errorf("unsupported MrToken Wan 3.0 aspect ratio")
	}
	result := &wan3Request{Model: modelName}
	result.Input.Prompt = req.Prompt
	result.Parameters.Duration, result.Parameters.Resolution, result.Parameters.Ratio = &seconds, &resolution, &ratio
	result.Parameters.Audio, result.Parameters.Seed = options.GenerateAudio, options.Seed
	result.Parameters.PromptExtend, result.Parameters.Watermark = options.PromptExtend, options.Watermark

	if req.Image != "" {
		result.Input.Media = append(result.Input.Media, wan27Media{Type: "first_frame", URL: req.Image})
		if options.LastFrameImage != "" {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "last_frame", URL: options.LastFrameImage})
		}
	}
	if len(options.WanMedia) > 0 && (len(options.ReferenceImages) > 0 || len(options.ReferenceAudios) > 0 || req.Image != "") {
		return nil, fmt.Errorf("MrToken Wan 3.0 ordered references cannot be combined with grouped references or frames")
	}
	for _, item := range options.WanMedia {
		if item.Type != "reference_image" && item.Type != "reference_audio" {
			return nil, fmt.Errorf("MrToken Wan 3.0 ordered media currently accepts image and audio references only")
		}
	}
	for _, source := range options.ReferenceImages {
		result.Input.Media = append(result.Input.Media, wan27Media{Type: "reference_image", URL: source})
	}
	for _, source := range options.ReferenceAudios {
		result.Input.Media = append(result.Input.Media, wan27Media{Type: "reference_audio", URL: source})
	}
	result.Input.Media = append(result.Input.Media, options.WanMedia...)
	imageCount, audioCount := 0, 0
	for _, item := range result.Input.Media {
		switch item.Type {
		case "first_frame", "last_frame":
			if !validWan27MediaURL(item.URL, true) {
				return nil, fmt.Errorf("invalid MrToken Wan 3.0 frame URL")
			}
		case "reference_image":
			imageCount++
			if !validWan27MediaURL(item.URL, true) {
				return nil, fmt.Errorf("invalid MrToken Wan 3.0 image URL")
			}
		case "reference_audio":
			audioCount++
			if !validWan27MediaURL(item.URL, false) {
				return nil, fmt.Errorf("invalid MrToken Wan 3.0 audio URL")
			}
		default:
			return nil, fmt.Errorf("MrToken Wan 3.0 video, file and link references are not enabled")
		}
		if item.ReferenceVoice != "" {
			return nil, fmt.Errorf("MrToken Wan 3.0 does not use per-reference voice")
		}
	}
	if imageCount > 10 || audioCount > 5 {
		return nil, fmt.Errorf("MrToken Wan 3.0 accepts at most 10 reference images and 5 reference audios")
	}
	return result, nil
}

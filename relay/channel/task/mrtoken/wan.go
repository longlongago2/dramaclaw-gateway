package mrtoken

import (
	"fmt"
	"net/url"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

// wan27Media preserves the order used by 图N / 视频N prompt references.
type wan27Media struct {
	Type           string `json:"type"`
	URL            string `json:"url"`
	ReferenceVoice string `json:"reference_voice,omitempty"`
}

type wan27Request struct {
	Model string `json:"model"`
	Input struct {
		Prompt         string       `json:"prompt"`
		NegativePrompt string       `json:"negative_prompt,omitempty"`
		AudioURL       string       `json:"audio_url,omitempty"`
		Media          []wan27Media `json:"media,omitempty"`
	} `json:"input"`
	Parameters struct {
		Duration     *int    `json:"duration,omitempty"`
		Resolution   *string `json:"resolution,omitempty"`
		Ratio        *string `json:"ratio,omitempty"`
		PromptExtend *bool   `json:"prompt_extend,omitempty"`
		Watermark    *bool   `json:"watermark,omitempty"`
		Seed         *int64  `json:"seed,omitempty"`
	} `json:"parameters"`
}

type wan27Options struct {
	relaycommon.DCMediaMetadata
	Seed           *int64       `json:"seed,omitempty"`
	PromptExtend   *bool        `json:"prompt_extend,omitempty"`
	NegativePrompt string       `json:"negative_prompt,omitempty"`
	WanMedia       []wan27Media `json:"wan_media,omitempty"`
}

// IsWan27GenerationModel is limited to models returned by MrToken discovery.
func IsWan27GenerationModel(name string) bool {
	return name == "wan2.7-t2v" || name == "wan2.7-i2v" || name == "wan2.7-r2v"
}

func validWan27MediaURL(source string, image bool) bool {
	if image && (strings.HasPrefix(source, "data:image/png;base64,") || strings.HasPrefix(source, "data:image/jpeg;base64,") || strings.HasPrefix(source, "data:image/webp;base64,")) {
		return true
	}
	parsed, err := url.Parse(source)
	return err == nil && parsed.User == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" && parsed.Fragment == ""
}

func planWan27Request(req relaycommon.TaskSubmitReq, modelName string) (*wan27Request, error) {
	if !IsWan27GenerationModel(modelName) {
		return nil, fmt.Errorf("unsupported MrToken Wan 2.7 generation model")
	}
	if _, err := relaycommon.ValidateDCMediaTaskRequestForModel(&req, modelName); err != nil {
		return nil, err
	}
	if req.DurationAuto {
		return nil, fmt.Errorf("MrToken Wan 2.7 requires a fixed duration")
	}
	options := wan27Options{}
	if err := req.UnmarshalMetadata(&options); err != nil {
		return nil, fmt.Errorf("invalid MrToken Wan metadata")
	}
	for _, field := range []string{"duration", "frames", "input", "parameters", "media", "audio_url"} {
		if _, exists := req.Metadata[field]; exists {
			return nil, fmt.Errorf("use canonical fields for MrToken Wan %s", field)
		}
	}
	if options.ReferenceFile != "" || options.ReferenceLink != "" || options.ReturnLastFrame != nil && *options.ReturnLastFrame || options.GenerateAudio != nil && *options.GenerateAudio {
		return nil, fmt.Errorf("unsupported MrToken Wan metadata")
	}
	if strings.TrimSpace(req.Prompt) == "" || len([]rune(req.Prompt)) > 5000 || len([]rune(options.NegativePrompt)) > 500 {
		return nil, fmt.Errorf("MrToken Wan prompt must contain 1 to 5000 characters and negative prompt at most 500")
	}
	if options.Seed != nil && (*options.Seed < 0 || *options.Seed > 2147483647) {
		return nil, fmt.Errorf("MrToken Wan seed must be between 0 and 2147483647")
	}
	if req.Fps != 0 {
		return nil, fmt.Errorf("MrToken Wan does not support custom fps")
	}
	// DramaClaw includes dimensions derived from these canonical tier fields.
	// Validation above checks their ratio; the provider receives only the tier.
	if req.Width != 0 && (options.Resolution == "" || options.Ratio == "") {
		return nil, fmt.Errorf("MrToken Wan dimensions require metadata.resolution and metadata.ratio")
	}
	resolution := strings.ToUpper(strings.TrimSpace(options.Resolution))
	if resolution == "" {
		resolution = "1080P"
	}
	if resolution != "720P" && resolution != "1080P" {
		return nil, fmt.Errorf("unsupported MrToken Wan resolution")
	}
	ratio := options.Ratio
	if ratio == "" {
		ratio = "16:9"
	}
	switch ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4":
	default:
		return nil, fmt.Errorf("unsupported MrToken Wan aspect ratio")
	}
	seconds := req.Duration
	if !req.DurationSet && seconds == 0 {
		seconds = 5
	}
	if seconds < 2 || seconds > 15 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("MrToken Wan duration must be between 2 and 15 seconds")
	}
	result := &wan27Request{Model: modelName}
	result.Input.Prompt, result.Input.NegativePrompt = req.Prompt, options.NegativePrompt
	result.Parameters.Duration, result.Parameters.Resolution = &seconds, &resolution
	result.Parameters.PromptExtend, result.Parameters.Watermark, result.Parameters.Seed = options.PromptExtend, options.Watermark, options.Seed

	switch modelName {
	case "wan2.7-t2v":
		if req.Image != "" || options.LastFrameImage != "" || len(options.ReferenceImages) > 0 || len(options.ReferenceVideos) > 0 || len(options.WanMedia) > 0 || len(options.ReferenceAudios) > 1 {
			return nil, fmt.Errorf("MrToken Wan T2V accepts text and at most one audio URL")
		}
		if len(options.ReferenceAudios) == 1 {
			if !validWan27MediaURL(options.ReferenceAudios[0], false) {
				return nil, fmt.Errorf("invalid MrToken Wan audio URL")
			}
			result.Input.AudioURL = options.ReferenceAudios[0]
		}
		result.Parameters.Ratio = &ratio
	case "wan2.7-i2v":
		if len(options.ReferenceImages) > 0 || len(options.WanMedia) > 0 || len(options.ReferenceVideos) > 1 || len(options.ReferenceAudios) > 1 || (req.Image == "") == (len(options.ReferenceVideos) == 0) {
			return nil, fmt.Errorf("MrToken Wan I2V requires one first frame or first clip")
		}
		if req.Image != "" {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "first_frame", URL: req.Image})
		} else {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "first_clip", URL: options.ReferenceVideos[0]})
		}
		if options.LastFrameImage != "" {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "last_frame", URL: options.LastFrameImage})
		}
		if len(options.ReferenceAudios) == 1 {
			if req.Image == "" {
				return nil, fmt.Errorf("MrToken Wan I2V driving audio requires a first frame")
			}
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "driving_audio", URL: options.ReferenceAudios[0]})
		}
	case "wan2.7-r2v":
		if options.LastFrameImage != "" || len(options.ReferenceAudios) > 0 || len(options.WanMedia) > 0 && (len(options.ReferenceImages) > 0 || len(options.ReferenceVideos) > 0) {
			return nil, fmt.Errorf("MrToken Wan R2V requires unambiguous ordered references")
		}
		for _, item := range options.WanMedia {
			if item.Type != "reference_image" && item.Type != "reference_video" {
				return nil, fmt.Errorf("MrToken Wan R2V ordered media accepts references only")
			}
		}
		if req.Image != "" {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "first_frame", URL: req.Image})
		}
		for _, source := range options.ReferenceImages {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "reference_image", URL: source})
		}
		for _, source := range options.ReferenceVideos {
			result.Input.Media = append(result.Input.Media, wan27Media{Type: "reference_video", URL: source})
		}
		result.Input.Media = append(result.Input.Media, options.WanMedia...)
		referenceCount := 0
		for _, item := range result.Input.Media {
			if item.Type == "reference_image" || item.Type == "reference_video" {
				referenceCount++
			}
		}
		if referenceCount < 1 || referenceCount > 5 {
			return nil, fmt.Errorf("MrToken Wan R2V requires 1 to 5 image/video references")
		}
		if len(options.ReferenceVideos) > 0 {
			if seconds > 10 {
				return nil, fmt.Errorf("MrToken Wan R2V with video references is limited to 10 seconds")
			}
		}
		if req.Image == "" {
			result.Parameters.Ratio = &ratio
		}
	}
	for _, item := range result.Input.Media {
		if item.Type != "first_frame" && item.Type != "last_frame" && item.Type != "first_clip" && item.Type != "driving_audio" && item.Type != "reference_image" && item.Type != "reference_video" {
			return nil, fmt.Errorf("unsupported MrToken Wan media type")
		}
		image := item.Type == "first_frame" || item.Type == "last_frame" || item.Type == "reference_image"
		if !validWan27MediaURL(item.URL, image) || item.ReferenceVoice != "" && (item.Type != "reference_image" && item.Type != "reference_video" || !validWan27MediaURL(item.ReferenceVoice, false)) {
			return nil, fmt.Errorf("invalid MrToken Wan media URL")
		}
		if modelName == "wan2.7-r2v" && item.Type == "reference_video" && seconds > 10 {
			return nil, fmt.Errorf("MrToken Wan R2V with video references is limited to 10 seconds")
		}
	}
	return result, nil
}

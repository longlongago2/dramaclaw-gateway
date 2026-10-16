package mrtoken

import (
	"fmt"
	"net/url"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type happyHorseMedia struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type happyHorseRequest struct {
	Model string `json:"model"`
	Input struct {
		Prompt string            `json:"prompt"`
		Media  []happyHorseMedia `json:"media,omitempty"`
	} `json:"input"`
	Parameters struct {
		Duration   *int    `json:"duration,omitempty"`
		Resolution *string `json:"resolution,omitempty"`
		Ratio      *string `json:"ratio,omitempty"`
		Seed       *int64  `json:"seed,omitempty"`
		Watermark  *bool   `json:"watermark,omitempty"`
	} `json:"parameters"`
}

// IsHappyHorseModel matches the generation variants discovered on MrToken.
// Video editing follows input duration and requires separate billing support.
func IsHappyHorseModel(name string) bool {
	switch name {
	case "happyhorse-1.0-t2v", "happyhorse-1.0-i2v", "happyhorse-1.0-r2v",
		"happyhorse-1.1-t2v", "happyhorse-1.1-i2v", "happyhorse-1.1-r2v":
		return true
	}
	return false
}

func planHappyHorseRequest(req relaycommon.TaskSubmitReq, modelName string) (*happyHorseRequest, error) {
	if !IsHappyHorseModel(modelName) {
		return nil, fmt.Errorf("unsupported MrToken HappyHorse generation model")
	}
	if _, err := relaycommon.ValidateDCMediaTaskRequest(&req); err != nil {
		return nil, err
	}
	if req.DurationAuto {
		return nil, fmt.Errorf("MrToken HappyHorse generation requires a fixed duration")
	}
	seconds := req.Duration
	if !req.DurationSet && seconds == 0 {
		seconds = 5
	}
	if seconds < 3 || seconds > 15 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("MrToken HappyHorse duration must be between 3 and 15 seconds")
	}
	options := seedanceOptions{}
	if err := req.UnmarshalMetadata(&options); err != nil {
		return nil, fmt.Errorf("invalid MrToken HappyHorse metadata")
	}
	for _, field := range []string{"duration", "frames", "parameters", "input", "media", "image_url", "video_url"} {
		if _, exists := req.Metadata[field]; exists {
			return nil, fmt.Errorf("use canonical fields for MrToken HappyHorse %s", field)
		}
	}
	if options.LastFrameImage != "" || len(options.ReferenceVideos) > 0 || len(options.ReferenceAudios) > 0 || options.ReferenceFile != "" || options.ReferenceLink != "" || (options.GenerateAudio != nil && *options.GenerateAudio) || (options.ReturnLastFrame != nil && *options.ReturnLastFrame) {
		return nil, fmt.Errorf("MrToken HappyHorse generation supports text, first frame or reference images only")
	}
	if strings.TrimSpace(req.Prompt) == "" || len([]rune(req.Prompt)) > 2500 {
		return nil, fmt.Errorf("MrToken HappyHorse prompt must contain 1 to 2500 characters")
	}
	if options.Seed != nil && (*options.Seed < 0 || *options.Seed > 2147483647) {
		return nil, fmt.Errorf("MrToken HappyHorse seed must be between 0 and 2147483647")
	}
	resolution := strings.ToUpper(options.Resolution)
	if resolution == "" {
		resolution = "720P"
	}
	if resolution != "480P" && resolution != "720P" && resolution != "1080P" {
		return nil, fmt.Errorf("unsupported MrToken HappyHorse resolution")
	}
	ratio := options.Ratio
	if ratio == "" {
		ratio = "16:9"
	}
	switch ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "4:5", "5:4", "9:21", "21:9", "auto":
	default:
		return nil, fmt.Errorf("unsupported MrToken HappyHorse aspect ratio")
	}
	result := &happyHorseRequest{Model: modelName}
	result.Input.Prompt = req.Prompt
	result.Parameters.Duration, result.Parameters.Resolution = &seconds, &resolution
	result.Parameters.Seed, result.Parameters.Watermark = options.Seed, options.Watermark
	var sources []string
	mediaType := "reference_image"
	switch {
	case strings.HasSuffix(modelName, "-t2v"):
		if req.Image != "" || len(options.ReferenceImages) > 0 {
			return nil, fmt.Errorf("MrToken HappyHorse T2V does not accept images")
		}
	case strings.HasSuffix(modelName, "-i2v"):
		sources = options.ReferenceImages
		if req.Image != "" {
			sources = []string{req.Image}
		}
		if len(sources) != 1 {
			return nil, fmt.Errorf("MrToken HappyHorse I2V requires exactly one first frame")
		}
		mediaType = "first_frame"
	case strings.HasSuffix(modelName, "-r2v"):
		sources = options.ReferenceImages
		if req.Image != "" || len(sources) < 1 || len(sources) > 9 {
			return nil, fmt.Errorf("MrToken HappyHorse R2V requires 1 to 9 reference images")
		}
	}
	// I2V derives its aspect ratio from the input. Other modes need a fixed ratio.
	if mediaType != "first_frame" {
		if ratio == "auto" {
			return nil, fmt.Errorf("MrToken HappyHorse auto ratio requires a first frame")
		}
		result.Parameters.Ratio = &ratio
	}
	for _, source := range sources {
		parsed, err := url.Parse(source)
		if err != nil || parsed.User != nil || !((parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" || strings.HasPrefix(source, "data:image/")) {
			return nil, fmt.Errorf("invalid MrToken HappyHorse image URL")
		}
		result.Input.Media = append(result.Input.Media, happyHorseMedia{Type: mediaType, URL: source})
	}
	return result, nil
}

package mrtoken

import (
	"fmt"
	"strings"

	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

var modelList = []string{
	"minimax-h3-t2v", "minimax-h3-i2v", "minimax-h3-r2v",
	"minimax-h3-max-t2v", "minimax-h3-max-i2v",
	"doubao-seedance-2.0", "doubao-seedance-2.0-fast", "doubao-seedance-2.0-mini",
	"happyhorse-1.0-t2v", "happyhorse-1.0-i2v", "happyhorse-1.0-r2v",
	"happyhorse-1.1-t2v", "happyhorse-1.1-i2v", "happyhorse-1.1-r2v",
	"wan2.7-t2v", "wan2.7-i2v", "wan2.7-r2v",
	"wan3.0-video", "wan3.0-video-prime",
}

type fileReference struct {
	FileID string `json:"file_id"`
}

type videoRequest struct {
	Model           string          `json:"model"`
	Prompt          string          `json:"prompt"`
	Size            string          `json:"size"`
	Seconds         *int            `json:"seconds,omitempty"`
	Seed            *int64          `json:"seed,omitempty"`
	Steps           *int            `json:"steps,omitempty"`
	Sampler         *string         `json:"sampler,omitempty"`
	Scheduler       *string         `json:"scheduler,omitempty"`
	Parameters      map[string]any  `json:"parameters,omitempty"`
	InputReference  *fileReference  `json:"input_reference,omitempty"`
	LastFrame       *fileReference  `json:"last_frame,omitempty"`
	ReferenceImages []fileReference `json:"reference_images,omitempty"`
	firstFrame      string
	lastFrame       string
	references      []string
}

type videoOptions struct {
	relaycommon.DCMediaMetadata
	Seed       *int64         `json:"seed,omitempty"`
	Steps      *int           `json:"steps,omitempty"`
	Sampler    *string        `json:"sampler,omitempty"`
	Scheduler  *string        `json:"scheduler,omitempty"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

func supportedModel(name string) bool {
	for _, candidate := range modelList {
		if name == candidate {
			return true
		}
	}
	return false
}

// planVideoRequest validates the DC-Media boundary before uploads or billing.
func planVideoRequest(req relaycommon.TaskSubmitReq, modelName string) (*videoRequest, error) {
	if !supportedModel(modelName) || IsSeedanceModel(modelName) || IsHappyHorseModel(modelName) {
		return nil, fmt.Errorf("MrToken video model is not supported by the H3 adapter")
	}
	if _, err := relaycommon.ValidateDCMediaTaskRequest(&req); err != nil {
		return nil, err
	}
	if req.DurationAuto {
		return nil, fmt.Errorf("MrToken H3 requires a fixed duration")
	}
	seconds := req.Duration
	if !req.DurationSet && seconds == 0 {
		seconds = 5
	}
	if seconds < 1 || seconds > 30 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("MrToken H3 duration must be between 1 and 30 seconds")
	}
	options := videoOptions{}
	if err := req.UnmarshalMetadata(&options); err != nil {
		return nil, fmt.Errorf("invalid MrToken H3 metadata")
	}
	if len(options.ReferenceVideos) > 0 || len(options.ReferenceAudios) > 0 || options.ReferenceFile != "" || options.ReferenceLink != "" {
		return nil, fmt.Errorf("MrToken H3 supports image references only")
	}
	result := &videoRequest{
		Model: modelName, Prompt: req.Prompt, Seconds: &seconds,
		Seed: options.Seed, Steps: options.Steps, Sampler: options.Sampler,
		Scheduler: options.Scheduler, Parameters: options.Parameters,
		firstFrame: strings.TrimSpace(req.Image), lastFrame: strings.TrimSpace(options.LastFrameImage),
		references: options.ReferenceImages,
	}
	if strings.TrimSpace(result.Prompt) == "" || len([]rune(result.Prompt)) > 30000 {
		return nil, fmt.Errorf("MrToken H3 prompt must contain 1 to 30000 characters")
	}
	if result.Seed == nil && req.Seed != 0 {
		seed := int64(req.Seed)
		result.Seed = &seed
	}
	if result.Seed != nil && *result.Seed < 0 {
		return nil, fmt.Errorf("MrToken H3 seed must be non-negative")
	}
	if result.Steps != nil && (*result.Steps < 1 || *result.Steps > 100) {
		return nil, fmt.Errorf("MrToken H3 steps must be between 1 and 100")
	}
	isMax := strings.Contains(modelName, "-max-")
	if strings.EqualFold(options.Ratio, "auto") && !(isMax && strings.HasSuffix(modelName, "-i2v")) {
		return nil, fmt.Errorf("MrToken H3 auto size is supported only by MAX I2V")
	}
	switch {
	case strings.HasSuffix(modelName, "-t2v"):
		if result.firstFrame != "" || result.lastFrame != "" || len(result.references) > 0 {
			return nil, fmt.Errorf("MrToken H3 T2V does not accept image inputs")
		}
	case strings.HasSuffix(modelName, "-r2v"):
		if result.firstFrame != "" || result.lastFrame != "" || len(result.references) < 1 || len(result.references) > 4 {
			return nil, fmt.Errorf("MrToken H3 R2V requires 1 to 4 reference images")
		}
	default:
		// The canvas image-to-video mode uses a reference image rather than image.
		if len(result.references) == 1 && result.firstFrame == "" {
			result.firstFrame = result.references[0]
			result.references = nil
		}
		if len(result.references) > 0 || (result.lastFrame != "" && result.firstFrame == "") || (isMax && result.firstFrame == "") {
			return nil, fmt.Errorf("MrToken H3 I2V requires one first frame and an optional last frame")
		}
	}
	for key, value := range result.Parameters {
		switch key {
		case "turbo":
			if _, ok := value.(bool); !ok || isMax {
				return nil, fmt.Errorf("invalid MrToken H3 turbo parameter")
			}
		case "lora_strength":
			number, ok := value.(float64)
			if !ok || number < 0 || number > 2 || isMax {
				return nil, fmt.Errorf("invalid MrToken H3 lora_strength parameter")
			}
		default:
			return nil, fmt.Errorf("unsupported MrToken H3 parameters field")
		}
	}
	width, height := req.Width, req.Height
	if width > 2048 || height > 2048 {
		return nil, fmt.Errorf("MrToken H3 dimensions must not exceed 2048 pixels")
	}
	if width > 0 && height > 0 {
		// H3 requires multiples of 32; standard 720p/480p tiers need alignment.
		width, height = (width+31)/32*32, (height+31)/32*32
		result.Size = fmt.Sprintf("%dx%d", width, height)
	} else if isMax && result.firstFrame != "" && strings.EqualFold(options.Ratio, "auto") {
		result.Size = "auto"
	} else if strings.HasSuffix(modelName, "-r2v") {
		result.Size = "1280x736"
	} else if isMax {
		result.Size = "640x640"
	} else {
		result.Size = "864x480"
	}
	return result, nil
}

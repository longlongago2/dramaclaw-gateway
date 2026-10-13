package mrtoken

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relay/channel/task/doubao"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

type seedanceRequest struct {
	Model           string               `json:"model"`
	Content         []doubao.ContentItem `json:"content"`
	Duration        *int                 `json:"duration,omitempty"`
	Resolution      *string              `json:"resolution,omitempty"`
	Ratio           *string              `json:"ratio,omitempty"`
	Seed            *int64               `json:"seed,omitempty"`
	GenerateAudio   *bool                `json:"generate_audio,omitempty"`
	ReturnLastFrame *bool                `json:"return_last_frame,omitempty"`
	Watermark       *bool                `json:"watermark,omitempty"`
}

type seedanceOptions struct {
	relaycommon.DCMediaMetadata
	Seed *int64 `json:"seed,omitempty"`
}

// IsSeedanceModel identifies the exact native Seedance IDs returned by MrToken.
func IsSeedanceModel(name string) bool {
	return name == "doubao-seedance-2.0" || name == "doubao-seedance-2.0-fast" || name == "doubao-seedance-2.0-mini"
}

func planSeedanceRequest(req relaycommon.TaskSubmitReq, modelName string) (*seedanceRequest, error) {
	if !IsSeedanceModel(modelName) {
		return nil, fmt.Errorf("unsupported MrToken Seedance model")
	}
	if _, err := relaycommon.ValidateDCMediaTaskRequest(&req); err != nil {
		return nil, err
	}
	if req.DurationAuto {
		return nil, fmt.Errorf("MrToken Seedance 2.0 requires a fixed duration")
	}
	seconds := req.Duration
	if !req.DurationSet && seconds == 0 {
		seconds = 5
	}
	if seconds < 4 || seconds > 15 || seconds > relaycommon.MaxTaskDurationSeconds {
		return nil, fmt.Errorf("MrToken Seedance 2.0 duration must be between 4 and 15 seconds")
	}
	options := seedanceOptions{}
	if err := req.UnmarshalMetadata(&options); err != nil {
		return nil, fmt.Errorf("invalid MrToken Seedance metadata")
	}
	if options.ReferenceFile != "" || options.ReferenceLink != "" {
		return nil, fmt.Errorf("MrToken Seedance accepts image, video and audio references only")
	}
	if _, ok := req.Metadata["duration"]; ok {
		return nil, fmt.Errorf("use the canonical duration field for MrToken Seedance")
	}
	if _, ok := req.Metadata["frames"]; ok {
		return nil, fmt.Errorf("MrToken Seedance 2.0 does not support frames")
	}
	imageCount := len(options.ReferenceImages)
	if req.Image != "" {
		imageCount++
	}
	if options.LastFrameImage != "" {
		imageCount++
		if req.Image == "" {
			return nil, fmt.Errorf("MrToken Seedance last frame requires a first frame")
		}
	}
	if imageCount > 9 || len(options.ReferenceVideos) > 3 || len(options.ReferenceAudios) > 3 || imageCount+len(options.ReferenceVideos)+len(options.ReferenceAudios) > 12 {
		return nil, fmt.Errorf("MrToken Seedance accepts at most 9 images, 3 videos, 3 audios and 12 references in total")
	}
	if len(options.ReferenceAudios) > 0 && imageCount == 0 && len(options.ReferenceVideos) == 0 {
		return nil, fmt.Errorf("MrToken Seedance audio references require an image or video reference")
	}
	resolution := strings.ToLower(strings.TrimSpace(options.Resolution))
	if resolution == "" {
		resolution = "720p"
	}
	if resolution != "480p" && resolution != "720p" && !(resolution == "1080p" && modelName == "doubao-seedance-2.0" && len(options.ReferenceImages) == 0 && len(options.ReferenceVideos) == 0 && len(options.ReferenceAudios) == 0) {
		return nil, fmt.Errorf("unsupported MrToken Seedance resolution for this model and input mode")
	}
	ratio := options.Ratio
	if ratio == "" {
		ratio = "16:9"
	}
	if ratio == "auto" {
		ratio = "adaptive"
	}
	switch ratio {
	case "16:9", "9:16", "1:1", "4:3", "3:4", "21:9", "adaptive":
	default:
		return nil, fmt.Errorf("unsupported MrToken Seedance aspect ratio")
	}
	if options.Seed != nil && (*options.Seed < -1 || *options.Seed > 2147483647) {
		return nil, fmt.Errorf("MrToken Seedance seed must be between -1 and 2147483647")
	}
	if strings.TrimSpace(req.Prompt) == "" && imageCount == 0 && len(options.ReferenceVideos) == 0 {
		return nil, fmt.Errorf("MrToken Seedance requires a prompt or visual reference")
	}
	result := &seedanceRequest{
		Model: modelName, Duration: &seconds, Resolution: &resolution, Ratio: &ratio,
		Seed: options.Seed, GenerateAudio: options.GenerateAudio,
		ReturnLastFrame: options.ReturnLastFrame, Watermark: options.Watermark,
		Content: []doubao.ContentItem{},
	}
	for _, group := range []struct {
		kind, role string
		values     []string
	}{
		{"image", "first_frame", []string{req.Image}},
		{"image", "last_frame", []string{options.LastFrameImage}},
		{"image", "reference_image", options.ReferenceImages},
		{"video", "reference_video", options.ReferenceVideos},
		{"audio", "reference_audio", options.ReferenceAudios},
	} {
		for _, source := range group.values {
			if source == "" {
				continue
			}
			parsed, err := url.Parse(source)
			if err != nil || parsed.User != nil || !((parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" || group.kind == "image" && strings.HasPrefix(source, "data:image/")) {
				return nil, fmt.Errorf("invalid MrToken Seedance reference URL")
			}
			item := doubao.ContentItem{Type: group.kind + "_url", Role: group.role}
			media := &doubao.MediaURL{URL: source}
			switch group.kind {
			case "image":
				item.ImageURL = media
			case "video":
				item.VideoURL = media
			case "audio":
				item.AudioURL = media
			}
			result.Content = append(result.Content, item)
		}
	}
	if req.Prompt != "" {
		result.Content = append(result.Content, doubao.ContentItem{Type: "text", Text: req.Prompt})
	}
	return result, nil
}

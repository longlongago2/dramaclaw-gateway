package mrtoken

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
)

const maxImageBytes = 20 << 20

var fileIDPattern = regexp.MustCompile(`^file_[a-f0-9]{32}$`)

func platformClient(proxy string) (*http.Client, error) {
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil || client == nil {
		return nil, fmt.Errorf("cannot create MrToken HTTP client")
	}
	copy := *client
	copy.Timeout = 60 * time.Second
	copy.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &copy, nil
}

// errorMessage extracts stable error fields, never raw provider bodies or credentials.
func errorMessage(data []byte, key string) string {
	if key != "" {
		data = bytes.ReplaceAll(data, []byte(key), []byte("[REDACTED]"))
	}
	var payload struct {
		Detail any `json:"detail"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	message := "platform request failed"
	if common.Unmarshal(data, &payload) == nil {
		if payload.Message != "" {
			message = payload.Message
		}
		if detail, ok := payload.Detail.(string); ok && detail != "" {
			message = detail
		}
		if payload.Error != nil && payload.Error.Message != "" {
			message = payload.Error.Message
		}
		if detail, ok := payload.Detail.(map[string]any); ok {
			if reasons, ok := detail["reasons"].([]any); ok {
				var messages []string
				for _, reason := range reasons {
					if text, ok := reason.(string); ok && text != "" {
						messages = append(messages, text)
					}
				}
				if len(messages) > 0 {
					message = strings.Join(messages, "; ")
				}
			}
		}
	}
	message = strings.Join(strings.Fields(message), " ")
	if len(message) > 400 {
		message = message[:400]
	}
	return message
}

func sanitizedResponse(resp *http.Response, key string) (*http.Response, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return nil, fmt.Errorf("invalid MrToken platform response")
	}
	if key != "" {
		data = bytes.ReplaceAll(data, []byte(key), []byte("[REDACTED]"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ = common.Marshal(map[string]any{"error": map[string]string{"message": errorMessage(data, key)}})
	}
	resp.Body = io.NopCloser(bytes.NewReader(data))
	resp.ContentLength = int64(len(data))
	return resp, nil
}

func (a *TaskAdaptor) uploadImage(ctx context.Context, modelName, source string, vendorID *string, proxy string) (string, error) {
	if fileIDPattern.MatchString(source) {
		return source, nil
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var data []byte
	var err error
	if strings.HasPrefix(source, "data:") {
		parts := strings.SplitN(source, ",", 2)
		if len(parts) != 2 || !strings.HasPrefix(parts[0], "data:image/") || !strings.HasSuffix(parts[0], ";base64") || len(parts[1]) > (maxImageBytes+2)/3*4 {
			return "", fmt.Errorf("invalid MrToken reference image data URL")
		}
		data, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return "", fmt.Errorf("invalid MrToken reference image encoding")
		}
	} else {
		if err := service.ValidateSSRFProtectedFetchURL(source); err != nil {
			return "", fmt.Errorf("MrToken reference image URL is blocked")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return "", fmt.Errorf("invalid MrToken reference image URL")
		}
		client := service.GetSSRFProtectedHTTPClient()
		if client == nil {
			return "", fmt.Errorf("reference image HTTP client is unavailable")
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("cannot fetch MrToken reference image")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("cannot fetch MrToken reference image: HTTP %d", resp.StatusCode)
		}
		data, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
		if err != nil {
			return "", fmt.Errorf("cannot read MrToken reference image")
		}
	}
	if len(data) == 0 || len(data) > maxImageBytes {
		return "", fmt.Errorf("MrToken reference image must contain at most 20 MiB")
	}
	extension := ""
	switch http.DetectContentType(data) {
	case "image/png":
		extension = ".png"
	case "image/jpeg":
		extension = ".jpg"
	case "image/webp":
		extension = ".webp"
	default:
		return "", fmt.Errorf("MrToken reference image must be PNG, JPEG or WebP")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference"+extension)
	if err != nil {
		return "", fmt.Errorf("cannot encode MrToken file upload")
	}
	if _, err = part.Write(data); err != nil {
		return "", fmt.Errorf("cannot encode MrToken file upload")
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("cannot encode MrToken file upload")
	}
	query := url.Values{"model": {modelName}}
	if *vendorID != "" {
		query.Set("vendorKeyId", *vendorID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/files?"+query.Encode(), &body)
	if err != nil {
		return "", fmt.Errorf("invalid MrToken upload URL")
	}
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client, err := platformClient(proxy)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MrToken reference image upload failed")
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("cannot read MrToken upload response")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("MrToken reference image upload failed: HTTP %d (%s)", resp.StatusCode, errorMessage(responseBody, a.apiKey))
	}
	var file struct {
		ID string `json:"id"`
	}
	if common.Unmarshal(responseBody, &file) != nil || !fileIDPattern.MatchString(file.ID) {
		return "", fmt.Errorf("MrToken upload response is missing a valid file ID")
	}
	routeID := resp.Header.Get("X-OpenImgVideo-Vendor-Key-Id")
	if routeID == "" || (*vendorID != "" && *vendorID != routeID) {
		return "", fmt.Errorf("MrToken image upload did not preserve the vendor route")
	}
	*vendorID = routeID
	return file.ID, nil
}

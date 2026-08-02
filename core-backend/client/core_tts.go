package client

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"time"

	"core-backend/types"

	"github.com/bytedance/sonic"
)

type CoreTTSClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

func NewCoreTTSClient(baseURL string, timeoutSeconds int) *CoreTTSClient {
	timeout := time.Duration(timeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &CoreTTSClient{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

type CoreVoice struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	Descriptions []string `json:"descriptions,omitempty"`
}

type SynthesizeRequest struct {
	Text    string   `json:"text"`
	VoiceID string   `json:"voice_id"`
	Speed   float64  `json:"speed"`
	Engine  string   `json:"engine"`
	Pitch   *float64 `json:"pitch,omitempty"`
	Emotion *string  `json:"emotion,omitempty"`
}

func (c *CoreTTSClient) GetInfo() (*types.UniversalManifest, error) {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result types.UniversalManifest
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetVoices lấy danh sách giọng cho một Mode.
//
// Việc lọc thuộc về Engine: chỉ nó biết giọng nào chạy được ở Mode nào, vì các Mode có thể
// dùng những backend khác nhau. Nền tảng không lọc lại — nếu Engine trả về một giọng thì
// nền tảng tin rằng Mode đó dùng được.
//
// modelID rỗng hoặc "all" nghĩa là hỏi toàn bộ, dùng cho màn hình quản lý.
func (c *CoreTTSClient) GetVoices(modelID string) ([]CoreVoice, error) {
	endpoint := c.BaseURL + "/voices"
	if modelID != "" && modelID != "all" {
		endpoint += "?model_id=" + url.QueryEscape(modelID)
	}

	resp, err := c.HTTPClient.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Core TTS returned status %d", resp.StatusCode)
	}

	var voices []CoreVoice
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&voices); err != nil {
		return nil, err
	}
	return voices, nil
}

func (c *CoreTTSClient) Synthesize(text, voice string, speed float64, engine string, pitch *float64, emotion *string) ([]byte, error) {
	payload := SynthesizeRequest{
		Text:    text,
		VoiceID: voice,
		Speed:   speed,
		Engine:  engine,
		Pitch:   pitch,
		Emotion: emotion,
	}

	bodyBytes, err := sonic.Marshal(payload)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Post(c.BaseURL+"/synthesize", "application/json", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Detail string `json:"detail"`
		}
		if sonic.Unmarshal(respBody, &errResp) == nil && errResp.Detail != "" {
			return nil, fmt.Errorf("Core TTS Error (%d): %s", resp.StatusCode, errResp.Detail)
		}
		return nil, fmt.Errorf("Core TTS Error (%d): %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

func (c *CoreTTSClient) CloneVoice(fileBytes []byte, filename, name string) (map[string]interface{}, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(fileBytes); err != nil {
		return nil, err
	}

	if err := writer.WriteField("name", name); err != nil {
		return nil, err
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", c.BaseURL+"/voices/clone", body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Lỗi Clone giọng từ Core TTS: %s", string(respBytes))
	}

	var result map[string]interface{}
	if err := sonic.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

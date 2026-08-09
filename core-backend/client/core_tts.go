package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net"
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
			// Transport khai riêng vì mặc định của thư viện chuẩn chỉ giữ 2 kết nối rỗi
			// cho mỗi host. Mọi lượt tổng hợp đều đi tới cùng một Engine, nên từ lượt thứ
			// ba đồng thời trở đi mỗi request phải bắt tay TCP mới rồi bỏ kết nối ngay sau
			// đó — một đợt 50 lượt là 48 lần bắt tay và 48 socket rơi vào TIME_WAIT.
			//
			// ResponseHeaderTimeout tách riêng khỏi Timeout tổng: Engine im lặng hoàn toàn
			// thì biết sớm, còn tổng hợp một đoạn dài vẫn được phép chạy hết thời gian.
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
				ResponseHeaderTimeout: timeout,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
			},
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

	// Kiểm mã trạng thái như GetVoices và Synthesize vẫn làm. Không kiểm thì một trang lỗi 5xx
	// tình cờ hợp cú pháp JSON sẽ giải mã thành manifest rỗng, và người vận hành nhận báo
	// "manifest không hợp lệ" cho một sự cố thật ra là Engine đang chết — chỉ sai chỗ cần sửa.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Core TTS trả mã %d khi lấy manifest", resp.StatusCode)
	}

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

// Synthesize gọi Engine sinh âm thanh, huỷ được qua ctx.
//
// Nhận Context vì lượt gọi này là phần dài nhất của cả hệ thống — tới TTS_CLIENT_TIMEOUT_SECONDS
// giây — và là thứ duy nhất đáng huỷ khi người dùng bấm dừng. Không có nó, cờ cancel chỉ đổi
// được con số hiện trên giao diện còn GPU vẫn chạy hết lượt.
func (c *CoreTTSClient) Synthesize(ctx context.Context, text, voice string, speed float64, engine string, pitch *float64, emotion *string) ([]byte, error) {
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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/synthesize", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTPClient.Do(req)
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

func (c *CoreTTSClient) CloneVoice(src io.Reader, filename, name string) (map[string]interface{}, error) {
	// Stream multipart qua pipe thay vì dựng toàn bộ trong bytes.Buffer: một tệp tham chiếu
	// dài cả phút là hàng chục MB, giữ một bản copy nữa trong RAM ở đây là copy thứ ba của
	// cùng một tệp (sau parseUpload và storage). Với io.Pipe, engine đọc trực tiếp từ luồng.
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	go func() {
		defer pw.Close()
		part, err := writer.CreateFormFile("file", filename)
		if err != nil {
			pw.CloseWithError(err)
			return
		}
		if _, err := io.Copy(part, src); err != nil {
			pw.CloseWithError(err)
			return
		}
		if err := writer.WriteField("name", name); err != nil {
			pw.CloseWithError(err)
			return
		}
		writer.Close()
	}()

	req, err := http.NewRequest("POST", c.BaseURL+"/voices/clone", pr)
	if err != nil {
		pr.Close()
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		pr.Close() // ngắt pipe để goroutine ghi dừng lại
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

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

	"backend/types"

	"github.com/bytedance/sonic"
)

type CoreTTSClient struct {
	BaseURL    string
	HTTPClient *http.Client
	grpcClient *GrpcTTSClient // nil when gRPC is not configured
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
			// Custom Transport because the standard library default only keeps 2 idle connections
			// per host. Every synthesis request goes to the same Engine, so starting from the third
			// concurrent request onward, each request must do a fresh TCP handshake and then drop
			// the connection immediately — a batch of 50 requests means 48 handshakes and 48
			// sockets in TIME_WAIT.
			//
			// ResponseHeaderTimeout is separate from the overall Timeout: if the Engine is
			// completely silent we know early, while a long synthesis is still allowed to run its
			// full duration.
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

// SetGrpcClient wires a gRPC client for Synthesize calls.
// When set, Synthesize dispatches via gRPC; all other methods stay HTTP.
func (c *CoreTTSClient) SetGrpcClient(grpcClient *GrpcTTSClient) {
	c.grpcClient = grpcClient
}

// Close shuts down the gRPC connection if one is active.
func (c *CoreTTSClient) Close() {
	if c.grpcClient != nil {
		c.grpcClient.Close()
	}
}

type CoreVoice struct {
	ID       string            `json:"id,omitempty"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"metadata,omitempty"`
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

	// Check status code as GetVoices and Synthesize do. Without the check, a 5xx error page
	// that happens to be valid JSON would decode into an empty manifest, and the operator gets
	// "invalid manifest" for a problem that is actually the Engine being down — only the wrong
	// place to fix.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Core TTS returned status %d when fetching manifest", resp.StatusCode)
	}

	var result types.UniversalManifest
	if err := sonic.ConfigDefault.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetVoices fetches the voice list for a given Mode.
//
// Filtering belongs to the Engine: only it knows which voices work in which Mode, since Modes
// may use different backends. The platform does not re-filter — if the Engine returns a voice,
// the platform trusts that the Mode can use it.
//
// An empty modelID or "all" means query everything, used for the management screen.
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

// Synthesize calls the Engine to generate audio, cancellable via ctx.
//
// Accepts a Context because this call is the longest-running part of the whole system — up to
// TTS_CLIENT_TIMEOUT_SECONDS seconds — and the only thing worth cancelling when the user hits
// stop. Without it, the cancel flag only changes a number on the UI while the GPU keeps running
// the full synthesis.
func (c *CoreTTSClient) Synthesize(ctx context.Context, text, voice string, speed float64, engine string, pitch *float64, emotion *string) ([]byte, error) {
	// Dispatch to gRPC when available; fall back to HTTP REST if gRPC call fails.
	if c.grpcClient != nil {
		audio, err := c.grpcClient.Synthesize(ctx, text, voice, speed, engine, pitch, emotion)
		if err == nil {
			return audio, nil
		}
		// If context was cancelled by user, return immediately without fallback
		if ctx.Err() != nil {
			return nil, err
		}
	}

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
	// Stream multipart via pipe instead of buffering everything in bytes.Buffer: a reference
	// file a minute long is tens of MB, and keeping an extra copy in RAM here would be the
	// third copy of the same file (after parseUpload and storage). With io.Pipe, the engine
	// reads directly from the stream.
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
		pr.Close() // break pipe so the write goroutine stops
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Core TTS voice clone error: %s", string(respBytes))
	}

	var result map[string]interface{}
	if err := sonic.Unmarshal(respBytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

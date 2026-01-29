package api

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"vico_home/native/internal/domain"
)

const ticketURL = "https://api-us.vicoo.tech/device/getWebrtcTicket"

type ticketRequest struct {
	SerialNumber              string      `json:"serialNumber"`
	CountryNo                 string      `json:"countryNo"`
	RequestID                 string      `json:"requestId"`
	Language                  string      `json:"language"`
	SupportUnlimitedWebsocket bool        `json:"supportUnlimitedWebsocket"`
	List                      []any       `json:"list"`
	App                       appMetadata `json:"app"`
}

type appMetadata struct {
	VersionName string `json:"versionName"`
	Bundle      string `json:"bundle"`
	TimeZone    string `json:"timeZone"`
	AppName     string `json:"appName"`
	TenantID    string `json:"tenantId"`
	Env         string `json:"env"`
	Version     int    `json:"version"`
	AppType     string `json:"appType"`
}

type ticketResponse struct {
	Result int           `json:"result"`
	Msg    string        `json:"msg"`
	Data   domain.Ticket `json:"data"`
}

// Client fetches WebRTC tickets from the VicoHome API.
type Client struct{}

// NewClient creates an API client.
func NewClient() *Client {
	return &Client{}
}

func generateRequestID() string {
	buf := make([]byte, 32)
	_, _ = rand.Read(buf)
	h := sha1.Sum(buf)
	return fmt.Sprintf("%x", h)[:32]
}

// FetchTicket calls the VicoHome API to obtain signaling credentials and ICE servers.
func (c *Client) FetchTicket(jwt, serialNumber string) (*domain.Ticket, error) {
	req := ticketRequest{
		SerialNumber:              serialNumber,
		CountryNo:                 "US",
		RequestID:                 generateRequestID(),
		Language:                  "en",
		SupportUnlimitedWebsocket: true,
		List:                      []any{},
		App: appMetadata{
			VersionName: "3.50.0(2f68e2)",
			Bundle:      "addx.ai.vicoo",
			TimeZone:    "America/New_York",
			AppName:     "VicoHome",
			TenantID:    "vicoo",
			Env:         "prod-k8s",
			Version:     14148,
			AppType:     "iOS",
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal ticket request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", ticketURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+jwt)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	var ticketResp ticketResponse
	if err := json.Unmarshal(respBody, &ticketResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if ticketResp.Result != 0 {
		return nil, fmt.Errorf("API error (result=%d): %s", ticketResp.Result, ticketResp.Msg)
	}

	return &ticketResp.Data, nil
}

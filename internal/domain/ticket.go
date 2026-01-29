package domain

// Ticket holds signaling credentials and ICE server configuration returned by the API.
type Ticket struct {
	TraceID             string      `json:"traceId"`
	GroupID             string      `json:"groupId"`
	Role                string      `json:"role"`
	ID                  string      `json:"id"`
	ICEServers          []ICEServer `json:"iceServer"`
	SignalServer        string      `json:"signalServer"`
	SignalServerIP      string      `json:"signalServerIpAddress"`
	Sign                string      `json:"sign"`
	SignalPingInterval  int         `json:"signalPingInterval"`
	MaxAllocationLimit  int         `json:"maxAllocationLimit"`
	AppStopLiveTimeout  int         `json:"appStopLiveTimeout"`
	DeviceSleepTimeout  int         `json:"deviceSleepTimeout"`
	Time                int64       `json:"time"`
	ExpirationTime      int64       `json:"expirationTime"`
	WebsocketPath       string      `json:"websocketPath"`
	AccessToken         string      `json:"accessToken"`
	RealCxSerialNumber  *string     `json:"realCxSerialNumber"`
	CountryNo           *string     `json:"countryNo"`
}

// ICEServer holds STUN/TURN server configuration.
type ICEServer struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
	IPAddress  string `json:"ipAddress"`
}

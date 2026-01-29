package signal

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"vico_home/native/internal/domain"

	"github.com/gorilla/websocket"
)

// message is the generic WebSocket message envelope.
type message struct {
	Method             string `json:"method"`
	Code               *int   `json:"code,omitempty"`
	Message            string `json:"message,omitempty"`
	ClientType         string `json:"clientType,omitempty"`
	ClientID           string `json:"clientId,omitempty"`
	Status             string `json:"status,omitempty"`
	AccessToken        string `json:"accessToken,omitempty"`
	ID                 string `json:"id,omitempty"`
	Role               string `json:"role,omitempty"`
	Name               string `json:"name,omitempty"`
	Group              string `json:"group,omitempty"`
	TraceID            string `json:"traceId,omitempty"`
	RecipientClientID  string `json:"recipientClientId,omitempty"`
	SenderClientID     string `json:"senderClientId,omitempty"`
	SessionID          string `json:"sessionId,omitempty"`
	MessageType        string `json:"messageType,omitempty"`
	MessagePayload     string `json:"messagePayload,omitempty"`
	Mode               string `json:"mode,omitempty"`
	ViewerType         string `json:"viewerType,omitempty"`
	Resolution         string `json:"resolution,omitempty"`
	Version            string `json:"version,omitempty"`
	Timestamp          int64  `json:"timestamp,omitempty"`
	Reason             int    `json:"reason,omitempty"`
}

// Client manages the WebSocket connection to the signaling server.
type Client struct {
	conn      *websocket.Conn
	ticket    *domain.Ticket
	serial    string
	sessionID string
	handler   domain.Handler

	mu     sync.Mutex
	closed chan struct{}
}

// NewClient creates a new signaling client.
func NewClient(ticket *domain.Ticket, serialNumber string, handler domain.Handler) *Client {
	sessionID := fmt.Sprintf("Android-%s-%d", ticket.ID, time.Now().UnixMilli())
	return &Client{
		ticket:    ticket,
		serial:    serialNumber,
		sessionID: sessionID,
		handler:   handler,
		closed:    make(chan struct{}),
	}
}

// Connect dials the signaling WebSocket and starts the read loop.
func (c *Client) Connect() error {
	u, err := url.Parse(c.ticket.SignalServer)
	if err != nil {
		return fmt.Errorf("parse signal server: %w", err)
	}
	u.Path = c.ticket.WebsocketPath

	log.Printf("[signal] connecting to %s", u.String())

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("websocket dial: %w", err)
	}
	c.conn = conn

	c.sendAuth()

	go c.readLoop()
	go c.pingLoop()

	return nil
}

// Close shuts down the WebSocket connection.
func (c *Client) Close() {
	select {
	case <-c.closed:
		return
	default:
		close(c.closed)
	}
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *Client) sendJSON(msg any) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[signal] marshal error: %v", err)
		return
	}
	log.Printf("[signal] >>> %s", string(data))
	if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[signal] write error: %v", err)
	}
}

func (c *Client) sendAuth() {
	c.sendJSON(message{
		Method:      "AUTH",
		ClientType:  "app",
		Status:      "normal",
		AccessToken: c.ticket.AccessToken,
		ID:          c.ticket.ID,
	})
}

// SendJoinLive sends the JOIN_LIVE message to the signaling server.
func (c *Client) SendJoinLive() {
	c.sendJSON(message{
		Method:            "JOIN_LIVE",
		Role:              "viewer",
		Name:              c.ticket.ID,
		Group:             c.ticket.GroupID,
		TraceID:           c.ticket.TraceID,
		RecipientClientID: c.serial,
	})
}

// SendSDPOffer sends the SDP offer via TRANSMIT.
func (c *Client) SendSDPOffer(sdp string) {
	payload := domain.SDPPayload{Type: "offer", SDP: sdp}
	payloadJSON, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(payloadJSON)

	c.sendJSON(message{
		Method:            "TRANSMIT",
		MessageType:       "SDP_OFFER",
		MessagePayload:    encoded,
		Mode:              "vicoo",
		RecipientClientID: c.serial,
		SenderClientID:    c.ticket.ID,
		SessionID:         c.sessionID,
		ViewerType:        "a4x_sdk",
		Resolution:        "1280x720",
		Version:           "0.0.1",
	})
}

// SendICECandidate sends a local ICE candidate via TRANSMIT.
func (c *Client) SendICECandidate(sdpMid string, sdpMLineIndex int, candidate string) {
	payload := domain.ICECandidatePayload{
		SDPMid:        sdpMid,
		SDPMLineIndex: sdpMLineIndex,
		Candidate:     candidate,
	}
	payloadJSON, _ := json.Marshal(payload)
	encoded := base64.StdEncoding.EncodeToString(payloadJSON)

	c.sendJSON(message{
		Method:            "TRANSMIT",
		MessageType:       "ICE_CANDIDATE",
		MessagePayload:    encoded,
		RecipientClientID: c.serial,
		SenderClientID:    c.ticket.ID,
		SessionID:         c.sessionID,
		Version:           "0.0.1",
	})
}

func (c *Client) readLoop() {
	defer c.Close()

	for {
		select {
		case <-c.closed:
			return
		default:
		}

		_, data, err := c.conn.ReadMessage()
		if err != nil {
			select {
			case <-c.closed:
				return
			default:
				log.Printf("[signal] read error: %v", err)
				return
			}
		}

		log.Printf("[signal] <<< %s", string(data))

		var msg message
		if err := json.Unmarshal(data, &msg); err != nil {
			log.Printf("[signal] unmarshal error: %v", err)
			continue
		}

		c.dispatch(msg)
	}
}

func (c *Client) dispatch(msg message) {
	switch msg.Method {
	case "AUTH_RESPONSE":
		if msg.Code != nil && *msg.Code == 0 {
			log.Printf("[signal] auth successful")
			c.handler.OnAuthSuccess()
		} else {
			code := -1
			if msg.Code != nil {
				code = *msg.Code
			}
			log.Printf("[signal] auth failed: code=%d msg=%s", code, msg.Message)
		}

	case "JOIN_LIVE_RESPONSE":
		log.Printf("[signal] join_live response: code=%v msg=%s", msg.Code, msg.Message)

	case "PEER_IN":
		log.Printf("[signal] peer in: clientId=%s", msg.ClientID)
		c.handler.OnPeerIn()

	case "PEER_OUT":
		log.Printf("[signal] peer out: clientId=%s", msg.ClientID)
		c.handler.OnPeerOut()

	case "TRANSMIT":
		switch msg.MessageType {
		case "SDP_ANSWER":
			decoded, err := base64.StdEncoding.DecodeString(msg.MessagePayload)
			if err != nil {
				log.Printf("[signal] decode SDP_ANSWER: %v", err)
				return
			}
			var sdp domain.SDPPayload
			if err := json.Unmarshal(decoded, &sdp); err != nil {
				log.Printf("[signal] unmarshal SDP_ANSWER: %v", err)
				return
			}
			log.Printf("[signal] received SDP answer")
			c.handler.OnSDPAnswer(sdp)

		case "ICE_CANDIDATE":
			decoded, err := base64.StdEncoding.DecodeString(msg.MessagePayload)
			if err != nil {
				log.Printf("[signal] decode ICE_CANDIDATE: %v", err)
				return
			}
			var candidate domain.ICECandidatePayload
			if err := json.Unmarshal(decoded, &candidate); err != nil {
				log.Printf("[signal] unmarshal ICE_CANDIDATE: %v", err)
				return
			}
			log.Printf("[signal] received remote ICE candidate")
			c.handler.OnRemoteICECandidate(candidate)
		}

	case "TRANSMIT_RESPONSE", "RESPONSE":
		// no-op

	default:
		log.Printf("[signal] unhandled method: %s", msg.Method)
	}
}

func (c *Client) pingLoop() {
	ticker := time.NewTicker(time.Duration(c.ticket.SignalPingInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.closed:
			return
		case <-ticker.C:
			c.mu.Lock()
			err := c.conn.WriteControl(
				websocket.PingMessage,
				[]byte{},
				time.Now().Add(5*time.Second),
			)
			c.mu.Unlock()
			if err != nil {
				select {
				case <-c.closed:
					return
				default:
					log.Printf("[signal] ping error: %v", err)
					return
				}
			}
		}
	}
}

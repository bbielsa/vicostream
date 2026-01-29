package domain

import "io"

// TicketFetcher retrieves signaling credentials from the API.
type TicketFetcher interface {
	FetchTicket(jwt, serialNumber string) (*Ticket, error)
}

// Signaler manages the WebSocket signaling connection.
type Signaler interface {
	Connect() error
	SendJoinLive()
	SendSDPOffer(sdp string)
	SendICECandidate(sdpMid string, sdpMLineIndex int, candidate string)
	Close()
}

// Handler receives signaling events.
type Handler interface {
	OnAuthSuccess()
	OnPeerIn()
	OnPeerOut()
	OnSDPAnswer(sdp SDPPayload)
	OnRemoteICECandidate(candidate ICECandidatePayload)
}

// Peer manages the WebRTC peer connection.
type Peer interface {
	AddTransceivers() error
	SetOnTrack(videoOut io.Writer)
	SetOnICECandidate(send func(sdpMid string, sdpMLineIndex int, candidate string))
	CreateOffer() (string, error)
	SetRemoteDescription(sdp SDPPayload) error
	AddRemoteICECandidate(candidate ICECandidatePayload) error
	Close()
}

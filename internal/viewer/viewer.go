package viewer

import (
	"context"
	"log"

	"vico_home/native/internal/domain"
)

// Viewer coordinates the signaling and WebRTC flows.
// It implements domain.Handler.
type Viewer struct {
	peer   domain.Peer
	signal domain.Signaler
	cancel context.CancelFunc
}

// New creates a Viewer with the given peer and context cancel function.
// Call SetSignaler before use to complete the circular dependency.
func New(peer domain.Peer, cancel context.CancelFunc) *Viewer {
	return &Viewer{
		peer:   peer,
		cancel: cancel,
	}
}

// SetSignaler injects the signaler after construction to resolve the
// circular dependency (Viewer needs Signaler, Signal needs Handler).
func (v *Viewer) SetSignaler(s domain.Signaler) {
	v.signal = s
}

func (v *Viewer) OnAuthSuccess() {
	log.Printf("[viewer] authenticated, joining live")
	v.signal.SendJoinLive()
}

func (v *Viewer) OnPeerIn() {
	log.Printf("[viewer] camera peer in, creating offer")

	sdp, err := v.peer.CreateOffer()
	if err != nil {
		log.Fatalf("[viewer] create offer: %v", err)
	}
	v.signal.SendSDPOffer(sdp)
}

func (v *Viewer) OnPeerOut() {
	log.Printf("[viewer] camera peer out, shutting down")
	v.cancel()
}

func (v *Viewer) OnSDPAnswer(sdp domain.SDPPayload) {
	if err := v.peer.SetRemoteDescription(sdp); err != nil {
		log.Printf("[viewer] set remote description: %v", err)
	}
}

func (v *Viewer) OnRemoteICECandidate(candidate domain.ICECandidatePayload) {
	go func() {
		if err := v.peer.AddRemoteICECandidate(candidate); err != nil {
			log.Printf("[viewer] add remote ICE candidate: %v", err)
		}
	}()
}

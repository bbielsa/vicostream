package viewer

import (
	"context"
	"io"
	"testing"
	"time"

	"vico_home/native/internal/domain"
)

// mockSignaler records calls for verification.
type mockSignaler struct {
	joinLiveCalled    bool
	sdpOfferSent      string
	iceCandidateSent  bool
	closeCalled       bool
}

func (m *mockSignaler) Connect() error                                                  { return nil }
func (m *mockSignaler) SendJoinLive()                                                   { m.joinLiveCalled = true }
func (m *mockSignaler) SendSDPOffer(sdp string)                                         { m.sdpOfferSent = sdp }
func (m *mockSignaler) SendICECandidate(sdpMid string, sdpMLineIndex int, candidate string) {
	m.iceCandidateSent = true
}
func (m *mockSignaler) Close() { m.closeCalled = true }

// mockPeer records calls for verification.
type mockPeer struct {
	offerSDP         string
	remoteDescSet    bool
	iceCandidateAdded bool
}

func (m *mockPeer) AddTransceivers() error               { return nil }
func (m *mockPeer) SetOnTrack(videoOut io.Writer)         {}
func (m *mockPeer) SetOnICECandidate(send func(string, int, string)) {}
func (m *mockPeer) CreateOffer() (string, error)          { return m.offerSDP, nil }
func (m *mockPeer) SetRemoteDescription(sdp domain.SDPPayload) error {
	m.remoteDescSet = true
	return nil
}
func (m *mockPeer) AddRemoteICECandidate(candidate domain.ICECandidatePayload) error {
	m.iceCandidateAdded = true
	return nil
}
func (m *mockPeer) Close() {}

func TestOnAuthSuccess_SendsJoinLive(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := &mockSignaler{}
	peer := &mockPeer{}
	v := New(peer, cancel)
	v.SetSignaler(sig)

	v.OnAuthSuccess()

	if !sig.joinLiveCalled {
		t.Error("expected SendJoinLive to be called")
	}
}

func TestOnPeerIn_CreatesOfferAndSends(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := &mockSignaler{}
	peer := &mockPeer{offerSDP: "v=0\r\ntest-sdp"}
	v := New(peer, cancel)
	v.SetSignaler(sig)

	v.OnPeerIn()

	if sig.sdpOfferSent != "v=0\r\ntest-sdp" {
		t.Errorf("expected SDP offer 'v=0\\r\\ntest-sdp', got %q", sig.sdpOfferSent)
	}
}

func TestOnPeerOut_CancelsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	peer := &mockPeer{}
	v := New(peer, cancel)
	v.SetSignaler(&mockSignaler{})

	v.OnPeerOut()

	select {
	case <-ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Error("expected context to be cancelled")
	}
}

func TestOnSDPAnswer_SetsRemoteDescription(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := &mockPeer{}
	v := New(peer, cancel)
	v.SetSignaler(&mockSignaler{})

	v.OnSDPAnswer(domain.SDPPayload{Type: "answer", SDP: "v=0\r\nanswer-sdp"})

	if !peer.remoteDescSet {
		t.Error("expected SetRemoteDescription to be called")
	}
}

func TestOnRemoteICECandidate_AddsCandidate(t *testing.T) {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	peer := &mockPeer{}
	v := New(peer, cancel)
	v.SetSignaler(&mockSignaler{})

	v.OnRemoteICECandidate(domain.ICECandidatePayload{
		SDPMid:    "0",
		Candidate: "candidate:123",
	})

	// Give the goroutine time to execute
	time.Sleep(50 * time.Millisecond)

	if !peer.iceCandidateAdded {
		t.Error("expected AddRemoteICECandidate to be called")
	}
}

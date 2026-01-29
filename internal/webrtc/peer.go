package webrtc

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"vico_home/native/internal/domain"

	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/nack"
	pion "github.com/pion/webrtc/v4"
)

// Peer wraps a Pion PeerConnection and DataChannel.
type Peer struct {
	pc            *pion.PeerConnection
	dc            *pion.DataChannel
	serialNumber  string
	remoteDescSet chan struct{}
}

// NewPeer creates a PeerConnection with minimal codec registration and a DataChannel.
func NewPeer(iceServers []domain.ICEServer, serialNumber string) (*Peer, error) {
	m := &pion.MediaEngine{}

	h264Codec := pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:    pion.MimeTypeH264,
			ClockRate:   90000,
			SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=64001f",
		},
		PayloadType: 121,
	}
	if err := m.RegisterCodec(h264Codec, pion.RTPCodecTypeVideo); err != nil {
		return nil, fmt.Errorf("register H264: %w", err)
	}

	pcmuCodec := pion.RTPCodecParameters{
		RTPCodecCapability: pion.RTPCodecCapability{
			MimeType:  pion.MimeTypePCMU,
			ClockRate: 8000,
			Channels:  1,
		},
		PayloadType: 0,
	}
	if err := m.RegisterCodec(pcmuCodec, pion.RTPCodecTypeAudio); err != nil {
		return nil, fmt.Errorf("register PCMU: %w", err)
	}

	i := &interceptor.Registry{}
	responderFactory, err := nack.NewResponderInterceptor()
	if err != nil {
		return nil, fmt.Errorf("create nack responder: %w", err)
	}
	i.Add(responderFactory)

	api := pion.NewAPI(
		pion.WithMediaEngine(m),
		pion.WithInterceptorRegistry(i),
	)

	var servers []pion.ICEServer
	for _, s := range iceServers {
		servers = append(servers, pion.ICEServer{
			URLs:       []string{s.URL},
			Username:   s.Username,
			Credential: s.Credential,
		})
	}

	pc, err := api.NewPeerConnection(pion.Configuration{
		ICEServers:   servers,
		BundlePolicy: pion.BundlePolicyMaxBundle,
	})
	if err != nil {
		return nil, fmt.Errorf("create peer connection: %w", err)
	}

	dc, err := pc.CreateDataChannel(serialNumber, nil)
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("create data channel: %w", err)
	}

	p := &Peer{
		pc:            pc,
		dc:            dc,
		serialNumber:  serialNumber,
		remoteDescSet: make(chan struct{}),
	}

	dc.OnOpen(func() {
		log.Printf("[webrtc] data channel opened")
		p.sendStartLive()
	})
	dc.OnMessage(func(msg pion.DataChannelMessage) {
		log.Printf("[webrtc] data channel message: %s", string(msg.Data))
	})
	dc.OnClose(func() {
		log.Printf("[webrtc] data channel closed")
	})

	pc.OnICEConnectionStateChange(func(state pion.ICEConnectionState) {
		log.Printf("[webrtc] ICE connection state: %s", state.String())
	})
	pc.OnConnectionStateChange(func(state pion.PeerConnectionState) {
		log.Printf("[webrtc] peer connection state: %s", state.String())
	})

	return p, nil
}

// AddTransceivers adds audio (sendrecv) and video (recvonly) transceivers.
func (p *Peer) AddTransceivers() error {
	_, err := p.pc.AddTransceiverFromKind(pion.RTPCodecTypeAudio, pion.RTPTransceiverInit{
		Direction: pion.RTPTransceiverDirectionSendrecv,
	})
	if err != nil {
		return fmt.Errorf("add audio transceiver: %w", err)
	}

	_, err = p.pc.AddTransceiverFromKind(pion.RTPCodecTypeVideo, pion.RTPTransceiverInit{
		Direction: pion.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		return fmt.Errorf("add video transceiver: %w", err)
	}

	return nil
}

// SetOnTrack sets up the track handler. Video H264 is written to videoOut, audio is drained.
func (p *Peer) SetOnTrack(videoOut io.Writer) {
	p.pc.OnTrack(func(track *pion.TrackRemote, receiver *pion.RTPReceiver) {
		codec := track.Codec()
		log.Printf("[webrtc] got track: kind=%s codec=%s pt=%d", track.Kind(), codec.MimeType, codec.PayloadType)

		if track.Kind() == pion.RTPCodecTypeVideo {
			go p.readVideoTrack(track, videoOut)
		} else {
			go func() {
				buf := make([]byte, 1500)
				for {
					_, _, err := track.Read(buf)
					if err != nil {
						return
					}
				}
			}()
		}
	})
}

func (p *Peer) readVideoTrack(track *pion.TrackRemote, w io.Writer) {
	log.Printf("[webrtc] reading H264 video track")

	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	depack := NewH264Depacketizer()

	for {
		pkt, _, err := track.ReadRTP()
		if err != nil {
			log.Printf("[webrtc] video track read error: %v", err)
			return
		}

		nalus := depack.Depacketize(pkt.Payload)
		for _, nalu := range nalus {
			if len(nalu) == 0 {
				continue
			}
			w.Write(startCode)
			w.Write(nalu)
		}
	}
}

// SetOnICECandidate registers the callback for locally discovered ICE candidates.
func (p *Peer) SetOnICECandidate(send func(sdpMid string, sdpMLineIndex int, candidate string)) {
	p.pc.OnICECandidate(func(c *pion.ICECandidate) {
		if c == nil {
			log.Printf("[webrtc] ICE gathering complete")
			return
		}

		candidateStr := c.ToJSON().Candidate
		if isLoopback(candidateStr) {
			log.Printf("[webrtc] filtering loopback ICE candidate")
			return
		}

		sdpMid := ""
		if c.ToJSON().SDPMid != nil {
			sdpMid = *c.ToJSON().SDPMid
		}
		sdpMLineIndex := 0
		if c.ToJSON().SDPMLineIndex != nil {
			sdpMLineIndex = int(*c.ToJSON().SDPMLineIndex)
		}

		log.Printf("[webrtc] local ICE candidate: %s", candidateStr)
		send(sdpMid, sdpMLineIndex, candidateStr)
	})
}

// CreateOffer creates an SDP offer and sets it as the local description.
func (p *Peer) CreateOffer() (string, error) {
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("create offer: %w", err)
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("set local description: %w", err)
	}

	log.Printf("[webrtc] local SDP offer set")
	return offer.SDP, nil
}

// SetRemoteDescription sets the SDP answer and unblocks remote ICE candidate addition.
func (p *Peer) SetRemoteDescription(sdp domain.SDPPayload) error {
	answer := pion.SessionDescription{
		Type: pion.SDPTypeAnswer,
		SDP:  sdp.SDP,
	}

	if err := p.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("set remote description: %w", err)
	}

	log.Printf("[webrtc] remote SDP answer set")
	close(p.remoteDescSet)
	return nil
}

// AddRemoteICECandidate waits for the remote description to be set, then adds the candidate.
func (p *Peer) AddRemoteICECandidate(candidate domain.ICECandidatePayload) error {
	<-p.remoteDescSet

	sdpMLineIndex := uint16(candidate.SDPMLineIndex)
	init := pion.ICECandidateInit{
		Candidate:     candidate.Candidate,
		SDPMid:        &candidate.SDPMid,
		SDPMLineIndex: &sdpMLineIndex,
	}

	if err := p.pc.AddICECandidate(init); err != nil {
		return fmt.Errorf("add ice candidate: %w", err)
	}

	log.Printf("[webrtc] added remote ICE candidate")
	return nil
}

// startLiveCommand is the JSON command sent over the DataChannel to start live streaming.
type startLiveCommand struct {
	Action       string `json:"action"`
	RequestID    string `json:"requestID"`
	ConnectionID string `json:"connectionID"`
	TimeStamp    string `json:"timeStamp"`
	Size         string `json:"size"`
	Resolution   string `json:"resolution"`
}

func (p *Peer) sendStartLive() {
	ts := strconv.FormatInt(time.Now().UnixMilli(), 10)
	cmd := startLiveCommand{
		Action:       "startLive",
		RequestID:    ts,
		ConnectionID: "",
		TimeStamp:    ts,
		Size:         "medium",
		Resolution:   "1280x720",
	}

	data, _ := json.Marshal(cmd)
	log.Printf("[webrtc] sending startLive: %s", string(data))
	if err := p.dc.SendText(string(data)); err != nil {
		log.Printf("[webrtc] sendStartLive error: %v", err)
	}
}

// Close shuts down the DataChannel and PeerConnection.
func (p *Peer) Close() {
	if p.dc != nil {
		p.dc.Close()
	}
	if p.pc != nil {
		p.pc.Close()
	}
}

func isLoopback(candidate string) bool {
	return strings.Contains(candidate, "127.0.0.1") || strings.Contains(candidate, "::1 ")
}

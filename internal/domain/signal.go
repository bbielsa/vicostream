package domain

// SDPPayload is the JSON structure for SDP offer/answer messages.
type SDPPayload struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// ICECandidatePayload is the JSON structure for ICE candidate messages.
type ICECandidatePayload struct {
	SDPMid        string `json:"sdpMid"`
	SDPMLineIndex int    `json:"sdpMLineIndex"`
	Candidate     string `json:"candidate"`
}

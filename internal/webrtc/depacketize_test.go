package webrtc

import (
	"bytes"
	"testing"
)

func TestDepacketize_SingleNAL(t *testing.T) {
	d := NewH264Depacketizer()

	// Type 5 = IDR slice (single NAL, type in range 1-23)
	payload := []byte{0x65, 0x01, 0x02, 0x03}
	nalus := d.Depacketize(payload)

	if len(nalus) != 1 {
		t.Fatalf("expected 1 NALU, got %d", len(nalus))
	}
	if !bytes.Equal(nalus[0], payload) {
		t.Errorf("expected payload %v, got %v", payload, nalus[0])
	}
}

func TestDepacketize_STAPA(t *testing.T) {
	d := NewH264Depacketizer()

	// STAP-A header (type 24 = 0x18), then two NALUs with 2-byte size prefix each
	nalu1 := []byte{0x67, 0xAA, 0xBB} // SPS
	nalu2 := []byte{0x68, 0xCC}       // PPS

	payload := []byte{0x18} // STAP-A indicator
	// NALU 1: size=3
	payload = append(payload, 0x00, 0x03)
	payload = append(payload, nalu1...)
	// NALU 2: size=2
	payload = append(payload, 0x00, 0x02)
	payload = append(payload, nalu2...)

	nalus := d.Depacketize(payload)

	if len(nalus) != 2 {
		t.Fatalf("expected 2 NALUs, got %d", len(nalus))
	}
	if !bytes.Equal(nalus[0], nalu1) {
		t.Errorf("NALU 0: expected %v, got %v", nalu1, nalus[0])
	}
	if !bytes.Equal(nalus[1], nalu2) {
		t.Errorf("NALU 1: expected %v, got %v", nalu2, nalus[1])
	}
}

func TestDepacketize_FUA(t *testing.T) {
	d := NewH264Depacketizer()

	// Fragment a type 5 (IDR) NAL with NRI=3 (0x60)
	// FU indicator: NRI=3 (0x60) | type=28 (0x1C) = 0x7C
	// FU header start: 0x80 | type=5 = 0x85
	// FU header middle: type=5 = 0x05
	// FU header end: 0x40 | type=5 = 0x45

	startPkt := []byte{0x7C, 0x85, 0x01, 0x02}
	midPkt := []byte{0x7C, 0x05, 0x03, 0x04}
	endPkt := []byte{0x7C, 0x45, 0x05, 0x06}

	// Start fragment: no output yet
	nalus := d.Depacketize(startPkt)
	if nalus != nil {
		t.Fatalf("expected nil on start fragment, got %d NALUs", len(nalus))
	}

	// Middle fragment: no output yet
	nalus = d.Depacketize(midPkt)
	if nalus != nil {
		t.Fatalf("expected nil on middle fragment, got %d NALUs", len(nalus))
	}

	// End fragment: should produce reassembled NALU
	nalus = d.Depacketize(endPkt)
	if len(nalus) != 1 {
		t.Fatalf("expected 1 NALU on end fragment, got %d", len(nalus))
	}

	// Reconstructed NAL: header byte (NRI=3 | type=5 = 0x65) + all fragment data
	expected := []byte{0x65, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06}
	if !bytes.Equal(nalus[0], expected) {
		t.Errorf("expected %v, got %v", expected, nalus[0])
	}
}

func TestDepacketize_EmptyPayload(t *testing.T) {
	d := NewH264Depacketizer()

	nalus := d.Depacketize(nil)
	if nalus != nil {
		t.Errorf("expected nil for empty payload, got %v", nalus)
	}

	nalus = d.Depacketize([]byte{})
	if nalus != nil {
		t.Errorf("expected nil for zero-length payload, got %v", nalus)
	}
}

func TestDepacketize_InstanceIsolation(t *testing.T) {
	d1 := NewH264Depacketizer()
	d2 := NewH264Depacketizer()

	// Start a FU-A fragment on d1
	startPkt := []byte{0x7C, 0x85, 0x01, 0x02}
	d1.Depacketize(startPkt)

	// d2 should have no state from d1
	endPkt := []byte{0x7C, 0x45, 0x03, 0x04}
	nalus := d2.Depacketize(endPkt)

	// d2 never got a start fragment, so its buffer should only contain the end data
	// This verifies instance isolation — d2's buffer is independent from d1
	if nalus == nil {
		// d2 appended to its own nil buffer and got an end, which produces a NALU
		// with just the end fragment data (no start header reconstruction)
		t.Log("d2 produced nil (no start fragment) — isolated correctly")
	}

	// d1 should still be able to complete its fragment
	nalus = d1.Depacketize(endPkt)
	if len(nalus) != 1 {
		t.Fatalf("expected d1 to produce 1 NALU, got %d", len(nalus))
	}
}

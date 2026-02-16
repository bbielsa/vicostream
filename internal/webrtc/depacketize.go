package webrtc

// H264Depacketizer extracts NAL units from RTP H264 payloads.
// It maintains instance state for FU-A fragment reassembly,
// preventing corruption when multiple streams are active.
type H264Depacketizer struct {
	fuaBuf      []byte
	fuaStarted  bool
	expectedSeq uint16
}

// NewH264Depacketizer creates a new depacketizer with its own reassembly buffer.
func NewH264Depacketizer() *H264Depacketizer {
	return &H264Depacketizer{}
}

// Depacketize extracts NAL units from an RTP H264 payload.
// Handles single NAL, STAP-A, and FU-A packet types.
func (d *H264Depacketizer) Depacketize(sequenceNumber uint16, payload []byte) [][]byte {
	if len(payload) < 1 {
		return nil
	}

	naluType := payload[0] & 0x1f

	switch {
	case naluType >= 1 && naluType <= 23:
		return [][]byte{payload}

	case naluType == 24:
		return d.depacketizeSTAPA(payload)

	case naluType == 28:
		return d.depacketizeFUA(sequenceNumber, payload)

	default:
		return nil
	}
}

func (d *H264Depacketizer) depacketizeSTAPA(payload []byte) [][]byte {
	var nalus [][]byte
	offset := 1 // skip STAP-A header byte

	for offset+2 <= len(payload) {
		size := int(payload[offset])<<8 | int(payload[offset+1])
		offset += 2
		if size == 0 {
			break
		}
		if offset+size > len(payload) {
			break
		}
		nalus = append(nalus, payload[offset:offset+size])
		offset += size
	}
	return nalus
}

func (d *H264Depacketizer) depacketizeFUA(sequenceNumber uint16, payload []byte) [][]byte {
	if len(payload) < 2 {
		return nil
	}

	fnri := payload[0] & 0xe0 // F + NRI bits from FU indicator
	fuHeader := payload[1]
	start := fuHeader&0x80 != 0
	end := fuHeader&0x40 != 0
	naluType := fuHeader & 0x1f

	if start {
		// Reconstruct NAL header: F+NRI from FU indicator + type from FU header
		d.fuaBuf = []byte{fnri | naluType}
		d.fuaStarted = true
		d.expectedSeq = sequenceNumber + 1
		d.fuaBuf = append(d.fuaBuf, payload[2:]...)

		if end {
			nalu := d.fuaBuf
			d.fuaBuf = nil
			d.fuaStarted = false
			return [][]byte{nalu}
		}
		return nil
	}

	// Drop orphan middle/end fragments.
	if !d.fuaStarted {
		return nil
	}

	// Sequence discontinuity means a missing/reordered fragment. Drop this NAL.
	if sequenceNumber != d.expectedSeq {
		d.fuaBuf = nil
		d.fuaStarted = false
		return nil
	}

	d.expectedSeq = sequenceNumber + 1
	d.fuaBuf = append(d.fuaBuf, payload[2:]...)

	if end {
		nalu := d.fuaBuf
		d.fuaBuf = nil
		d.fuaStarted = false
		return [][]byte{nalu}
	}

	return nil
}

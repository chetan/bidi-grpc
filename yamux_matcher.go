package bidigrpc

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
)

// constants and header type cribbed from yamux
// copied here so we can use them to properly read the header

const (
	// protoVersion is the only version we support
	protoVersion uint8 = 0
)

const (
	// Data is used for data frames. They are followed
	// by length bytes worth of payload.
	typeData uint8 = iota

	// WindowUpdate is used to change the window of
	// a given stream. The length indicates the delta
	// update to the window.
	typeWindowUpdate

	// Ping is sent as a keep-alive or to measure
	// the RTT. The StreamID and Length value are echoed
	// back in the response.
	typePing

	// GoAway is sent to terminate a session. The StreamID
	// should be 0 and the length is an error code.
	typeGoAway
)

const (
	sizeOfVersion  = 1
	sizeOfType     = 1
	sizeOfFlags    = 2
	sizeOfStreamID = 4
	sizeOfLength   = 4
	headerSize     = sizeOfVersion + sizeOfType + sizeOfFlags +
		sizeOfStreamID + sizeOfLength
)

// yamuxHeader frames every yamux session packet
type yamuxHeader []byte

func (h yamuxHeader) Version() uint8 {
	return h[0]
}

func (h yamuxHeader) MsgType() uint8 {
	return h[1]
}

func (h yamuxHeader) Flags() uint16 {
	return binary.BigEndian.Uint16(h[2:4])
}

func (h yamuxHeader) StreamID() uint32 {
	return binary.BigEndian.Uint32(h[4:8])
}

func (h yamuxHeader) Length() uint32 {
	return binary.BigEndian.Uint32(h[8:12])
}

func (h yamuxHeader) String() string {
	return fmt.Sprintf("Vsn:%d Type:%d Flags:%d StreamID:%d Length:%d",
		h.Version(), h.MsgType(), h.Flags(), h.StreamID(), h.Length())
}

// YamuxMatcher is a cmux.Matcher impl for detecting yamux sessions
func YamuxMatcher(r io.Reader) bool {
	hdr := yamuxHeader(make([]byte, headerSize))
	bufRead := bufio.NewReader(r)

	// Read the header
	if _, err := io.ReadFull(bufRead, hdr); err != nil {
		if err != io.EOF && !strings.Contains(err.Error(), "closed") && !strings.Contains(err.Error(), "reset by peer") {
			// s.logger.Printf("[ERR] yamux: Failed to read header: %v", err)
		}
		return false
	}

	// Verify the version
	if hdr.Version() != protoVersion {
		// s.logger.Printf("[ERR] yamux: Invalid protocol version: %d", hdr.Version())
		return false
	}

	// Switch on the type
	switch hdr.MsgType() {
	case typeData:
	case typeWindowUpdate:
	case typeGoAway:
	case typePing:
		return true
	default:
		// got some other type which is considered invalid
		return false
	}

	return true
}

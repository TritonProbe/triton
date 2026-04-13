package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/packet"
)

func EncodeFrames(frames []frame.Frame) ([]byte, error) {
	var buf bytes.Buffer
	for _, f := range frames {
		if err := f.Serialize(&buf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func BuildInitialPacket(version uint32, dcid, scid []byte, packetNumber uint64, frames []frame.Frame) ([]byte, error) {
	if len(dcid) > 255 || len(scid) > 255 {
		return nil, fmt.Errorf("connection id length exceeds 255 bytes")
	}
	payload, err := EncodeFrames(frames)
	if err != nil {
		return nil, err
	}
	pnLen := packetNumberLen(packetNumber)
	// #nosec G115 -- pnLen is bounded to 1..4 and packet type bits are constrained.
	first := byte(0xc0 | byte(packet.PacketTypeInitial<<4) | byte(pnLen-1))

	var buf bytes.Buffer
	buf.WriteByte(first)
	if err := binary.Write(&buf, binary.BigEndian, version); err != nil {
		return nil, err
	}
	// #nosec G115 -- connection ID length is bounded to <=255 above.
	buf.WriteByte(byte(len(dcid)))
	buf.Write(dcid)
	// #nosec G115 -- connection ID length is bounded to <=255 above.
	buf.WriteByte(byte(len(scid)))
	buf.Write(scid)
	tokenLen, _ := packet.EncodeVarInt(0)
	buf.Write(tokenLen)

	// #nosec G115 -- pnLen is bounded to 1..4 and len(payload) is non-negative.
	length := uint64(pnLen) + uint64(len(payload))
	lengthBuf, err := packet.EncodeVarInt(length)
	if err != nil {
		return nil, err
	}
	buf.Write(lengthBuf)
	writePacketNumber(&buf, packetNumber, pnLen)
	buf.Write(payload)
	return buf.Bytes(), nil
}

func BuildShortPacket(dcid []byte, packetNumber uint64, frames []frame.Frame) ([]byte, error) {
	if len(dcid) > 255 {
		return nil, fmt.Errorf("connection id length exceeds 255 bytes")
	}
	payload, err := EncodeFrames(frames)
	if err != nil {
		return nil, err
	}
	pnLen := packetNumberLen(packetNumber)
	// #nosec G115 -- pnLen is bounded to 1..4.
	first := byte(0x40 | byte(pnLen-1))
	var buf bytes.Buffer
	buf.WriteByte(first)
	buf.Write(dcid)
	writePacketNumber(&buf, packetNumber, pnLen)
	buf.Write(payload)
	return buf.Bytes(), nil
}

func ParsePacketFrames(data []byte, shortHeaderDCIDLen int) (packet.Header, []frame.Frame, error) {
	h, payload, err := packet.ParseHeader(data, shortHeaderDCIDLen)
	if err != nil {
		return nil, nil, err
	}
	frames, err := frame.ParseFrames(payload)
	if err != nil {
		return nil, nil, err
	}
	return h, frames, nil
}

func packetNumberLen(v uint64) int {
	switch {
	case v <= 0xff:
		return 1
	case v <= 0xffff:
		return 2
	case v <= 0xffffff:
		return 3
	default:
		return 4
	}
}

func writePacketNumber(buf *bytes.Buffer, packetNumber uint64, pnLen int) {
	for i := pnLen - 1; i >= 0; i-- {
		shift := uint(i * 8)
		// #nosec G115 -- only the low byte is written for each packet-number chunk.
		buf.WriteByte(byte(packetNumber >> shift))
	}
}

package wire

import (
	"bytes"
	"encoding/binary"

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
	payload, err := EncodeFrames(frames)
	if err != nil {
		return nil, err
	}
	pnLen := packetNumberLen(packetNumber)
	first := byte(0xc0 | byte(packet.PacketTypeInitial<<4) | byte(pnLen-1))

	var buf bytes.Buffer
	buf.WriteByte(first)
	if err := binary.Write(&buf, binary.BigEndian, version); err != nil {
		return nil, err
	}
	buf.WriteByte(byte(len(dcid)))
	buf.Write(dcid)
	buf.WriteByte(byte(len(scid)))
	buf.Write(scid)
	tokenLen, _ := packet.EncodeVarInt(0)
	buf.Write(tokenLen)

	length := uint64(pnLen + len(payload))
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
	payload, err := EncodeFrames(frames)
	if err != nil {
		return nil, err
	}
	pnLen := packetNumberLen(packetNumber)
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
		buf.WriteByte(byte(packetNumber >> shift))
	}
}

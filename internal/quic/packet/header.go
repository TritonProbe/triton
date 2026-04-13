package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

type PacketType uint8

const (
	PacketTypeInitial PacketType = iota
	PacketTypeZeroRTT
	PacketTypeHandshake
	PacketTypeRetry
	PacketTypeOneRTT
)

type Header interface {
	IsLongHeader() bool
	PacketType() PacketType
	Version() uint32
	DestConnectionID() []byte
	SrcConnectionID() []byte
	PacketNumberLength() int
}

type LongHeader struct {
	Type         PacketType
	VersionNum   uint32
	DCID         []byte
	SCID         []byte
	Token        []byte
	Length       uint64
	PacketNumber uint64
	PNLength     int
}

type ShortHeader struct {
	DCID         []byte
	PacketNumber uint64
	KeyPhase     bool
	SpinBit      bool
	PNLength     int
}

var errHeaderTooShort = errors.New("quic header: buffer too short")

func ParseHeader(data []byte, shortHeaderDCIDLen int) (Header, []byte, error) {
	if len(data) < 1 {
		return nil, nil, errHeaderTooShort
	}
	if data[0]&0x80 != 0 {
		return parseLongHeader(data)
	}
	return parseShortHeader(data, shortHeaderDCIDLen)
}

func (h *LongHeader) IsLongHeader() bool       { return true }
func (h *LongHeader) PacketType() PacketType   { return h.Type }
func (h *LongHeader) Version() uint32          { return h.VersionNum }
func (h *LongHeader) DestConnectionID() []byte { return append([]byte(nil), h.DCID...) }
func (h *LongHeader) SrcConnectionID() []byte  { return append([]byte(nil), h.SCID...) }
func (h *LongHeader) PacketNumberLength() int  { return h.PNLength }

func (h *ShortHeader) IsLongHeader() bool       { return false }
func (h *ShortHeader) PacketType() PacketType   { return PacketTypeOneRTT }
func (h *ShortHeader) Version() uint32          { return 0 }
func (h *ShortHeader) DestConnectionID() []byte { return append([]byte(nil), h.DCID...) }
func (h *ShortHeader) SrcConnectionID() []byte  { return nil }
func (h *ShortHeader) PacketNumberLength() int  { return h.PNLength }

func parseLongHeader(data []byte) (Header, []byte, error) {
	if len(data) < 7 {
		return nil, nil, errHeaderTooShort
	}
	first := data[0]
	version := binary.BigEndian.Uint32(data[1:5])
	idx := 5

	dcidLen := int(data[idx])
	idx++
	if len(data) < idx+dcidLen+1 {
		return nil, nil, errHeaderTooShort
	}
	dcid := append([]byte(nil), data[idx:idx+dcidLen]...)
	idx += dcidLen

	scidLen := int(data[idx])
	idx++
	if len(data) < idx+scidLen {
		return nil, nil, errHeaderTooShort
	}
	scid := append([]byte(nil), data[idx:idx+scidLen]...)
	idx += scidLen

	header := &LongHeader{
		Type:       PacketType((first >> 4) & 0x03),
		VersionNum: version,
		DCID:       dcid,
		SCID:       scid,
		PNLength:   int(first&0x03) + 1,
	}

	if header.Type == PacketTypeRetry {
		if len(data) < idx+16 {
			return nil, nil, errHeaderTooShort
		}
		header.Token = append([]byte(nil), data[idx:len(data)-16]...)
		return header, data[len(data)-16:], nil
	}

	if header.Type == PacketTypeInitial {
		tokenLen, consumed, err := ParseVarInt(data[idx:])
		if err != nil {
			return nil, nil, err
		}
		idx += consumed
		// #nosec G115 -- len(data)-idx and math.MaxInt bound the conversion before use.
		if tokenLen > uint64(len(data)-idx) || tokenLen > uint64(math.MaxInt) {
			return nil, nil, errHeaderTooShort
		}
		tokenLenInt := int(tokenLen)
		header.Token = append([]byte(nil), data[idx:idx+tokenLenInt]...)
		idx += tokenLenInt
	}

	length, consumed, err := ParseVarInt(data[idx:])
	if err != nil {
		return nil, nil, err
	}
	header.Length = length
	idx += consumed
	if len(data) < idx+header.PNLength {
		return nil, nil, errHeaderTooShort
	}
	header.PacketNumber = readPacketNumber(data[idx : idx+header.PNLength])
	idx += header.PNLength
	return header, data[idx:], nil
}

func parseShortHeader(data []byte, dcidLen int) (Header, []byte, error) {
	if dcidLen <= 0 {
		return nil, nil, fmt.Errorf("quic header: short header DCID length must be positive")
	}
	first := data[0]
	pnLen := int(first&0x03) + 1
	if len(data) < 1+dcidLen+pnLen {
		return nil, nil, errHeaderTooShort
	}
	idx := 1
	dcid := append([]byte(nil), data[idx:idx+dcidLen]...)
	idx += dcidLen
	pn := readPacketNumber(data[idx : idx+pnLen])
	idx += pnLen
	return &ShortHeader{
		DCID:         dcid,
		PacketNumber: pn,
		KeyPhase:     first&0x04 != 0,
		SpinBit:      first&0x20 != 0,
		PNLength:     pnLen,
	}, data[idx:], nil
}

func readPacketNumber(data []byte) uint64 {
	var out uint64
	for _, b := range data {
		out = (out << 8) | uint64(b)
	}
	return out
}

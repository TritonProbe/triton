package packet

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

var errVarIntTooShort = errors.New("quic varint: buffer too short")

func ReadVarInt(r io.Reader) (uint64, int, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, 0, err
	}
	length := 1 << (first[0] >> 6)
	buf := make([]byte, length)
	buf[0] = first[0]
	if length > 1 {
		if _, err := io.ReadFull(r, buf[1:]); err != nil {
			return 0, 1, err
		}
	}
	value, err := DecodeVarInt(buf)
	return value, length, err
}

func DecodeVarInt(buf []byte) (uint64, error) {
	value, _, err := ParseVarInt(buf)
	return value, err
}

func ParseVarInt(buf []byte) (uint64, int, error) {
	if len(buf) == 0 {
		return 0, 0, errVarIntTooShort
	}
	length := 1 << (buf[0] >> 6)
	if len(buf) < length {
		return 0, 0, errVarIntTooShort
	}
	switch length {
	case 1:
		return uint64(buf[0] & 0x3f), 1, nil
	case 2:
		return uint64(binary.BigEndian.Uint16(buf[:2]) & 0x3fff), 2, nil
	case 4:
		return uint64(binary.BigEndian.Uint32(buf[:4]) & 0x3fffffff), 4, nil
	case 8:
		return binary.BigEndian.Uint64(buf[:8]) & 0x3fffffffffffffff, 8, nil
	default:
		return 0, 0, fmt.Errorf("quic varint: invalid length %d", length)
	}
}

func WriteVarInt(w io.Writer, v uint64) (int, error) {
	buf, err := EncodeVarInt(v)
	if err != nil {
		return 0, err
	}
	n, err := w.Write(buf)
	return n, err
}

func EncodeVarInt(v uint64) ([]byte, error) {
	switch {
	case v <= 63:
		return []byte{byte(v)}, nil
	case v <= 16383:
		buf := make([]byte, 2)
		binary.BigEndian.PutUint16(buf, uint16(v)|0x4000)
		return buf, nil
	case v <= 1073741823:
		buf := make([]byte, 4)
		binary.BigEndian.PutUint32(buf, uint32(v)|0x80000000)
		return buf, nil
	case v <= 4611686018427387903:
		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, v|0xc000000000000000)
		return buf, nil
	default:
		return nil, fmt.Errorf("quic varint: value %d too large", v)
	}
}

func VarIntLen(v uint64) int {
	switch {
	case v <= 63:
		return 1
	case v <= 16383:
		return 2
	case v <= 1073741823:
		return 4
	default:
		return 8
	}
}

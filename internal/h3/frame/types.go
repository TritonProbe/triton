package frame

import (
	"bytes"
	"fmt"
	"io"

	quicpacket "github.com/tritonprobe/triton/internal/quic/packet"
)

type Type uint64

const (
	TypeData    Type = 0x00
	TypeHeaders Type = 0x01
)

type Frame interface {
	Type() Type
	Serialize(w io.Writer) error
}

type DataFrame struct {
	Data []byte
}

type HeadersFrame struct {
	Block []byte
}

func (f DataFrame) Type() Type    { return TypeData }
func (f HeadersFrame) Type() Type { return TypeHeaders }

func (f DataFrame) Serialize(w io.Writer) error {
	if _, err := quicpacket.WriteVarInt(w, uint64(TypeData)); err != nil {
		return err
	}
	if _, err := quicpacket.WriteVarInt(w, uint64(len(f.Data))); err != nil {
		return err
	}
	_, err := w.Write(f.Data)
	return err
}

func (f HeadersFrame) Serialize(w io.Writer) error {
	if _, err := quicpacket.WriteVarInt(w, uint64(TypeHeaders)); err != nil {
		return err
	}
	if _, err := quicpacket.WriteVarInt(w, uint64(len(f.Block))); err != nil {
		return err
	}
	_, err := w.Write(f.Block)
	return err
}

func Encode(frames []Frame) ([]byte, error) {
	var buf bytes.Buffer
	for _, f := range frames {
		if err := f.Serialize(&buf); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

func Parse(data []byte) ([]Frame, error) {
	out := make([]Frame, 0)
	for len(data) > 0 {
		frameType, consumed, err := quicpacket.ParseVarInt(data)
		if err != nil {
			return nil, err
		}
		data = data[consumed:]
		length, consumed, err := quicpacket.ParseVarInt(data)
		if err != nil {
			return nil, err
		}
		data = data[consumed:]
		if uint64(len(data)) < length {
			return nil, fmt.Errorf("h3 frame: truncated payload")
		}
		payload := append([]byte(nil), data[:length]...)
		data = data[length:]
		switch Type(frameType) {
		case TypeData:
			out = append(out, DataFrame{Data: payload})
		case TypeHeaders:
			out = append(out, HeadersFrame{Block: payload})
		default:
			return nil, fmt.Errorf("h3 frame: unsupported type 0x%x", frameType)
		}
	}
	return out, nil
}

package frame

import (
	"fmt"
	"io"

	"github.com/tritonprobe/triton/internal/quic/packet"
)

type FrameType uint64

const (
	FrameTypePadding            FrameType = 0x00
	FrameTypePing               FrameType = 0x01
	FrameTypeAck                FrameType = 0x02
	FrameTypeAckECN             FrameType = 0x03
	FrameTypeResetStream        FrameType = 0x04
	FrameTypeStopSending        FrameType = 0x05
	FrameTypeCrypto             FrameType = 0x06
	FrameTypeNewToken           FrameType = 0x07
	FrameTypeStreamBase         FrameType = 0x08
	FrameTypeMaxData            FrameType = 0x10
	FrameTypeMaxStreamData      FrameType = 0x11
	FrameTypeMaxStreamsBidi     FrameType = 0x12
	FrameTypeMaxStreamsUni      FrameType = 0x13
	FrameTypeDataBlocked        FrameType = 0x14
	FrameTypeStreamDataBlocked  FrameType = 0x15
	FrameTypeStreamsBlockedBidi FrameType = 0x16
	FrameTypeStreamsBlockedUni  FrameType = 0x17
	FrameTypeNewConnectionID    FrameType = 0x18
	FrameTypeRetireConnectionID FrameType = 0x19
	FrameTypePathChallenge      FrameType = 0x1a
	FrameTypePathResponse       FrameType = 0x1b
	FrameTypeConnectionClose    FrameType = 0x1c
	FrameTypeConnectionCloseApp FrameType = 0x1d
	FrameTypeHandshakeDone      FrameType = 0x1e
)

type Frame interface {
	Type() FrameType
	Length() int
	Serialize(w io.Writer) error
}

type PaddingFrame struct{ Count int }
type PingFrame struct{}
type ACKRange struct {
	Gap   uint64
	Range uint64
}
type ECNCounts struct {
	ECT0 uint64
	ECT1 uint64
	CE   uint64
}
type ACKFrame struct {
	LargestAcked  uint64
	ACKDelay      uint64
	ACKRangeCount uint64
	FirstACKRange uint64
	ACKRanges     []ACKRange
	ECNCounts     *ECNCounts
}
type ResetStreamFrame struct {
	StreamID  uint64
	ErrorCode uint64
	FinalSize uint64
}
type StopSendingFrame struct {
	StreamID  uint64
	ErrorCode uint64
}
type CryptoFrame struct {
	Offset uint64
	Data   []byte
}
type NewTokenFrame struct {
	Token []byte
}
type StreamFrame struct {
	StreamID uint64
	Offset   uint64
	LengthV  uint64
	Fin      bool
	Data     []byte
}
type MaxDataFrame struct {
	MaximumData uint64
}
type NewConnectionIDFrame struct {
	SequenceNumber      uint64
	RetirePriorTo       uint64
	ConnectionID        []byte
	StatelessResetToken [16]byte
}
type RetireConnectionIDFrame struct {
	SequenceNumber uint64
}
type PathChallengeFrame struct {
	Data [8]byte
}
type PathResponseFrame struct {
	Data [8]byte
}
type HandshakeDoneFrame struct{}

func (f PaddingFrame) Type() FrameType { return FrameTypePadding }
func (f PingFrame) Type() FrameType    { return FrameTypePing }
func (f ACKFrame) Type() FrameType {
	if f.ECNCounts != nil {
		return FrameTypeAckECN
	}
	return FrameTypeAck
}
func (f ResetStreamFrame) Type() FrameType        { return FrameTypeResetStream }
func (f StopSendingFrame) Type() FrameType        { return FrameTypeStopSending }
func (f CryptoFrame) Type() FrameType             { return FrameTypeCrypto }
func (f NewTokenFrame) Type() FrameType           { return FrameTypeNewToken }
func (f StreamFrame) Type() FrameType             { return FrameTypeStreamBase }
func (f MaxDataFrame) Type() FrameType            { return FrameTypeMaxData }
func (f NewConnectionIDFrame) Type() FrameType    { return FrameTypeNewConnectionID }
func (f RetireConnectionIDFrame) Type() FrameType { return FrameTypeRetireConnectionID }
func (f PathChallengeFrame) Type() FrameType      { return FrameTypePathChallenge }
func (f PathResponseFrame) Type() FrameType       { return FrameTypePathResponse }
func (f HandshakeDoneFrame) Type() FrameType      { return FrameTypeHandshakeDone }

func (f PaddingFrame) Length() int { return f.Count }
func (f PingFrame) Length() int    { return 1 }
func (f ACKFrame) Length() int {
	length := 1 + packet.VarIntLen(f.LargestAcked) + packet.VarIntLen(f.ACKDelay) +
		packet.VarIntLen(uint64(len(f.ACKRanges))) + packet.VarIntLen(f.FirstACKRange)
	for _, ackRange := range f.ACKRanges {
		length += packet.VarIntLen(ackRange.Gap) + packet.VarIntLen(ackRange.Range)
	}
	if f.ECNCounts != nil {
		length += packet.VarIntLen(f.ECNCounts.ECT0) + packet.VarIntLen(f.ECNCounts.ECT1) + packet.VarIntLen(f.ECNCounts.CE)
	}
	return length
}
func (f ResetStreamFrame) Length() int {
	return 1 + packet.VarIntLen(f.StreamID) + packet.VarIntLen(f.ErrorCode) + packet.VarIntLen(f.FinalSize)
}
func (f StopSendingFrame) Length() int {
	return 1 + packet.VarIntLen(f.StreamID) + packet.VarIntLen(f.ErrorCode)
}
func (f CryptoFrame) Length() int {
	return 1 + packet.VarIntLen(f.Offset) + packet.VarIntLen(uint64(len(f.Data))) + len(f.Data)
}
func (f NewTokenFrame) Length() int { return 1 + packet.VarIntLen(uint64(len(f.Token))) + len(f.Token) }
func (f StreamFrame) Length() int {
	length := 1 + packet.VarIntLen(f.StreamID)
	if f.Offset > 0 {
		length += packet.VarIntLen(f.Offset)
	}
	length += packet.VarIntLen(uint64(len(f.Data))) + len(f.Data)
	return length
}
func (f MaxDataFrame) Length() int { return 1 + packet.VarIntLen(f.MaximumData) }
func (f NewConnectionIDFrame) Length() int {
	return 1 + packet.VarIntLen(f.SequenceNumber) + packet.VarIntLen(f.RetirePriorTo) + 1 + len(f.ConnectionID) + 16
}
func (f RetireConnectionIDFrame) Length() int { return 1 + packet.VarIntLen(f.SequenceNumber) }
func (f PathChallengeFrame) Length() int      { return 9 }
func (f PathResponseFrame) Length() int       { return 9 }
func (f HandshakeDoneFrame) Length() int      { return 1 }

func (f PaddingFrame) Serialize(w io.Writer) error {
	if f.Count <= 0 {
		f.Count = 1
	}
	buf := make([]byte, f.Count)
	_, err := w.Write(buf)
	return err
}

func (f PingFrame) Serialize(w io.Writer) error {
	_, err := w.Write([]byte{byte(FrameTypePing)})
	return err
}

func (f ACKFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(f.Type())); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.LargestAcked); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.ACKDelay); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, uint64(len(f.ACKRanges))); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.FirstACKRange); err != nil {
		return err
	}
	for _, ackRange := range f.ACKRanges {
		if _, err := packet.WriteVarInt(w, ackRange.Gap); err != nil {
			return err
		}
		if _, err := packet.WriteVarInt(w, ackRange.Range); err != nil {
			return err
		}
	}
	if f.ECNCounts != nil {
		if _, err := packet.WriteVarInt(w, f.ECNCounts.ECT0); err != nil {
			return err
		}
		if _, err := packet.WriteVarInt(w, f.ECNCounts.ECT1); err != nil {
			return err
		}
		if _, err := packet.WriteVarInt(w, f.ECNCounts.CE); err != nil {
			return err
		}
	}
	return nil
}

func (f ResetStreamFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeResetStream)); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.StreamID); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.ErrorCode); err != nil {
		return err
	}
	_, err := packet.WriteVarInt(w, f.FinalSize)
	return err
}

func (f StopSendingFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeStopSending)); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.StreamID); err != nil {
		return err
	}
	_, err := packet.WriteVarInt(w, f.ErrorCode)
	return err
}

func (f CryptoFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeCrypto)); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.Offset); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, uint64(len(f.Data))); err != nil {
		return err
	}
	_, err := w.Write(f.Data)
	return err
}

func (f NewTokenFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeNewToken)); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, uint64(len(f.Token))); err != nil {
		return err
	}
	_, err := w.Write(f.Token)
	return err
}

func (f StreamFrame) Serialize(w io.Writer) error {
	frameType := uint64(FrameTypeStreamBase) | 0x02
	if f.Offset > 0 {
		frameType |= 0x04
	}
	if f.Fin {
		frameType |= 0x01
	}
	if _, err := packet.WriteVarInt(w, frameType); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.StreamID); err != nil {
		return err
	}
	if f.Offset > 0 {
		if _, err := packet.WriteVarInt(w, f.Offset); err != nil {
			return err
		}
	}
	if _, err := packet.WriteVarInt(w, uint64(len(f.Data))); err != nil {
		return err
	}
	_, err := w.Write(f.Data)
	return err
}

func (f MaxDataFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeMaxData)); err != nil {
		return err
	}
	_, err := packet.WriteVarInt(w, f.MaximumData)
	return err
}

func (f NewConnectionIDFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeNewConnectionID)); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.SequenceNumber); err != nil {
		return err
	}
	if _, err := packet.WriteVarInt(w, f.RetirePriorTo); err != nil {
		return err
	}
	if _, err := w.Write([]byte{byte(len(f.ConnectionID))}); err != nil {
		return err
	}
	if _, err := w.Write(f.ConnectionID); err != nil {
		return err
	}
	_, err := w.Write(f.StatelessResetToken[:])
	return err
}

func (f RetireConnectionIDFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypeRetireConnectionID)); err != nil {
		return err
	}
	_, err := packet.WriteVarInt(w, f.SequenceNumber)
	return err
}

func (f PathChallengeFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypePathChallenge)); err != nil {
		return err
	}
	_, err := w.Write(f.Data[:])
	return err
}

func (f PathResponseFrame) Serialize(w io.Writer) error {
	if _, err := packet.WriteVarInt(w, uint64(FrameTypePathResponse)); err != nil {
		return err
	}
	_, err := w.Write(f.Data[:])
	return err
}

func (f HandshakeDoneFrame) Serialize(w io.Writer) error {
	_, err := packet.WriteVarInt(w, uint64(FrameTypeHandshakeDone))
	return err
}

func ParseFrames(data []byte) ([]Frame, error) {
	frames := make([]Frame, 0)
	for len(data) > 0 {
		frameType, consumed, err := packet.ParseVarInt(data)
		if err != nil {
			return nil, err
		}
		if frameType == uint64(FrameTypePadding) {
			count := 0
			for len(data) > 0 && data[0] == 0x00 {
				count++
				data = data[1:]
			}
			frames = append(frames, PaddingFrame{Count: count})
			continue
		}
		data = data[consumed:]
		switch {
		case frameType == uint64(FrameTypePing):
			frames = append(frames, PingFrame{})
		case frameType == uint64(FrameTypeAck), frameType == uint64(FrameTypeAckECN):
			frame, rest, err := parseACKFrame(frameType == uint64(FrameTypeAckECN), data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeResetStream):
			frame, rest, err := parseResetStreamFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeStopSending):
			frame, rest, err := parseStopSendingFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeCrypto):
			frame, rest, err := parseCryptoFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeNewToken):
			frame, rest, err := parseNewTokenFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType >= 0x08 && frameType <= 0x0f:
			frame, rest, err := parseStreamFrame(frameType, data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeMaxData):
			frame, rest, err := parseMaxDataFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeNewConnectionID):
			frame, rest, err := parseNewConnectionIDFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeRetireConnectionID):
			frame, rest, err := parseRetireConnectionIDFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypePathChallenge):
			frame, rest, err := parsePathChallengeFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypePathResponse):
			frame, rest, err := parsePathResponseFrame(data)
			if err != nil {
				return nil, err
			}
			frames = append(frames, frame)
			data = rest
		case frameType == uint64(FrameTypeHandshakeDone):
			frames = append(frames, HandshakeDoneFrame{})
		default:
			return nil, fmt.Errorf("quic frame: unsupported type 0x%x", frameType)
		}
	}
	return frames, nil
}

func parseACKFrame(withECN bool, data []byte) (ACKFrame, []byte, error) {
	largestAcked, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ACKFrame{}, nil, err
	}
	data = data[consumed:]
	ackDelay, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ACKFrame{}, nil, err
	}
	data = data[consumed:]
	rangeCount, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ACKFrame{}, nil, err
	}
	data = data[consumed:]
	firstRange, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ACKFrame{}, nil, err
	}
	data = data[consumed:]
	frame := ACKFrame{
		LargestAcked:  largestAcked,
		ACKDelay:      ackDelay,
		ACKRangeCount: rangeCount,
		FirstACKRange: firstRange,
	}
	for i := uint64(0); i < rangeCount; i++ {
		gap, used, err := packet.ParseVarInt(data)
		if err != nil {
			return ACKFrame{}, nil, err
		}
		data = data[used:]
		rng, used, err := packet.ParseVarInt(data)
		if err != nil {
			return ACKFrame{}, nil, err
		}
		data = data[used:]
		frame.ACKRanges = append(frame.ACKRanges, ACKRange{Gap: gap, Range: rng})
	}
	if withECN {
		ect0, used, err := packet.ParseVarInt(data)
		if err != nil {
			return ACKFrame{}, nil, err
		}
		data = data[used:]
		ect1, used, err := packet.ParseVarInt(data)
		if err != nil {
			return ACKFrame{}, nil, err
		}
		data = data[used:]
		ce, used, err := packet.ParseVarInt(data)
		if err != nil {
			return ACKFrame{}, nil, err
		}
		data = data[used:]
		frame.ECNCounts = &ECNCounts{ECT0: ect0, ECT1: ect1, CE: ce}
	}
	return frame, data, nil
}

func parseResetStreamFrame(data []byte) (ResetStreamFrame, []byte, error) {
	streamID, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ResetStreamFrame{}, nil, err
	}
	data = data[consumed:]
	errorCode, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ResetStreamFrame{}, nil, err
	}
	data = data[consumed:]
	finalSize, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return ResetStreamFrame{}, nil, err
	}
	return ResetStreamFrame{StreamID: streamID, ErrorCode: errorCode, FinalSize: finalSize}, data[consumed:], nil
}

func parseStopSendingFrame(data []byte) (StopSendingFrame, []byte, error) {
	streamID, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return StopSendingFrame{}, nil, err
	}
	data = data[consumed:]
	errorCode, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return StopSendingFrame{}, nil, err
	}
	return StopSendingFrame{StreamID: streamID, ErrorCode: errorCode}, data[consumed:], nil
}

func parseCryptoFrame(data []byte) (CryptoFrame, []byte, error) {
	offset, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return CryptoFrame{}, nil, err
	}
	data = data[consumed:]
	length, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return CryptoFrame{}, nil, err
	}
	data = data[consumed:]
	if uint64(len(data)) < length {
		return CryptoFrame{}, nil, fmt.Errorf("quic frame: crypto payload truncated")
	}
	frame := CryptoFrame{
		Offset: offset,
		Data:   append([]byte(nil), data[:length]...),
	}
	return frame, data[length:], nil
}

func parseNewTokenFrame(data []byte) (NewTokenFrame, []byte, error) {
	length, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return NewTokenFrame{}, nil, err
	}
	data = data[consumed:]
	if uint64(len(data)) < length {
		return NewTokenFrame{}, nil, fmt.Errorf("quic frame: new token payload truncated")
	}
	return NewTokenFrame{Token: append([]byte(nil), data[:length]...)}, data[length:], nil
}

func parseStreamFrame(frameType uint64, data []byte) (StreamFrame, []byte, error) {
	streamID, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return StreamFrame{}, nil, err
	}
	data = data[consumed:]
	var offset uint64
	if frameType&0x04 != 0 {
		offset, consumed, err = packet.ParseVarInt(data)
		if err != nil {
			return StreamFrame{}, nil, err
		}
		data = data[consumed:]
	}
	var length uint64
	if frameType&0x02 != 0 {
		length, consumed, err = packet.ParseVarInt(data)
		if err != nil {
			return StreamFrame{}, nil, err
		}
		data = data[consumed:]
	} else {
		length = uint64(len(data))
	}
	if uint64(len(data)) < length {
		return StreamFrame{}, nil, fmt.Errorf("quic frame: stream payload truncated")
	}
	frame := StreamFrame{
		StreamID: streamID,
		Offset:   offset,
		LengthV:  length,
		Fin:      frameType&0x01 != 0,
		Data:     append([]byte(nil), data[:length]...),
	}
	return frame, data[length:], nil
}

func parseMaxDataFrame(data []byte) (MaxDataFrame, []byte, error) {
	maximumData, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return MaxDataFrame{}, nil, err
	}
	return MaxDataFrame{MaximumData: maximumData}, data[consumed:], nil
}

func parseNewConnectionIDFrame(data []byte) (NewConnectionIDFrame, []byte, error) {
	seq, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return NewConnectionIDFrame{}, nil, err
	}
	data = data[consumed:]
	retire, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return NewConnectionIDFrame{}, nil, err
	}
	data = data[consumed:]
	if len(data) < 1 {
		return NewConnectionIDFrame{}, nil, fmt.Errorf("quic frame: new connection id missing length")
	}
	cidLen := int(data[0])
	data = data[1:]
	if len(data) < cidLen+16 {
		return NewConnectionIDFrame{}, nil, fmt.Errorf("quic frame: new connection id truncated")
	}
	frame := NewConnectionIDFrame{
		SequenceNumber: seq,
		RetirePriorTo:  retire,
		ConnectionID:   append([]byte(nil), data[:cidLen]...),
	}
	copy(frame.StatelessResetToken[:], data[cidLen:cidLen+16])
	return frame, data[cidLen+16:], nil
}

func parseRetireConnectionIDFrame(data []byte) (RetireConnectionIDFrame, []byte, error) {
	seq, consumed, err := packet.ParseVarInt(data)
	if err != nil {
		return RetireConnectionIDFrame{}, nil, err
	}
	return RetireConnectionIDFrame{SequenceNumber: seq}, data[consumed:], nil
}

func parsePathChallengeFrame(data []byte) (PathChallengeFrame, []byte, error) {
	if len(data) < 8 {
		return PathChallengeFrame{}, nil, fmt.Errorf("quic frame: path challenge truncated")
	}
	var frame PathChallengeFrame
	copy(frame.Data[:], data[:8])
	return frame, data[8:], nil
}

func parsePathResponseFrame(data []byte) (PathResponseFrame, []byte, error) {
	if len(data) < 8 {
		return PathResponseFrame{}, nil, fmt.Errorf("quic frame: path response truncated")
	}
	var frame PathResponseFrame
	copy(frame.Data[:], data[:8])
	return frame, data[8:], nil
}

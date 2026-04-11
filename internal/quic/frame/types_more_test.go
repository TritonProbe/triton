package frame

import (
	"bytes"
	"strings"
	"testing"
)

func TestSerializeAndParseAdditionalFrames(t *testing.T) {
	var token [16]byte
	copy(token[:], []byte("0123456789abcdef"))
	var path [8]byte
	copy(path[:], []byte("12345678"))

	frames := []Frame{
		PaddingFrame{},
		ACKFrame{
			LargestAcked:  7,
			ACKDelay:      1,
			ACKRangeCount: 0,
			FirstACKRange: 1,
			ECNCounts:     &ECNCounts{ECT0: 1, ECT1: 2, CE: 3},
		},
		ResetStreamFrame{StreamID: 4, ErrorCode: 9, FinalSize: 12},
		StopSendingFrame{StreamID: 4, ErrorCode: 9},
		NewConnectionIDFrame{SequenceNumber: 1, RetirePriorTo: 0, ConnectionID: []byte{0xaa, 0xbb}, StatelessResetToken: token},
		RetireConnectionIDFrame{SequenceNumber: 1},
		PathResponseFrame{Data: path},
	}

	var buf bytes.Buffer
	for _, f := range frames {
		if err := f.Serialize(&buf); err != nil {
			t.Fatalf("Serialize(%T) returned error: %v", f, err)
		}
	}

	parsed, err := ParseFrames(buf.Bytes())
	if err != nil {
		t.Fatalf("ParseFrames returned error: %v", err)
	}
	if len(parsed) != len(frames) {
		t.Fatalf("unexpected parsed frame count: got %d want %d", len(parsed), len(frames))
	}
	if _, ok := parsed[0].(PaddingFrame); !ok {
		t.Fatalf("expected padding frame, got %T", parsed[0])
	}
	if ack, ok := parsed[1].(ACKFrame); !ok || ack.ECNCounts == nil || ack.ECNCounts.CE != 3 {
		t.Fatalf("unexpected ACK frame: %#v", parsed[1])
	}
	if _, ok := parsed[2].(ResetStreamFrame); !ok {
		t.Fatalf("expected reset stream frame, got %T", parsed[2])
	}
	if _, ok := parsed[3].(StopSendingFrame); !ok {
		t.Fatalf("expected stop sending frame, got %T", parsed[3])
	}
	if cid, ok := parsed[4].(NewConnectionIDFrame); !ok || len(cid.ConnectionID) != 2 {
		t.Fatalf("unexpected new connection id frame: %#v", parsed[4])
	}
	if _, ok := parsed[5].(RetireConnectionIDFrame); !ok {
		t.Fatalf("expected retire connection id frame, got %T", parsed[5])
	}
	if _, ok := parsed[6].(PathResponseFrame); !ok {
		t.Fatalf("expected path response frame, got %T", parsed[6])
	}
}

func TestParseFramesErrors(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{name: "unsupported", data: []byte{0x20}, want: "unsupported type"},
		{name: "crypto truncated", data: []byte{0x06, 0x00, 0x05, 'a'}, want: "crypto payload truncated"},
		{name: "token truncated", data: []byte{0x07, 0x05, 'a'}, want: "new token payload truncated"},
		{name: "stream truncated", data: []byte{0x0a, 0x00, 0x05, 'a'}, want: "stream payload truncated"},
		{name: "new cid missing length", data: []byte{0x18, 0x00, 0x00}, want: "missing length"},
		{name: "path challenge truncated", data: []byte{0x1a, 0x01}, want: "path challenge truncated"},
		{name: "path response truncated", data: []byte{0x1b, 0x01}, want: "path response truncated"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFrames(tc.data)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected error containing %q, got %v", tc.want, err)
			}
		})
	}
}

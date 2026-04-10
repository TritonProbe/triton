package frame

import (
	"bytes"
	"testing"
)

func TestParseFrames(t *testing.T) {
	var buf bytes.Buffer
	if err := (PingFrame{}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (ACKFrame{
		LargestAcked:  10,
		ACKDelay:      1,
		ACKRangeCount: 1,
		FirstACKRange: 3,
		ACKRanges:     []ACKRange{{Gap: 0, Range: 2}},
	}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (CryptoFrame{Offset: 5, Data: []byte("hello")}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (NewTokenFrame{Token: []byte("token")}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (StreamFrame{StreamID: 3, Offset: 7, Fin: true, Data: []byte("world")}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (MaxDataFrame{MaximumData: 4096}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	var challenge [8]byte
	copy(challenge[:], []byte("12345678"))
	if err := (PathChallengeFrame{Data: challenge}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}
	if err := (HandshakeDoneFrame{}).Serialize(&buf); err != nil {
		t.Fatal(err)
	}

	frames, err := ParseFrames(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 8 {
		t.Fatalf("unexpected frame count: %d", len(frames))
	}
	if _, ok := frames[0].(PingFrame); !ok {
		t.Fatalf("unexpected first frame: %T", frames[0])
	}
	ack, ok := frames[1].(ACKFrame)
	if !ok || ack.LargestAcked != 10 || len(ack.ACKRanges) != 1 {
		t.Fatalf("unexpected ack frame: %#v", frames[1])
	}
	crypto, ok := frames[2].(CryptoFrame)
	if !ok || string(crypto.Data) != "hello" {
		t.Fatalf("unexpected crypto frame: %#v", frames[2])
	}
	token, ok := frames[3].(NewTokenFrame)
	if !ok || string(token.Token) != "token" {
		t.Fatalf("unexpected token frame: %#v", frames[3])
	}
	stream, ok := frames[4].(StreamFrame)
	if !ok || stream.StreamID != 3 || string(stream.Data) != "world" || !stream.Fin {
		t.Fatalf("unexpected stream frame: %#v", frames[4])
	}
	maxData, ok := frames[5].(MaxDataFrame)
	if !ok || maxData.MaximumData != 4096 {
		t.Fatalf("unexpected max data frame: %#v", frames[5])
	}
	path, ok := frames[6].(PathChallengeFrame)
	if !ok || string(path.Data[:]) != "12345678" {
		t.Fatalf("unexpected path challenge frame: %#v", frames[6])
	}
	if _, ok := frames[7].(HandshakeDoneFrame); !ok {
		t.Fatalf("unexpected final frame: %#v", frames[7])
	}
}

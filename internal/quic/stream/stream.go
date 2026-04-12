package stream

import (
	"errors"
	"io"
	"sort"
	"sync"
)

type State int

const (
	StateIdle State = iota
	StateOpen
	StateHalfClosedLocal
	StateHalfClosedRemote
	StateClosed
)

type Stream struct {
	id            uint64
	stateMu       sync.Mutex
	state         State
	sendMu        sync.Mutex
	recvMu        sync.Mutex
	sendBuf       []byte
	sendOffset    uint64
	sendFin       bool
	maxSendData   uint64
	recvBuf       *recvBuffer
	recvOffset    uint64
	recvFin       bool
	recvFinOffset uint64
	maxRecvData   uint64
	flowUpdate    chan struct{}
}

func New(id uint64, maxSendData, maxRecvData uint64) *Stream {
	return &Stream{
		id:          id,
		state:       StateOpen,
		maxSendData: maxSendData,
		maxRecvData: maxRecvData,
		recvBuf:     newRecvBuffer(),
		flowUpdate:  make(chan struct{}, 1),
	}
}

func (s *Stream) ID() uint64 { return s.id }

func (s *Stream) MaxSendData() uint64 {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.maxSendData
}

func (s *Stream) SetMaxSendData(v uint64) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.maxSendData = v
}

func (s *Stream) BufferedSendBytes() int {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return len(s.sendBuf)
}

func (s *Stream) State() State {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.state
}

func (s *Stream) Read(p []byte) (int, error) {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	n, _ := s.recvBuf.ReadAtOffset(s.recvOffset, p)
	s.recvOffset += uint64(n)
	if n > 0 {
		if s.recvFin && s.recvOffset >= s.recvFinOffset {
			s.transitionAfterReadEOF()
			return n, io.EOF
		}
		return n, nil
	}
	if s.recvFin && s.recvOffset >= s.recvFinOffset {
		s.transitionAfterReadEOF()
		return 0, io.EOF
	}
	return 0, nil
}

func (s *Stream) Write(p []byte) (int, error) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()

	if s.sendFin {
		return 0, errors.New("stream write after fin")
	}
	if s.maxSendData > 0 && s.sendOffset+uint64(len(p)) > s.maxSendData {
		return 0, errors.New("stream flow control exceeded")
	}
	s.sendBuf = append(s.sendBuf, p...)
	s.sendOffset += uint64(len(p))
	return len(p), nil
}

func (s *Stream) CloseWrite() error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	if s.sendFin {
		return nil
	}
	s.sendFin = true
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	switch s.state {
	case StateOpen:
		s.state = StateHalfClosedLocal
	case StateHalfClosedRemote:
		s.state = StateClosed
	}
	return nil
}

func (s *Stream) Close() error {
	s.sendMu.Lock()
	if !s.sendFin {
		s.sendFin = true
	}
	s.recvMu.Lock()
	defer s.sendMu.Unlock()
	defer s.recvMu.Unlock()
	s.recvFin = true
	s.recvFinOffset = s.recvOffset
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = StateClosed
	return nil
}

func (s *Stream) Reset(_ uint64) {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.recvMu.Lock()
	defer s.recvMu.Unlock()
	s.sendFin = true
	s.recvFin = true
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	s.state = StateClosed
}

func (s *Stream) SendBuffer() []byte {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return append([]byte(nil), s.sendBuf...)
}

func (s *Stream) PushRecv(offset uint64, data []byte, fin bool) error {
	s.recvMu.Lock()
	defer s.recvMu.Unlock()

	if s.maxRecvData > 0 && offset+uint64(len(data)) > s.maxRecvData {
		return errors.New("stream receive flow control exceeded")
	}
	if err := s.recvBuf.Insert(offset, data); err != nil {
		return err
	}
	if fin {
		s.recvFin = true
		s.recvFinOffset = offset + uint64(len(data))
		s.stateMu.Lock()
		if s.state == StateOpen {
			s.state = StateHalfClosedRemote
		} else if s.state == StateHalfClosedLocal {
			s.state = StateClosed
		}
		s.stateMu.Unlock()
	}
	select {
	case s.flowUpdate <- struct{}{}:
	default:
	}
	return nil
}

func (s *Stream) transitionAfterReadEOF() {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	switch s.state {
	case StateHalfClosedRemote:
		s.state = StateClosed
	case StateOpen:
		s.state = StateHalfClosedRemote
	}
}

type recvBuffer struct {
	chunks []dataChunk
}

type dataChunk struct {
	offset uint64
	data   []byte
}

func newRecvBuffer() *recvBuffer {
	return &recvBuffer{chunks: make([]dataChunk, 0)}
}

func (r *recvBuffer) Insert(offset uint64, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	chunk := dataChunk{offset: offset, data: append([]byte(nil), data...)}
	r.chunks = append(r.chunks, chunk)
	sort.Slice(r.chunks, func(i, j int) bool { return r.chunks[i].offset < r.chunks[j].offset })
	r.merge()
	return nil
}

func (r *recvBuffer) merge() {
	if len(r.chunks) < 2 {
		return
	}
	merged := make([]dataChunk, 0, len(r.chunks))
	current := r.chunks[0]
	for _, next := range r.chunks[1:] {
		currentEnd := current.offset + uint64(len(current.data))
		if next.offset <= currentEnd {
			overlap := int(currentEnd - next.offset)
			if overlap < len(next.data) {
				current.data = append(current.data, next.data[overlap:]...)
			}
			continue
		}
		merged = append(merged, current)
		current = next
	}
	merged = append(merged, current)
	r.chunks = merged
}

func (r *recvBuffer) ReadAtOffset(offset uint64, p []byte) (int, error) {
	if len(r.chunks) == 0 || len(p) == 0 {
		return 0, nil
	}
	first := r.chunks[0]
	if first.offset > offset {
		return 0, nil
	}
	start := int(offset - first.offset)
	if start >= len(first.data) {
		r.chunks = r.chunks[1:]
		return r.ReadAtOffset(offset, p)
	}
	n := copy(p, first.data[start:])
	if start+n >= len(first.data) {
		r.chunks = r.chunks[1:]
	} else {
		first.offset += uint64(start + n)
		first.data = first.data[start+n:]
		r.chunks[0] = first
	}
	return n, nil
}

func (r *recvBuffer) Readable(offset uint64) int {
	if len(r.chunks) == 0 {
		return 0
	}
	first := r.chunks[0]
	if first.offset > offset {
		return 0
	}
	return len(first.data) - int(offset-first.offset)
}

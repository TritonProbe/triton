package stream

import (
	"errors"
	"sync"
)

type Role int

const (
	RoleClient Role = iota
	RoleServer
)

type Manager struct {
	mu             sync.Mutex
	streams        map[uint64]*Stream
	acceptQueue    chan *Stream
	nextBidiLocal  uint64
	nextBidiRemote uint64
	nextUniLocal   uint64
	nextUniRemote  uint64
	maxBidiLocal   uint64
	maxBidiRemote  uint64
	maxUniLocal    uint64
	maxUniRemote   uint64
	role           Role
}

func NewManager(role Role) *Manager {
	m := &Manager{
		streams:       make(map[uint64]*Stream),
		acceptQueue:   make(chan *Stream, 128),
		role:          role,
		maxBidiLocal:  100,
		maxBidiRemote: 100,
		maxUniLocal:   100,
		maxUniRemote:  100,
	}
	if role == RoleClient {
		m.nextBidiLocal = 0
		m.nextBidiRemote = 1
		m.nextUniLocal = 2
		m.nextUniRemote = 3
	} else {
		m.nextBidiLocal = 1
		m.nextBidiRemote = 0
		m.nextUniLocal = 3
		m.nextUniRemote = 2
	}
	return m
}

func (m *Manager) OpenStream(bidi bool) (*Stream, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var id uint64
	if bidi {
		if uint64(len(m.streams)) >= m.maxBidiLocal {
			return nil, errors.New("max bidi streams reached")
		}
		id = m.nextBidiLocal
		m.nextBidiLocal += 4
	} else {
		if uint64(len(m.streams)) >= m.maxUniLocal {
			return nil, errors.New("max uni streams reached")
		}
		id = m.nextUniLocal
		m.nextUniLocal += 4
	}
	s := New(id, 1<<20, 1<<20)
	m.streams[id] = s
	return s, nil
}

func (m *Manager) AcceptStream() (*Stream, error) {
	s, ok := <-m.acceptQueue
	if !ok {
		return nil, errors.New("stream manager closed")
	}
	return s, nil
}

func (m *Manager) GetStream(id uint64) (*Stream, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.streams[id]
	return s, ok
}

func (m *Manager) CloseStream(id uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.streams, id)
}

func (m *Manager) GetOrCreateRemoteStream(id uint64) (*Stream, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.streams[id]; ok {
		return s, false
	}
	s := New(id, 1<<20, 1<<20)
	m.streams[id] = s
	select {
	case m.acceptQueue <- s:
	default:
	}
	return s, true
}

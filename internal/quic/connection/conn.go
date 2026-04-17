package connection

import (
	"errors"
	"sync"
	"time"

	"github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/stream"
)

type State int

const (
	StateIdle State = iota
	StateInitialSent
	StateInitialReceived
	StateHandshake
	StateConnected
	StateDraining
	StateClosed
)

type Role int

const (
	RoleClient Role = iota
	RoleServer
)

type Connection struct {
	mu             sync.Mutex
	state          State
	remoteCIDs     *CIDManager
	originalDCID   []byte
	streamManager  *stream.Manager
	localMaxData   uint64
	remoteMaxData  uint64
	dataSent       uint64
	dataReceived   uint64
	handshakeDone  chan struct{}
	idleTimeout    time.Duration
	lastActivity   time.Time
	closeErr       error
	pathChallenges map[[8]byte]time.Time
}

func New(role Role, originalDCID []byte) *Connection {
	streamRole := stream.RoleClient
	if role == RoleServer {
		streamRole = stream.RoleServer
	}
	return &Connection{
		state:          StateIdle,
		remoteCIDs:     NewCIDManager(),
		originalDCID:   append([]byte(nil), originalDCID...),
		streamManager:  NewStreamManager(streamRole),
		localMaxData:   1 << 20,
		remoteMaxData:  1 << 20,
		handshakeDone:  make(chan struct{}),
		idleTimeout:    30 * time.Second,
		lastActivity:   time.Now(),
		pathChallenges: make(map[[8]byte]time.Time),
	}
}

func NewStreamManager(role stream.Role) *stream.Manager {
	return stream.NewManager(role)
}

func (c *Connection) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Connection) Streams() *stream.Manager { return c.streamManager }

func (c *Connection) OriginalDCID() []byte {
	return append([]byte(nil), c.originalDCID...)
}

func (c *Connection) Transition(next State) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == StateClosed {
		return errors.New("connection is closed")
	}
	switch next {
	case StateInitialSent, StateInitialReceived:
		if c.state != StateIdle {
			return errors.New("invalid initial transition")
		}
	case StateHandshake:
		if c.state != StateInitialSent && c.state != StateInitialReceived {
			return errors.New("invalid handshake transition")
		}
	case StateConnected:
		if c.state != StateHandshake {
			return errors.New("invalid connected transition")
		}
		select {
		case <-c.handshakeDone:
		default:
			close(c.handshakeDone)
		}
	case StateDraining:
		if c.state != StateConnected && c.state != StateHandshake {
			return errors.New("invalid draining transition")
		}
	case StateClosed:
	default:
		return errors.New("unknown state transition")
	}
	c.state = next
	c.lastActivity = time.Now()
	return nil
}

func (c *Connection) RecordSend(n uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dataSent+n > c.remoteMaxData {
		return errors.New("connection send flow control exceeded")
	}
	c.dataSent += n
	c.lastActivity = time.Now()
	return nil
}

func (c *Connection) RecordReceive(n uint64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dataReceived+n > c.localMaxData {
		return errors.New("connection receive flow control exceeded")
	}
	c.dataReceived += n
	c.lastActivity = time.Now()
	return nil
}

func (c *Connection) Touch() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastActivity = time.Now()
}

func (c *Connection) IdleExpired(now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return now.Sub(c.lastActivity) > c.idleTimeout
}

func (c *Connection) CloseWithError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeErr = err
	c.state = StateClosed
}

func (c *Connection) CloseError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeErr
}

func (c *Connection) HandleFrames(frames []frame.Frame) error {
	for _, f := range frames {
		if err := c.handleFrame(f); err != nil {
			return err
		}
	}
	c.Touch()
	return nil
}

func (c *Connection) handleFrame(f frame.Frame) error {
	switch ft := f.(type) {
	case frame.PaddingFrame, frame.PingFrame:
		return nil
	case frame.ACKFrame:
		return nil
	case frame.CryptoFrame:
		return c.RecordReceive(uint64(len(ft.Data)))
	case frame.StreamFrame:
		return c.handleStreamFrame(ft)
	case frame.MaxDataFrame:
		c.mu.Lock()
		c.remoteMaxData = ft.MaximumData
		c.mu.Unlock()
		return nil
	case frame.ResetStreamFrame:
		return c.handleResetStream(ft)
	case frame.StopSendingFrame:
		return c.handleStopSending(ft)
	case frame.NewConnectionIDFrame:
		c.remoteCIDs.Store(ft.ConnectionID)
		return nil
	case frame.RetireConnectionIDFrame:
		return nil
	case frame.PathChallengeFrame:
		c.mu.Lock()
		c.pathChallenges[ft.Data] = time.Now()
		c.mu.Unlock()
		return nil
	case frame.PathResponseFrame:
		c.mu.Lock()
		delete(c.pathChallenges, ft.Data)
		c.mu.Unlock()
		return nil
	case frame.HandshakeDoneFrame:
		if c.State() == StateHandshake {
			return c.Transition(StateConnected)
		}
		return nil
	case frame.NewTokenFrame:
		return nil
	default:
		return errors.New("unsupported frame dispatch")
	}
}

func (c *Connection) handleStreamFrame(f frame.StreamFrame) error {
	s, _ := c.streamManager.GetOrCreateRemoteStream(f.StreamID)
	if err := s.PushRecv(f.Offset, f.Data, f.Fin); err != nil {
		return err
	}
	return c.RecordReceive(uint64(len(f.Data)))
}

func (c *Connection) handleResetStream(f frame.ResetStreamFrame) error {
	s, ok := c.streamManager.GetStream(f.StreamID)
	if !ok {
		return nil
	}
	s.Reset(f.ErrorCode)
	c.streamManager.CloseStream(f.StreamID)
	return nil
}

func (c *Connection) handleStopSending(f frame.StopSendingFrame) error {
	s, ok := c.streamManager.GetStream(f.StreamID)
	if !ok {
		return nil
	}
	s.Reset(f.ErrorCode)
	return nil
}

func (c *Connection) PendingPathChallenges() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pathChallenges)
}

func (c *Connection) StoreRemoteCID(cid []byte) {
	c.remoteCIDs.Store(cid)
}

func (c *Connection) PrimaryRemoteCID() ([]byte, bool) {
	return c.remoteCIDs.First()
}

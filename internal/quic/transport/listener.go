package transport

import (
	"encoding/hex"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/tritonprobe/triton/internal/quic/connection"
	"github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/packet"
	"github.com/tritonprobe/triton/internal/quic/wire"
)

type Listener struct {
	transport *UDPTransport
	mu        sync.Mutex
	conns     map[string]*connection.Connection
	nextPN    map[string]uint64
	addrs     map[string]*net.UDPAddr
	autoEcho  bool
	acceptCh  chan *connection.Connection
	closed    chan struct{}
}

func ListenQUIC(address string) (*Listener, error) {
	udp, err := Listen(address)
	if err != nil {
		return nil, err
	}
	l := &Listener{
		transport: udp,
		conns:     make(map[string]*connection.Connection),
		nextPN:    make(map[string]uint64),
		addrs:     make(map[string]*net.UDPAddr),
		autoEcho:  true,
		acceptCh:  make(chan *connection.Connection, 32),
		closed:    make(chan struct{}),
	}
	go l.serve()
	return l, nil
}

func (l *Listener) Addr() *net.UDPAddr {
	return l.transport.LocalAddr()
}

func (l *Listener) Accept() (*connection.Connection, error) {
	select {
	case conn := <-l.acceptCh:
		return conn, nil
	case <-l.closed:
		return nil, errors.New("listener closed")
	}
}

func (l *Listener) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return l.transport.Close()
}

func (l *Listener) serve() {
	for {
		data, addr, err := l.transport.ReadPacket()
		if err != nil {
			select {
			case <-l.closed:
				return
			default:
				continue
			}
		}
		h, frames, err := wire.ParsePacketFrames(data, 8)
		if err != nil {
			continue
		}
		key := hex.EncodeToString(h.DestConnectionID())

		l.mu.Lock()
		l.addrs[key] = addr
		conn, ok := l.conns[key]
		if !ok && h.IsLongHeader() && h.PacketType() == packet.PacketTypeInitial {
			conn = connection.New(connection.RoleServer, h.Version(), h.DestConnectionID())
			conn.StoreRemoteCID(h.SrcConnectionID())
			_ = conn.Transition(connection.StateInitialReceived)
			_ = conn.Transition(connection.StateHandshake)
			l.conns[key] = conn
			l.nextPN[key] = 1
			select {
			case l.acceptCh <- conn:
			default:
			}
		}
		l.mu.Unlock()

		if conn == nil {
			continue
		}
		_ = conn.HandleFrames(frames)

		if h.IsLongHeader() && h.PacketType() == packet.PacketTypeInitial {
			responseFrames := []frame.Frame{
				frame.MaxDataFrame{MaximumData: 4096},
				frame.HandshakeDoneFrame{},
			}
			response, err := wire.BuildShortPacket(h.SrcConnectionID(), l.nextPacketNumber(key), responseFrames)
			if err == nil {
				_ = l.transport.WritePacket(response, addr)
				_ = conn.Transition(connection.StateConnected)
			}
			continue
		}

		streamResponses := make([]frame.Frame, 0)
		for _, f := range frames {
			streamFrame, ok := f.(frame.StreamFrame)
			if !ok {
				continue
			}
			streamResponses = append(streamResponses, frame.StreamFrame{
				StreamID: streamFrame.StreamID,
				Offset:   0,
				Data:     append([]byte("ack:"), streamFrame.Data...),
				Fin:      true,
			})
		}
		if len(streamResponses) > 0 {
			remoteCID, ok := conn.PrimaryRemoteCID()
			if !ok {
				continue
			}
			if l.AutoEcho() {
				response, err := wire.BuildShortPacket(remoteCID, l.nextPacketNumber(key), streamResponses)
				if err == nil {
					_ = l.transport.WritePacket(response, addr)
				}
			}
		}
	}
}

func (l *Listener) nextPacketNumber(key string) uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	pn := l.nextPN[key]
	if pn == 0 {
		pn = 1
	}
	l.nextPN[key] = pn + 1
	return pn
}

func (l *Listener) WaitForConnections(n int, timeout time.Duration) ([]*connection.Connection, error) {
	result := make([]*connection.Connection, 0, n)
	deadline := time.After(timeout)
	for len(result) < n {
		select {
		case conn := <-l.acceptCh:
			result = append(result, conn)
		case <-deadline:
			return result, errors.New("timeout waiting for connections")
		case <-l.closed:
			return result, errors.New("listener closed")
		}
	}
	return result, nil
}

func (l *Listener) SetAutoEcho(v bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.autoEcho = v
}

func (l *Listener) AutoEcho() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.autoEcho
}

func (l *Listener) SendFrames(conn *connection.Connection, frames []frame.Frame) error {
	remoteCID, ok := conn.PrimaryRemoteCID()
	if !ok {
		return errors.New("connection has no remote cid")
	}
	key := hex.EncodeToString(conn.OriginalDCID())
	l.mu.Lock()
	addr := l.addrs[key]
	l.mu.Unlock()
	if addr == nil {
		return errors.New("connection address unknown")
	}
	packetBytes, err := wire.BuildShortPacket(remoteCID, l.nextPacketNumber(key), frames)
	if err != nil {
		return err
	}
	return l.transport.WritePacket(packetBytes, addr)
}

func (l *Listener) RemoteAddr(conn *connection.Connection) *net.UDPAddr {
	if conn == nil {
		return nil
	}
	key := hex.EncodeToString(conn.OriginalDCID())
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.addrs[key]
}

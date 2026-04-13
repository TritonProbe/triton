package transport

import (
	"encoding/hex"
	"errors"
	"log"
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
	accepted  []*connection.Connection
	acceptQ   []*connection.Connection
	connReady chan struct{}
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
		accepted:  make([]*connection.Connection, 0, 32),
		acceptQ:   make([]*connection.Connection, 0, 32),
		connReady: make(chan struct{}, 1),
		closed:    make(chan struct{}),
	}
	go l.serve()
	return l, nil
}

func (l *Listener) Addr() *net.UDPAddr {
	return l.transport.LocalAddr()
}

func (l *Listener) Accept() (*connection.Connection, error) {
	for {
		l.mu.Lock()
		if len(l.acceptQ) > 0 {
			conn := l.acceptQ[0]
			l.acceptQ[0] = nil
			l.acceptQ = l.acceptQ[1:]
			l.mu.Unlock()
			return conn, nil
		}
		l.mu.Unlock()

		select {
		case <-l.connReady:
		case <-l.closed:
			return nil, errors.New("listener closed")
		}
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
			if err := conn.Transition(connection.StateInitialReceived); err != nil {
				l.mu.Unlock()
				log.Printf("listener: failed to transition connection to initial-received for %s: %v", key, err)
				continue
			}
			if err := conn.Transition(connection.StateHandshake); err != nil {
				l.mu.Unlock()
				log.Printf("listener: failed to transition connection to handshake for %s: %v", key, err)
				continue
			}
			l.conns[key] = conn
			l.nextPN[key] = 1
			l.accepted = append(l.accepted, conn)
			l.acceptQ = append(l.acceptQ, conn)
			select {
			case l.connReady <- struct{}{}:
			default:
			}
		}
		l.mu.Unlock()

		if conn == nil {
			continue
		}
		if err := conn.HandleFrames(frames); err != nil {
			log.Printf("listener: failed to handle frames for %s: %v", key, err)
			continue
		}

		if h.IsLongHeader() && h.PacketType() == packet.PacketTypeInitial {
			responseFrames := []frame.Frame{
				frame.MaxDataFrame{MaximumData: 4096},
				frame.HandshakeDoneFrame{},
			}
			response, err := wire.BuildShortPacket(h.SrcConnectionID(), l.nextPacketNumber(key), responseFrames)
			if err == nil {
				if err := l.transport.WritePacket(response, addr); err != nil {
					log.Printf("listener: failed to write handshake response for %s: %v", key, err)
					continue
				}
				if err := conn.Transition(connection.StateConnected); err != nil {
					log.Printf("listener: failed to transition connection to connected for %s: %v", key, err)
				}
			} else {
				log.Printf("listener: failed to build handshake response for %s: %v", key, err)
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
					if err := l.transport.WritePacket(response, addr); err != nil {
						log.Printf("listener: failed to write stream response for %s: %v", key, err)
					}
				} else {
					log.Printf("listener: failed to build stream response for %s: %v", key, err)
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
	deadline := time.After(timeout)
	for {
		l.mu.Lock()
		if len(l.accepted) >= n {
			result := append([]*connection.Connection(nil), l.accepted[:n]...)
			l.mu.Unlock()
			return result, nil
		}
		l.mu.Unlock()

		select {
		case <-l.connReady:
		case <-deadline:
			l.mu.Lock()
			result := append([]*connection.Connection(nil), l.accepted...)
			l.mu.Unlock()
			return result, errors.New("timeout waiting for connections")
		case <-l.closed:
			l.mu.Lock()
			result := append([]*connection.Connection(nil), l.accepted...)
			l.mu.Unlock()
			return result, errors.New("listener closed")
		}
	}
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

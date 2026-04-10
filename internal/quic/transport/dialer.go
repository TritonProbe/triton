package transport

import (
	"net"
	"time"

	"github.com/tritonprobe/triton/internal/quic/connection"
	"github.com/tritonprobe/triton/internal/quic/frame"
	"github.com/tritonprobe/triton/internal/quic/wire"
)

type Dialer struct {
	timeout time.Duration
}

type Session struct {
	transport    *UDPTransport
	conn         *connection.Connection
	remoteAddr   *net.UDPAddr
	destCID      []byte
	packetNumber uint64
}

func NewDialer(timeout time.Duration) *Dialer {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Dialer{timeout: timeout}
}

func (d *Dialer) Dial(address string) (*connection.Connection, error) {
	session, err := d.DialSession(address)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	return session.Connection(), nil
}

func (d *Dialer) DialSession(address string) (*Session, error) {
	udp, err := Listen("127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	udp.SetReadTimeout(d.timeout)
	remoteAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}

	dcid := []byte{0xde, 0xad, 0xbe, 0xef, 0, 0, 0, 1}
	scid := []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 1}
	conn := connection.New(connection.RoleClient, 1, dcid)
	_ = conn.Transition(connection.StateInitialSent)
	_ = conn.Transition(connection.StateHandshake)

	initial, err := wire.BuildInitialPacket(1, dcid, scid, 1, []frame.Frame{
		frame.CryptoFrame{Offset: 0, Data: []byte("client hello")},
	})
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	if err := udp.WritePacket(initial, remoteAddr); err != nil {
		_ = udp.Close()
		return nil, err
	}

	response, _, err := udp.ReadPacket()
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	_, frames, err := wire.ParsePacketFrames(response, len(scid))
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	if err := conn.HandleFrames(frames); err != nil {
		_ = udp.Close()
		return nil, err
	}
	return &Session{
		transport:    udp,
		conn:         conn,
		remoteAddr:   remoteAddr,
		destCID:      dcid,
		packetNumber: 2,
	}, nil
}

func (s *Session) Connection() *connection.Connection {
	return s.conn
}

func (s *Session) Close() error {
	if s.transport == nil {
		return nil
	}
	return s.transport.Close()
}

func (s *Session) SendFrames(frames []frame.Frame) error {
	packetBytes, err := wire.BuildShortPacket(s.destCID, s.packetNumber, frames)
	if err != nil {
		return err
	}
	if err := s.transport.WritePacket(packetBytes, s.remoteAddr); err != nil {
		return err
	}
	if err := s.conn.RecordSend(uint64(len(packetBytes))); err != nil {
		return err
	}
	s.packetNumber++
	return nil
}

func (s *Session) SendStream(streamID uint64, data []byte, fin bool) error {
	return s.SendFrames([]frame.Frame{
		frame.StreamFrame{StreamID: streamID, Offset: 0, Data: data, Fin: fin},
	})
}

func (s *Session) ReceiveFrames() error {
	response, _, err := s.transport.ReadPacket()
	if err != nil {
		return err
	}
	_, frames, err := wire.ParsePacketFrames(response, len(s.srcCID()))
	if err != nil {
		return err
	}
	return s.conn.HandleFrames(frames)
}

func (s *Session) srcCID() []byte {
	return []byte{0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 1}
}

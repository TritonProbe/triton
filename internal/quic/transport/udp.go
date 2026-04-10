package transport

import (
	"errors"
	"net"
	"sync"
	"time"
)

const (
	DefaultMTU   = 1200
	MaxUDPPacket = 65535
)

var packetPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 1452)
		return &buf
	},
}

type UDPTransport struct {
	conn         *net.UDPConn
	readTimeout  time.Duration
	writeTimeout time.Duration
	mtu          int
}

func Listen(address string) (*UDPTransport, error) {
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		return nil, err
	}
	return &UDPTransport{conn: conn, mtu: DefaultMTU}, nil
}

func New(conn *net.UDPConn) *UDPTransport {
	return &UDPTransport{conn: conn, mtu: DefaultMTU}
}

func (t *UDPTransport) ReadPacket() ([]byte, *net.UDPAddr, error) {
	if t.conn == nil {
		return nil, nil, errors.New("udp transport is closed")
	}
	if t.readTimeout > 0 {
		_ = t.conn.SetReadDeadline(time.Now().Add(t.readTimeout))
	}
	bufPtr := packetPool.Get().(*[]byte)
	buf := *bufPtr
	n, addr, err := t.conn.ReadFromUDP(buf)
	if err != nil {
		packetPool.Put(bufPtr)
		return nil, nil, err
	}
	out := make([]byte, n)
	copy(out, buf[:n])
	packetPool.Put(bufPtr)
	return out, addr, nil
}

func (t *UDPTransport) WritePacket(data []byte, addr *net.UDPAddr) error {
	if t.conn == nil {
		return errors.New("udp transport is closed")
	}
	if t.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.writeTimeout))
	}
	_, err := t.conn.WriteToUDP(data, addr)
	return err
}

func (t *UDPTransport) WriteBatch(packets [][]byte, addr *net.UDPAddr) error {
	for _, packet := range packets {
		if err := t.WritePacket(packet, addr); err != nil {
			return err
		}
	}
	return nil
}

func (t *UDPTransport) SetReadDeadline(deadline time.Time) error {
	if t.conn == nil {
		return errors.New("udp transport is closed")
	}
	return t.conn.SetReadDeadline(deadline)
}

func (t *UDPTransport) SetReadTimeout(timeout time.Duration) {
	t.readTimeout = timeout
}

func (t *UDPTransport) SetWriteTimeout(timeout time.Duration) {
	t.writeTimeout = timeout
}

func (t *UDPTransport) Close() error {
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	return err
}

func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	if t.conn == nil {
		return nil
	}
	addr, _ := t.conn.LocalAddr().(*net.UDPAddr)
	return addr
}

func (t *UDPTransport) MTU() int {
	return t.mtu
}

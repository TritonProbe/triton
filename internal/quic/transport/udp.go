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
	mu           sync.RWMutex
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
	t.mu.RLock()
	conn := t.conn
	readTimeout := t.readTimeout
	t.mu.RUnlock()
	if conn == nil {
		return nil, nil, errors.New("udp transport is closed")
	}
	if readTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(readTimeout))
	}
	bufPtr := packetPool.Get().(*[]byte)
	buf := *bufPtr
	n, addr, err := conn.ReadFromUDP(buf)
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
	t.mu.RLock()
	conn := t.conn
	writeTimeout := t.writeTimeout
	t.mu.RUnlock()
	if conn == nil {
		return errors.New("udp transport is closed")
	}
	if writeTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	}
	_, err := conn.WriteToUDP(data, addr)
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
	t.mu.RLock()
	conn := t.conn
	t.mu.RUnlock()
	if conn == nil {
		return errors.New("udp transport is closed")
	}
	return conn.SetReadDeadline(deadline)
}

func (t *UDPTransport) SetReadTimeout(timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.readTimeout = timeout
}

func (t *UDPTransport) SetWriteTimeout(timeout time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.writeTimeout = timeout
}

func (t *UDPTransport) Close() error {
	t.mu.Lock()
	conn := t.conn
	t.conn = nil
	t.mu.Unlock()
	if conn == nil {
		return nil
	}
	return conn.Close()
}

func (t *UDPTransport) LocalAddr() *net.UDPAddr {
	t.mu.RLock()
	conn := t.conn
	t.mu.RUnlock()
	if conn == nil {
		return nil
	}
	addr, _ := conn.LocalAddr().(*net.UDPAddr)
	return addr
}

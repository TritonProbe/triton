package connection

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type CIDManager struct {
	mu   sync.Mutex
	cids map[string][]byte
}

func NewCIDManager() *CIDManager {
	return &CIDManager{cids: make(map[string][]byte)}
}

func (m *CIDManager) Generate(length int) ([]byte, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return nil, err
	}
	m.Store(buf)
	return append([]byte(nil), buf...), nil
}

func (m *CIDManager) Store(cid []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cids[hex.EncodeToString(cid)] = append([]byte(nil), cid...)
}

func (m *CIDManager) Has(cid []byte) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.cids[hex.EncodeToString(cid)]
	return ok
}

func (m *CIDManager) Retire(cid []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.cids, hex.EncodeToString(cid))
}

func (m *CIDManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.cids)
}

func (m *CIDManager) First() ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cid := range m.cids {
		return append([]byte(nil), cid...), true
	}
	return nil, false
}

package connection

import (
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

func (m *CIDManager) Store(cid []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cids[hex.EncodeToString(cid)] = append([]byte(nil), cid...)
}

func (m *CIDManager) First() ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cid := range m.cids {
		return append([]byte(nil), cid...), true
	}
	return nil, false
}

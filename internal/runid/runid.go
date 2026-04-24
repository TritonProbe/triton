package runid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

var sequence atomic.Uint64

func New(prefix string) string {
	if prefix == "" {
		prefix = "run"
	}

	seq := sequence.Add(1)
	var entropy [4]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UTC().UnixNano(), seq)
	}

	return fmt.Sprintf("%s-%d-%d-%s", prefix, time.Now().UTC().UnixNano(), seq, hex.EncodeToString(entropy[:]))
}

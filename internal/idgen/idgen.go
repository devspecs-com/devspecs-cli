// Package idgen generates stable, sortable IDs for DevSpecs artifacts.
package idgen

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Factory generates DevSpecs IDs. It is safe for concurrent use.
type Factory struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// NewFactory creates a new ID factory with a cryptographic entropy source.
func NewFactory() *Factory {
	return &Factory{
		entropy: ulid.Monotonic(rand.Reader, 0),
	}
}

// New generates a new DevSpecs ID in the form ds_<ULID>.
func (f *Factory) New() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), f.entropy)
	return fmt.Sprintf("ds_%s", id.String())
}

// NewWithPrefix generates an ID with a custom prefix (e.g. "rev_", "src_").
func (f *Factory) NewWithPrefix(prefix string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	id := ulid.MustNew(ulid.Timestamp(time.Now()), f.entropy)
	return fmt.Sprintf("%s%s", prefix, id.String())
}

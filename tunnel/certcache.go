package tunnel

import (
	"context"
	"sync"

	"golang.org/x/crypto/acme/autocert"
)

type certCache struct {
	data  map[string][]byte
	mutex sync.RWMutex
}

func newCertCache() *certCache {
	return &certCache{
		data: make(map[string][]byte),
	}
}

// Get returns a certificate data for the specified key.
// If there's no such key, Get returns ErrCacheMiss.
func (c *certCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()

	data, ok := c.data[key]
	if !ok {
		return nil, autocert.ErrCacheMiss
	}
	return data, nil
}

// Put stores the data in the cache under the specified key.
// Underlying implementations may use any data storage format,
// as long as the reverse operation, Get, results in the original data.
func (c *certCache) Put(ctx context.Context, key string, data []byte) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.data[key] = data
	return nil
}

// Delete removes a certificate data from the cache under the specified key.
// If there's no such key in the cache, Delete returns nil.
func (c *certCache) Delete(ctx context.Context, key string) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	delete(c.data, key)
	return nil
}

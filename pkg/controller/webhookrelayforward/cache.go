package webhookrelayforward

import (
	"sync"

	"github.com/jinzhu/copier"
	"github.com/webhookrelay/webhookrelay-go"
)

type bucketsCache struct {
	items map[string]*webhookrelay.Bucket
	mu    *sync.RWMutex
}

func newBucketsCache() *bucketsCache {
	return &bucketsCache{
		items: make(map[string]*webhookrelay.Bucket),
		mu:    &sync.RWMutex{},
	}
}

// Reset removes all entries from the cache
func (c *bucketsCache) Reset() {
	c.mu.Lock()
	c.items = make(map[string]*webhookrelay.Bucket)
	c.mu.Unlock()
}

// Set a list of buckets, any previous entries are removed
func (c *bucketsCache) Set(buckets []*webhookrelay.Bucket) {
	items := make(map[string]*webhookrelay.Bucket)

	for idx := range buckets {
		items[buckets[idx].Name] = buckets[idx]
	}

	c.mu.Lock()
	c.items = items
	c.mu.Unlock()
}

// AddOutput if bucket is found, it updates existing output or
// appends it to the output list.
func (c *bucketsCache) AddOutput(o *webhookrelay.Output) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for k, bucket := range c.items {
		if bucket.ID == o.BucketID {
			// checking outputs
			var found bool
			for idx, output := range bucket.Outputs {
				if output.ID == o.ID {
					found = true
					// updating item in the list
					bucket.Outputs[idx] = o
				}
			}

			if !found {
				bucket.Outputs = append(bucket.Outputs, o)
			}
			c.items[k] = bucket
		}
	}

}

// Add a bucket to the cache. Can be used after creation or bucket update
func (c *bucketsCache) Add(b *webhookrelay.Bucket) {
	c.mu.Lock()
	c.items[b.Name] = b
	c.mu.Unlock()
}

// Get - get bucket by name or ID
func (c *bucketsCache) Get(ref string) (*webhookrelay.Bucket, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Since BucketSpec only contains the name, we primarily search by name too
	// and only if it doesn't match, we go and look by the ID
	if !webhookrelay.IsUUID(ref) {
		existing, ok := c.items[ref]
		if ok {
			cp := new(webhookrelay.Bucket)
			err := copier.Copy(cp, existing)
			if err != nil {
				return existing, true
			}
			return cp, true
		}
		// continuing to search by ID
	}

	// looking for the bucket by ID
	for _, v := range c.items {
		if v.ID == ref {
			cp := new(webhookrelay.Bucket)
			err := copier.Copy(cp, v)
			if err != nil {
				return v, true
			}
			return cp, true
		}
	}

	return nil, false
}

// List all cached buckets
func (c *bucketsCache) List() []*webhookrelay.Bucket {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var err error
	var items []*webhookrelay.Bucket

	for _, v := range c.items {
		cp := new(webhookrelay.Bucket)
		err = copier.Copy(v, cp)
		if err != nil {
			//
		}
		items = append(items, cp)
	}

	return items
}

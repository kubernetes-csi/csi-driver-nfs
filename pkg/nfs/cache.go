/*
Copyright 2025 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package nfs

import (
	"fmt"
	"sync"
	"time"

	"k8s.io/client-go/tools/cache"
)

// CacheReadType defines the read type for cache data
type CacheReadType int

const (
	// CacheReadTypeDefault returns data from cache if cache entry not expired
	// if cache entry expired, then it will refetch the data using getter
	// save the entry in cache and then return
	CacheReadTypeDefault CacheReadType = iota
	// Note: The original Azure cache had CacheReadTypeUnsafe and CacheReadTypeForceRefresh,
	// but these are not used in the NFS CSI driver codebase, so they are not implemented here.
)

// GetFunc defines a getter function for timedCache.
type GetFunc func(key string) (interface{}, error)

// CacheEntry is the internal structure stores inside cache Store.
type CacheEntry struct {
	Key  string
	Data interface{}

	// The lock to ensure not updating same entry simultaneously.
	Lock sync.Mutex
	// time when entry was fetched and created
	CreatedOn time.Time
}

// cacheKeyFunc defines the key function required in cache Store.
func cacheKeyFunc(obj interface{}) (string, error) {
	return obj.(*CacheEntry).Key, nil
}

// Resource operations
// Note: This interface only includes the methods actually used by the NFS CSI driver.
// The original Azure cache had additional methods (GetWithDeepCopy, Delete, Update, 
// GetStore, Lock, Unlock) that are not used in this codebase and thus not implemented.
type Resource interface {
	Get(key string, crt CacheReadType) (interface{}, error)
	Set(key string, data interface{})
}

// TimedCache is a cache with TTL.
type TimedCache struct {
	Store     cache.Store
	MutexLock sync.RWMutex
	TTL       time.Duration
	Getter    GetFunc
}

// NewTimedCache creates a new cache.Resource.
func NewTimedCache(ttl time.Duration, getter GetFunc, disabled bool) (Resource, error) {
	if getter == nil {
		return nil, fmt.Errorf("getter is not provided")
	}

	if disabled {
		return &DisabledCache{Getter: getter}, nil
	}

	timedCache := &TimedCache{
		Store:     cache.NewStore(cacheKeyFunc),
		MutexLock: sync.RWMutex{},
		TTL:       ttl,
		Getter:    getter,
	}
	return timedCache, nil
}

// getInternal returns CacheEntry by key. If the key is not cached yet,
// it returns a CacheEntry with nil data.
func (t *TimedCache) getInternal(key string) (*CacheEntry, error) {
	entry, exists, err := t.Store.GetByKey(key)
	if err != nil {
		return nil, err
	}
	// if entry exists, return the entry
	if exists {
		return entry.(*CacheEntry), nil
	}

	// lock here to ensure if entry doesn't exist, we add a new entry
	// avoiding overwrites
	t.MutexLock.Lock()
	defer t.MutexLock.Unlock()

	// Another goroutine might have written the same key.
	entry, exists, err = t.Store.GetByKey(key)
	if err != nil {
		return nil, err
	}
	if exists {
		return entry.(*CacheEntry), nil
	}

	// Still not found, add new entry with nil data.
	// Note the data will be filled later by getter.
	newEntry := &CacheEntry{
		Key:  key,
		Data: nil,
	}
	_ = t.Store.Add(newEntry)
	return newEntry, nil
}

// Get returns the requested item by key.
func (t *TimedCache) Get(key string, crt CacheReadType) (interface{}, error) {
	entry, err := t.getInternal(key)
	if err != nil {
		return nil, err
	}

	entry.Lock.Lock()
	defer entry.Lock.Unlock()

	// entry exists and cache is not expired
	if entry.Data != nil && time.Since(entry.CreatedOn) < t.TTL {
		return entry.Data, nil
	}

	// Data is not cached yet or cache data is expired
	// cache it by getter. entry is locked before getting to ensure concurrent
	// gets don't result in multiple calls.
	data, err := t.Getter(key)
	if err != nil {
		return nil, err
	}

	// set the data in cache and also set the last update time
	// to now as the data was recently fetched
	entry.Data = data
	entry.CreatedOn = time.Now().UTC()

	return entry.Data, nil
}

// Set sets the data cache for the key.
func (t *TimedCache) Set(key string, data interface{}) {
	_ = t.Store.Add(&CacheEntry{
		Key:       key,
		Data:      data,
		CreatedOn: time.Now().UTC(),
	})
}

// DisabledCache is a cache that is disabled and always calls the getter.
type DisabledCache struct {
	Getter GetFunc
}

// Get returns the requested item by key using the getter.
func (c *DisabledCache) Get(key string, _ CacheReadType) (interface{}, error) {
	return c.Getter(key)
}

// Set is a no-op for disabled cache.
func (c *DisabledCache) Set(_ string, _ interface{}) {}

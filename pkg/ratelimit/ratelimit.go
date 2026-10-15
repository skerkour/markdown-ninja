// Package ratelimit provides a simple fixed-window rate limiter.
package ratelimit

import (
	"encoding/binary"
	"math/rand/v2"
	"sync"
	"time"

	"markdown.ninja/stdx/xxh3"
)

// Limiter tracks request counts within time buckets.
type Limiter struct {
	mutex    sync.Mutex
	buckets  map[uint64]*bucket
	stop     chan struct{}
	hashSeed uint64
}

type bucket struct {
	count   uint64
	expires time.Time
}

// New creates a new rate limiter with automatic cleanup of expired buckets.
func New() *Limiter {

	limiter := &Limiter{
		mutex:    sync.Mutex{},
		buckets:  make(map[uint64]*bucket),
		stop:     make(chan struct{}),
		hashSeed: rand.Uint64(),
	}
	go limiter.cleanupLoop()
	return limiter
}

// IsAllowed checks if an action by an actor is allowed within the rate limit.
// It returns true if the action is allowed, false if rate limited.
//
// Parameters:
//   - namespace: optional namespace for the action check (e.g. tenant ID). Can be nil.
//   - action: identifies the type of action being rate limited (e.g., "login", "api-call")
//   - actor: identifies who is performing the action (e.g., user ID, IP address)
//   - timeBucket: the duration of each rate limit window
//   - allowed: maximum number of actions allowed per time bucket
func (limiter *Limiter) IsAllowed(action string, namespace []byte, actor []byte, timeBucket time.Duration, allowed uint64) bool {
	now := time.Now()
	bucketStart := now.Truncate(timeBucket)
	key := limiter.makeKey(action, namespace, actor, uint64(bucketStart.UnixNano()), uint64(timeBucket.Nanoseconds()))

	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	existingBucket, exists := limiter.buckets[key]
	if !exists {
		limiter.buckets[key] = &bucket{
			count:   1,
			expires: bucketStart.Add(timeBucket * 2), // Keep for one extra period for safety
		}
		return true
	}

	if existingBucket.count >= allowed {
		return false
	}

	existingBucket.count++
	return true
}

// Count returns the current count for an action/actor in the current time bucket.
// Useful for showing users how many requests they have remaining.
func (limiter *Limiter) Count(action string, namespace []byte, actor []byte, timeBucket time.Duration) uint64 {
	now := time.Now()
	bucketStart := now.Truncate(timeBucket)
	key := limiter.makeKey(action, namespace, actor, uint64(bucketStart.UnixNano()), uint64(timeBucket.Nanoseconds()))

	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	if b, exists := limiter.buckets[key]; exists {
		return b.count
	}
	return 0
}

// Remaining returns how many requests are remaining for an action/actor.
func (limiter *Limiter) Remaining(action string, namespace []byte, actor []byte, timeBucket time.Duration, allowed uint64) uint64 {
	count := limiter.Count(action, namespace, actor, timeBucket)
	if count >= allowed {
		return 0
	}
	return allowed - count
}

// Stop stops the background cleanup goroutine.
// Call this when the limiter is no longer needed.
func (limiter *Limiter) Stop() {
	close(limiter.stop)
}

func (limiter *Limiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			limiter.cleanup()
		case <-limiter.stop:
			return
		}
	}
}

func (limiter *Limiter) cleanup() {
	now := time.Now()

	limiter.mutex.Lock()
	defer limiter.mutex.Unlock()

	for key, b := range limiter.buckets {
		if now.After(b.expires) {
			delete(limiter.buckets, key)
		}
	}
}

// makeKey returns a stable key by hashing the inputs. It currently uses xxh3.
func (limiter *Limiter) makeKey(action string, namespace []byte, actor []byte, bucketStartNanos uint64, timeBucketNanos uint64) uint64 {
	hasher := xxh3.NewSeed(limiter.hashSeed)
	hasher.Write([]byte(action))
	hasher.Write(namespace)
	hasher.Write(actor)
	binary.Write(hasher, binary.LittleEndian, bucketStartNanos)
	binary.Write(hasher, binary.LittleEndian, timeBucketNanos)

	return hasher.Sum64()
}

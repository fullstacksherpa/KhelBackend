package ratelimiter

import (
	"sync"
	"time"
)

type FixedWindowRateLimiter struct {
	sync.RWMutex
	clients map[string]int //string:UserIP, int count
	limit   int
	window  time.Duration
}

func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowRateLimiter {
	rl := &FixedWindowRateLimiter{
		clients: make(map[string]int),
		limit:   limit,
		window:  window,
	}
	go rl.cleanup()
	return rl
}

func (rl *FixedWindowRateLimiter) cleanup() {
	ticker := time.NewTicker(rl.window)
	for range ticker.C {
		rl.Lock()
		rl.clients = make(map[string]int) // reset all
		rl.Unlock()
	}
}

func (rl *FixedWindowRateLimiter) Allow(ip string) (bool, time.Duration) {
	rl.RLock()
	count, exists := rl.clients[ip]
	rl.RUnlock()

	if !exists || count < rl.limit {
		rl.Lock()
		if !exists {
			go rl.resetCount(ip)
		}

		rl.clients[ip]++
		rl.Unlock()
		return true, 0
	}

	return false, rl.window
}

func (rl *FixedWindowRateLimiter) resetCount(ip string) {
	time.Sleep(rl.window)
	rl.Lock()
	delete(rl.clients, ip)
	rl.Unlock()
}

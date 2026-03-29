package plugin

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"k8s.io/klog/v2"
)

// PluginRateLimiter implements per-plugin token bucket rate limiting.
// Each plugin gets its own bucket sized by the RateLimit value in its manifest.
type PluginRateLimiter struct {
	buckets map[string]*tokenBucket
	mu      sync.RWMutex
}

// NewPluginRateLimiter creates a new rate limiter.
func NewPluginRateLimiter() *PluginRateLimiter {
	return &PluginRateLimiter{
		buckets: make(map[string]*tokenBucket),
	}
}

// Register creates a token bucket for a plugin. ratePerSecond is the
// maximum sustained request rate. Burst capacity is set to 2× the rate.
func (rl *PluginRateLimiter) Register(pluginName string, ratePerSecond int) {
	if ratePerSecond <= 0 {
		ratePerSecond = 100
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.buckets[pluginName] = newTokenBucket(ratePerSecond, ratePerSecond*2)
}

// Unregister removes a plugin's rate limiter bucket.
func (rl *PluginRateLimiter) Unregister(pluginName string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.buckets, pluginName)
}

// Allow returns true if the request is within the plugin's rate limit.
func (rl *PluginRateLimiter) Allow(pluginName string) bool {
	rl.mu.RLock()
	bucket, ok := rl.buckets[pluginName]
	rl.mu.RUnlock()

	if !ok {
		// No bucket → allow (plugin has no rate limit configured)
		return true
	}

	return bucket.take()
}

// RateLimitMiddleware returns Gin middleware that enforces per-plugin
// rate limits on plugin HTTP endpoints.
func (rl *PluginRateLimiter) RateLimitMiddleware(getPluginName func(*gin.Context) string) gin.HandlerFunc {
	return func(c *gin.Context) {
		pluginName := getPluginName(c)
		if pluginName == "" {
			c.Next()
			return
		}

		if !rl.Allow(pluginName) {
			klog.Warningf("[PLUGIN:%s] rate limit exceeded", pluginName)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded for plugin " + pluginName,
			})
			return
		}

		c.Next()
	}
}

// --------------------------------------------------------------------------
// Token bucket implementation
// --------------------------------------------------------------------------

type tokenBucket struct {
	rate       float64 // tokens per second
	capacity   float64 // maximum burst tokens
	tokens     float64
	lastRefill time.Time
	mu         sync.Mutex
}

func newTokenBucket(ratePerSecond, burstCapacity int) *tokenBucket {
	return &tokenBucket{
		rate:       float64(ratePerSecond),
		capacity:   float64(burstCapacity),
		tokens:     float64(burstCapacity),
		lastRefill: time.Now(),
	}
}

func (tb *tokenBucket) take() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.lastRefill = now

	// Refill tokens
	tb.tokens += elapsed * tb.rate
	if tb.tokens > tb.capacity {
		tb.tokens = tb.capacity
	}

	if tb.tokens < 1 {
		return false
	}

	tb.tokens--
	return true
}

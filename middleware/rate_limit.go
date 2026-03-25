package middleware

import (
	"net"
	"net/http"
	"sync"

	"golang.org/x/time/rate"
)

type IPRateLimiter struct {
	ips       map[string]*rate.Limiter
	mu        sync.RWMutex
	rate      rate.Limit
	burstSize int
}

func NewIPRateLimiter(r rate.Limit, b int) *IPRateLimiter {
	return &IPRateLimiter{
		ips:       make(map[string]*rate.Limiter),
		rate:      r,
		burstSize: b,
	}
}

func (i *IPRateLimiter) getLimiter(ip string) *rate.Limiter {
	i.mu.RLock()
	limiter, exists := i.ips[ip]
	i.mu.RUnlock()
	if exists {
		return limiter
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	limiter, exists = i.ips[ip]
	if !exists {
		limiter = rate.NewLimiter(i.rate, i.burstSize)
		i.ips[ip] = limiter
	}
	return limiter
}

func (i *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		limiter := i.getLimiter(ip)

		if !limiter.Allow() {
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getClientIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-IP")
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	}
	return ip
}

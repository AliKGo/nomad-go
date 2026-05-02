package middleware

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// SECURITY NOTE: sync.Map без механизма очистки старых записей может привести к утечке памяти (Memory Leak).
// Өндірістік (production) ортада time.Ticker арқылы ескірген IP-лерді тазартатын қосымша горутина қосу қажет.
// Мысалы, context.Context арқылы басқарылатын background worker құру ұсынылады.
var visitors = sync.Map{}

type visitor struct {
	requests []time.Time
	mu       sync.Mutex
}

// RateLimit функциясы /auth/login немесе /drivers/{id}/location эндпоинттеріне қолданылады
func RateLimit(limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			v, _ := visitors.LoadOrStore(ip, &visitor{})
			visitorNode := v.(*visitor)

			visitorNode.mu.Lock()
			defer visitorNode.mu.Unlock()

			now := time.Now()
			var activeRequests []time.Time
			for _, t := range visitorNode.requests {
				if now.Sub(t) < window {
					activeRequests = append(activeRequests, t)
				}
			}
			visitorNode.requests = activeRequests
			if len(visitorNode.requests) >= limit {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				logEntry := map[string]any{
					"timestamp": now.Format(time.RFC3339),
					"level":     "WARN",
					"service":   "auth-service",
					"message":   "Rate limit exceeded",
					"ip":        ip,
				}
				json.NewEncoder(w).Encode(logEntry)
				return
			}

			visitorNode.requests = append(visitorNode.requests, now)
			next.ServeHTTP(w, r)
		})
	}
}

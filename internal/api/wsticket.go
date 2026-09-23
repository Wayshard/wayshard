package api

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"sync"
	"time"

	"github.com/Wayshard/wayshard/internal/auth"
)

// wsTicketTTL bounds how long an issued WebSocket ticket stays valid.
const wsTicketTTL = 30 * time.Second

// wsTickets issues short-lived, single-use tickets so a native client that
// holds its device credential in platform-secure storage can authenticate a
// WebSocket handshake without placing the long-lived credential in a URL and
// without relying on a cross-origin cookie (browsers and webviews cannot set an
// Authorization header on a WebSocket).
type wsTickets struct {
	mu   sync.Mutex
	ttl  time.Duration
	now  func() time.Time
	live map[string]time.Time
}

func newWSTickets(ttl time.Duration) *wsTickets {
	if ttl <= 0 {
		ttl = wsTicketTTL
	}
	return &wsTickets{ttl: ttl, now: time.Now, live: map[string]time.Time{}}
}

func (t *wsTickets) issue() (string, time.Time) {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	ticket := base64.RawURLEncoding.EncodeToString(b)
	exp := t.now().Add(t.ttl)
	t.mu.Lock()
	t.gcLocked()
	t.live[ticket] = exp
	t.mu.Unlock()
	return ticket, exp
}

// consume validates and invalidates a ticket (single use).
func (t *wsTickets) consume(ticket string) bool {
	if ticket == "" {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	exp, ok := t.live[ticket]
	if !ok {
		return false
	}
	delete(t.live, ticket)
	return t.now().Before(exp)
}

func (t *wsTickets) gcLocked() {
	now := t.now()
	for k, exp := range t.live {
		if !now.Before(exp) {
			delete(t.live, k)
		}
	}
}

// wsTicket issues a single-use WebSocket ticket for an authenticated device.
func (s *Server) wsTicket(w http.ResponseWriter, r *http.Request, _ *auth.Principal) {
	ticket, exp := s.ticketStore().issue()
	writeJSON(w, http.StatusOK, map[string]any{
		"ticket":    ticket,
		"expiresAt": exp.UTC().Format(time.RFC3339),
	})
}

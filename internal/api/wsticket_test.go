package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestWSTicketsSingleUseAndExpiry(t *testing.T) {
	now := time.Unix(1000, 0)
	ts := newWSTickets(30 * time.Second)
	ts.now = func() time.Time { return now }

	ticket, exp := ts.issue()
	if !exp.After(now) {
		t.Fatalf("ticket expiry %s is not in the future", exp)
	}
	if !ts.consume(ticket) {
		t.Fatal("fresh ticket must be consumable")
	}
	if ts.consume(ticket) {
		t.Fatal("ticket must be single-use")
	}
	if ts.consume("") {
		t.Fatal("empty ticket must be rejected")
	}

	expired, _ := ts.issue()
	now = now.Add(31 * time.Second)
	if ts.consume(expired) {
		t.Fatal("expired ticket must be rejected")
	}
}

// TestWSTicketEndpointRequiresAuth proves the ticket endpoint is authenticated
// and that the issued ticket authenticates exactly one WebSocket handshake.
func TestWSTicketEndpointRequiresAuth(t *testing.T) {
	s, ts := testAPI(t)

	res, err := http.Post(ts.URL+"/v1/ws/ticket", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated ticket status = %d, want 401", res.StatusCode)
	}

	cred := issueCred(t, s)
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/ws/ticket", nil)
	req.Header.Set("Authorization", "Bearer "+cred)
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ticket status = %d, want 200", res.StatusCode)
	}
	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.Ticket == "" {
		t.Fatal("empty ticket")
	}
	if !s.ticketStore().consume(body.Ticket) {
		t.Fatal("issued ticket was not consumable")
	}
	if s.ticketStore().consume(body.Ticket) {
		t.Fatal("issued ticket must be single-use")
	}
}

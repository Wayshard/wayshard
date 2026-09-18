package events

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/Wayshard/wayshard/internal/domain"
	"github.com/Wayshard/wayshard/internal/storage"
)

type Frame struct {
	Type    string          `json:"type"`
	Seq     int64           `json:"seq,omitempty"`
	Channel string          `json:"channel,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type sub struct {
	ch        chan Frame
	projectID string
	runID     string
	lastSeq   int64
}

type Hub struct {
	Store *storage.Store
	mu    sync.Mutex
	subs  map[chan Frame]*sub
}

func NewHub(store *storage.Store) *Hub {
	return &Hub{Store: store, subs: map[chan Frame]*sub{}}
}

func (h *Hub) Subscribe(lastSeq int64, projectID, runID string) (<-chan Frame, func()) {
	ch := make(chan Frame, 64)
	h.mu.Lock()
	h.subs[ch] = &sub{ch: ch, projectID: projectID, runID: runID, lastSeq: lastSeq}
	h.mu.Unlock()
	go h.replay(ch, lastSeq, projectID, runID)
	return ch, func() {
		h.mu.Lock()
		delete(h.subs, ch)
		h.mu.Unlock()
		close(ch)
	}
}

func (h *Hub) replay(ch chan Frame, after int64, projectID, runID string) {
	evs, err := h.Store.EventsSince(context.Background(), after, projectID, runID, 500)
	if err != nil {
		h.send(ch, Frame{Type: "resync"})
		return
	}
	if after > 0 && len(evs) == 0 {
		latest, _ := h.Store.LatestEventSeq(context.Background())
		if after < latest {
			// history trimmed
			h.send(ch, Frame{Type: "resync"})
			return
		}
	}
	for _, e := range evs {
		h.send(ch, frameOf(e))
	}
	h.send(ch, Frame{Type: "live"})
}

func (h *Hub) Broadcast(e domain.Event) {
	fr := frameOf(e)
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch, s := range h.subs {
		if s.projectID != "" && e.ProjectID != "" && s.projectID != e.ProjectID {
			continue
		}
		if s.runID != "" && e.RunID != "" && s.runID != e.RunID {
			continue
		}
		h.sendLocked(ch, fr)
	}
}

func (h *Hub) Stream(ch chan Frame, fr Frame) {
	h.send(ch, fr)
}

func (h *Hub) send(ch chan Frame, fr Frame) {
	select {
	case ch <- fr:
	default:
	}
}

func (h *Hub) sendLocked(ch chan Frame, fr Frame) {
	select {
	case ch <- fr:
	default:
	}
}

func frameOf(e domain.Event) Frame {
	return Frame{Type: "event", Seq: e.Seq, Payload: json.RawMessage(e.Payload), Channel: e.Type}
}

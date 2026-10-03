package workspace

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"aihub.dev/server/internal/auth"
)

// Hub is the v1 change-hint transport (INH-513): Server-Sent Events with an
// in-memory per-user replay buffer, plus the WebSocket delivery swap on the
// same route and semantics (hello, Last-Event-ID resume, coarse pointers only).
type Hub struct {
	mu      sync.Mutex
	subs    map[string]map[chan Event]struct{}
	buffer  map[string][]Event
	nextSeq map[string]uint64
}

// Event is one change hint; Data carries coarse pointers only, never payloads.
type Event struct {
	Seq  uint64           `json:"seq"`
	Type string           `json:"type"`
	At   time.Time        `json:"at"`
	Data map[string]string `json:"data,omitempty"`
}

const hubBuffer = 256

func NewHub() *Hub {
	return &Hub{subs: map[string]map[chan Event]struct{}{}, buffer: map[string][]Event{}, nextSeq: map[string]uint64{}}
}

// Publish records an event for the user and wakes live subscribers.
func (h *Hub) Publish(userID, eventType string, data map[string]string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextSeq[userID]++
	ev := Event{Seq: h.nextSeq[userID], Type: eventType, At: time.Now().UTC(), Data: data}
	h.buffer[userID] = append(h.buffer[userID], ev)
	if len(h.buffer[userID]) > hubBuffer {
		h.buffer[userID] = h.buffer[userID][len(h.buffer[userID])-hubBuffer:]
	}
	for ch := range h.subs[userID] {
		select {
		case ch <- ev:
		default: // slow consumer: replay covers it after reconnect
		}
	}
}

// PublishToWorkspace fans an event out to every current member of a workspace.
func (h *Hub) PublishToWorkspace(ctx context.Context, database *sql.DB, workspaceID, eventType string, data map[string]string) {
	rows, e := database.QueryContext(ctx, `SELECT user_id FROM workspace_members WHERE workspace_id=$1`, workspaceID)
	if e != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			h.Publish(uid, eventType, data)
		}
	}
}

// subscribe registers a live channel for the user and returns the replay
// backlog plus the hello marker; lastEventID resumes after a reconnect.
func (h *Hub) subscribe(userID, lastEventID string) (chan Event, []Event, Event) {
	ch := make(chan Event, 32)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextSeq[userID]++
	hello := Event{Seq: h.nextSeq[userID], Type: "hello", At: time.Now().UTC()}
	h.buffer[userID] = append(h.buffer[userID], hello)
	if len(h.buffer[userID]) > hubBuffer {
		h.buffer[userID] = h.buffer[userID][len(h.buffer[userID])-hubBuffer:]
	}
	if h.subs[userID] == nil {
		h.subs[userID] = map[chan Event]struct{}{}
	}
	h.subs[userID][ch] = struct{}{}
	var last uint64
	if lastEventID != "" {
		last, _ = strconv.ParseUint(lastEventID, 10, 64)
	}
	pending := make([]Event, 0, len(h.buffer[userID]))
	for _, ev := range h.buffer[userID] {
		if ev.Seq > last && ev.Seq < hello.Seq {
			pending = append(pending, ev)
		}
	}
	return ch, pending, hello
}

func (h *Hub) unsubscribe(userID string, ch chan Event) {
	h.mu.Lock()
	delete(h.subs[userID], ch)
	h.mu.Unlock()
}

// Stream serves the change-hints channel: WebSocket when the request upgrades,
// Server-Sent Events otherwise. Both carry identical hello, replay and live
// semantics; the event payload stays coarse pointers only.
func (h *Hub) Stream(w http.ResponseWriter, r *http.Request) {
	if isWebSocketUpgrade(r) {
		h.streamWebSocket(w, r)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	userID := auth.Who(r).UserID
	ch, pending, hello := h.subscribe(userID, r.Header.Get("Last-Event-ID"))
	defer h.unsubscribe(userID, ch)

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	fmt.Fprint(w, "retry: 3000\n\n")
	for _, ev := range pending {
		writeEvent(w, ev)
	}
	writeEvent(w, hello)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev := <-ch:
			writeEvent(w, ev)
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, ev Event) {
	data, _ := json.Marshal(ev)
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.Seq, ev.Type, data)
}

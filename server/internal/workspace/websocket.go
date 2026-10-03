package workspace

import (
	"net/http"
	"os"
	"strings"
	"time"

	"aihub.dev/server/internal/auth"

	"github.com/gorilla/websocket"
)

var hintsUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	// Same origin policy as httpx.CORS: empty origin (native clients) and the
	// serving host pass through, plus the AIHUB_ALLOWED_ORIGINS list.
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" || origin == "https://"+r.Host || origin == "http://"+r.Host {
			return true
		}
		for _, entry := range strings.Split(os.Getenv("AIHUB_ALLOWED_ORIGINS"), ",") {
			if entry != "" && origin == strings.TrimSpace(entry) {
				return true
			}
		}
		return false
	},
}

// isWebSocketUpgrade reports whether the client asked for the WebSocket
// handshake on this request.
func isWebSocketUpgrade(r *http.Request) bool {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return false
	}
	for _, v := range r.Header["Connection"] {
		for _, token := range strings.Split(v, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "upgrade") {
				return true
			}
		}
	}
	return false
}

// streamWebSocket is the INH-513 delivery swap: the hello marker, Last-Event-ID
// replay and live events already defined by the SSE channel, carried as JSON
// text frames. Browsers cannot set request headers on WebSocket connections,
// so resume also accepts the last_event_id query parameter next to the
// standard Last-Event-ID header used by native clients.
func (h *Hub) streamWebSocket(w http.ResponseWriter, r *http.Request) {
	userID := auth.Who(r).UserID
	conn, e := hintsUpgrader.Upgrade(w, r, nil)
	if e != nil {
		return
	}
	defer conn.Close()
	ch, pending, hello := h.subscribe(userID, firstNonEmpty(r.Header.Get("Last-Event-ID"), r.URL.Query().Get("last_event_id")))
	defer h.unsubscribe(userID, ch)

	// Client frames carry nothing but protocol control traffic; drop the
	// connection as soon as a read fails so writes unblock on close.
	conn.SetReadLimit(512)
	stop := make(chan struct{})
	go func() {
		defer close(stop)
		for {
			if _, _, e := conn.ReadMessage(); e != nil {
				return
			}
		}
	}()

	write := func(v any) bool {
		_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return conn.WriteJSON(v) == nil
	}
	for _, ev := range pending {
		if !write(ev) {
			return
		}
	}
	if !write(hello) {
		return
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-stop:
			return
		case <-r.Context().Done():
			return
		case <-heartbeat.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if e := conn.WriteMessage(websocket.PingMessage, nil); e != nil {
				return
			}
		case ev := <-ch:
			if !write(ev) {
				return
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

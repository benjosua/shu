package server

import "strings"

func NewDaemonHub() *DaemonHub {
	return &DaemonHub{subs: make(map[string]map[chan []byte]struct{})}
}

func (h *DaemonHub) Subscribe(executorIDs []string) (chan []byte, func()) {
	ch := make(chan []byte, 32)
	h.mu.Lock()
	for _, id := range executorIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if h.subs[id] == nil {
			h.subs[id] = make(map[chan []byte]struct{})
		}
		h.subs[id][ch] = struct{}{}
	}
	h.mu.Unlock()
	return ch, func() {
		h.mu.Lock()
		for _, set := range h.subs {
			delete(set, ch)
		}
		h.mu.Unlock()
		close(ch)
	}
}

func (h *DaemonHub) Send(executorID string, payload []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.subs[executorID] {
		select {
		case ch <- payload:
		default:
			// Slow daemon connection. Drop wakeup; poll fallback remains authoritative.
		}
	}
}

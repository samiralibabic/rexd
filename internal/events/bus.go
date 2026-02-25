package events

import (
	"sync"

	"github.com/samiralibabic/rexd/internal/protocol"
)

type Bus struct {
	mu          sync.RWMutex
	nextSubID   int
	subscribers map[string]map[int]chan protocol.Notification
}

func NewBus() *Bus {
	return &Bus{
		subscribers: map[string]map[int]chan protocol.Notification{},
	}
}

func (b *Bus) Subscribe(sessionID string) (chan protocol.Notification, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextSubID++
	id := b.nextSubID
	ch := make(chan protocol.Notification, 128)
	if _, ok := b.subscribers[sessionID]; !ok {
		b.subscribers[sessionID] = map[int]chan protocol.Notification{}
	}
	b.subscribers[sessionID][id] = ch
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if sessSubs, ok := b.subscribers[sessionID]; ok {
			if c, ok := sessSubs[id]; ok {
				close(c)
				delete(sessSubs, id)
			}
			if len(sessSubs) == 0 {
				delete(b.subscribers, sessionID)
			}
		}
	}
}

func (b *Bus) Publish(sessionID, method string, params any) {
	evt := protocol.Notification{
		JSONRPC: protocol.Version,
		Method:  method,
		Params:  params,
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers[sessionID] {
		select {
		case ch <- evt:
		default:
		}
	}
}

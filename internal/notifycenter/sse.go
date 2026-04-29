package notifycenter

import "sync"

// Bus is a tiny pub/sub fan-out used by the SSE endpoint. Each
// Subscribe returns a buffered channel; Publish copies the event onto
// every live subscriber non-blockingly. A subscriber whose buffer is
// full is dropped from this Publish call (event is shed) — back-
// pressure on a stalled SSE client must NOT block other consumers or
// the aggregator goroutine.
//
// The bus is in-memory and per-process. Multi-replica nova-api
// deployments would need a Redis pub/sub bridge in front of it; v1
// runs single-instance so this is fine.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[int]chan Event
	next        int
}

// NewBus constructs an empty Bus.
func NewBus() *Bus {
	return &Bus{subscribers: make(map[int]chan Event)}
}

// busBuffer is the per-subscriber channel depth. Sized for "burst of
// 16 events while the SSE writer is mid-flush"; rare bursts above
// this drop the overflow rather than blocking the publisher.
const busBuffer = 16

// Subscribe registers a new consumer. The returned cleanup function
// removes the subscription and closes the channel; callers MUST defer
// it.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	b.mu.Lock()
	id := b.next
	b.next++
	ch := make(chan Event, busBuffer)
	b.subscribers[id] = ch
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if c, ok := b.subscribers[id]; ok {
			delete(b.subscribers, id)
			close(c)
		}
		b.mu.Unlock()
	}
}

// Publish broadcasts ev to every current subscriber. Non-blocking;
// shed-on-full.
func (b *Bus) Publish(ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, ch := range b.subscribers {
		select {
		case ch <- ev:
		default:
			// drop — see Bus doc for rationale
		}
	}
}

// SubscriberCount is exposed for tests and metrics.
func (b *Bus) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

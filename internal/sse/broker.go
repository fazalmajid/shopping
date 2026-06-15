package sse

// Broker is a single-goroutine SSE pub/sub hub. The worker goroutine is the
// sole owner of the clients map, so no mutex is needed.
type Broker struct {
	subscribe   chan chan Event
	unsubscribe chan chan Event
	publish     chan Event
	quit        chan struct{}
}

type Event struct {
	Type string // item_added | item_classified | item_checked | item_deleted | items_cleared | section_added
	Data any    // marshaled to JSON by the SSE handler
}

func NewBroker() *Broker {
	return &Broker{
		subscribe:   make(chan chan Event),
		unsubscribe: make(chan chan Event),
		publish:     make(chan Event, 64),
		quit:        make(chan struct{}),
	}
}

func (b *Broker) Start() {
	clients := make(map[chan Event]struct{})
	for {
		select {
		case ch := <-b.subscribe:
			clients[ch] = struct{}{}
		case ch := <-b.unsubscribe:
			delete(clients, ch)
			close(ch)
		case ev := <-b.publish:
			for ch := range clients {
				select {
				case ch <- ev:
				default:
					// slow client: drop the event rather than blocking
				}
			}
		case <-b.quit:
			return
		}
	}
}

func (b *Broker) Stop() { close(b.quit) }

func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, 16)
	b.subscribe <- ch
	return ch
}

func (b *Broker) Unsubscribe(ch chan Event) {
	b.unsubscribe <- ch
}

func (b *Broker) Publish(ev Event) {
	select {
	case b.publish <- ev:
	default:
	}
}

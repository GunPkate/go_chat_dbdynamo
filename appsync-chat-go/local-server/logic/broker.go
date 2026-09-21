package logic

import "sync"

// Broker fans a published value out to every subscriber channel currently
// registered under a topic (a chatRoomId, a userId, etc). This is the
// hand-rolled stand-in for AppSync's @aws_subscribe(mutations: [...])
// directive, which has no equivalent outside AppSync itself — here, each
// Mutation function calls Publish() explicitly after a successful write.
//
// This is process-local: it works for a single running instance of this
// server, which is exactly the local-dev use case it's built for. A real
// multi-instance production deployment is precisely the problem AppSync
// (or a shared broker like Redis pub/sub) solves for you.
type Broker[T any] struct {
	mu   sync.Mutex
	subs map[string]map[chan T]struct{}
}

func NewBroker[T any]() *Broker[T] {
	return &Broker[T]{subs: make(map[string]map[chan T]struct{})}
}

// Subscribe registers a new channel under topic. Call cancel() when the
// subscriber goes away (e.g. the GraphQL subscription's context is done)
// to avoid leaking channels.
func (b *Broker[T]) Subscribe(topic string) (ch chan T, cancel func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch = make(chan T, 8)
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[chan T]struct{})
	}
	b.subs[topic][ch] = struct{}{}

	cancel = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[topic][ch]; ok {
			delete(b.subs[topic], ch)
			close(ch)
		}
	}
	return ch, cancel
}

// Publish sends value to every subscriber currently registered under
// topic. A slow subscriber's buffer filling up drops the message for that
// subscriber rather than blocking the publisher (the sender / mutation).
func (b *Broker[T]) Publish(topic string, value T) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[topic] {
		select {
		case ch <- value:
		default:
		}
	}
}

// One broker per Subscription field in schema.graphqls.
var (
	MessageSentBroker      = NewBroker[Message]()
	MessageEditedBroker    = NewBroker[Message]()
	MessageDeletedBroker   = NewBroker[Message]()
	ReadReceiptBroker      = NewBroker[Message]()
	DeliveredReceiptBroker = NewBroker[Message]()
	TypingBroker           = NewBroker[TypingEvent]()
	PresenceBroker         = NewBroker[PresenceEvent]()
)

package mock

import "sync"

// PendingMessage is a pending message to be sent
type PendingMessage struct {
	// The sender node id
	From     string
	EngineID uint8
	Event    interface{}
	// The id of the receiver nodes
	TargetIDs []string
}

// Buffer buffers all the pending messages to be sent over the mock network from one node to a list of nodes
type Buffer struct {
	sync.Mutex
	pending []*PendingMessage
}

// NewBuffer initialize the Buffer
func NewBuffer() *Buffer {
	return &Buffer{
		pending: make([]*PendingMessage, 0),
	}
}

// Save stores a pending message to the buffer
func (b *Buffer) Save(from string, engineID uint8, event interface{}, targetIDs []string) {
	b.Lock()
	defer b.Unlock()
	b.pending = append(b.pending, &PendingMessage{
		From:      from,
		EngineID:  engineID,
		Event:     event,
		TargetIDs: targetIDs,
	})
}

// Flush recursively delivers the pending messages until the buffer is empty
func (b *Buffer) Flush(sendOne func(*PendingMessage) error) {
	for {
		toSend := b.takeAll()

		if len(toSend) == 0 {
			return
		}

		for _, msg := range toSend {
			sendOne(msg)
		}
	}
}

// popAll takes all pending messages from the buffer and empty the buffer.
func (b *Buffer) takeAll() []*PendingMessage {
	b.Lock()
	defer b.Unlock()

	toSend := make([]*PendingMessage, 0)

	for _, msg := range b.pending {
		toSend = append(toSend, msg)
	}

	b.pending = make([]*PendingMessage, 0)
	return toSend
}

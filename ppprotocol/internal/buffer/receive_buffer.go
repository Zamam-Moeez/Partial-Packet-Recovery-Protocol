package buffer

import (
	"sync"
	"time"
)

// PartialMessage represents a message being assembled from packets
type PartialMessage struct {
	// Buffer for the full message data
	Data []byte

	// Bitmap tracking which parts of the message have been received
	ReceivedMap []bool

	// Total expected size of the message
	TotalSize uint32

	// Timestamp of last update (for timeout management)
	LastUpdate time.Time

	// Flag indicating if the message is complete
	Complete bool

	// Mutex for concurrent access
	mutex sync.Mutex
}

// NewPartialMessage creates a new partial message with the given size
func NewPartialMessage(totalSize uint32) *PartialMessage {
	return &PartialMessage{
		Data:        make([]byte, totalSize),
		ReceivedMap: make([]bool, totalSize),
		TotalSize:   totalSize,
		LastUpdate:  time.Now(),
		Complete:    false,
	}
}

// Update adds data to the partial message at the specified offset
func (pm *PartialMessage) Update(data []byte, offset uint32) (bytesReceived uint32, complete bool) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	pm.LastUpdate = time.Now()

	// Copy data into the buffer at the correct offset
	for i := 0; i < len(data) && offset+uint32(i) < pm.TotalSize; i++ {
		if !pm.ReceivedMap[offset+uint32(i)] {
			pm.Data[offset+uint32(i)] = data[i]
			pm.ReceivedMap[offset+uint32(i)] = true
		}
	}

	// Count received bytes and check if complete
	receivedCount := uint32(0)
	allReceived := true

	for i := uint32(0); i < pm.TotalSize; i++ {
		if pm.ReceivedMap[i] {
			receivedCount++
		} else {
			allReceived = false
		}
	}

	pm.Complete = allReceived

	return receivedCount, pm.Complete
}

// IsComplete returns true if the message is complete
func (pm *PartialMessage) IsComplete() bool {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	return pm.Complete
}

// GetData returns a copy of the message data
func (pm *PartialMessage) GetData() []byte {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	result := make([]byte, pm.TotalSize)
	copy(result, pm.Data)
	return result
}

// GetMissingRanges returns ranges of bytes that haven't been received yet
// Returns slices of [start, end) ranges
func (pm *PartialMessage) GetMissingRanges(maxRanges int) [][2]uint32 {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	if pm.Complete {
		return nil
	}

	var ranges [][2]uint32
	start := uint32(0)
	inGap := false

	for i := uint32(0); i < pm.TotalSize; i++ {
		if !pm.ReceivedMap[i] && !inGap {
			// Start of a gap
			start = i
			inGap = true
		} else if pm.ReceivedMap[i] && inGap {
			// End of a gap
			ranges = append(ranges, [2]uint32{start, i})
			inGap = false

			if len(ranges) >= maxRanges {
				break
			}
		}
	}

	// Handle case where the gap extends to the end
	if inGap {
		ranges = append(ranges, [2]uint32{start, pm.TotalSize})
	}

	return ranges
}

// TimeSinceLastUpdate returns the time elapsed since the last update
func (pm *PartialMessage) TimeSinceLastUpdate() time.Duration {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()
	return time.Since(pm.LastUpdate)
}

// ReceiveBuffer manages a collection of partial messages being assembled
type ReceiveBuffer struct {
	messages     map[uint32]*PartialMessage
	mutex        sync.Mutex
	messageQueue []uint32
	timeout      time.Duration
}

// NewReceiveBuffer creates a new receive buffer with default settings
func NewReceiveBuffer(timeout time.Duration) *ReceiveBuffer {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &ReceiveBuffer{
		messages:     make(map[uint32]*PartialMessage),
		messageQueue: make([]uint32, 0),
		timeout:      timeout,
	}
}

// ProcessPacket adds data from a packet to the appropriate message
func (rb *ReceiveBuffer) ProcessPacket(sequenceNum uint32, data []byte, offset uint32, totalSize uint32) (bytesReceived uint32, complete bool) {
	rb.mutex.Lock()

	// Check if we're already tracking this message
	message, exists := rb.messages[sequenceNum]
	if !exists {
		// Create a new partial message
		message = NewPartialMessage(totalSize)
		rb.messages[sequenceNum] = message
		rb.messageQueue = append(rb.messageQueue, sequenceNum)
	}

	rb.mutex.Unlock()

	// Update the message with the new data
	return message.Update(data, offset)
}

// GetNextComplete returns the next complete message in sequence
func (rb *ReceiveBuffer) GetNextComplete() (uint32, []byte, bool) {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	// Check if we have any messages
	if len(rb.messageQueue) == 0 {
		return 0, nil, false
	}

	// Get the next sequence number
	seqNum := rb.messageQueue[0]
	message, exists := rb.messages[seqNum]

	if !exists {
		// This shouldn't happen, but handle it anyway
		rb.messageQueue = rb.messageQueue[1:]
		return 0, nil, false
	}

	if !message.IsComplete() {
		return 0, nil, false
	}

	// Remove the message from the buffer
	data := message.GetData()
	delete(rb.messages, seqNum)
	rb.messageQueue = rb.messageQueue[1:]

	return seqNum, data, true
}

// GetMissingRanges returns ranges of bytes that haven't been received for a message
func (rb *ReceiveBuffer) GetMissingRanges(sequenceNum uint32, maxRanges int) [][2]uint32 {
	rb.mutex.Lock()
	message, exists := rb.messages[sequenceNum]
	rb.mutex.Unlock()

	if !exists {
		return nil
	}

	return message.GetMissingRanges(maxRanges)
}

// CleanExpired removes expired messages from the buffer
func (rb *ReceiveBuffer) CleanExpired() {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	var newQueue []uint32

	for _, seqNum := range rb.messageQueue {
		message, exists := rb.messages[seqNum]
		if !exists {
			continue
		}

		if message.TimeSinceLastUpdate() > rb.timeout {
			delete(rb.messages, seqNum)
		} else {
			newQueue = append(newQueue, seqNum)
		}
	}

	rb.messageQueue = newQueue
}

// IsComplete returns true if the message with the given sequence number is complete
func (rb *ReceiveBuffer) IsComplete(sequenceNum uint32) bool {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	message, exists := rb.messages[sequenceNum]
	if !exists {
		return false
	}

	return message.IsComplete()
}

// Close cleans up resources used by the receive buffer
func (rb *ReceiveBuffer) Close() error {
	rb.mutex.Lock()
	defer rb.mutex.Unlock()

	// Clear all messages
	rb.messages = make(map[uint32]*PartialMessage)
	rb.messageQueue = make([]uint32, 0)

	return nil
}
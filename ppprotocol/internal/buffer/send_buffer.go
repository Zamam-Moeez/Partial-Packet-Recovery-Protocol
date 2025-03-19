package buffer

import (
	"sync"
	"time"
)

// SendingState represents the current state of an outgoing message
type SendingState struct {
	// Original message data
	Data []byte

	// Bitmap tracking which parts have been acknowledged
	AckedMap []bool

	// Time the message was first sent
	FirstSent time.Time

	// Time of last retransmission attempt
	LastSent time.Time

	// Number of retransmission attempts
	RetryCount int

	// Flag indicating if the message is fully acknowledged
	Complete bool

	// Mutex for concurrent access
	mutex sync.Mutex
}

// NewSendingState creates a new sending state for the given data
func NewSendingState(data []byte) *SendingState {
	now := time.Now()
	return &SendingState{
		Data:       data,
		AckedMap:   make([]bool, len(data)),
		FirstSent:  now,
		LastSent:   now,
		RetryCount: 0,
		Complete:   false,
	}
}

// Acknowledge marks a range of bytes as acknowledged
func (ss *SendingState) Acknowledge(offset, length uint32) (bytesAcked uint32, complete bool) {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	// Mark the acknowledged bytes
	end := offset + length
	if end > uint32(len(ss.Data)) {
		end = uint32(len(ss.Data))
	}

	for i := offset; i < end; i++ {
		if !ss.AckedMap[i] {
			ss.AckedMap[i] = true
			bytesAcked++
		}
	}

	// Check if the entire message is acknowledged
	ss.Complete = true
	for _, acked := range ss.AckedMap {
		if !acked {
			ss.Complete = false
			break
		}
	}

	return bytesAcked, ss.Complete
}

// IsComplete returns true if the message is fully acknowledged
func (ss *SendingState) IsComplete() bool {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.Complete
}

// UpdateSendTime updates the last sent time
func (ss *SendingState) UpdateSendTime() {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	ss.LastSent = time.Now()
	ss.RetryCount++
}

// GetUnackedRanges returns ranges of bytes that haven't been acknowledged
// Returns slices of [start, end) ranges
func (ss *SendingState) GetUnackedRanges(maxRanges int, maxBytes uint32) [][2]uint32 {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()

	if ss.Complete {
		return nil
	}

	var ranges [][2]uint32
	start := uint32(0)
	inGap := false
	totalBytes := uint32(0)

	for i := uint32(0); i < uint32(len(ss.Data)); i++ {
		if !ss.AckedMap[i] && !inGap {
			// Start of an unacked range
			start = i
			inGap = true
		} else if ss.AckedMap[i] && inGap {
			// End of an unacked range
			ranges = append(ranges, [2]uint32{start, i})
			totalBytes += (i - start)
			inGap = false

			if len(ranges) >= maxRanges || totalBytes >= maxBytes {
				break
			}
		}
	}

	// Handle case where the unacked range extends to the end
	if inGap {
		end := uint32(len(ss.Data))
		ranges = append(ranges, [2]uint32{start, end})
	}

	return ranges
}

// TimeSinceLastSend returns the time elapsed since the last transmission
func (ss *SendingState) TimeSinceLastSend() time.Duration {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return time.Since(ss.LastSent)
}

// GetRetryCount returns the number of retransmission attempts
func (ss *SendingState) GetRetryCount() int {
	ss.mutex.Lock()
	defer ss.mutex.Unlock()
	return ss.RetryCount
}

// SendBuffer manages outgoing messages that are awaiting acknowledgment
type SendBuffer struct {
	messages       map[uint32]*SendingState
	mutex          sync.Mutex
	retryInterval  time.Duration
	maxRetries     int
	expireTime     time.Duration
}

// NewSendBuffer creates a new send buffer with the specified parameters
func NewSendBuffer(retryInterval time.Duration, maxRetries int, expireTime time.Duration) *SendBuffer {
	// Set defaults if not specified
	if retryInterval == 0 {
		retryInterval = 200 * time.Millisecond
	}
	if maxRetries == 0 {
		maxRetries = 10
	}
	if expireTime == 0 {
		expireTime = 30 * time.Second
	}

	return &SendBuffer{
		messages:      make(map[uint32]*SendingState),
		retryInterval: retryInterval,
		maxRetries:    maxRetries,
		expireTime:    expireTime,
	}
}

// AddMessage adds a new message to the buffer
func (sb *SendBuffer) AddMessage(sequenceNum uint32, data []byte) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	sb.messages[sequenceNum] = NewSendingState(data)
}

// Acknowledge updates the acknowledgment state for a message
func (sb *SendBuffer) Acknowledge(sequenceNum uint32, offset, length uint32) (bytesAcked uint32, complete bool) {
	sb.mutex.Lock()
	message, exists := sb.messages[sequenceNum]
	sb.mutex.Unlock()

	if !exists {
		return 0, false
	}

	bytesAcked, complete = message.Acknowledge(offset, length)

	if complete {
		sb.mutex.Lock()
		delete(sb.messages, sequenceNum)
		sb.mutex.Unlock()
	}

	return bytesAcked, complete
}

// GetPendingRetransmissions returns messages that need retransmission
func (sb *SendBuffer) GetPendingRetransmissions() map[uint32][][2]uint32 {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	pendingRetransmissions := make(map[uint32][][2]uint32)

	for seqNum, message := range sb.messages {
		// Skip if it's too soon to retry or if we've exceeded max retries
		if message.TimeSinceLastSend() < sb.retryInterval || message.GetRetryCount() >= sb.maxRetries {
			continue
		}

		// Get unacknowledged ranges that need retransmission
		ranges := message.GetUnackedRanges(5, 1400) // Limit to 5 ranges and 1400 bytes max
		if len(ranges) > 0 {
			pendingRetransmissions[seqNum] = ranges
			message.UpdateSendTime()
		}
	}

	return pendingRetransmissions
}

// GetMessageData returns the original message data
func (sb *SendBuffer) GetMessageData(sequenceNum uint32) ([]byte, bool) {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	message, exists := sb.messages[sequenceNum]
	if !exists {
		return nil, false
	}

	// Return a copy of the data
	data := make([]byte, len(message.Data))
	copy(data, message.Data)

	return data, true
}

// IsComplete returns true if the message is fully acknowledged
func (sb *SendBuffer) IsComplete(sequenceNum uint32) bool {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	message, exists := sb.messages[sequenceNum]
	if !exists {
		return true // If it's not in the buffer, consider it complete
	}

	return message.IsComplete()
}

// CleanExpired removes expired messages from the buffer
func (sb *SendBuffer) CleanExpired() {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	now := time.Now()

	for seqNum, message := range sb.messages {
		message.mutex.Lock()
		elapsed := now.Sub(message.FirstSent)
		retries := message.RetryCount
		message.mutex.Unlock()

		// Remove if expired or max retries exceeded
		if elapsed > sb.expireTime || retries >= sb.maxRetries {
			delete(sb.messages, seqNum)
		}
	}
}

// Close cleans up resources used by the send buffer
func (sb *SendBuffer) Close() error {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()

	// Clear all messages
	sb.messages = make(map[uint32]*SendingState)

	return nil
}

// Count returns the number of messages in the buffer
func (sb *SendBuffer) Count() int {
	sb.mutex.Lock()
	defer sb.mutex.Unlock()
	return len(sb.messages)
}
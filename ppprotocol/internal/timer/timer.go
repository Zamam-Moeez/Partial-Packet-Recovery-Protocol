package timer

import (
	"sync"
	"time"
)

// RetransmitCallback is called when a timer expires
type RetransmitCallback func(uint32)

// CleanupCallback is called periodically to clean up resources
type CleanupCallback func()

// Timer manages timeouts for retransmissions and other periodic tasks
type Timer struct {
	retransmitCallbacks map[uint32]RetransmitCallback
	cleanupCallback     CleanupCallback
	interval            time.Duration
	done                chan struct{}
	mutex               sync.Mutex
	started             bool
}

// NewTimer creates a new timer with the specified interval
func NewTimer(interval time.Duration, cleanupCallback CleanupCallback) *Timer {
	if interval == 0 {
		interval = 100 * time.Millisecond
	}

	return &Timer{
		retransmitCallbacks: make(map[uint32]RetransmitCallback),
		cleanupCallback:     cleanupCallback,
		interval:            interval,
		done:                make(chan struct{}),
		started:             false,
	}
}

// Start begins the timer execution
func (t *Timer) Start() {
	t.mutex.Lock()
	if t.started {
		t.mutex.Unlock()
		return
	}
	t.started = true
	t.mutex.Unlock()

	go t.run()
}

// Stop halts the timer execution
func (t *Timer) Stop() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	if !t.started {
		return
	}

	close(t.done)
	t.started = false
}

// run is the main timer loop
func (t *Timer) run() {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	cleanupTicker := time.NewTicker(5 * time.Second)
	defer cleanupTicker.Stop()

	for {
		select {
		case <-t.done:
			return
		case <-ticker.C:
			t.processRetransmissions()
		case <-cleanupTicker.C:
			if t.cleanupCallback != nil {
				t.cleanupCallback()
			}
		}
	}
}

// processRetransmissions calls registered retransmit callbacks
func (t *Timer) processRetransmissions() {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Make a copy to avoid holding the lock during callbacks
	callbacks := make(map[uint32]RetransmitCallback, len(t.retransmitCallbacks))
	for seqNum, callback := range t.retransmitCallbacks {
		callbacks[seqNum] = callback
	}

	t.mutex.Unlock()

	// Call each callback
	for seqNum, callback := range callbacks {
		callback(seqNum)
	}

	t.mutex.Lock()
}

// SetRetransmit registers a callback for the given sequence number
func (t *Timer) SetRetransmit(sequenceNum uint32, callback RetransmitCallback) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.retransmitCallbacks[sequenceNum] = callback
}

// ClearRetransmit removes the callback for the given sequence number
func (t *Timer) ClearRetransmit(sequenceNum uint32) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	delete(t.retransmitCallbacks, sequenceNum)
}

// RetransmitCount returns the number of registered retransmit callbacks
func (t *Timer) RetransmitCount() int {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	return len(t.retransmitCallbacks)
}

// RTTEstimator manages round-trip time estimation for adaptive timeouts
type RTTEstimator struct {
	srtt          float64 // Smoothed round-trip time
	rttVar        float64 // Round-trip time variation
	rto           float64 // Retransmission timeout
	alpha         float64 // Smoothing factor for SRTT (typically 0.125)
	beta          float64 // Smoothing factor for RTTVAR (typically 0.25)
	minRTO        float64 // Minimum RTO in milliseconds
	maxRTO        float64 // Maximum RTO in milliseconds
	mutex         sync.Mutex
	initialized   bool
}

// NewRTTEstimator creates a new RTT estimator with default parameters
func NewRTTEstimator() *RTTEstimator {
	return &RTTEstimator{
		srtt:        0,
		rttVar:      0,
		rto:         300, // Initial RTO of 300ms
		alpha:       0.125,
		beta:        0.25,
		minRTO:      100,  // Minimum RTO of 100ms
		maxRTO:      30000, // Maximum RTO of 30 seconds
		initialized: false,
	}
}

// Update updates the RTT estimate with a new measurement
func (r *RTTEstimator) Update(rtt float64) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	if !r.initialized {
		// First measurement
		r.srtt = rtt
		r.rttVar = rtt / 2
		r.initialized = true
	} else {
		// Update estimates using Jacobson/Karels algorithm (RFC 6298)
		r.rttVar = (1 - r.beta) * r.rttVar + r.beta * abs(r.srtt - rtt)
		r.srtt = (1 - r.alpha) * r.srtt + r.alpha * rtt
	}

	// Update RTO
	r.rto = r.srtt + 4 * r.rttVar

	// Clamp RTO to min/max values
	if r.rto < r.minRTO {
		r.rto = r.minRTO
	}
	if r.rto > r.maxRTO {
		r.rto = r.maxRTO
	}
}

// GetRTO returns the current retransmission timeout in milliseconds
func (r *RTTEstimator) GetRTO() time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	return time.Duration(r.rto) * time.Millisecond
}

// BackoffRTO increases the RTO exponentially (for retransmissions)
func (r *RTTEstimator) BackoffRTO() time.Duration {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Double the RTO (capped at maxRTO)
	r.rto = min(r.rto * 2, r.maxRTO)
	
	return time.Duration(r.rto) * time.Millisecond
}

// abs returns the absolute value of a float64
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// min returns the minimum of two float64s
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
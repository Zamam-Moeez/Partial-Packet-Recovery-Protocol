package ppprotocol

import (
	"errors"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/internal/buffer"
	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/internal/packet"
	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/internal/timer"
)

// Connection states
const (
	StateInitial int32 = iota
	StateConnecting
	StateConnected
	StateClosing
	StateClosed
)

// Common errors
var (
	ErrConnectionClosed = errors.New("connection is closed")
	ErrTimeout          = errors.New("operation timed out")
	ErrInvalidState     = errors.New("invalid connection state")
	ErrMessageTooLarge  = errors.New("message exceeds maximum size")
)

// Connection represents a PPP connection
type Connection struct {
	// Socket and addressing
	conn        *net.UDPConn
	localAddr   *net.UDPAddr
	remoteAddr  *net.UDPAddr
	listener    *Listener

	// Connection state
	state       atomic.Int32
	closeOnce   sync.Once
	closeChan   chan struct{}
	errorChan   chan error
	
	// Sequence tracking
	nextSeqNum  atomic.Uint32
	expectedSeqNum atomic.Uint32
	
	// Buffers for reliability
	sendBuffer   *buffer.SendBuffer
	receiveBuffer *buffer.ReceiveBuffer
	
	// Timing and retransmission
	timer        *timer.Timer
	rttEstimator *timer.RTTEstimator
	
	// Statistics
	stats        *Stats
	
	// Configuration
	options      *Options
	
	// Synchronization
	sendMutex    sync.Mutex
	receiveMutex sync.Mutex
}

// Listener accepts incoming PPP connections
type Listener struct {
	conn        *net.UDPConn
	connections map[string]*Connection
	connMutex   sync.Mutex
	closeChan   chan struct{}
	closed      atomic.Bool
	options     *Options
}

// dial establishes a new outgoing connection
func dial(remoteAddr string, options *Options) (*Connection, error) {
	// Apply default options if not provided
	if options == nil {
		options = DefaultOptions()
	}

	// Resolve remote address
	raddr, err := net.ResolveUDPAddr("udp", remoteAddr)
	if err != nil {
		return nil, err
	}

	// Create local socket
	conn, err := net.ListenUDP("udp", nil) // Bind to a random port
	if err != nil {
		return nil, err
	}

	// Create connection
	c := &Connection{
		conn:        conn,
		remoteAddr:  raddr,
		closeChan:   make(chan struct{}),
		errorChan:   make(chan error, 10),
		sendBuffer:  buffer.NewSendBuffer(options.RetransmitInterval, options.MaxRetries, options.IdleTimeout),
		receiveBuffer: buffer.NewReceiveBuffer(options.IdleTimeout),
		stats:       NewStats(),
		options:     options,
	}

	// Initialize state
	c.state.Store(StateInitial)
	c.nextSeqNum.Store(1) // Start from 1, 0 is reserved
	c.expectedSeqNum.Store(1)

	// Get local address
	c.localAddr = conn.LocalAddr().(*net.UDPAddr)

	// Initialize RTT estimator
	c.rttEstimator = timer.NewRTTEstimator()

	// Initialize timer for retransmissions
	c.timer = timer.NewTimer(options.RetransmitInterval/2, func() {
		c.cleanupExpired()
	})

	// Start background goroutines
	go c.receiveLoop()
	go c.processRetransmissions()
	c.timer.Start()

	// Perform handshake
	err = c.handshake()
	if err != nil {
		c.Close()
		return nil, err
	}

	return c, nil
}

// Dial establishes a new outgoing connection
func Dial(remoteAddr string, options *Options) (*Connection, error) {
	return dial(remoteAddr, options)
}

// newConnection creates a new incoming connection from an accepted packet
func newConnection(conn *net.UDPConn, remoteAddr *net.UDPAddr, listener *Listener, options *Options) *Connection {
	c := &Connection{
		conn:        conn,
		localAddr:   conn.LocalAddr().(*net.UDPAddr),
		remoteAddr:  remoteAddr,
		listener:    listener,
		closeChan:   make(chan struct{}),
		errorChan:   make(chan error, 10),
		sendBuffer:  buffer.NewSendBuffer(options.RetransmitInterval, options.MaxRetries, options.IdleTimeout),
		receiveBuffer: buffer.NewReceiveBuffer(options.IdleTimeout),
		stats:       NewStats(),
		options:     options,
	}

	// Initialize state
	c.state.Store(StateConnecting)
	c.nextSeqNum.Store(1)
	c.expectedSeqNum.Store(1)

	// Initialize RTT estimator
	c.rttEstimator = timer.NewRTTEstimator()

	// Initialize timer for retransmissions
	c.timer = timer.NewTimer(options.RetransmitInterval/2, func() {
		c.cleanupExpired()
	})

	// Start background goroutines
	go c.receiveLoop()
	go c.processRetransmissions()
	c.timer.Start()

	return c
}

// Listen creates a new listener for incoming connections
func Listen(address string, options *Options) (*Listener, error) {
	// Apply default options if not provided
	if options == nil {
		options = DefaultOptions()
	}

	// Resolve local address
	laddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return nil, err
	}

	// Create socket
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		return nil, err
	}

	// Create listener
	l := &Listener{
		conn:        conn,
		connections: make(map[string]*Connection),
		closeChan:   make(chan struct{}),
		options:     options,
	}

	// Start accepting connections
	go l.acceptLoop()

	return l, nil
}

// Accept waits for and returns the next connection
func (l *Listener) Accept() (*Connection, error) {
	// This would need a channel to queue accepted connections
	// For simplicity, we'll just return an error for now
	return nil, errors.New("not implemented")
}

// acceptLoop processes incoming packets and creates new connections
func (l *Listener) acceptLoop() {
	buffer := make([]byte, 2048)

	for {
		// Check if listener is closed
		if l.closed.Load() {
			return
		}

		// Set read deadline for non-blocking read
		l.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		// Read packet
		n, addr, err := l.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Just a timeout, continue
				continue
			}
			
			// Handle other errors
			if l.closed.Load() {
				return
			}
			
			log.Printf("Error reading from UDP: %v", err)
			continue
		}

		// Parse packet
		p, err := packet.Deserialize(buffer[:n])
		if err != nil {
			log.Printf("Error deserializing packet: %v", err)
			continue
		}

		// Get connection key (remote address)
		connKey := addr.String()

		// Check if this is a known connection
		l.connMutex.Lock()
		conn, exists := l.connections[connKey]
		
		if !exists && p.IsSyn() {
			// New connection (SYN packet)
			conn = newConnection(l.conn, addr, l, l.options)
			l.connections[connKey] = conn
			
			// TODO: Implement a proper Accept() queue
		}
		l.connMutex.Unlock()

		if conn != nil {
			// Forward packet to the connection
			conn.handlePacket(p)
		} else if !p.IsSyn() {
			// Not a SYN and no existing connection, send RST
			rst := packet.CreateResetPacket(p.SequenceNum)
			rstBytes, _ := rst.Serialize()
			l.conn.WriteToUDP(rstBytes, addr)
		}
	}
}

// Close stops the listener and all associated connections
func (l *Listener) Close() error {
	if l.closed.CompareAndSwap(false, true) {
		close(l.closeChan)
		
		// Close all connections
		l.connMutex.Lock()
		for _, conn := range l.connections {
			conn.Close()
		}
		l.connections = make(map[string]*Connection)
		l.connMutex.Unlock()
		
		return l.conn.Close()
	}
	
	return nil
}

// handshake performs the connection establishment handshake
func (c *Connection) handshake() error {
	if !c.state.CompareAndSwap(StateInitial, StateConnecting) {
		return ErrInvalidState
	}

	// Create SYN packet
	syn := &packet.Packet{
		SequenceNum: 0,
		TotalSize:   uint16(packet.HeaderSize + packet.ChecksumSize),
		Flags:       packet.FlagSYN,
		Offset:      0,
		Payload:     []byte{},
	}

	// Serialize and send SYN
	synBytes, err := syn.Serialize()
	if err != nil {
		return err
	}

	// Send SYN packet
	_, err = c.conn.WriteToUDP(synBytes, c.remoteAddr)
	if err != nil {
		return err
	}

	// Wait for SYN-ACK with timeout
	timeout := time.NewTimer(c.options.ConnectionTimeout)
	defer timeout.Stop()

	for {
		select {
		case <-timeout.C:
			return ErrTimeout
		case err := <-c.errorChan:
			return err
		case <-c.closeChan:
			return ErrConnectionClosed
		default:
			// Check connection state
			if c.state.Load() == StateConnected {
				return nil
			}
			
			// Small sleep to avoid CPU spinning
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// receiveLoop handles incoming packets
func (c *Connection) receiveLoop() {
	buffer := make([]byte, 2048)

	for {
		// Check if connection is closed
		if c.state.Load() >= StateClosing {
			return
		}

		// If we're part of a listener, the listener handles packet reception
		if c.listener != nil {
			// Just wait for close signal
			select {
			case <-c.closeChan:
				return
			case <-time.After(100 * time.Millisecond):
				continue
			}
		}

		// Set read deadline for non-blocking read
		c.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))

		// Read packet
		n, addr, err := c.conn.ReadFromUDP(buffer)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				// Just a timeout, continue
				continue
			}
			
			// Handle other errors
			if c.state.Load() >= StateClosing {
				return
			}
			
			c.reportError(err)
			continue
		}

		// Verify sender
		if !addr.IP.Equal(c.remoteAddr.IP) || addr.Port != c.remoteAddr.Port {
			// Packet from unknown sender, ignore
			continue
		}

		// Parse packet
		p, err := packet.Deserialize(buffer[:n])
		if err != nil {
			c.stats.RecordChecksumError()
			continue
		}

		// Handle the packet
		c.handlePacket(p)
	}
}

// handlePacket processes an incoming packet
func (c *Connection) handlePacket(p *packet.Packet) {
	// Update statistics
	c.stats.RecordPacketReceived(uint64(p.TotalSize), p.IsPartial())

	// Handle based on packet type
	if p.IsAck() {
		c.handleAck(p)
	} else if p.IsSyn() {
		c.handleSyn(p)
	} else if p.IsFin() {
		c.handleFin(p)
	} else if p.IsRst() {
		c.handleRst(p)
	} else {
		c.handleData(p)
	}
}

// handleData processes a data packet
func (c *Connection) handleData(p *packet.Packet) {
	// Only process data packets in connected state
	if c.state.Load() != StateConnected {
		return
	}

	// Process the data
	received, complete := c.receiveBuffer.ProcessPacket(
		p.SequenceNum,
		p.Payload,
		p.Offset,
		uint32(len(p.Payload)),
	)

	// Send acknowledgment
	ack := packet.CreateAckPacket(p.SequenceNum, received)
	ackBytes, _ := ack.Serialize()

	if c.listener != nil {
		c.listener.conn.WriteToUDP(ackBytes, c.remoteAddr)
	} else {
		c.conn.WriteToUDP(ackBytes, c.remoteAddr)
	}

	// If the packet was partial, calculate bytes saved
	if p.IsPartial() {
		// Assume the packet would have been full size if not partial
		c.stats.RecordBytesSaved(uint64(packet.MaxPayloadSize - len(p.Payload)))
	}
}

// handleAck processes an acknowledgment packet
func (c *Connection) handleAck(p *packet.Packet) {
	// If this is a SYN-ACK during connection establishment
	if c.state.Load() == StateConnecting && p.SequenceNum == 0 {
		// Connection established
		c.state.Store(StateConnected)
		return
	}

	// Regular ACK processing
	bytesAcked, complete := c.sendBuffer.Acknowledge(
		p.SequenceNum,
		p.Offset,
		uint32(p.TotalSize - packet.HeaderSize - packet.ChecksumSize),
	)

	// Update RTT estimator if we have a valid acknowledgment
	if bytesAcked > 0 {
		// TODO: Implement RTT calculation
		// c.rttEstimator.Update(...)
	}
}

// handleSyn processes a SYN packet
func (c *Connection) handleSyn(p *packet.Packet) {
	// If we're the client, ignore SYN packets
	if c.listener == nil {
		return
	}

	// If we're in connecting state, send SYN-ACK
	if c.state.Load() == StateConnecting {
		synAck := &packet.Packet{
			SequenceNum: 0,
			TotalSize:   uint16(packet.HeaderSize + packet.ChecksumSize),
			Flags:       packet.FlagSYN | packet.FlagACK,
			Offset:      0,
			Payload:     []byte{},
		}

		synAckBytes, _ := synAck.Serialize()
		c.listener.conn.WriteToUDP(synAckBytes, c.remoteAddr)

		// Move to connected state
		c.state.Store(StateConnected)
	}
}

// handleFin processes a FIN packet
func (c *Connection) handleFin(p *packet.Packet) {
	// Send FIN-ACK
	finAck := &packet.Packet{
		SequenceNum: p.SequenceNum,
		TotalSize:   uint16(packet.HeaderSize + packet.ChecksumSize),
		Flags:       packet.FlagFIN | packet.FlagACK,
		Offset:      0,
		Payload:     []byte{},
	}

	finAckBytes, _ := finAck.Serialize()
	
	if c.listener != nil {
		c.listener.conn.WriteToUDP(finAckBytes, c.remoteAddr)
	} else {
		c.conn.WriteToUDP(finAckBytes, c.remoteAddr)
	}

	// Move to closing state
	c.state.Store(StateClosing)

	// Close the connection
	c.Close()
}

// handleRst processes a RST packet
func (c *Connection) handleRst(p *packet.Packet) {
	// Connection was reset by peer
	c.state.Store(StateClosing)
	c.reportError(errors.New("connection reset by peer"))
	c.Close()
}

// reportError sends an error to the error channel
func (c *Connection) reportError(err error) {
	select {
	case c.errorChan <- err:
		// Error sent
	default:
		// Channel full, log error
		log.Printf("Connection error: %v", err)
	}
}

// processRetransmissions handles packet retransmissions
func (c *Connection) processRetransmissions() {
	ticker := time.NewTicker(c.options.RetransmitInterval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-c.closeChan:
			return
		case <-ticker.C:
			// Get pending retransmissions
			pendingRetransmissions := c.sendBuffer.GetPendingRetransmissions()

			for seqNum, ranges := range pendingRetransmissions {
				// Get original data
				data, exists := c.sendBuffer.GetMessageData(seqNum)
				if !exists {
					continue
				}

				// Send partial packets for each range
				for _, r := range ranges {
					start, end := r[0], r[1]
					if end > uint32(len(data)) {
						end = uint32(len(data))
					}

					// Create partial packet
					partialData := data[start:end]
					p, err := packet.CreatePartialPacket(
						seqNum,
						uint16(packet.HeaderSize + len(data) + packet.ChecksumSize),
						start,
						partialData,
					)
					if err != nil {
						continue
					}

					// Serialize and send
					packetBytes, _ := p.Serialize()
					
					if c.listener != nil {
						c.listener.conn.WriteToUDP(packetBytes, c.remoteAddr)
					} else {
						c.conn.WriteToUDP(packetBytes, c.remoteAddr)
					}

					// Update statistics
					c.stats.RecordPacketSent(uint64(p.TotalSize), true)
					c.stats.RecordRetransmit()
				}
			}
		}
	}
}

// cleanupExpired removes expired packets from buffers
func (c *Connection) cleanupExpired() {
	c.sendBuffer.CleanExpired()
	c.receiveBuffer.CleanExpired()

	// If we're in connected state, check if we need to send a keep-alive
	if c.state.Load() == StateConnected && c.stats.GetIdleTime() >= c.options.KeepAliveInterval {
		c.sendKeepAlive()
	}
}

// sendKeepAlive sends a keep-alive packet
func (c *Connection) sendKeepAlive() {
	keepAlive := &packet.Packet{
		SequenceNum: 0,
		TotalSize:   uint16(packet.HeaderSize + packet.ChecksumSize),
		Flags:       packet.FlagKeepAlive,
		Offset:      0,
		Payload:     []byte{},
	}

	keepAliveBytes, _ := keepAlive.Serialize()
	
	if c.listener != nil {
		c.listener.conn.WriteToUDP(keepAliveBytes, c.remoteAddr)
	} else {
		c.conn.WriteToUDP(keepAliveBytes, c.remoteAddr)
	}
}

// Send transmits data to the remote endpoint
func (c *Connection) Send(data []byte) error {
	// Check connection state
	if c.state.Load() != StateConnected {
		return ErrConnectionClosed
	}

	// Check message size
	if len(data) > c.options.MaxPayloadSize*c.options.WindowSize {
		return ErrMessageTooLarge
	}

	c.sendMutex.Lock()
	defer c.sendMutex.Unlock()

	// Get next sequence number
	seqNum := c.nextSeqNum.Add(1) - 1

	// Buffer the message
	c.sendBuffer.AddMessage(seqNum, data)

	// Split into packets if necessary
	maxPayload := c.options.MaxPayloadSize
	
	for offset := 0; offset < len(data); offset += maxPayload {
		// Calculate end of this chunk
		end := offset + maxPayload
		if end > len(data) {
			end = len(data)
		}

		// Create packet
		p, err := packet.NewPacket(
			seqNum,
			0, // No flags for regular data
			uint32(offset),
			data[offset:end],
		)
		if err != nil {
			// Continue with other chunks even if one fails
			log.Printf("Error creating packet: %v", err)
			continue
		}

		// Serialize packet
		packetBytes, err := p.Serialize()
		if err != nil {
			continue
		}

		// Send packet
		if c.listener != nil {
			_, err = c.listener.conn.WriteToUDP(packetBytes, c.remoteAddr)
		} else {
			_, err = c.conn.WriteToUDP(packetBytes, c.remoteAddr)
		}
		
		if err != nil {
			return err
		}

		// Update statistics
		c.stats.RecordPacketSent(uint64(p.TotalSize), false)
	}

	return nil
}

// Receive receives a message from the remote endpoint
func (c *Connection) Receive(timeout time.Duration) ([]byte, error) {
	// Check connection state
	if c.state.Load() != StateConnected {
		return nil, ErrConnectionClosed
	}

	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		timeoutChan = timer.C
	} else {
		// No timeout
		timeoutChan = make(chan time.Time)
	}

	for {
		// Check for complete message
		seqNum, data, complete := c.receiveBuffer.GetNextComplete()
		if complete {
			return data, nil
		}

		// Wait for more data or timeout
		select {
		case <-timeoutChan:
			return nil, ErrTimeout
		case err := <-c.errorChan:
			return nil, err
		case <-c.closeChan:
			return nil, ErrConnectionClosed
		case <-time.After(10 * time.Millisecond):
			// Continue checking
		}
	}
}

// Close terminates the connection
func (c *Connection) Close() error {
	c.closeOnce.Do(func() {
		// Update state
		prevState := c.state.Swap(StateClosing)
		
		// Only send FIN if we were connected
		if prevState == StateConnected {
			// Send FIN packet
			fin := &packet.Packet{
				SequenceNum: c.nextSeqNum.Add(1) - 1,
				TotalSize:   uint16(packet.HeaderSize + packet.ChecksumSize),
				Flags:       packet.FlagFIN,
				Offset:      0,
				Payload:     []byte{},
			}

			finBytes, _ := fin.Serialize()
			
			if c.listener != nil {
				c.listener.conn.WriteToUDP(finBytes, c.remoteAddr)
			} else {
				c.conn.WriteToUDP(finBytes, c.remoteAddr)
			}
		}
		
		// Signal close
		close(c.closeChan)
		
		// Stop timer
		c.timer.Stop()
		
		// Clean up buffers
		c.sendBuffer.Close()
		c.receiveBuffer.Close()
		
		// Set state to closed
		c.state.Store(StateClosed)
		
		// Remove from listener if applicable
		if c.listener != nil {
			c.listener.connMutex.Lock()
			delete(c.listener.connections, c.remoteAddr.String())
			c.listener.connMutex.Unlock()
		} else {
			// Close socket if not part of a listener
			c.conn.Close()
		}
	})
	
	return nil
}

// LocalAddr returns the local network address
func (c *Connection) LocalAddr() net.Addr {
	return c.localAddr
}

// RemoteAddr returns the remote network address
func (c *Connection) RemoteAddr() net.Addr {
	return c.remoteAddr
}

// State returns the current connection state
func (c *Connection) State() string {
	switch c.state.Load() {
	case StateInitial:
		return "Initial"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// Stats returns the connection statistics
func (c *Connection) Stats() *Stats {
	return c.stats
}

// SetOption sets a connection option
func (c *Connection) SetOption(option string, value interface{}) error {
	// This would implement runtime configuration changes
	return errors.New("not implemented")
}
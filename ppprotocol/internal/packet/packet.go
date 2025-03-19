package packet

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
)

const (
	// HeaderSize is the size of the packet header in bytes
	HeaderSize = 12 // 4 bytes seq + 2 bytes total size + 2 bytes flags + 4 bytes offset

	// ChecksumSize is the size of the packet checksum in bytes
	ChecksumSize = 4 // 4 bytes CRC32

	// MaxPayloadSize is the maximum size of the packet payload in bytes
	MaxPayloadSize = 1400

	// MaxPacketSize is the maximum size of a complete packet
	MaxPacketSize = HeaderSize + MaxPayloadSize + ChecksumSize
)

// Flags for packet control
const (
	FlagNone    uint16 = 0
	FlagSYN     uint16 = 1 << 0 // Connection establishment
	FlagACK     uint16 = 1 << 1 // Acknowledgment
	FlagFIN     uint16 = 1 << 2 // Connection termination
	FlagRST     uint16 = 1 << 3 // Connection reset
	FlagPartial uint16 = 1 << 4 // Partial packet
	FlagKeepAlive uint16 = 1 << 5 // Keep-alive packet
)

// ErrChecksumMismatch is returned when a packet's checksum is invalid
var ErrChecksumMismatch = errors.New("checksum mismatch")

// ErrPacketTooSmall is returned when a packet is too small to be valid
var ErrPacketTooSmall = errors.New("packet too small")

// ErrPacketTooLarge is returned when a packet exceeds MaxPacketSize
var ErrPacketTooLarge = errors.New("packet too large")

// Packet represents a protocol data unit for transmission
type Packet struct {
	// SequenceNum is the packet sequence number
	SequenceNum uint32

	// TotalSize is the total size of the packet in bytes
	TotalSize uint16

	// Flags contains control flags
	Flags uint16

	// Offset is the starting offset for partial packets
	Offset uint32

	// Payload contains the packet data
	Payload []byte

	// Checksum is the CRC32 checksum of the packet
	Checksum uint32
}

// NewPacket creates a new packet with the given parameters
func NewPacket(sequenceNum uint32, flags uint16, offset uint32, payload []byte) (*Packet, error) {
	if len(payload) > MaxPayloadSize {
		return nil, ErrPacketTooLarge
	}

	return &Packet{
		SequenceNum: sequenceNum,
		TotalSize:   uint16(HeaderSize + len(payload) + ChecksumSize),
		Flags:       flags,
		Offset:      offset,
		Payload:     payload,
		Checksum:    0, // Computed during serialization
	}, nil
}

// Serialize converts a packet to bytes for transmission
func (p *Packet) Serialize() ([]byte, error) {
	totalLen := int(p.TotalSize)
	if totalLen > MaxPacketSize {
		return nil, ErrPacketTooLarge
	}

	buf := make([]byte, totalLen)

	// Write header
	binary.BigEndian.PutUint32(buf[0:4], p.SequenceNum)
	binary.BigEndian.PutUint16(buf[4:6], p.TotalSize)
	binary.BigEndian.PutUint16(buf[6:8], p.Flags)
	binary.BigEndian.PutUint32(buf[8:12], p.Offset)

	// Copy payload
	copy(buf[HeaderSize:HeaderSize+len(p.Payload)], p.Payload)

	// Calculate and write checksum (of header and payload)
	checksum := crc32.ChecksumIEEE(buf[:HeaderSize+len(p.Payload)])
	binary.BigEndian.PutUint32(buf[HeaderSize+len(p.Payload):], checksum)

	return buf, nil
}

// Deserialize parses bytes into a packet
func Deserialize(data []byte) (*Packet, error) {
	if len(data) < HeaderSize+ChecksumSize {
		return nil, ErrPacketTooSmall
	}

	totalSize := binary.BigEndian.Uint16(data[4:6])
	if len(data) < int(totalSize) {
		return nil, ErrPacketTooSmall
	}

	p := &Packet{
		SequenceNum: binary.BigEndian.Uint32(data[0:4]),
		TotalSize:   totalSize,
		Flags:       binary.BigEndian.Uint16(data[6:8]),
		Offset:      binary.BigEndian.Uint32(data[8:12]),
		Payload:     data[HeaderSize : len(data)-ChecksumSize],
	}

	// Verify checksum
	expectedChecksum := binary.BigEndian.Uint32(data[len(data)-ChecksumSize:])
	actualChecksum := crc32.ChecksumIEEE(data[:len(data)-ChecksumSize])

	if expectedChecksum != actualChecksum {
		return nil, ErrChecksumMismatch
	}

	return p, nil
}

// IsPartial returns true if this is a partial packet
func (p *Packet) IsPartial() bool {
	return p.Flags&FlagPartial != 0
}

// IsAck returns true if this is an acknowledgment packet
func (p *Packet) IsAck() bool {
	return p.Flags&FlagACK != 0
}

// IsSyn returns true if this is a connection establishment packet
func (p *Packet) IsSyn() bool {
	return p.Flags&FlagSYN != 0
}

// IsFin returns true if this is a connection termination packet
func (p *Packet) IsFin() bool {
	return p.Flags&FlagFIN != 0
}

// IsRst returns true if this is a connection reset packet
func (p *Packet) IsRst() bool {
	return p.Flags&FlagRST != 0
}

// IsKeepAlive returns true if this is a keep-alive packet
func (p *Packet) IsKeepAlive() bool {
	return p.Flags&FlagKeepAlive != 0
}

// AckType defines different types of acknowledgments
type AckType uint8

const (
	// AckFull indicates a complete packet has been received
	AckFull AckType = iota
	
	// AckPartial indicates only a portion of the packet has been received
	AckPartial
	
	// AckDuplicate indicates a duplicate packet was received
	AckDuplicate
)

// CreatePartialPacket creates a new partial packet with the given parameters
func CreatePartialPacket(sequenceNum uint32, originalSize uint16, offset uint32, payload []byte) (*Packet, error) {
	if len(payload) > MaxPayloadSize {
		return nil, ErrPacketTooLarge
	}

	return &Packet{
		SequenceNum: sequenceNum,
		TotalSize:   originalSize,
		Flags:       FlagPartial,
		Offset:      offset,
		Payload:     payload,
		Checksum:    0, // Computed during serialization
	}, nil
}

// CreateAckPacket creates a new acknowledgment packet
func CreateAckPacket(sequenceNum uint32, bytesReceived uint32) *Packet {
	// Acknowledgment packets have empty payloads
	return &Packet{
		SequenceNum: sequenceNum,
		TotalSize:   uint16(HeaderSize + ChecksumSize),
		Flags:       FlagACK,
		Offset:      bytesReceived, // Use offset field to store bytes received
		Payload:     []byte{},
		Checksum:    0, // Computed during serialization
	}
}

// CreateSynAckPacket creates a SYN-ACK packet for connection establishment
func CreateSynAckPacket() *Packet {
	return &Packet{
		SequenceNum: 0,
		TotalSize:   uint16(HeaderSize + ChecksumSize),
		Flags:       FlagSYN | FlagACK,
		Offset:      0,
		Payload:     []byte{},
		Checksum:    0, // Computed during serialization
	}
}

// CreateFinAckPacket creates a FIN-ACK packet for connection termination
func CreateFinAckPacket(sequenceNum uint32) *Packet {
	return &Packet{
		SequenceNum: sequenceNum,
		TotalSize:   uint16(HeaderSize + ChecksumSize),
		Flags:       FlagFIN | FlagACK,
		Offset:      0,
		Payload:     []byte{},
		Checksum:    0, // Computed during serialization
	}
}

// CreateResetPacket creates a RST packet for connection reset
func CreateResetPacket(sequenceNum uint32) *Packet {
	return &Packet{
		SequenceNum: sequenceNum,
		TotalSize:   uint16(HeaderSize + ChecksumSize),
		Flags:       FlagRST,
		Offset:      0,
		Payload:     []byte{},
		Checksum:    0, // Computed during serialization
	}
}

// BytesReceived returns the number of bytes received (for ACK packets)
func (p *Packet) BytesReceived() uint32 {
	if p.IsAck() {
		return p.Offset
	}
	return 0
}

// AckTypeSent returns the type of acknowledgment
func (p *Packet) AckTypeSent() AckType {
	if !p.IsAck() {
		return AckFull // Not an ACK
	}
	
	if p.BytesReceived() == uint32(p.TotalSize) {
		return AckFull
	}
	
	return AckPartial
}

// IsFullyAcknowledged returns true if this ACK indicates full reception
func (p *Packet) IsFullyAcknowledged() bool {
	return p.IsAck() && p.BytesReceived() == uint32(p.TotalSize)
}
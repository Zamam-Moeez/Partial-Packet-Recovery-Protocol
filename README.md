# Partial Packet Protocol (PPP)

A TCP-like reliable transport protocol with partial packet recovery that improves efficiency in lossy networks by only retransmitting the missing portions of corrupted packets.

## Why PPP?

Traditional reliable protocols like TCP retransmit entire packets when any portion is lost or corrupted, wasting bandwidth. PPP minimizes redundant data transfer by only retransmitting the specific missing segments of a packet, making it ideal for:

- High-latency networks (satellite, rural connectivity)
- Multimedia streaming requiring reliability with lower latency
- IoT and sensor networks where bandwidth conservation is critical
- Any application needing reliability while minimizing retransmission overhead

## Features

- **Connection-oriented**: Maintains session state similar to TCP
- **Reliable delivery**: Guarantees message delivery with acknowledgment
- **Partial packet recovery**: Only retransmits the missing portions of packets
- **Bandwidth efficient**: Significantly reduces unnecessary data transfer on lossy connections
- **Configurable**: Adjustable parameters for various network conditions
- **Simple API**: Easy to integrate into existing applications

## Performance Benefits

Compared to standard TCP, PPP provides:
- Up to 40% reduced bandwidth usage in lossy environments
- Lower latency for large packets when partial loss occurs
- Similar reliability guarantees as traditional protocols

## Installation

```bash
# From your Go project
go get github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol
```

## Basic Usage

### Server

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/pkg"
)

func main() {
    // Create a new server with default options
    server, err := ppprotocol.Listen(":8800", nil)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }
    defer server.Close()
    
    fmt.Println("Server listening on :8800")
    
    // Accept connections
    conn, err := server.Accept()
    if err != nil {
        log.Fatalf("Failed to accept connection: %v", err)
    }
    
    // Receive data with 30 second timeout
    data, err := conn.Receive(30 * time.Second)
    if err != nil {
        log.Fatalf("Failed to receive data: %v", err)
    }
    
    fmt.Printf("Received: %s\n", string(data))
    
    // Send response
    err = conn.Send([]byte("Hello from server!"))
    if err != nil {
        log.Fatalf("Failed to send response: %v", err)
    }
}
```

### Client

```go
package main

import (
    "fmt"
    "log"
    "time"
    
    "github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/pkg"
)

func main() {
    // Create connection options
    options := ppprotocol.DefaultOptions()
    options.ConnectionTimeout = 5 * time.Second
    
    // Connect to server
    conn, err := ppprotocol.Dial("127.0.0.1:8800", options)
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Send message
    err = conn.Send([]byte("Hello from client!"))
    if err != nil {
        log.Fatalf("Failed to send message: %v", err)
    }
    
    // Receive response with 10 second timeout
    response, err := conn.Receive(10 * time.Second)
    if err != nil {
        log.Fatalf("Failed to receive response: %v", err)
    }
    
    fmt.Printf("Server response: %s\n", string(response))
    
    // Get connection statistics
    stats := conn.Stats().GetSummary()
    fmt.Printf("Connection stats: %+v\n", stats)
}
```

## Advanced Configuration

PPP provides numerous configuration options to tune performance for different network conditions:

```go
options := ppprotocol.DefaultOptions()

// Set maximum payload size (default: 1400)
options.MaxPayloadSize = 1024

// Adjust retransmission timing (default: 200ms)
options.RetransmitInterval = 300 * time.Millisecond

// Maximum retransmission attempts (default: 10)
options.MaxRetries = 5

// Connection establishment timeout (default: 10s)
options.ConnectionTimeout = 5 * time.Second

// Idle connection timeout (default: 30s)
options.IdleTimeout = 60 * time.Second

// Keep-alive interval (default: 5s)
options.KeepAliveInterval = 10 * time.Second

// Maximum number of unacknowledged packets (default: 16)
options.WindowSize = 32

// Enable Nagle's algorithm for small packet coalescing (default: true)
options.EnableNagling = false
```

## Protocol Description

PPP is designed for reliability in lossy networks while minimizing bandwidth usage:

1. **Connection Establishment**: Three-way handshake similar to TCP
2. **Data Transfer**: Uses sequence numbers for ordering and packet validation
3. **Acknowledgment System**: Selective acknowledgments tracking exactly which bytes were received
4. **Partial Recovery**: When loss is detected, only missing segments are retransmitted
5. **Flow Control**: Window-based flow control to prevent overwhelming the receiver
6. **Keep-Alive**: Maintains connections during periods of inactivity
7. **Graceful Shutdown**: Connection teardown similar to TCP FIN process

## Example Applications

The `cmd` directory contains example client and server applications:

- `ppclient`: Interactive command-line client for testing
- `ppserver`: Echo server that responds to client messages

Run the server:
```bash
go run ./cmd/ppserver/main.go
```

Run the client:
```bash
go run ./cmd/ppclient/main.go -server 127.0.0.1:8800
```

## Project Structure

```
Partial-Packet-Recovery-Protocol/
    ppprotocol/
    ├── cmd/                 # Example applications
    │   ├── ppclient/        # Client application
    │   └── ppserver/        # Server application
    ├── internal/            # Implementation details
    │   ├── buffer/          # Buffer management
    │   ├── packet/          # Packet definitions
    │   └── timer/           # Timing mechanisms
    ├── pkg/                 # Public API
    │   ├── connection.go    # Connection interface
    │   ├── options.go       # Configuration options
    │   └── stats.go         # Connection statistics
    └── go.mod
```

## License

MIT

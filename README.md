# Partial Packet Protocol (PPP)

A TCP-like reliable transport protocol with partial packet recovery for improved efficiency in lossy networks.

## Features

- **Connection-oriented**: Maintains connection state similar to TCP
- **Reliable delivery**: Guarantees message delivery
- **Partial packet recovery**: Only retransmits the missing portions of packets
- **Efficient**: Reduces unnecessary bandwidth usage on retransmissions
- **Simple API**: Easy to integrate into existing applications

## Use Cases

- High-latency networks (satellite, rural connectivity)
- Multimedia streaming where complete reliability is needed but with lower latency
- IoT and sensor networks where bandwidth conservation is critical
- Any application that needs reliability but wants to avoid full packet retransmissions

## Installation

```bash
go get github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol
```

## Usage Example

### Server

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol"
)

func main() {
    // Create a new server with default options
    server, err := ppprotocol.Listen(":8080", nil)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }
    defer server.Close()
    
    fmt.Println("Server listening on :8080")
    
    // Accept connections
    conn, err := server.Accept()
    if err != nil {
        log.Fatalf("Failed to accept connection: %v", err)
    }
    
    // Receive data
    data, err := conn.Receive(1024)
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
    
    "github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol"
)

func main() {
    // Create a new client
    conn, err := ppprotocol.Dial("127.0.0.1:8080", nil)
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    // Send message
    err = conn.Send([]byte("Hello from client!"))
    if err != nil {
        log.Fatalf("Failed to send message: %v", err)
    }
    
    // Receive response
    response, err := conn.Receive(1024)
    if err != nil {
        log.Fatalf("Failed to receive response: %v", err)
    }
    
    fmt.Printf("Server response: %s\n", string(response))
}
```

## Performance

Compared to standard TCP, Partial Packet Protocol can provide:

- Reduced bandwidth usage in lossy environments
- Lower latency for large packets when partial loss occurs
- Similar reliability guarantees

## License

MIT
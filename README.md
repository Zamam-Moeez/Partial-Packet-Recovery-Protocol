# Partial Packet Protocol (PPP)

A TCP-like reliable transport protocol **built in Go on top of UDP**. PPP improves efficiency in lossy networks by **only retransmitting the missing portions** of corrupted or dropped data, rather than resending entire packets. This approach is especially advantageous in high-latency or bandwidth-constrained environments.

---

## Why PPP?

Standard TCP retransmits **the entire packet** if **any** part of it is lost or corrupted, often leading to excessive bandwidth consumption in noisy networks. **PPP** takes a more granular approach:

- **Partial Packet Recovery**: Only resend the unacknowledged or lost segments of a message.  
- **Explicit Sequence & Offset Management**: Built-in tracking ensures data is reassembled in the correct order, even if segments arrive out of order or partially.  
- **Configurable**: Users can tune retransmission intervals, window sizes, keep-alive intervals, etc., to match different network conditions.

---

## How It’s Built

### Language & Libraries
- **Go (Golang)**: Concurrency-friendly language that simplifies network programming.  
- **UDP**: Uses Go’s `net.UDPConn` for sending and receiving raw datagrams.  
- **CRC32 Checksums**: Ensures data integrity; corrupt packets are discarded.  
- **Goroutines & Channels**: Multiple background loops (e.g., receive loops, retransmission timers) run concurrently.

### Architecture Highlights
1. **Connection State Machine**  
   Defined in [`connection.go`](./connection.go) with states similar to TCP: `Initial`, `Connecting`, `Connected`, `Closing`, `Closed`. It manages:
   - **Handshake** (SYN, SYN-ACK)  
   - **ACK Processing**  
   - **FIN / RST** teardown  

2. **Buffers for Partial Reliability**  
   - **[`send_buffer.go`](./send_buffer.go)**: Holds outbound messages, tracks unacknowledged bytes, and constructs retransmission ranges for missing segments.  
   - **[`receive_buffer.go`](./receive_buffer.go)**: Reassembles incoming data chunk by chunk until a complete message is formed.

3. **Packet Definition & Serialization**  
   - **[`packet.go`](./packet.go)**: Defines the `Packet` structure (sequence number, flags, offset, payload).  
   - Includes routines for partial packet creation, checksum calculation, and (de)serialization.

4. **Timers & Retransmissions**  
   - **[`timer.go`](./timer.go)**: Implements a simple scheduling mechanism to periodically check for unacknowledged data.  
   - Retransmits only the missing portions (offset-based) instead of the full message.

5. **Configuration & Options**  
   - **[`options.go`](./options.go)**: User-configurable parameters (like `MaxPayloadSize`, `RetransmitInterval`, `WindowSize`, etc.) let you tailor PPP for different link conditions.

6. **Statistics & Diagnostics**  
   - **[`stats.go`](./stats.go)**: Collects packet counts, retransmission counts, partial-saved bytes, RTT samples, and more.

---

## Key Features

1. **Connection-Oriented**  
   - Similar to TCP: handshake, stateful sessions, and graceful shutdown (FIN).

2. **Partial Packet Recovery**  
   - If a segment of a packet is lost, the protocol resends only that segment (offset range), saving bandwidth on large messages.

3. **Selective Acknowledgments**  
   - The receiver reports precisely which bytes have arrived, allowing the sender to skip retransmissions of fully acknowledged data.

4. **Window-Based Flow Control**  
   - A `WindowSize` parameter limits the number of unacknowledged packets in flight to avoid overwhelming the receiver.

5. **Keep-Alive Support**  
   - Periodic keep-alive packets prevent idle connections from being silently dropped in some network environments.

6. **Congestion Control Placeholder**  
   - Basic reliability is implemented, but advanced congestion control (like TCP Reno/CUBIC) is **not** (yet) included. This is primarily an experimental/research protocol.

---

## Performance Benefits

- **Lower Bandwidth Usage**: By only resending lost segments, PPP can reduce overall retransmissions by up to 40% in lossy scenarios.  
- **Reduced Latency**: Large data transmissions recover more quickly since you don’t need to retransmit the entire chunk.  
- **Fine-Grained Control**: Tweak timeouts, offsets, and partial windows for more stable performance on challenging links (e.g., satellite, rural).

---

## Installation

```bash
# From your Go project
go get github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol
```

---

## Basic Usage

Below are minimal examples for setting up a PPP server and client.

### Server Example

```go
package main

import (
    "fmt"
    "log"
    "time"

    "github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/pkg"
)

func main() {
    // Create a new server listening on :8800
    server, err := ppprotocol.Listen(":8800", nil)
    if err != nil {
        log.Fatalf("Failed to create server: %v", err)
    }
    defer server.Close()

    fmt.Println("Server listening on :8800")

    // Accept an incoming connection
    conn, err := server.Accept()
    if err != nil {
        log.Fatalf("Failed to accept connection: %v", err)
    }

    // Receive data with a 30-second timeout
    data, err := conn.Receive(30 * time.Second)
    if err != nil {
        log.Fatalf("Failed to receive data: %v", err)
    }

    fmt.Printf("Received: %s\n", string(data))

    // Send a response
    err = conn.Send([]byte("Hello from server!"))
    if err != nil {
        log.Fatalf("Failed to send response: %v", err)
    }
}
```

### Client Example

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

    // Connect to the server
    conn, err := ppprotocol.Dial("127.0.0.1:8800", options)
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()

    // Send a message
    err = conn.Send([]byte("Hello from client!"))
    if err != nil {
        log.Fatalf("Failed to send message: %v", err)
    }

    // Receive a response within 10 seconds
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

---

## Advanced Configuration

Use the `DefaultOptions()` and modify any fields:

```go
options := ppprotocol.DefaultOptions()

// Maximum payload size for each packet chunk (default: 1400)
options.MaxPayloadSize = 1024

// Base interval between retransmissions (default: 200 ms)
options.RetransmitInterval = 300 * time.Millisecond

// Maximum number of retransmissions (default: 10)
options.MaxRetries = 5

// Timeout for connection setup (default: 10 s)
options.ConnectionTimeout = 5 * time.Second

// Idle connection timeout (default: 30 s)
options.IdleTimeout = 60 * time.Second

// Interval for keep-alive packets (default: 5 s)
options.KeepAliveInterval = 10 * time.Second

// Window size (default: 16) for unacknowledged packets
options.WindowSize = 32

// Enable or disable Nagle’s algorithm (default: true)
options.EnableNagling = false
```

---

## Internals: Step-by-Step

1. **Three-Way Handshake**  
   - `SYN` -> `SYN-ACK` -> client acknowledges -> **Connected**.  
   - Both sides switch to a `Connected` state and begin exchanging data.

2. **Data Transmission**  
   - Each message is chunked into UDP-based packets with sequence numbers and offsets.  
   - See [`send_buffer.go`](./send_buffer.go) for how partial segments are stored and tracked.

3. **Partial Recovery & ACKs**  
   - The receiver records which offsets within a sequence number have arrived in [`receive_buffer.go`](./receive_buffer.go).  
   - ACK packets indicate exactly which bytes were successfully received, letting the sender retransmit only missing parts.

4. **Keep-Alive & Timeouts**  
   - Periodic keep-alive packets prevent timeouts on idle links.  
   - Stale data is pruned from buffers after a configurable `IdleTimeout`.

5. **Teardown**  
   - Similar to TCP’s FIN handshake or a hard RST.  
   - Buffers are flushed, and resources are closed.

---

## Example Applications

In the `cmd` directory:

- **`ppclient`**: A basic interactive CLI client.  
- **`ppserver`**: An echo server responding with what it receives.

```bash
# Run the server
go run ./cmd/ppserver/main.go

# Then, in another terminal, run the client
go run ./cmd/ppclient/main.go -server 127.0.0.1:8800
```

---

## Project Structure

```text
Partial-Packet-Recovery-Protocol/
├── ppprotocol/
│   ├── cmd/
│   │   ├── ppclient/     # Example client app
│   │   └── ppserver/     # Example server app
│   ├── internal/         # Implementation details
│   │   ├── buffer/       # Buffer management (send/receive)
│   │   ├── packet/       # Packet definitions, checksums, serialization
│   │   └── timer/        # Retransmission timers, RTT estimation
│   ├── pkg/              # Public API
│   │   ├── connection.go # Core Connection logic (handshake, read/write)
│   │   ├── options.go    # Tunable protocol parameters
│   │   └── stats.go      # Connection statistics
└── go.mod
```

- **`connection.go`**: Orchestrates sending/receiving loops, partial retransmissions, and connection states.  
- **`buffer/`**:  
  - `send_buffer.go`: Manages unacked data, coordinates partial retransmissions.  
  - `receive_buffer.go`: Reassembles partial chunks into full messages.  
- **`packet.go`**: Data structures and flags for PPP packets, plus checksum logic.  
- **`timer.go`**: Manages timed events (retransmission triggers, keep-alive).  
- **`stats.go`**: Gathers metrics like bandwidth usage, retransmissions, partial transmissions, etc.

---

## License

This project is licensed under the **MIT License**. Feel free to use, modify, and distribute it.

---

## FAQ / More Details

- **Is PPP production-ready?**  
  It’s primarily an experimental protocol demonstrating partial retransmission. Key TCP features (e.g., sophisticated congestion control) are **not implemented**. For production, consider QUIC, SCTP, or a standard TCP with robust control.

- **Security & Encryption?**  
  PPP doesn’t provide built-in encryption. You can wrap it in DTLS or run it through an encrypted tunnel (like WireGuard) if secure transport is required.

- **Performance Benchmarks?**  
  Basic tests show up to ~40% bandwidth savings in high-loss scenarios compared to naive TCP. Actual gains depend on the packet loss rate, message size distribution, and RTT.

- **How does the partial retransmit logic actually work?**  
  The sender keeps a bitmap for every byte in the message (`sendBuffer`). When the receiver ACKs certain offsets, the sender marks them as acknowledged. Missing offsets get resent in smaller, targeted packets. The receiver side (`receiveBuffer`) similarly tracks which offsets it has.

- **Future Plans**  
  - Congestion control module (e.g., TCP Reno or CUBIC-like approach).  
  - Improved selective ACK messages for large window sizes.  
  - Possibly a built-in encryption layer or direct integration with DTLS.

---

**Contributions & Feedback:**  
We welcome PRs and issue reports. Feel free to open a discussion or submit new features!

*Happy hacking! Enjoy more efficient, partial retransmissions in your custom UDP-based adventures.*

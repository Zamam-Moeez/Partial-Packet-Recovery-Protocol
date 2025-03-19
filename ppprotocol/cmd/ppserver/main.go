package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol/pkg"
)

var (
	port          = flag.Int("port", 8800, "Server port")
	statsInterval = flag.Duration("stats", 5*time.Second, "Statistics reporting interval")
)

// Global variables to track active connections
var (
	activeConnections = make(map[*ppprotocol.Connection]struct{})
	connMutex         sync.Mutex
)

func main() {
	flag.Parse()

	// Configure logging to not show date/time prefix (cleaner output)
	log.SetFlags(0)

	// Create options
	options := ppprotocol.DefaultOptions()
	options.LogLevel = 2 // More verbose logging

	// Create server
	addr := fmt.Sprintf(":%d", *port)
	server, err := ppprotocol.Listen(addr, options)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	fmt.Printf("Server listening on %s\n", addr)
	fmt.Println("Press Ctrl+C to exit")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Prepare a done channel to signal shutdown
	done := make(chan struct{})

	// Start server loop in a goroutine
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				// Accept connections
				conn, err := server.Accept()
				if err != nil {
					// Only log if we're not shutting down
					select {
					case <-done:
						return
					default:
						log.Printf("Error accepting connection: %v", err)
					}
					continue
				}

				// Track the connection
				connMutex.Lock()
				activeConnections[conn] = struct{}{}
				connMutex.Unlock()

				// Handle connection in a separate goroutine
				go handleConnection(conn)
			}
		}
	}()

	// Wait for termination signal
	<-sigChan
	fmt.Println("\nShutting down server...")
	
	// Signal the accept loop to stop
	close(done)
	
	// Close the server
	server.Close()
	
	// Close all active connections
	connMutex.Lock()
	count := len(activeConnections)
	if count > 0 {
		fmt.Printf("Closing %d active connections...\n", count)
		for conn := range activeConnections {
			conn.Close()
		}
	}
	connMutex.Unlock()
	
	fmt.Println("Server shutdown complete")
	os.Exit(0)
}

func handleConnection(conn *ppprotocol.Connection) {
	fmt.Printf("Connection established with %s\n", conn.RemoteAddr())
	
	// Ensure we remove this connection from tracking when done
	defer func() {
		connMutex.Lock()
		delete(activeConnections, conn)
		connMutex.Unlock()
	}()

	messageCount := 0
	
	// Echo loop
	for {
		// Receive data with 30 second timeout
		data, err := conn.Receive(30 * time.Second)
		if err != nil {
			log.Printf("Connection with %s closed: %v", conn.RemoteAddr(), err)
			break
		}

		fmt.Printf("Received message from %s: %s\n", conn.RemoteAddr(), string(data))
		messageCount++

		// Echo back with a timestamp
		response := fmt.Sprintf("ECHO: %s (received at %s)", string(data), time.Now().Format(time.RFC3339))
		err = conn.Send([]byte(response))
		if err != nil {
			log.Printf("Error sending response to %s: %v", conn.RemoteAddr(), err)
			break
		}

		// Print connection stats every few messages
		if messageCount % 5 == 0 {
			stats := conn.Stats().GetSummary()
			statsJSON, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Printf("Connection stats for %s:\n%s\n", conn.RemoteAddr(), string(statsJSON))
		}
	}
}
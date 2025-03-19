package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol"
)

var (
	port     = flag.Int("port", 8800, "Server port")
	statsInterval = flag.Duration("stats", 5*time.Second, "Statistics reporting interval")
)

func main() {
	flag.Parse()

	// Create options
	options := ppprotocol.DefaultOptions()
	options.LogLevel = 2 // More verbose logging

	// Create server
	addr := fmt.Sprintf(":%d", *port)
	server, err := ppprotocol.Listen(addr, options)
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}
	defer server.Close()

	fmt.Printf("Server listening on %s\n", addr)
	fmt.Println("Press Ctrl+C to exit")

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start server loop in a goroutine
	go func() {
		for {
			// Accept connections (not fully implemented in the listener)
			conn, err := server.Accept()
			if err != nil {
				log.Printf("Error accepting connection: %v", err)
				continue
			}

			// Handle connection in a separate goroutine
			go handleConnection(conn)
		}
	}()

	// Display stats periodically
	ticker := time.NewTicker(*statsInterval)
	defer ticker.Stop()

	// Wait for termination signal
	select {
	case <-sigChan:
		fmt.Println("\nShutting down server...")
	}
}

func handleConnection(conn *ppprotocol.Connection) {
	fmt.Printf("Connection established with %s\n", conn.RemoteAddr())

	// Echo loop
	for {
		// Receive data with 30 second timeout
		data, err := conn.Receive(30 * time.Second)
		if err != nil {
			log.Printf("Error receiving: %v", err)
			break
		}

		fmt.Printf("Received message from %s: %s\n", conn.RemoteAddr(), string(data))

		// Echo back with a timestamp
		response := fmt.Sprintf("ECHO: %s (received at %s)", string(data), time.Now().Format(time.RFC3339))
		err = conn.Send([]byte(response))
		if err != nil {
			log.Printf("Error sending response: %v", err)
			break
		}

		// Print connection stats every 10 messages
		stats := conn.Stats().GetSummary()
		statsJSON, _ := json.MarshalIndent(stats, "", "  ")
		fmt.Printf("Connection stats for %s:\n%s\n", conn.RemoteAddr(), string(statsJSON))
	}
}
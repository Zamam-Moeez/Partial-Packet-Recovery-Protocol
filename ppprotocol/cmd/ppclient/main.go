package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Zamam-Moeez/Partial-Packet-Recovery-Protocol/ppprotocol"
)

var (
	serverAddr = flag.String("server", "127.0.0.1:8800", "Server address")
	timeout    = flag.Duration("timeout", 5*time.Second, "Connection timeout")
)

func main() {
	flag.Parse()

	// Create options
	options := ppprotocol.DefaultOptions()
	options.ConnectionTimeout = *timeout
	options.LogLevel = 2 // More verbose logging

	fmt.Printf("Connecting to %s...\n", *serverAddr)

	// Connect to server
	conn, err := ppprotocol.Dial(*serverAddr, options)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	fmt.Println("Connected! Type a message and press Enter to send.")
	fmt.Println("Type 'exit' to quit, 'stats' to show connection statistics.")

	// Start receive loop in a goroutine
	go receiveLoop(conn)

	// Main send loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}

		input := scanner.Text()
		if strings.ToLower(input) == "exit" {
			break
		}

		if strings.ToLower(input) == "stats" {
			// Print connection stats
			stats := conn.Stats().GetSummary()
			statsJSON, _ := json.MarshalIndent(stats, "", "  ")
			fmt.Printf("Connection stats:\n%s\n", string(statsJSON))
			continue
		}

		// Send the message
		err = conn.Send([]byte(input))
		if err != nil {
			log.Printf("Error sending: %v", err)
			continue
		}
	}

	fmt.Println("Closing connection...")
}

// receiveLoop handles incoming messages
func receiveLoop(conn *ppprotocol.Connection) {
	for {
		// Receive with 10 second timeout
		data, err := conn.Receive(10 * time.Second)
		if err != nil {
			if err.Error() == "operation timed out" {
				// Just a timeout, continue
				continue
			}
			log.Printf("Error receiving: %v", err)
			return
		}

		fmt.Printf("\nServer: %s\n> ", string(data))
	}
}
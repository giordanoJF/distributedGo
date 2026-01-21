package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"

	pb "distributed-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultRegistryAddr = "localhost:5000"
	sumServiceName      = "sum_service"
	incrementServiceName = "increment_service"
	strategyRandom      = "random"
	strategyLowestCPU   = "lowest_cpu"
)

var (
	cachedServers []*pb.ServerInfo
	registryClient pb.RegistryServiceClient
)

// ===== Server Selection Logic =====

// supportsService checks if a server offers a specific service.
func supportsService(server *pb.ServerInfo, serviceName string) bool {
	for _, s := range server.Services {
		if s == serviceName {
			return true
		}
	}
	return false
}

// refreshCache contacts the registry and updates the cached server list.
func refreshCache() error {
	log.Println("Refreshing server cache from registry...")
	
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	res, err := registryClient.GetServers(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("failed to retrieve server list: %w", err)
	}
	
	cachedServers = res.Servers
	log.Printf("✓ Cache updated: %d active servers found", len(cachedServers))
	
	return nil
}

// selectServer chooses a server address based on the service and strategy.
// Returns empty string if no suitable server is found.
func selectServer(serviceName string, strategy string) (string, error) {
	// Filter candidates that support the requested service
	var candidates []*pb.ServerInfo
	for _, s := range cachedServers {
		if supportsService(s, serviceName) {
			candidates = append(candidates, s)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no servers available for service '%s'", serviceName)
	}

	// Apply selection strategy
	switch strategy {
	case strategyRandom:
		// Stateless approach: random selection for load distribution
		selected := candidates[rand.Intn(len(candidates))]
		return selected.Address, nil

	case strategyLowestCPU:
		// Stateful approach: select server with lowest CPU usage
		var bestAddr string
		minCPU := int32(101) // Initialize with value > 100
		
		for _, s := range candidates {
			if s.CpuUsage < minCPU {
				minCPU = s.CpuUsage
				bestAddr = s.Address
			}
		}
		return bestAddr, nil

	default:
		return "", fmt.Errorf("unknown strategy: %s", strategy)
	}
}

// ===== RPC Calls =====

// callSum invokes the Sum service on a specific worker server.
func callSum(addr string, a, b int32) error {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.Sum(ctx, &pb.SumRequest{A: a, B: b})
	if err != nil {
		return fmt.Errorf("RPC call failed: %w", err)
	}

	fmt.Printf("✓ Sum Result: %d + %d = %d (from %s)\n", a, b, res.Result, addr)
	return nil
}

// callIncrement invokes the Increment service on a specific worker server.
func callIncrement(addr string) error {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	res, err := client.Increment(ctx, &pb.IncrementRequest{})
	if err != nil {
		return fmt.Errorf("RPC call failed: %w", err)
	}

	fmt.Printf("✓ Global Counter: %d (from %s)\n", res.NewValue, addr)
	return nil
}

// ===== UI Functions =====

// displayMenu shows the main menu with current cache status.
func displayMenu() {
	time.Sleep(300 * time.Millisecond)
	
	fmt.Println("\n═════════════════════════════════════════")
	fmt.Println("         DISTRIBUTED SYSTEM CLIENT       ")
	fmt.Println("═════════════════════════════════════════")
	
	fmt.Printf("\nCurrent Cache (%d servers):\n", len(cachedServers))
	if len(cachedServers) == 0 {
		fmt.Println("  (No servers available)")
	} else {
		for _, s := range cachedServers {
			fmt.Printf("  • %s | CPU: %d%% | Services: %v\n", 
				s.Address, s.CpuUsage, s.Services)
		}
	}
	
	fmt.Println("\n─────────────────────────────────────────")
	fmt.Println("Options:")
	fmt.Println("  1. Call Sum Service (Stateless/Random)")
	fmt.Println("  2. Call Increment Service (Stateful/Lowest CPU)")
	fmt.Println("  3. Refresh Server Cache")
	fmt.Println("  q. Quit")
	fmt.Println("─────────────────────────────────────────")
	fmt.Print("\nSelect option: ")
}

// readInput reads and trims user input from stdin.
func readInput(scanner *bufio.Scanner, prompt string) string {
	fmt.Print(prompt)
	scanner.Scan()
	return strings.TrimSpace(scanner.Text())
}

// readInteger reads an integer from user input with basic validation.
func readInteger(scanner *bufio.Scanner, prompt string) (int32, error) {
	input := readInput(scanner, prompt)
	value, err := strconv.Atoi(input)
	if err != nil {
		return 0, fmt.Errorf("invalid integer: %s", input)
	}
	return int32(value), nil
}

// handleSumService manages the Sum service workflow.
func handleSumService(scanner *bufio.Scanner) {
	targetAddr, err := selectServer(sumServiceName, strategyRandom)
	if err != nil {
		fmt.Printf("\n✗ Error: %v\n", err)
		fmt.Println("→ Try refreshing the cache (option 3)")
		return
	}

	a, err := readInteger(scanner, "Enter first number (A): ")
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		return
	}

	b, err := readInteger(scanner, "Enter second number (B): ")
	if err != nil {
		fmt.Printf("✗ %v\n", err)
		return
	}

	fmt.Printf("\n→ Sending request to %s...\n", targetAddr)
	if err := callSum(targetAddr, a, b); err != nil {
		fmt.Printf("\n✗ Request failed: %v\n", err)
		fmt.Println("→ The server may be offline. Try refreshing the cache.")
	}
}

// handleIncrementService manages the Increment service workflow.
func handleIncrementService() {
	targetAddr, err := selectServer(incrementServiceName, strategyLowestCPU)
	if err != nil {
		fmt.Printf("\n✗ Error: %v\n", err)
		fmt.Println("→ Try refreshing the cache (option 3)")
		return
	}

	fmt.Printf("\n→ Sending request to %s (lowest CPU)...\n", targetAddr)
	if err := callIncrement(targetAddr); err != nil {
		fmt.Printf("\n✗ Request failed: %v\n", err)
		fmt.Println("→ The server may be offline. Try refreshing the cache.")
	}
}

// handleRefreshCache manages the cache refresh workflow.
func handleRefreshCache() {
	if err := refreshCache(); err != nil {
		fmt.Printf("\n✗ Failed to refresh cache: %v\n", err)
	}
}

// ===== Main =====

func main() {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Get registry address from environment or use default
	registryAddr := os.Getenv("REGISTRY_ADDR")
	if registryAddr == "" {
		registryAddr = defaultRegistryAddr
	}

	fmt.Printf("Starting client...\n")
	fmt.Printf("Connecting to registry at: %s\n", registryAddr)

	// Establish persistent connection to registry
	conn, err := grpc.Dial(
		registryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to registry: %v", err)
	}
	defer conn.Close()

	registryClient = pb.NewRegistryServiceClient(conn)
	
	// Initial cache load
	if err := refreshCache(); err != nil {
		log.Printf("Warning: Initial cache refresh failed: %v", err)
	}

	// Main menu loop
	scanner := bufio.NewScanner(os.Stdin)
	
	for {
		displayMenu()
		
		if !scanner.Scan() {
			break
		}
		
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			handleSumService(scanner)
			
		case "2":
			handleIncrementService()
			
		case "3":
			handleRefreshCache()
			
		case "q", "Q":
			fmt.Println("\nExiting client. Goodbye!")
			return
			
		default:
			fmt.Println("\n✗ Invalid option. Please select 1, 2, 3, or q.")
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Scanner error: %v", err)
	}
}

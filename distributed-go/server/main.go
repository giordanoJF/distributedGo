package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "distributed-go/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultPort         = "8080"
	defaultRegistryAddr = "localhost:5000"
	defaultRedisAddr    = "localhost:6379"
	redisCounterKey     = "global_counter"
)

var (
	redisClient *redis.Client
)

// WorkerServer implements the gRPC WorkerService interface.
// It handles computational requests and maintains connection to the registry.
type WorkerServer struct {
	pb.UnimplementedWorkerServiceServer
	registryClient pb.RegistryServiceClient
	myAddress      string
	currentCPU     int32
}

// NewWorkerServer creates a new instance of WorkerServer.
func NewWorkerServer(registryClient pb.RegistryServiceClient, address string) *WorkerServer {
	return &WorkerServer{
		registryClient: registryClient,
		myAddress:      address,
		currentCPU:     generateRandomCPU(),
	}
}

// Sum implements the stateless sum service.
// It adds two integers and updates the server's CPU load.
func (s *WorkerServer) Sum(ctx context.Context, req *pb.SumRequest) (*pb.SumResponse, error) {
	result := req.A + req.B
	log.Printf("Sum request: %d + %d = %d", req.A, req.B, result)
	
	s.updateLoad(ctx)
	
	return &pb.SumResponse{Result: result}, nil
}

// Increment implements the stateful increment service.
// It increments a global counter in Redis and updates the server's CPU load.
func (s *WorkerServer) Increment(ctx context.Context, req *pb.IncrementRequest) (*pb.IncrementResponse, error) {
	newValue, err := redisClient.Incr(ctx, redisCounterKey).Result()
	if err != nil {
		log.Printf("Error incrementing counter in Redis: %v", err)
		return nil, fmt.Errorf("failed to increment counter: %w", err)
	}
	
	log.Printf("Increment request: counter = %d", newValue)
	s.updateLoad(ctx)
	
	return &pb.IncrementResponse{NewValue: int32(newValue)}, nil
}

// updateLoad simulates CPU load variation and notifies the registry.
// This allows the registry to maintain accurate load information for routing decisions.
func (s *WorkerServer) updateLoad(ctx context.Context) {
	s.currentCPU = generateRandomCPU()
	
	_, err := s.registryClient.UpdateCPU(ctx, &pb.ServerInfo{
		Address:  s.myAddress,
		CpuUsage: s.currentCPU,
	})
	
	if err != nil {
		log.Printf("Warning: Failed to update CPU to registry: %v", err)
	}
}

// generateRandomCPU simulates CPU usage between 0-100%.
func generateRandomCPU() int32 {
	return int32(rand.Intn(101))
}

// getEnvOrDefault returns the environment variable value or a default if not set.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// registerWithRegistry registers this worker with the service registry.
func registerWithRegistry(ctx context.Context, client pb.RegistryServiceClient, address string, cpu int32) error {
	_, err := client.Register(ctx, &pb.ServerInfo{
		Address:  address,
		CpuUsage: cpu,
		Services: []string{"sum_service", "increment_service"},
	})
	return err
}

// deregisterFromRegistry removes this worker from the service registry.
func deregisterFromRegistry(ctx context.Context, client pb.RegistryServiceClient, address string) {
	_, err := client.Deregister(ctx, &pb.ServerInfo{Address: address})
	if err != nil {
		log.Printf("Error during deregistration: %v", err)
	} else {
		log.Println("Successfully deregistered from registry")
	}
}

// setupGracefulShutdown handles OS signals for graceful shutdown.
func setupGracefulShutdown(grpcServer *grpc.Server, registryClient pb.RegistryServiceClient, address string) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		sig := <-signalChan
		log.Printf("Received signal: %v. Initiating graceful shutdown...", sig)
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		
		deregisterFromRegistry(ctx, registryClient, address)
		grpcServer.GracefulStop()
		
		os.Exit(0)
	}()
}

func main() {
	// Seed random number generator
	rand.Seed(time.Now().UnixNano())

	// Load configuration from environment variables
	port := getEnvOrDefault("PORT", defaultPort)
	registryAddr := getEnvOrDefault("REGISTRY_ADDR", defaultRegistryAddr)
	myAddr := getEnvOrDefault("MY_ADDR", fmt.Sprintf("localhost:%s", port))
	redisAddr := getEnvOrDefault("REDIS_ADDR", defaultRedisAddr)

	// Initialize Redis client
	redisClient = redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})
	defer redisClient.Close()

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis at %s: %v", redisAddr, err)
	}
	log.Printf("✓ Connected to Redis at %s", redisAddr)

	// Connect to Registry
	registryConn, err := grpc.Dial(
		registryAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("Failed to connect to registry at %s: %v", registryAddr, err)
	}
	defer registryConn.Close()
	
	registryClient := pb.NewRegistryServiceClient(registryConn)
	log.Printf("✓ Connected to registry at %s", registryAddr)

	// Create TCP listener for gRPC server
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", port))
	if err != nil {
		log.Fatalf("Failed to listen on port %s: %v", port, err)
	}

	// Create and configure gRPC server
	grpcServer := grpc.NewServer()
	worker := NewWorkerServer(registryClient, myAddr)
	pb.RegisterWorkerServiceServer(grpcServer, worker)

	// Register with the service registry
	log.Printf("Registering with service registry as %s...", myAddr)
	if err := registerWithRegistry(context.Background(), registryClient, myAddr, worker.currentCPU); err != nil {
		log.Fatalf("Failed to register with registry: %v", err)
	}
	log.Printf("✓ Successfully registered with registry")

	// Setup graceful shutdown handling
	setupGracefulShutdown(grpcServer, registryClient, myAddr)

	// Start serving requests
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Worker Server listening on :%s", port)
	log.Printf("Advertised address: %s", myAddr)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

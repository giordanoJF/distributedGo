package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"

	pb "distributed-go/proto"
	"google.golang.org/grpc"
)

const (
	defaultPort = "5000"
)

// RegistryServer implements the gRPC RegistryService interface.
// It maintains an in-memory registry of active worker servers with thread-safe access.
type RegistryServer struct {
	pb.UnimplementedRegistryServiceServer
	servers map[string]*pb.ServerInfo // Map of server address to server info
	mu      sync.RWMutex              // Protects concurrent access to servers map
}

// NewRegistryServer creates a new instance of RegistryServer with an initialized server map.
func NewRegistryServer() *RegistryServer {
	return &RegistryServer{
		servers: make(map[string]*pb.ServerInfo),
	}
}

// Register adds a new worker server to the registry.
// If the server already exists, it updates its information.
func (s *RegistryServer) Register(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.servers[info.Address] = info
	log.Printf("✓ Registered server: %s | CPU: %d%% | Services: %v", 
		info.Address, info.CpuUsage, info.Services)
	
	return &pb.Empty{}, nil
}

// Deregister removes a worker server from the registry.
// This is typically called during graceful shutdown.
func (s *RegistryServer) Deregister(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.servers, info.Address)
	log.Printf("✗ Deregistered server: %s", info.Address)
	
	return &pb.Empty{}, nil
}

// UpdateCPU updates the CPU usage for an existing server.
// This method is called by workers after processing each request.
func (s *RegistryServer) UpdateCPU(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if entry, exists := s.servers[info.Address]; exists {
		entry.CpuUsage = info.CpuUsage
		log.Printf("↻ Updated CPU for %s: %d%%", info.Address, info.CpuUsage)
	} else {
		log.Printf("⚠ Warning: Attempted to update CPU for unregistered server: %s", info.Address)
	}
	
	return &pb.Empty{}, nil
}

// GetServers returns the list of all currently registered servers.
// Uses read lock for better concurrency when multiple clients query simultaneously.
func (s *RegistryServer) GetServers(ctx context.Context, _ *pb.Empty) (*pb.ServerList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*pb.ServerInfo, 0, len(s.servers))
	for _, info := range s.servers {
		list = append(list, info)
	}

	log.Printf("→ Sent server list to client (%d servers)", len(list))
	return &pb.ServerList{Servers: list}, nil
}

func main() {
	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	// Create TCP listener
	address := fmt.Sprintf(":%s", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", address, err)
	}

	// Create gRPC server and register our service
	grpcServer := grpc.NewServer()
	registryServer := NewRegistryServer()
	pb.RegisterRegistryServiceServer(grpcServer, registryServer)

	// Start serving
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Service Registry started on %s", address)
	log.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	
	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}
}

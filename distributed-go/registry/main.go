package main

import (
	"context"
	"log"
	"net"
	"sync"
	"os"

	pb "distributed-go/proto"
	"google.golang.org/grpc"
)

type RegistryServer struct {
	pb.UnimplementedRegistryServiceServer
	servers map[string]*pb.ServerInfo 
	mu      sync.Mutex
}

func (s *RegistryServer) Register(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Store the full info (Address, CPU, Services)
	s.servers[info.Address] = info
	log.Printf("Registered %s with services: %v", info.Address, info.Services)
	return &pb.Empty{}, nil
}

func (s *RegistryServer) Deregister(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.servers, info.Address)
	log.Printf("Deregistered Server: %s", info.Address)
	return &pb.Empty{}, nil
}

func (s *RegistryServer) UpdateCPU(ctx context.Context, info *pb.ServerInfo) (*pb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, exists := s.servers[info.Address]; exists {
		entry.CpuUsage = info.CpuUsage // Update only CPU
	}
	return &pb.Empty{}, nil
}

func (s *RegistryServer) GetServers(ctx context.Context, _ *pb.Empty) (*pb.ServerList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var list []*pb.ServerInfo
	for _, info := range s.servers {
		list = append(list, info)
	}
	return &pb.ServerList{Servers: list}, nil
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "5000"
	}

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil { log.Fatalf("failed to listen on port %s: %v", port, err) }
	
	s := grpc.NewServer()
	// Initialize map
	pb.RegisterRegistryServiceServer(s, &RegistryServer{servers: make(map[string]*pb.ServerInfo)})
	
	log.Printf("Registry started on :%s", port)
	if err := s.Serve(lis); err != nil { log.Fatalf("failed to serve: %v", err) }
}
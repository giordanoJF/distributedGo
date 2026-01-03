package main

import (
	"context"
	//"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"os/signal"
	"syscall"
	//"time"

	pb "distributed-go/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var rdb *redis.Client

// --- Services Implementation ---
type WorkerServer struct {
	pb.UnimplementedWorkerServiceServer
	registryClient pb.RegistryServiceClient //interfaccia client per chiamare su di lui i metodi
	myAddress      string
	currentCPU     int32
}

func (s *WorkerServer) Sum(ctx context.Context, req *pb.SumRequest) (*pb.SumResponse, error) {
	s.updateLoad(ctx)
	return &pb.SumResponse{Result: req.A + req.B}, nil
}

func (s *WorkerServer) Increment(ctx context.Context, req *pb.IncrementRequest) (*pb.IncrementResponse, error) {
	val, err := rdb.Incr(ctx, "global_counter").Result()
	if err != nil {
		return nil, err
	}
	s.updateLoad(ctx)
	return &pb.IncrementResponse{NewValue: int32(val)}, nil
}

// Simulate updating CPU load and notifying registry
func (s *WorkerServer) updateLoad(ctx context.Context) {
	// Simulate new random CPU load
	s.currentCPU = int32(rand.Intn(100))
	
	// Notify Registry
	_, err := s.registryClient.UpdateCPU(ctx, &pb.ServerInfo{
		Address:  s.myAddress,
		CpuUsage: s.currentCPU,
	})
	if err != nil {
		log.Printf("Failed to update CPU to registry: %v", err)
	}
}

func main() {
	// 1. Get ENV variables (also for Docker)
	port := os.Getenv("PORT") // da usare quando lanci in worker con il terminale se ne vuoi piu di uno senza docker
	if port == "" { port = "8080" }
	registryAddr := os.Getenv("REGISTRY_ADDR") // e.g., "registry:5000"
	if registryAddr == "" { registryAddr = "localhost:5000" }
	myAddr := os.Getenv("MY_ADDR") // The address the client uses to reach me
	if myAddr == "" { myAddr = "localhost:" + port } // Si auto-assegna localhost
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" { redisAddr = "localhost:6379" } // Default Redis standard

	// 2. Setup Redis
	rdb = redis.NewClient(&redis.Options{Addr: redisAddr})

	// 3. Connect to Registry
	conn, err := grpc.Dial(registryAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil { log.Fatalf("did not connect to registry: %v", err) }
	registryClient := pb.NewRegistryServiceClient(conn)//montato e pronto per usarlo

	// 4. Start gRPC Server
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil { log.Fatalf("failed to listen: %v", err) }
	
	s := grpc.NewServer()
	worker := &WorkerServer{
		registryClient: registryClient,
		myAddress:      myAddr,
		currentCPU:     int32(rand.Intn(100)),
	}
	pb.RegisterWorkerServiceServer(s, worker)

	// 5. Register to Registry
	log.Printf("Registering service at %s...", myAddr)
	_, err = registryClient.Register(context.Background(), &pb.ServerInfo{
		Address:  myAddr,
		CpuUsage: worker.currentCPU,
		Services: []string{"sum_service", "increment_service"},
	})
	if err != nil { log.Fatalf("Register failed: %v", err) }

	// 6. Graceful Shutdown Handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down... Deregistering...")
		registryClient.Deregister(context.Background(), &pb.ServerInfo{Address: myAddr})
		s.GracefulStop()
		os.Exit(0)
	}()

	log.Printf("Server listening on %s", port)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
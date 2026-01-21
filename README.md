# DistributedGo

A microservices-based distributed system implementation in Go, demonstrating service discovery, load balancing, and shared state management using gRPC, Docker, and Redis.

## Overview

This project implements a minimal yet complete distributed system architecture featuring:

- **Service Discovery**: Dynamic server registration and health monitoring
- **Load Balancing**: Multiple strategies (random selection, CPU-based routing)
- **Shared State Management**: Distributed counter using Redis
- **Fault Tolerance**: Graceful shutdown and automatic deregistration
- **Microservices Communication**: gRPC-based inter-service communication

## Architecture

The system consists of four main components:

### 1. Service Registry
- Maintains real-time registry of active worker servers
- Tracks server addresses, CPU load, and available services
- Provides server discovery API for clients
- Thread-safe concurrent access using mutexes

### 2. Worker Servers
- Auto-register on startup and deregister on shutdown
- Simulate variable CPU usage (0-100%)
- Update registry after each request
- Expose two gRPC services:
  - **SumService** (Stateless): Adds two integers
  - **IncrementService** (Stateful): Increments a global counter in Redis

### 3. Client
- Queries registry for available servers
- Implements load balancing strategies:
  - **Random**: Distributes requests randomly (stateless services)
  - **Lowest CPU**: Routes to least-loaded server (stateful services)
- Interactive CLI menu for service invocation

### 4. Redis
- Centralized data store for shared state
- Maintains global counter synchronized across all workers
- Ensures consistency in distributed environment

## Prerequisites

- **Docker** (version 20.0+)
- **Docker Compose** (version 2.0+)
- **Go** (version 1.24+) - only for local development
- **Redis** (optional for local development without Docker)

## Quick Start

### Using Docker Compose (Recommended)

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd distributedGo
   ```

2. **Start all services**
   ```bash
   docker-compose up --build
   ```

3. **Access the client**
   The client container will start automatically with an interactive menu.

4. **Scale workers** (optional)
   ```bash
   docker-compose up --scale worker=3
   ```

### Local Development (Without Docker)

1. **Start Redis**
   ```bash
   redis-server
   ```

2. **Start the Registry**
   ```bash
   cd registry
   go run main.go
   ```

3. **Start Worker Server(s)**
   ```bash
   cd server
   PORT=8080 go run main.go
   # In another terminal for additional workers:
   PORT=8081 MY_ADDR=localhost:8081 go run main.go
   ```

4. **Start the Client**
   ```bash
   cd client
   go run main.go
   ```

## Project Structure

```
distributedGo/
├── client/          # Client application with interactive menu
├── server/          # Worker server implementation
├── registry/        # Service registry/discovery server
├── proto/           # Protocol Buffers definitions and generated code
├── docker-compose.yml
├── Dockerfile.*     # Individual Dockerfiles for each component
├── go.mod
├── go.sum
└── README.md
```

## Protocol Buffers

The project uses Protocol Buffers for efficient serialization. To regenerate:

```bash
protoc --go_out=. --go-grpc_out=. proto/service.proto
```

## Configuration

Environment variables can be set in `docker-compose.yml` or exported locally:

- `PORT`: Server listening port (default: varies by service)
- `REGISTRY_ADDR`: Registry server address (default: `localhost:5000`)
- `REDIS_ADDR`: Redis server address (default: `localhost:6379`)
- `MY_ADDR`: Worker's advertised address for client connections

## Features in Detail

### Load Balancing Strategies

**Random Selection (Stateless)**
- Used for `SumService`
- Distributes load evenly across available servers
- No server affinity required

**CPU-Based Selection (Stateful)**
- Used for `IncrementService`
- Routes requests to server with lowest CPU utilization
- Optimizes resource usage across cluster

### Fault Tolerance

- Workers deregister gracefully on shutdown (SIGTERM/SIGINT)
- Client caches server list and can refresh on demand
- Registry handles concurrent updates safely
- Connection timeouts prevent indefinite blocking

## Testing the System

1. Start the system with multiple workers
2. Use the client menu to invoke services
3. Observe load distribution across workers
4. Stop a worker and refresh the client cache
5. Verify the system continues operating with remaining workers

## Future Enhancements

- Health check mechanism with periodic heartbeats
- Persistent storage for registry state
- Authentication and authorization
- Metrics and monitoring integration
- Horizontal autoscaling based on load
- Circuit breaker pattern for fault tolerance

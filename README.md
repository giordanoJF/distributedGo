# distributedGo
A simple design exercise with clients, servers, and a service registry.

This project implements a minimal distributed system based on microservices, using **Go**, **gRPC**, **Docker**, and **Redis**. It simulates a real-world environment featuring Service Discovery, Load Balancing (stateless and stateful strategies), and shared state management.

##  Architecture

The system consists of 4 main components (optionally orchestrated via Docker Compose):

1.  **Service Registry**: Maintains an in-memory list of active servers, including their addresses, current CPU load, and offered services.
2.  **Worker Servers**:
    * Automatically register at startup and deregister upon shutdown.
    * Simulate variable CPU usage (0-100%), updating the registry after every request.
    * Expose two gRPC services:
        * `SumService` (Stateless): Sums two integers.
        * `IncrementService` (Stateful): Increments a global counter stored in Redis.
3.  **Client**: Connects to the Registry to retrieve the server list and implements selection logic (Random or Lowest CPU).
4.  **Redis**: The database used to keep the shared state (the global counter) synchronized across all worker nodes.

### Prerequisites
* Docker
* Docker Compose
* Redis (only if running locally without Docker, on default port)

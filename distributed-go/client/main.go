package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
	"strconv"

	pb "distributed-go/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Variabile globale per mantenere la lista dei server in memoria
var cachedServers []*pb.ServerInfo

// --- Funzioni di Utilità ---

// supports controlla se un server offre un determinato servizio (es. "sum_service")
func supports(server *pb.ServerInfo, serviceName string) bool {
	for _, s := range server.Services {
		if s == serviceName {
			return true
		}
	}
	return false
}

// refreshCache contatta il Registry e aggiorna la variabile globale cachedServers
func refreshCache(registry pb.RegistryServiceClient) {
	log.Println("--- Contatto il Registry per aggiornare la cache... ---")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()

	res, err := registry.GetServers(ctx, &pb.Empty{})
	if err != nil {
		log.Printf("ERRORE: Impossibile recuperare lista server: %v", err)
		return
	}
	cachedServers = res.Servers
	fmt.Printf("Cache aggiornata. Trovati %d server attivi.\n", len(cachedServers))
}

// getServer seleziona un indirizzo IP in base al servizio richiesto e alla strategia
func getServer(serviceName string, strategy string) string {
	// 1. Filtro: Trova tutti i server che supportano il servizio richiesto
	var candidates []*pb.ServerInfo
	for _, s := range cachedServers {
		if supports(s, serviceName) {
			candidates = append(candidates, s)
		}
	}

	if len(candidates) == 0 {
		return ""
	}

	// 2. Strategia: Seleziona il server migliore tra i candidati
	if strategy == "random" {
		// Approccio Stateless: Uno a caso
		return candidates[rand.Intn(len(candidates))].Address

	} else if strategy == "lowest_cpu" {
		// Approccio Stateful: Quello con meno carico CPU
		var bestAddr string
		minCPU := int32(101) // Impostiamo un max fittizio > 100
		for _, s := range candidates {
			if s.CpuUsage < minCPU {
				minCPU = s.CpuUsage
				bestAddr = s.Address
			}
		}
		return bestAddr
	}

	return ""
}

// --- Funzioni RPC ---

func callSum(addr string, a int32, b int32) {
	// Connessione al Worker Server specifico
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("ERRORE Connessione: %v\n", err)
		return
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// Eseguiamo la RPC
	res, err := client.Sum(ctx, &pb.SumRequest{A: a, B: b})
	if err != nil {
		fmt.Printf("\n  ATTENZIONE: Chiamata RPC fallita verso %s\n", addr)
		fmt.Printf("Dettaglio errore: %v\n", err)
		fmt.Println("SUGGERIMENTO: Il server potrebbe essere spento. Seleziona 'Refresh Cache' dal menu.\n")
	} else {
		fmt.Printf(" Risultato Somma (%d + %d): %d (risposto da %s)\n", a, b, res.Result, addr)
	}
}

func callCounter(addr string) {
	// Connessione al Worker Server specifico
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("ERRORE Connessione: %v\n", err)
		return
	}
	defer conn.Close()

	client := pb.NewWorkerServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
	defer cancel()

	// Eseguiamo la RPC
	res, err := client.Increment(ctx, &pb.IncrementRequest{})
	if err != nil {
		fmt.Printf("\n  ATTENZIONE: Chiamata RPC fallita verso %s\n", addr)
		fmt.Printf("Dettaglio errore: %v\n", err)
		fmt.Println("SUGGERIMENTO: Il server potrebbe essere spento. Seleziona 'Refresh Cache' dal menu.\n")
	} else {
		fmt.Printf(" Nuovo Valore Contatore Globale: %d (risposto da %s)\n", res.NewValue, addr)
	}
}

// --- Main ---

func main() {
	// Leggo l'indirizzo del registry dalle variabili d'ambiente (settate da Docker Compose)
	registryAddr := os.Getenv("REGISTRY_ADDR")
	if registryAddr == "" {
		registryAddr = "localhost:5000" // Fallback per test locale fuori docker
	}

	fmt.Printf("Avvio Client. Connessione al Registry su: %s\n", registryAddr)

	// Connessione persistente al Registry
	conn, err := grpc.Dial(registryAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Impossibile connettersi al registry: %v", err)
	}
	registryClient := pb.NewRegistryServiceClient(conn)

	// Caricamento iniziale della cache
	refreshCache(registryClient)

	scanner := bufio.NewScanner(os.Stdin)

	// Loop Principale del Menu
	for {
		time.Sleep(500 * time.Millisecond) // Piccola pausa per pulizia output
		fmt.Println("\n=============================================")
		fmt.Println("\\\\                MENU                \\\\")
		fmt.Println("=============================================")
		
		// Mostra lo stato attuale della cache
		fmt.Printf("CACHE ATTUALE (%d server):\n", len(cachedServers))
		for _, s := range cachedServers {
			fmt.Printf(" - [%s] CPU: %d%% | Servizi: %v\n", s.Address, s.CpuUsage, s.Services)
		}
		fmt.Println("---------------------------------------------")
		
		fmt.Println("1. Servizio SOMMA (Stateless - Strategy: Random)")
		fmt.Println("2. Servizio CONTATORE (Stateful - Strategy: Lowest CPU)")
		fmt.Println("3. Aggiorna Cache (Riconnetti al Registry)")
		fmt.Println("q. Esci")
		fmt.Print("\nSeleziona un'opzione: ")

		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		var targetAddr string

		switch choice {
		case "1":
			targetAddr = getServer("sum_service", "random")
			if targetAddr == "" {
				fmt.Println("\n ERRORE: Nessun server disponibile per 'sum_service'. Prova ad aggiornare la cache.")
				continue
			}

			//Richiesta input all'utente ---
			fmt.Print("Inserisci il primo numero (A): ")
			scanner.Scan()
			valA, _ := strconv.Atoi(scanner.Text())

			fmt.Print("Inserisci il secondo numero (B): ")
			scanner.Scan()
			valB, _ := strconv.Atoi(scanner.Text())
			// --------------------------------------------

			fmt.Printf("\n>>> Inoltro richiesta SOMMA a %s...\n", targetAddr)
			callSum(targetAddr, int32(valA), int32(valB))

		case "2":
			// strategia LOWEST CPU
			targetAddr = getServer("increment_service", "lowest_cpu")
			if targetAddr == "" {
				fmt.Println("\n ERRORE: Nessun server disponibile per 'increment_service'. Prova ad aggiornare la cache.")
				continue
			}
			fmt.Printf("\n>>> Inoltro richiesta CONTATORE a %s...\n", targetAddr)
			callCounter(targetAddr)

		case "3":
			refreshCache(registryClient)

		case "q":
			fmt.Println("Uscita...")
			return

		default:
			fmt.Println("Opzione non valida.")
		}
	}
}
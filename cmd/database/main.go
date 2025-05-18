package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	pb "code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/generated/rpc"
)

func main() {
	port := flag.Int("port", 50051, "Database server port")
	dataLimit := flag.Int("data-limit", 1_000_000, "Maximum number of data points to store")
	flag.Parse()

	addr := fmt.Sprintf("0.0.0.0:%d", *port)

	//create a TCP listener and listen on the provided addr
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	grpcServer := grpc.NewServer()

	databaseService := database.DatabaseServiceFactory(*dataLimit)
	pb.RegisterDatabaseServiceServer(grpcServer, databaseService)

	//set up signal handling for graceful shutdown like when ctrl c is pressed for example
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	//here since we are starting the server in a go-routine, it will spanw up
	go func() {
		log.Printf("Database server starting on %s", addr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve: %v", err)
		}
	}()

	<-sigChan
	log.Println("Shutting down database server...")

	//wait for the conns to die off on their own first (basically dont force stop)
	grpcServer.GracefulStop()
	log.Println("Database server stopped")
}

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/generated/rpc"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// Client represents a client for the database service
type Client struct {
	conn   *grpc.ClientConn
	client pb.DatabaseServiceClient
}

// TwoPhaseCommitClient manages our new 2PC operations across multiple(2) database instances
type TwoPhaseCommitClient struct {
	clients []*Client
	timeout time.Duration
}

// ClientFactory creates a new client connected to the database service
func ClientFactory(serverAddr string) (*Client, error) {
	//set up the conn to our server
	conn, err := grpc.NewClient(serverAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(200*1024*1024), //200MB receive limit
			grpc.MaxCallSendMsgSize(200*1024*1024), //200MB send limit
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database server: %w", err)
	}

	//use the package created using protoc to create a new client
	client := pb.NewDatabaseServiceClient(conn)

	return &Client{
		conn:   conn,
		client: client,
	}, nil
}

// TwoPhaseCommitClientFactory creates a new 2PC client that manages multiple database connections
func TwoPhaseCommitClientFactory(serverAddresses []string) (*TwoPhaseCommitClient, error) {
	if len(serverAddresses) < 2 {
		return nil, fmt.Errorf("2PC requires at least 2 database addresses, got %d", len(serverAddresses))
	}

	clients := make([]*Client, len(serverAddresses))
	for i, addr := range serverAddresses {
		client, err := ClientFactory(addr)
		if err != nil {
			//when creating a TwoPhaseCommitClient for our case here, we need to connect to multiple databases.
			//if any connection fails, we should clean up the connections that were already successful.
			//this prevents resource leaks.
			//j < i: Only close connections that were successfully created (indices 0 to i-1)
			//do not close clients[i] because this one failed to create already, so there's nothing to close
			for j := range i {
				clients[j].Close()
			}
			return nil, fmt.Errorf("failed to connect to database %s: %w", addr, err)
		}
		clients[i] = client
	}

	return &TwoPhaseCommitClient{
		clients: clients,
		timeout: 30 * time.Second, //30 second timeout for 2PC operations
	}, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// Close closes all client connections in the 2PC client
func (tpc *TwoPhaseCommitClient) Close() error {
	var lastError error
	for _, client := range tpc.clients {
		if err := client.Close(); err != nil {
			lastError = err
		}
	}
	return lastError
}

// generateTransactionID generates a unique transaction ID
func generateTransactionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		//fallback to timestamp-based ID if random generation somehow fails
		return fmt.Sprintf("txn_%d", time.Now().UnixNano())
	}
	return "txn_" + hex.EncodeToString(bytes)
}

// AddDataPoint adds a new sensor data point to the database (direct, non-2PC)
func (c *Client) AddDataPoint(sensorData types.SensorData) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.SensorDataRequest{
		SensorId:  sensorData.SensorID,
		Timestamp: timestamppb.New(sensorData.Timestamp),
		Value:     sensorData.Value,
		Unit:      sensorData.Unit,
	}

	resp, err := c.client.CreateSensorData(ctx, req)
	if err != nil {
		return fmt.Errorf("error adding data point: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to add data point: %s", resp.Message)
	}

	return nil
}

// PrepareTransaction sends a prepare request to the database (Phase 1 of 2PC)
func (c *Client) PrepareTransaction(transactionID string, sensorData types.SensorData) (*pb.PrepareResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.TransactionRequest{
		TransactionId: transactionID,
		SensorData: &pb.SensorDataRequest{
			SensorId:  sensorData.SensorID,
			Timestamp: timestamppb.New(sensorData.Timestamp),
			Value:     sensorData.Value,
			Unit:      sensorData.Unit,
		},
	}

	resp, err := c.client.PrepareTransaction(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("error preparing transaction %s: %w", transactionID, err)
	}

	return resp, nil
}

// CommitTransaction sends a commit request to the database (Phase 2 of 2PC)
func (c *Client) CommitTransaction(transactionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.TransactionId{
		TransactionId: transactionID,
	}

	resp, err := c.client.CommitTransaction(ctx, req)
	if err != nil {
		return fmt.Errorf("error committing transaction %s: %w", transactionID, err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to commit transaction %s: %s", transactionID, resp.Message)
	}

	return nil
}

// AbortTransaction sends an abort request to the database (Phase 2 of 2PC)
func (c *Client) AbortTransaction(transactionID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.TransactionId{
		TransactionId: transactionID,
	}

	resp, err := c.client.AbortTransaction(ctx, req)
	if err != nil {
		return fmt.Errorf("error aborting transaction %s: %w", transactionID, err)
	}

	if !resp.Success {
		return fmt.Errorf("failed to abort transaction %s: %s", transactionID, resp.Message)
	}

	return nil
}

// AddDataPointWithTwoPhaseCommit performs a full 2PC operation to add sensor data across all databases
func (tpc *TwoPhaseCommitClient) AddDataPointWithTwoPhaseCommit(sensorData types.SensorData) error {
	transactionID := generateTransactionID()

	log.Printf("Starting 2PC transaction %s for sensor %s", transactionID, sensorData.SensorID)

	//phase 1: Prepare
	log.Printf("Phase 1: Preparing transaction %s across %d databases", transactionID, len(tpc.clients))

	prepareResponses := make([]*pb.PrepareResponse, len(tpc.clients))
	prepareErrors := make([]error, len(tpc.clients))

	//send prepare to all databases
	for i, client := range tpc.clients {
		resp, err := client.PrepareTransaction(transactionID, sensorData)
		prepareResponses[i] = resp
		prepareErrors[i] = err

		if err != nil {
			log.Printf("Prepare failed for database %d: %v", i, err)
		} else if !resp.Success {
			log.Printf("Prepare rejected by database %d: %s", i, resp.Message)
		} else {
			log.Printf("Prepare successful for database %d", i)
		}
	}

	//check if all databases prepared successfully
	allPrepared := true
	for i, err := range prepareErrors {
		if err != nil || prepareResponses[i] == nil || !prepareResponses[i].Success {
			allPrepared = false
			break
		}
	}

	//phase 2: Commit or Abort
	if allPrepared {
		log.Printf("Phase 2: All databases prepared successfully, committing transaction %s", transactionID)
		return tpc.commitAll(transactionID)
	} else {
		log.Printf("Phase 2: One or more databases failed to prepare, aborting transaction %s", transactionID)
		return tpc.abortAll(transactionID)
	}
}

// commitAll sends commit to all databases
func (tpc *TwoPhaseCommitClient) commitAll(transactionID string) error {
	var lastError error
	successCount := 0

	for i, client := range tpc.clients {
		err := client.CommitTransaction(transactionID)
		if err != nil {
			log.Printf("Commit failed for database %d: %v", i, err)
			lastError = err
		} else {
			log.Printf("Commit successful for database %d", i)
			successCount++
		}
	}

	if successCount == len(tpc.clients) {
		log.Printf("Transaction %s committed successfully across all %d databases", transactionID, successCount)
		return nil
	} else {
		return fmt.Errorf("transaction %s: only %d of %d databases committed successfully, last error: %v",
			transactionID, successCount, len(tpc.clients), lastError)
	}
}

// abortAll sends abort to all databases
func (tpc *TwoPhaseCommitClient) abortAll(transactionID string) error {
	var lastError error
	abortCount := 0

	for i, client := range tpc.clients {
		err := client.AbortTransaction(transactionID)
		if err != nil {
			log.Printf("Abort failed for database %d: %v", i, err)
			lastError = err
		} else {
			log.Printf("Abort successful for database %d", i)
			abortCount++
		}
	}

	log.Printf("Transaction %s aborted on %d of %d databases", transactionID, abortCount, len(tpc.clients))

	if lastError != nil {
		return fmt.Errorf("transaction %s aborted, but some abort operations failed: %v", transactionID, lastError)
	}

	return fmt.Errorf("transaction %s was aborted due to prepare phase failures", transactionID)
}

// GetAllDataPoints returns all stored sensor data from the first database
func (c *Client) GetAllDataPoints() ([]types.SensorData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.client.GetAllSensorData(ctx, &pb.EmptyRequest{})
	if err != nil {
		return nil, fmt.Errorf("error getting all data points: %w", err)
	}

	result := make([]types.SensorData, len(resp.Data))
	for i, data := range resp.Data {
		result[i] = types.SensorData{
			SensorID:  data.SensorId,
			Timestamp: data.Timestamp.AsTime(),
			Value:     data.Value,
			Unit:      data.Unit,
		}
	}

	return result, nil
}

// GetAllDataPoints returns all stored sensor data from the first database (2PC client)
func (tpc *TwoPhaseCommitClient) GetAllDataPoints() ([]types.SensorData, error) {
	if len(tpc.clients) == 0 {
		return nil, fmt.Errorf("no database clients available")
	}

	//for read operations, we can use any database, but here i have taken the first one
	return tpc.clients[0].GetAllDataPoints()
}

// GetDataPointBySensorId returns data for a specific sensor
func (c *Client) GetDataPointBySensorId(sensorID string) ([]types.SensorData, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.client.GetSensorDataBySensorId(ctx, &pb.SensorIdRequest{
		SensorId: sensorID,
	})
	if err != nil {
		return nil, fmt.Errorf("error getting data points for sensor %s: %w", sensorID, err)
	}

	result := make([]types.SensorData, len(resp.Data))
	for i, data := range resp.Data {
		result[i] = types.SensorData{
			SensorID:  data.SensorId,
			Timestamp: data.Timestamp.AsTime(),
			Value:     data.Value,
			Unit:      data.Unit,
		}
	}

	return result, nil
}

// GetDataPointBySensorId returns data for a specific sensor (2PC client)
func (tpc *TwoPhaseCommitClient) GetDataPointBySensorId(sensorID string) ([]types.SensorData, error) {
	if len(tpc.clients) == 0 {
		return nil, fmt.Errorf("no database clients available")
	}

	//for read operations, we can use any database, but here i have taken the first one
	return tpc.clients[0].GetDataPointBySensorId(sensorID)
}

// MeasureRPCLatency measures the round-trip time for an RPC call
func (c *Client) MeasureRPCLatency() (time.Duration, error) {
	dummySensorData := types.SensorData{
		SensorID:  "perf-test",
		Timestamp: time.Now(),
		Value:     42.0,
		Unit:      "test",
	}

	//to measure time for a round-trip call
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req := &pb.SensorDataRequest{
		SensorId:  dummySensorData.SensorID,
		Timestamp: timestamppb.New(dummySensorData.Timestamp),
		Value:     dummySensorData.Value,
		Unit:      dummySensorData.Unit,
	}

	_, err := c.client.CreateSensorData(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("error during performance test: %w", err)
	}

	return time.Since(start), nil
}

// MeasureTwoPhaseCommitLatency measures the round-trip time for a 2PC operation
func (tpc *TwoPhaseCommitClient) MeasureTwoPhaseCommitLatency() (time.Duration, error) {
	sensorData := types.SensorData{
		SensorID:  "2pc-perf-test",
		Timestamp: time.Now(),
		Value:     42.0,
		Unit:      "test",
	}

	start := time.Now()
	err := tpc.AddDataPointWithTwoPhaseCommit(sensorData)
	if err != nil {
		return 0, fmt.Errorf("error during 2PC performance test: %w", err)
	}

	return time.Since(start), nil
}

// RunPerformanceTest runs a simple performance test and returns statistics
func (c *Client) RunPerformanceTest(iterations int) (min, max, avg time.Duration, err error) {
	log.Printf("Running RPC performance test with %d iterations", iterations)

	var total time.Duration
	min = time.Hour //start with a large value initially like before

	for range iterations {
		rtt, err := c.MeasureRPCLatency()
		if err != nil {
			return 0, 0, 0, err
		}

		if rtt < min {
			min = rtt
		}
		if rtt > max {
			max = rtt
		}
		total += rtt
	}

	avg = total / time.Duration(iterations)

	log.Printf("RPC Performance Test Results:")
	log.Printf("  Total requests: %d", iterations)
	log.Printf("  Min RTT:        %v", min)
	log.Printf("  Max RTT:        %v", max)
	log.Printf("  Mean RTT:       %v", avg)

	return min, max, avg, nil
}

// RunTwoPhaseCommitPerformanceTest runs a 2PC performance test
func (tpc *TwoPhaseCommitClient) RunTwoPhaseCommitPerformanceTest(iterations int) (min, max, avg time.Duration, err error) {
	log.Printf("Running 2PC performance test with %d iterations across %d databases", iterations, len(tpc.clients))

	var total time.Duration
	min = time.Hour

	for i := range iterations {
		rtt, err := tpc.MeasureTwoPhaseCommitLatency()
		if err != nil {
			log.Printf("2PC iteration %d failed: %v", i, err)
			continue
		}

		if rtt < min {
			min = rtt
		}
		if rtt > max {
			max = rtt
		}
		total += rtt
	}

	avg = total / time.Duration(iterations)

	log.Printf("2PC Performance Test Results:")
	log.Printf("  Total requests: %d", iterations)
	log.Printf("  Min RTT:        %v", min)
	log.Printf("  Max RTT:        %v", max)
	log.Printf("  Mean RTT:       %v", avg)
	log.Printf("  Databases:      %d", len(tpc.clients))

	return min, max, avg, nil
}

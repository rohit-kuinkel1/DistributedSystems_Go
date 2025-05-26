package database

import (
	"context"
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

// NewClient creates a new client connected to the database service
func NewClient(serverAddr string) (*Client, error) {
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

// Close closes the client connection
func (c *Client) Close() error {
	return c.conn.Close()
}

// AddDataPoint adds a new sensor data point to the database
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

// GetAllDataPoints returns all stored sensor data
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

// MeasureRPCLatency measures the round-trip time for an RPC call
func (c *Client) MeasureRPCLatency() (time.Duration, error) {
	//dummy data point for testing only
	sensorData := types.SensorData{
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
		SensorId:  sensorData.SensorID,
		Timestamp: timestamppb.New(sensorData.Timestamp),
		Value:     sensorData.Value,
		Unit:      sensorData.Unit,
	}

	_, err := c.client.CreateSensorData(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("error during performance test: %w", err)
	}

	return time.Since(start), nil
}

// RunPerformanceTest runs a simple performance test and returns statistics
func (c *Client) RunPerformanceTest(iterations int) (min, max, avg time.Duration, err error) {
	log.Printf("Running RPC performance test with %d iterations", iterations)

	var total time.Duration
	min = time.Hour //start with a large value initially

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

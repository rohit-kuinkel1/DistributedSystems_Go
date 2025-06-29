package database

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/generated/rpc"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TransactionState represents the state of a prepared transaction
type TransactionState struct {
	TransactionID string
	SensorData    types.SensorData
	PreparedAt    time.Time
}

// DatabaseService implements the DatabaseService gRPC service.
type DatabaseService struct {
	pb.UnimplementedDatabaseServiceServer
	mu            sync.RWMutex
	data          []types.SensorData
	maxDataPoints int

	// Two-Phase Commit state management
	preparedTxns  map[string]*TransactionState // transaction_id -> prepared transaction
	txnMutex      sync.RWMutex                 // separate mutex for transaction state
	txnTimeout    time.Duration                // timeout for prepared transactions
	cleanupTicker *time.Ticker                 // cleanup ticker for expired transactions
	stopCleanup   chan struct{}                // channel to stop cleanup goroutine
}

// DatabaseServiceFactory creates a new database service with a specified size limit.
func DatabaseServiceFactory(limit int) *DatabaseService {
	service := &DatabaseService{
		data:          make([]types.SensorData, 0, limit),
		maxDataPoints: limit,
		preparedTxns:  make(map[string]*TransactionState),
		txnTimeout:    30 * time.Second, //30 second timeout for prepared transactions
		stopCleanup:   make(chan struct{}),
	}

	//start cleanup goroutine for expired transactions
	service.startTransactionCleanup()

	return service
}

// startTransactionCleanup starts a goroutine to clean up expired prepared transactions
func (s *DatabaseService) startTransactionCleanup() {
	s.cleanupTicker = time.NewTicker(5 * time.Second) //check every 5 seconds

	go func() {
		for {
			select {
			case <-s.cleanupTicker.C:
				s.cleanupExpiredTransactions()
			case <-s.stopCleanup:
				s.cleanupTicker.Stop()
				return
			}
		}
	}()
}

// cleanupExpiredTransactions removes transactions that have exceeded the timeout
func (s *DatabaseService) cleanupExpiredTransactions() {
	s.txnMutex.Lock()
	defer s.txnMutex.Unlock()

	now := time.Now()
	for txnID, txnState := range s.preparedTxns {
		if now.Sub(txnState.PreparedAt) > s.txnTimeout {
			delete(s.preparedTxns, txnID)
			log.Printf("Cleaned up expired transaction: %s", txnID)
		}
	}
}

// Stop gracefully stops the database service
func (s *DatabaseService) Stop() {
	close(s.stopCleanup)
}

// Convert from SensorDataRequest (protobuf) to SensorData (internal type)
func protoToSensorData(req *pb.SensorDataRequest) types.SensorData {
	var timestamp time.Time
	if req.Timestamp != nil {
		timestamp = req.Timestamp.AsTime()
	} else {
		timestamp = time.Now()
	}

	return types.SensorData{
		SensorID:  req.SensorId,
		Timestamp: timestamp,
		Value:     req.Value,
		Unit:      req.Unit,
	}
}

// Convert from SensorData (internal type) to SensorDataRequest (protobuf)
func sensorDataToProto(data types.SensorData) *pb.SensorDataRequest {
	return &pb.SensorDataRequest{
		SensorId:  data.SensorID,
		Timestamp: timestamppb.New(data.Timestamp),
		Value:     data.Value,
		Unit:      data.Unit,
	}
}

// addDataPointInternal adds sensor data to the internal storage (used by both direct and 2PC paths)
func (s *DatabaseService) addDataPointInternal(sensorData types.SensorData) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = append(s.data, sensorData)

	//if we exceeded the limit, remove the oldest data points following FIFO
	if len(s.data) > s.maxDataPoints {
		s.data = s.data[len(s.data)-s.maxDataPoints:]
	}

	log.Printf("Stored data from sensor %s: %.2f %s", sensorData.SensorID, sensorData.Value, sensorData.Unit)
}

// CreateSensorData adds new sensor data to the store (direct path, non-2PC).
func (s *DatabaseService) CreateSensorData(ctx context.Context, req *pb.SensorDataRequest) (*pb.OperationResponse, error) {
	if req.SensorId == "" {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing sensor ID",
		}, nil
	}

	sensorData := protoToSensorData(req)
	s.addDataPointInternal(sensorData)

	return &pb.OperationResponse{
		Success: true,
		Message: "Data stored successfully",
	}, nil
}

// PrepareTransaction implements the prepare phase of Two-Phase Commit
func (s *DatabaseService) PrepareTransaction(ctx context.Context, req *pb.TransactionRequest) (*pb.PrepareResponse, error) {
	if req.TransactionId == "" {
		return &pb.PrepareResponse{
			Success: false,
			Message: "Missing transaction ID",
		}, nil
	}

	if req.SensorData == nil {
		return &pb.PrepareResponse{
			Success: false,
			Message: "Missing sensor data",
		}, nil
	}

	if req.SensorData.SensorId == "" {
		return &pb.PrepareResponse{
			Success: false,
			Message: "Missing sensor ID in sensor data",
		}, nil
	}

	s.txnMutex.Lock()
	defer s.txnMutex.Unlock()

	//check if transaction already exists
	if _, exists := s.preparedTxns[req.TransactionId]; exists {
		return &pb.PrepareResponse{
			Success:       false,
			Message:       "Transaction already prepared",
			TransactionId: req.TransactionId,
		}, nil
	}

	sensorData := protoToSensorData(req.SensorData)

	//store the transaction state in the prepared transactions for now
	s.preparedTxns[req.TransactionId] = &TransactionState{
		TransactionID: req.TransactionId,
		SensorData:    sensorData,
		PreparedAt:    time.Now(),
	}

	log.Printf("Prepared transaction %s for sensor %s", req.TransactionId, sensorData.SensorID)

	return &pb.PrepareResponse{
		Success:       true,
		Message:       "Transaction prepared successfully",
		TransactionId: req.TransactionId,
	}, nil
}

// CommitTransaction implements the commit phase of Two-Phase Commit
func (s *DatabaseService) CommitTransaction(ctx context.Context, req *pb.TransactionId) (*pb.OperationResponse, error) {
	if req.TransactionId == "" {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing transaction ID",
		}, nil
	}

	s.txnMutex.Lock()
	defer s.txnMutex.Unlock()

	//find the prepared transaction
	txnState, exists := s.preparedTxns[req.TransactionId]
	if !exists {
		return &pb.OperationResponse{
			Success: false,
			Message: fmt.Sprintf("Transaction %s not found or not prepared", req.TransactionId),
		}, nil
	}

	//the actual commit of the data is done here
	s.addDataPointInternal(txnState.SensorData)

	//after that, we need to remove from prepared transactions
	delete(s.preparedTxns, req.TransactionId)

	log.Printf("Committed transaction %s for sensor %s", req.TransactionId, txnState.SensorData.SensorID)

	return &pb.OperationResponse{
		Success: true,
		Message: "Transaction committed successfully",
	}, nil
}

// AbortTransaction implements the abort phase of Two-Phase Commit
func (s *DatabaseService) AbortTransaction(ctx context.Context, req *pb.TransactionId) (*pb.OperationResponse, error) {
	if req.TransactionId == "" {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing transaction ID",
		}, nil
	}

	s.txnMutex.Lock()
	defer s.txnMutex.Unlock()

	//find and remove the prepared transaction
	txnState, exists := s.preparedTxns[req.TransactionId]
	if !exists {
		return &pb.OperationResponse{
			Success: false,
			Message: fmt.Sprintf("Transaction %s not found or not prepared", req.TransactionId),
		}, nil
	}

	//remove from the prepared transactions (the data is discarded)
	delete(s.preparedTxns, req.TransactionId)

	log.Printf("Aborted transaction %s for sensor %s", req.TransactionId, txnState.SensorData.SensorID)

	return &pb.OperationResponse{
		Success: true,
		Message: "Transaction aborted successfully",
	}, nil
}

// GetAllSensorData returns all stored sensor data.
func (s *DatabaseService) GetAllSensorData(ctx context.Context, req *pb.EmptyRequest) (*pb.SensorDataList, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := &pb.SensorDataList{
		Data: make([]*pb.SensorDataRequest, len(s.data)),
	}

	for i, data := range s.data {
		result.Data[i] = sensorDataToProto(data)
	}

	return result, nil
}

// GetSensorDataBySensorId returns data for a specific sensor.
func (s *DatabaseService) GetSensorDataBySensorId(ctx context.Context, req *pb.SensorIdRequest) (*pb.SensorDataList, error) {
	if req.SensorId == "" {
		return &pb.SensorDataList{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*pb.SensorDataRequest
	for _, data := range s.data {
		if data.SensorID == req.SensorId {
			result = append(result, sensorDataToProto(data))
		}
	}

	return &pb.SensorDataList{
		Data: result,
	}, nil
}

// UpdateSensorData updates existing sensor data (matching by SensorID and Timestamp).
func (s *DatabaseService) UpdateSensorData(ctx context.Context, req *pb.SensorDataRequest) (*pb.OperationResponse, error) {
	if req.SensorId == "" || req.Timestamp == nil {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing sensor ID or timestamp",
		}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	updated := false
	timestamp := req.Timestamp.AsTime()

	for i, data := range s.data {
		if data.SensorID == req.SensorId && data.Timestamp.Equal(timestamp) {
			s.data[i].Value = req.Value
			s.data[i].Unit = req.Unit
			updated = true
			break
		}
	}

	if !updated {
		return &pb.OperationResponse{
			Success: false,
			Message: "Data not found",
		}, nil
	}

	return &pb.OperationResponse{
		Success: true,
		Message: "Data updated successfully",
	}, nil
}

// DeleteSensorData deletes all data for a specific sensor.
func (s *DatabaseService) DeleteSensorData(ctx context.Context, req *pb.SensorIdRequest) (*pb.OperationResponse, error) {
	if req.SensorId == "" {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing sensor ID",
		}, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	initialLen := len(s.data)
	newData := make([]types.SensorData, 0, initialLen)

	for _, data := range s.data {
		if data.SensorID != req.SensorId {
			newData = append(newData, data)
		}
	}

	s.data = newData

	return &pb.OperationResponse{
		Success: true,
		Message: "Deleted data for sensor",
	}, nil
}

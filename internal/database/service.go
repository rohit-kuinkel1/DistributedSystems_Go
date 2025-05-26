package database

import (
	"context"
	"log"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pb "code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/generated/rpc"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// DatabaseService implements the DatabaseService gRPC service.
type DatabaseService struct {
	pb.UnimplementedDatabaseServiceServer
	mu            sync.RWMutex
	data          []types.SensorData
	maxDataPoints int
}

// DatabaseServiceFactory creates a new database service with a specified size limit.
func DatabaseServiceFactory(limit int) *DatabaseService {
	return &DatabaseService{
		data:          make([]types.SensorData, 0, limit),
		maxDataPoints: limit,
	}
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

// CreateSensorData adds new sensor data to the store.
func (s *DatabaseService) CreateSensorData(ctx context.Context, req *pb.SensorDataRequest) (*pb.OperationResponse, error) {
	if req.SensorId == "" {
		return &pb.OperationResponse{
			Success: false,
			Message: "Missing sensor ID",
		}, nil
	}

	sensorData := protoToSensorData(req)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.data = append(s.data, sensorData)

	//if weve exceeded the limit, remove the oldest data points following FIFO
	if len(s.data) > s.maxDataPoints {
		s.data = s.data[len(s.data)-s.maxDataPoints:]
	}

	log.Printf(
		"Stored data from sensor %s: %.2f %s",
		sensorData.SensorID,
		sensorData.Value,
		sensorData.Unit,
	)

	return &pb.OperationResponse{
		Success: true,
		Message: "Data stored successfully",
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

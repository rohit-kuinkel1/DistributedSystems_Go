package functional

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TestHTTPServerWithRedundantStorage tests the HTTP server with 2PC redundant storage
func TestHTTPServerWithRedundantStorage(t *testing.T) {
	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client: %v", err)
	}
	defer tpcClient.Close()

	server := http.ServerFactory("localhost", 8082)
	register2PCHandlers(server, tpcClient)

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	//wait for server to start
	time.Sleep(100 * time.Millisecond)

	client := http.HttpClientFactory(5 * time.Second)
	testData := types.SensorData{
		SensorID:  "http-2pc-test",
		Timestamp: time.Now(),
		Value:     23.5,
		Unit:      "°C",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	resp, err := client.PostJSON("http://localhost:8082/data", jsonData)
	if err != nil {
		t.Fatalf("Failed to send POST request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
		t.Logf("Response body: %s", string(resp.Body))
	}

	//verify data was stored in both databases by checking through the 2PC client
	storedData, err := tpcClient.GetDataPointBySensorId(testData.SensorID)
	if err != nil {
		t.Errorf("Failed to retrieve stored data: %v", err)
	}

	if len(storedData) != 1 {
		t.Errorf("Expected 1 stored data point, got %d", len(storedData))
	} else {
		if storedData[0].SensorID != testData.SensorID {
			t.Errorf("Expected sensor ID %s, got %s", testData.SensorID, storedData[0].SensorID)
		}
		if storedData[0].Value != testData.Value {
			t.Errorf("Expected value %.1f, got %.1f", testData.Value, storedData[0].Value)
		}
		if storedData[0].Unit != testData.Unit {
			t.Errorf("Expected unit %s, got %s", testData.Unit, storedData[0].Unit)
		}
	}

	log.Println("HTTP server with redundant storage test passed")
}

// TestHTTPGetWithRedundantStorage tests GET requests with redundant storage
func TestHTTPGetWithRedundantStorage(t *testing.T) {
	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client: %v", err)
	}
	defer tpcClient.Close()

	server := http.ServerFactory("localhost", 8083)
	register2PCHandlers(server, tpcClient)

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	//wait for server to start
	time.Sleep(100 * time.Millisecond)

	testDataSet := []types.SensorData{
		{
			SensorID:  "http-get-test-1",
			Timestamp: time.Now(),
			Value:     10.0,
			Unit:      "°C",
		},
		{
			SensorID:  "http-get-test-2",
			Timestamp: time.Now().Add(1 * time.Second),
			Value:     20.0,
			Unit:      "°C",
		},
	}

	for _, data := range testDataSet {
		err = tpcClient.AddDataPointWithTwoPhaseCommit(data)
		if err != nil {
			t.Fatalf("Failed to add test data: %v", err)
		}
	}

	client := http.HttpClientFactory(5 * time.Second)

	resp, err := client.Get("http://localhost:8083/data")
	if err != nil {
		t.Fatalf("Failed to send GET request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	var allData []types.SensorData
	err = json.Unmarshal(resp.Body, &allData)
	if err != nil {
		t.Errorf("Failed to parse GET response: %v", err)
	}

	//filter to incl our test data
	testData := filterTestData2(allData, "http-get-test-")
	if len(testData) < len(testDataSet) {
		t.Errorf("Expected at least %d data points, got %d", len(testDataSet), len(testData))
	}

	resp, err = client.Get("http://localhost:8083/data/http-get-test-1")
	if err != nil {
		t.Fatalf("Failed to send GET request for specific sensor: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 for specific sensor, got %d", resp.StatusCode)
	}

	var sensorData []types.SensorData
	err = json.Unmarshal(resp.Body, &sensorData)
	if err != nil {
		t.Errorf("Failed to parse sensor-specific GET response: %v", err)
	}

	if len(sensorData) != 1 {
		t.Errorf("Expected 1 data point for specific sensor, got %d", len(sensorData))
	} else if sensorData[0].SensorID != "http-get-test-1" {
		t.Errorf("Expected sensor ID 'http-get-test-1', got %s", sensorData[0].SensorID)
	}

	log.Println("HTTP GET with redundant storage test passed")
}

// Helper function to filter test data by sensor ID prefix
func filterTestData2(data []types.SensorData, prefix string) []types.SensorData {
	var filtered []types.SensorData
	for _, d := range data {
		if len(d.SensorID) >= len(prefix) && d.SensorID[:len(prefix)] == prefix {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// TestHTTPDataConsistencyAfterMultiplePosts tests data consistency with multiple HTTP POST requests
func TestHTTPDataConsistencyAfterMultiplePosts(t *testing.T) {
	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client: %v", err)
	}
	defer tpcClient.Close()

	server := http.ServerFactory("localhost", 8084)
	register2PCHandlers(server, tpcClient)

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	client := http.HttpClientFactory(5 * time.Second)

	//send multiple POST requests
	testDataSet := []types.SensorData{
		{SensorID: "http-consistency-1", Value: 1.1, Unit: "°C", Timestamp: time.Now()},
		{SensorID: "http-consistency-2", Value: 2.2, Unit: "°C", Timestamp: time.Now()},
		{SensorID: "http-consistency-3", Value: 3.3, Unit: "°C", Timestamp: time.Now()},
	}

	for _, testData := range testDataSet {
		jsonData, err := json.Marshal(testData)
		if err != nil {
			t.Fatalf("Failed to marshal JSON: %v", err)
		}

		resp, err := client.PostJSON("http://localhost:8084/data", jsonData)
		if err != nil {
			t.Fatalf("Failed to send POST request: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Errorf("POST failed for %s: status %d", testData.SensorID, resp.StatusCode)
		}
	}

	//verify data consistency by checking both databases directly
	client1, err := database.ClientFactory("localhost:50051")
	if err != nil {
		t.Fatalf("Failed to connect to database1: %v", err)
	}
	defer client1.Close()

	client2, err := database.ClientFactory("localhost:50052")
	if err != nil {
		t.Fatalf("Failed to connect to database2: %v", err)
	}
	defer client2.Close()

	//get all data from both databases
	allData1, err := client1.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get data from database1: %v", err)
	}

	allData2, err := client2.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get data from database2: %v", err)
	}

	testData1 := filterTestData2(allData1, "http-consistency-")
	testData2 := filterTestData2(allData2, "http-consistency-")

	//now verify consistency
	if len(testData1) != len(testData2) {
		t.Errorf("Data count mismatch: db1=%d, db2=%d", len(testData1), len(testData2))
	}

	if len(testData1) != len(testDataSet) {
		t.Errorf("Expected %d records, got %d in database1", len(testDataSet), len(testData1))
	}

	for i := 0; i < len(testData1) && i < len(testData2); i++ {
		if testData1[i].SensorID != testData2[i].SensorID {
			t.Errorf("SensorID mismatch at index %d: db1=%s, db2=%s",
				i, testData1[i].SensorID, testData2[i].SensorID)
		}
		if testData1[i].Value != testData2[i].Value {
			t.Errorf("Value mismatch at index %d: db1=%.2f, db2=%.2f",
				i, testData1[i].Value, testData2[i].Value)
		}
	}

	log.Println("HTTP data consistency test passed")
}

// register2PCHandlers registers HTTP handlers that use 2PC for storage
func register2PCHandlers(server *http.Server, tpcClient *database.TwoPhaseCommitClient) {
	//handler for HTTP POST requests to add sensor data using 2PC
	server.RegisterHandler(
		http.POST,
		"/data",
		func(req *http.Request) *http.Response {
			var sensorData types.SensorData
			err := json.Unmarshal(req.Body, &sensorData)
			if err != nil {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString(fmt.Sprintf("Invalid JSON: %v", err))
				return resp
			}

			if sensorData.SensorID == "" {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Missing sensorId")
				return resp
			}

			if sensorData.Timestamp.IsZero() {
				sensorData.Timestamp = time.Now()
			}

			err = tpcClient.AddDataPointWithTwoPhaseCommit(sensorData)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error storing data: %v", err))
				return resp
			}

			resp := http.NewResponse(http.StatusOK)
			resp.SetBodyString("Data stored successfully using Two-Phase Commit")
			return resp
		},
	)

	// Handler for HTTP GET requests to retrieve all sensor data
	server.RegisterHandler(
		http.GET,
		"/data",
		func(req *http.Request) *http.Response {
			allData, err := tpcClient.GetAllDataPoints()
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error retrieving data: %v", err))
				return resp
			}

			jsonData, err := json.Marshal(allData)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error marshaling data: %v", err))
				return resp
			}

			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)

	// Handler for HTTP GET requests to retrieve data for a specific sensor
	server.RegisterHandler(
		http.GET,
		"*",
		func(req *http.Request) *http.Response {
			if !strings.HasPrefix(req.Path, "/data/") {
				resp := http.NewResponse(http.StatusNotFound)
				resp.SetBodyString("Not found")
				return resp
			}

			path := req.Path
			if path == "/data/" {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Missing sensor ID")
				return resp
			}

			sensorID := path[6:] // Remove "/data/"

			sensorData, err := tpcClient.GetDataPointBySensorId(sensorID)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error retrieving data: %v", err))
				return resp
			}

			if len(sensorData) == 0 {
				resp := http.NewResponse(http.StatusNotFound)
				resp.SetBodyString(fmt.Sprintf("No data found for sensor %s", sensorID))
				return resp
			}

			jsonData, err := json.Marshal(sensorData)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error marshaling data: %v", err))
				return resp
			}

			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)
}

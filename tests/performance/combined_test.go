package performance

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"slices"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TestCombinedHTTPRPCPerformance tests HTTP performance while RPC interface is under load
func TestCombinedHTTPRPCPerformance(t *testing.T) {
	serverHost := "localhost"
	serverPort := 8083
	dbAddr := "localhost:50051"

	dbClient, err := database.NewClient(dbAddr)
	if err != nil {
		t.Fatalf("Failed to connect to database service: %v", err)
	}
	defer dbClient.Close()

	//start HTTP server
	server := http.ServerFactory(serverHost, serverPort)
	registerTestHandler(server, dbClient)

	err = server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	time.Sleep(100 * time.Millisecond)

	//test parameters
	httpRequests := 1_000_000
	rpcRequests := 1_000_000
	concurrentHTTPClients := 10
	concurrentRPCClients := 10

	log.Printf("Starting combined HTTP+RPC performance test")
	log.Printf("HTTP: %d requests from %d concurrent clients", httpRequests, concurrentHTTPClients)
	log.Printf("RPC: %d requests from %d concurrent clients (background load)", rpcRequests, concurrentRPCClients)

	testData := types.SensorData{
		SensorID:  "combined-perf-test",
		Timestamp: time.Now(),
		Value:     25.5,
		Unit:      "°C",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	url := fmt.Sprintf("http://%s:%d/data", serverHost, serverPort)

	//channels for collecting results
	httpRTTs := make(chan time.Duration, httpRequests)
	rpcRTTs := make(chan time.Duration, rpcRequests)

	var wg sync.WaitGroup

	//start RPC background load
	log.Println("Starting RPC background load...")
	requestsPerRPCClient := rpcRequests / concurrentRPCClients
	for i := 0; i < concurrentRPCClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			for j := 0; j < requestsPerRPCClient; j++ {
				start := time.Now()
				err := dbClient.AddDataPoint(testData)
				if err != nil {
					log.Printf("RPC Client %d: Error: %v", clientID, err)
					continue
				}
				rtt := time.Since(start)
				rpcRTTs <- rtt
			}
		}(i)
	}

	//give RPC load time to ramp up
	time.Sleep(500 * time.Millisecond)

	//start HTTP performance test while RPC is under load
	log.Println("Starting HTTP performance test with RPC under load...")
	requestsPerHTTPClient := httpRequests / concurrentHTTPClients
	for i := 0; i < concurrentHTTPClients; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()
			client := http.HttpClientFactory(5 * time.Second)

			for j := 0; j < requestsPerHTTPClient; j++ {
				start := time.Now()
				resp, err := client.PostJSON(url, jsonData)
				if err != nil {
					log.Printf("HTTP Client %d: Error: %v", clientID, err)
					continue
				}
				rtt := time.Since(start)

				if resp.StatusCode != http.StatusOK {
					log.Printf("HTTP Client %d: Expected status 200, got %d", clientID, resp.StatusCode)
					continue
				}

				httpRTTs <- rtt
			}
		}(i)
	}

	wg.Wait()
	close(httpRTTs)
	close(rpcRTTs)

	//collect and analyze results
	var httpRTTValues []time.Duration
	var rpcRTTValues []time.Duration

	for rtt := range httpRTTs {
		httpRTTValues = append(httpRTTValues, rtt)
	}

	for rtt := range rpcRTTs {
		rpcRTTValues = append(rpcRTTValues, rtt)
	}

	httpStats := calculateCombinedStatistics(httpRTTValues, "HTTP")
	rpcStats := calculateCombinedStatistics(rpcRTTValues, "RPC")

	log.Printf("Combined Performance Test Results:")
	log.Printf("HTTP (under RPC load):")
	logStatistics(httpStats)
	log.Printf("RPC (background load):")
	logStatistics(rpcStats)

	err = writeCombinedResultsToFile(httpStats, rpcStats, "combined_http_rpc_performance_results.txt")
	if err != nil {
		t.Errorf("Failed to write results to file: %v", err)
	}

	log.Println("Combined HTTP+RPC performance test completed")
}

// registerTestHandler registers a simple handler for performance testing
func registerTestHandler(server *http.Server, dbClient *database.Client) {
	server.RegisterHandler(
		http.POST,
		"/data",
		func(req *http.Request) *http.Response {
			var sensorData types.SensorData
			err := json.Unmarshal(req.Body, &sensorData)
			if err != nil {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Invalid JSON")
				return resp
			}

			//store data via RPC
			err = dbClient.AddDataPoint(sensorData)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString("Database error")
				return resp
			}

			resp := http.NewResponse(http.StatusOK)
			resp.SetBodyString("OK")
			return resp
		},
	)
}

// CombinedStatistics contains statistical measures for performance tests
type CombinedStatistics struct {
	Protocol          string
	Count             int
	Min               time.Duration
	Max               time.Duration
	Mean              time.Duration
	Median            time.Duration
	StdDev            time.Duration
	Percentile90      time.Duration
	Percentile95      time.Duration
	Percentile99      time.Duration
	RequestsPerSecond float64
	TotalDuration     time.Duration
}

// calculateCombinedStatistics calculates statistical measures from RTT measurements
func calculateCombinedStatistics(rtts []time.Duration, protocol string) CombinedStatistics {
	if len(rtts) == 0 {
		return CombinedStatistics{Protocol: protocol}
	}

	slices.Sort(rtts)

	count := len(rtts)
	min := rtts[0]
	max := rtts[count-1]

	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	mean := sum / time.Duration(count)
	var median time.Duration
	if count%2 == 0 {
		median = (rtts[count/2-1] + rtts[count/2]) / 2
	} else {
		median = rtts[count/2]
	}

	var sumSquaredDifferences float64
	for _, rtt := range rtts {
		diff := float64(rtt - mean)
		sumSquaredDifferences += diff * diff
	}
	variance := sumSquaredDifferences / float64(count)
	stdDev := time.Duration(math.Sqrt(variance))

	p90Index := int(float64(count) * 0.9)
	p95Index := int(float64(count) * 0.95)
	p99Index := int(float64(count) * 0.99)

	percentile90 := rtts[p90Index]
	percentile95 := rtts[p95Index]
	percentile99 := rtts[p99Index]

	//requests per second
	totalDuration := sum
	requestsPerSecond := float64(count) / totalDuration.Seconds()

	return CombinedStatistics{
		Protocol:          protocol,
		Count:             count,
		Min:               min,
		Max:               max,
		Mean:              mean,
		Median:            median,
		StdDev:            stdDev,
		Percentile90:      percentile90,
		Percentile95:      percentile95,
		Percentile99:      percentile99,
		RequestsPerSecond: requestsPerSecond,
		TotalDuration:     totalDuration,
	}
}

// logStatistics logs performance statistics
func logStatistics(stats CombinedStatistics) {
	log.Printf("  Total requests:     %d", stats.Count)
	log.Printf("  Min RTT:            %v", stats.Min)
	log.Printf("  Max RTT:            %v", stats.Max)
	log.Printf("  Mean RTT:           %v", stats.Mean)
	log.Printf("  Median RTT:         %v", stats.Median)
	log.Printf("  Standard deviation: %v", stats.StdDev)
	log.Printf("  90th percentile:    %v", stats.Percentile90)
	log.Printf("  95th percentile:    %v", stats.Percentile95)
	log.Printf("  99th percentile:    %v", stats.Percentile99)
	log.Printf("  Requests per second: %.2f", stats.RequestsPerSecond)
}

// writeCombinedResultsToFile writes combined test results to a file
func writeCombinedResultsToFile(httpStats, rpcStats CombinedStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("Combined HTTP+RPC Performance Test Results\n")
	file.WriteString("==========================================\n\n")

	file.WriteString("HTTP Performance (under RPC load):\n")
	file.WriteString("----------------------------------\n")
	writeStatsToFile(file, httpStats)

	file.WriteString("\nRPC Performance (background load):\n")
	file.WriteString("----------------------------------\n")
	writeStatsToFile(file, rpcStats)

	return nil
}

// writeStatsToFile writes statistics to file
func writeStatsToFile(file *os.File, stats CombinedStatistics) {
	fmt.Fprintf(file, "Total requests:     %d\n", stats.Count)
	fmt.Fprintf(file, "Min RTT:            %v\n", stats.Min)
	fmt.Fprintf(file, "Max RTT:            %v\n", stats.Max)
	fmt.Fprintf(file, "Mean RTT:           %v\n", stats.Mean)
	fmt.Fprintf(file, "Median RTT:         %v\n", stats.Median)
	fmt.Fprintf(file, "Standard deviation: %v\n", stats.StdDev)
	fmt.Fprintf(file, "90th percentile:    %v\n", stats.Percentile90)
	fmt.Fprintf(file, "95th percentile:    %v\n", stats.Percentile95)
	fmt.Fprintf(file, "99th percentile:    %v\n", stats.Percentile99)
	fmt.Fprintf(file, "Requests per second: %.2f\n", stats.RequestsPerSecond)
	fmt.Fprintf(file, "Total duration:     %v\n", stats.TotalDuration)
}

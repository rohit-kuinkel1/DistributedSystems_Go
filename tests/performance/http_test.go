package performance

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TestHTTPPerformance tests the performance of the HTTP server and client
func TestHTTPPerformance(t *testing.T) {
	serverHost := "localhost"
	serverPort := 8082
	numRequests := 1_000_000 //a million requests to be sent
	concurrentClients := 10  //i think we have like 8 clusters so this should be enough (?)

	server := http.ServerFactory(serverHost, serverPort)

	//register handler for POST requests
	server.RegisterHandler(
		http.POST,
		"/data",
		func(req *http.Request) *http.Response {
			var sensorData types.SensorData
			err := json.Unmarshal(req.Body, &sensorData)
			if err != nil {
				return http.NewResponse(http.StatusBadRequest)
			}

			//return success response
			return http.NewResponse(http.StatusOK)
		},
	)

	err := server.Start()
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()

	//wait for the server to start
	time.Sleep(100 * time.Millisecond)

	testData := types.SensorData{
		SensorID:  "performance-test-sensor",
		Timestamp: time.Now(),
		Value:     23.5,
		Unit:      "°C",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	//create URL for test instance
	url := fmt.Sprintf("http://%s:%d/data", serverHost, serverPort)

	log.Printf("Starting performance test with %d requests from %d concurrent clients",
		numRequests, concurrentClients)

	//create channel for RTT measurements
	rtts := make(chan time.Duration, numRequests)

	//create wait channel to synchronize goroutines
	done := make(chan struct{})

	//start the clients
	requestsPerClient := numRequests / concurrentClients
	for i := 0; i < concurrentClients; i++ {
		go func(clientID int) {
			client := http.HttpClientFactory(5 * time.Second)

			for j := 0; j < requestsPerClient; j++ {
				//send request and measure RTT
				start := time.Now()
				resp, err := client.PostJSON(url, jsonData)
				if err != nil {
					log.Printf("Client %d: Error sending request: %v", clientID, err)
					continue
				}
				rtt := time.Since(start)

				//check response
				if resp.StatusCode != http.StatusOK {
					log.Printf("Client %d: Expected status 200, got %d", clientID, resp.StatusCode)
					continue
				}

				//send RTT measurement
				rtts <- rtt
			}

			//signal completion
			done <- struct{}{}
		}(i)
	}

	//wait for all the clients to finish
	for i := 0; i < concurrentClients; i++ {
		<-done
	}

	close(rtts)

	//collect RTT measurements
	var rttValues []time.Duration
	for rtt := range rtts {
		rttValues = append(rttValues, rtt)
	}

	//calculate statistics
	stats := calculateRTTStatistics(rttValues)

	log.Printf("HTTP Performance Test Results:")
	log.Printf("  Total requests:    %d", len(rttValues))
	log.Printf("  Min RTT:           %v", stats.Min)
	log.Printf("  Max RTT:           %v", stats.Max)
	log.Printf("  Mean RTT:          %v", stats.Mean)
	log.Printf("  Median RTT:        %v", stats.Median)
	log.Printf("  Standard deviation: %v", stats.StdDev)
	log.Printf("  90th percentile:    %v", stats.Percentile90)
	log.Printf("  95th percentile:    %v", stats.Percentile95)
	log.Printf("  99th percentile:    %v", stats.Percentile99)
	log.Printf("  Requests per second: %.2f", stats.RequestsPerSecond)

	writeResultsToFile(stats, "http_performance_results.txt")
}

// RTTStatistics contains statistical measures for RTT measurements
type RTTStatistics struct {
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

// calculateRTTStatistics calculates statistical measures from RTT measurements
func calculateRTTStatistics(rtts []time.Duration) RTTStatistics {
	if len(rtts) == 0 {
		return RTTStatistics{}
	}

	//sort the values for percentile calculations
	sort.Slice(rtts, func(i, j int) bool {
		return rtts[i] < rtts[j]
	})

	count := len(rtts)

	//min and max
	min := rtts[0]
	max := rtts[count-1]

	//mean
	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	mean := sum / time.Duration(count)

	//SD
	var sumSquaredDifferences float64
	for _, rtt := range rtts {
		diff := float64(rtt - mean)
		sumSquaredDifferences += diff * diff
	}
	variance := sumSquaredDifferences / float64(count)
	stdDev := time.Duration(math.Sqrt(variance))

	//percentiles
	p90Index := int(float64(count) * 0.9)
	p95Index := int(float64(count) * 0.95)
	p99Index := int(float64(count) * 0.99)

	percentile90 := rtts[p90Index]
	percentile95 := rtts[p95Index]
	percentile99 := rtts[p99Index]

	//total duration and requests per second
	totalDuration := sum
	requestsPerSecond := float64(count) / totalDuration.Seconds()

	return RTTStatistics{
		Count:             count,
		Min:               min,
		Max:               max,
		Mean:              mean,
		StdDev:            stdDev,
		Percentile90:      percentile90,
		Percentile95:      percentile95,
		Percentile99:      percentile99,
		RequestsPerSecond: requestsPerSecond,
		TotalDuration:     totalDuration,
	}
}

// writeResultsToFile writes test results to a file
func writeResultsToFile(stats RTTStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("HTTP Performance Test Results\n")
	file.WriteString("============================\n\n")

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
	fmt.Fprintf(file, "Cumulative RTT across all requests sent:     %v\n", stats.TotalDuration)

	return nil
}

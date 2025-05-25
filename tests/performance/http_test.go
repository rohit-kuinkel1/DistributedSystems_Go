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

// TestRawHTTPPerformance tests the performance of the raw HTTP server (Task 2 - local storage only)
func TestRawHTTPPerformance(t *testing.T) {
	serverHost := "localhost"
	serverPort := 8080
	numRequests := 1_000_000
	concurrentClients := 10

	time.Sleep(500 * time.Millisecond)

	testData := types.SensorData{
		SensorID:  "raw-http-perf-test",
		Timestamp: time.Now(),
		Value:     23.5,
		Unit:      "°C",
	}

	jsonData, err := json.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal JSON: %v", err)
	}

	url := fmt.Sprintf("http://%s:%d/data", serverHost, serverPort)

	log.Printf("Starting raw HTTP performance test with %d requests from %d concurrent clients",
		numRequests, concurrentClients)

	// Channel for RTT measurements
	rtts := make(chan time.Duration, numRequests)
	done := make(chan struct{})

	// Start the clients
	requestsPerClient := numRequests / concurrentClients
	for i := 0; i < concurrentClients; i++ {
		go func(clientID int) {
			client := http.HttpClientFactory(5 * time.Second)

			for j := 0; j < requestsPerClient; j++ {
				// Send request and measure RTT
				start := time.Now()
				resp, err := client.PostJSON(url, jsonData)
				if err != nil {
					log.Printf("Client %d: Error sending request: %v", clientID, err)
					continue
				}
				rtt := time.Since(start)

				// Check response
				if resp.StatusCode != http.StatusOK {
					log.Printf("Client %d: Expected status 200, got %d", clientID, resp.StatusCode)
					continue
				}

				// Send RTT measurement
				rtts <- rtt
			}

			// Signal completion
			done <- struct{}{}
		}(i)
	}

	// Wait for all clients to finish
	for i := 0; i < concurrentClients; i++ {
		<-done
	}

	close(rtts)

	// Collect RTT measurements
	var rttValues []time.Duration
	for rtt := range rtts {
		rttValues = append(rttValues, rtt)
	}

	// Calculate statistics
	stats := calculateRawHTTPStatistics(rttValues)

	log.Printf("Raw HTTP Performance Test Results:")
	log.Printf("  Total requests:    %d", stats.Count)
	log.Printf("  Min RTT:           %v", stats.Min)
	log.Printf("  Max RTT:           %v", stats.Max)
	log.Printf("  Mean RTT:          %v", stats.Mean)
	log.Printf("  Median RTT:        %v", stats.Median)
	log.Printf("  Standard deviation: %v", stats.StdDev)
	log.Printf("  90th percentile:    %v", stats.Percentile90)
	log.Printf("  95th percentile:    %v", stats.Percentile95)
	log.Printf("  99th percentile:    %v", stats.Percentile99)
	log.Printf("  Requests per second: %.2f", stats.RequestsPerSecond)

	writeRawHTTPResultsToFile(stats, "http_performance_results.txt")
	if err != nil {
		t.Errorf("Failed to write results to file: %v", err)
	}
}

// RawHTTPStatistics contains statistical measures for raw HTTP RTT measurements
type RawHTTPStatistics struct {
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

// calculateRawHTTPStatistics calculates statistical measures from RTT measurements
func calculateRawHTTPStatistics(rtts []time.Duration) RawHTTPStatistics {
	if len(rtts) == 0 {
		return RawHTTPStatistics{}
	}

	// Sort the values for percentile calculations
	sort.Slice(rtts, func(i, j int) bool {
		return rtts[i] < rtts[j]
	})

	count := len(rtts)

	// Min and max
	min := rtts[0]
	max := rtts[count-1]

	// Mean
	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	mean := sum / time.Duration(count)

	// Median
	var median time.Duration
	if count%2 == 0 {
		median = (rtts[count/2-1] + rtts[count/2]) / 2
	} else {
		median = rtts[count/2]
	}

	// Standard deviation
	var sumSquaredDifferences float64
	for _, rtt := range rtts {
		diff := float64(rtt - mean)
		sumSquaredDifferences += diff * diff
	}
	variance := sumSquaredDifferences / float64(count)
	stdDev := time.Duration(math.Sqrt(variance))

	// Percentiles
	p90Index := int(float64(count) * 0.9)
	p95Index := int(float64(count) * 0.95)
	p99Index := int(float64(count) * 0.99)

	percentile90 := rtts[p90Index]
	percentile95 := rtts[p95Index]
	percentile99 := rtts[p99Index]

	// Total duration and requests per second
	totalDuration := sum
	requestsPerSecond := float64(count) / totalDuration.Seconds()

	return RawHTTPStatistics{
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

// writeRawHTTPResultsToFile writes test results to a file
func writeRawHTTPResultsToFile(stats RawHTTPStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("Raw HTTP Performance Test Results (Task 2 - Local Storage)\n")
	file.WriteString("=========================================================\n\n")

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

	return nil
}

package performance

import (
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// TestRPCPerformance tests the performance of RPC calls to the database service
func TestRPCPerformance(t *testing.T) {
	client, err := database.ClientFactory("localhost:50051")
	if err != nil {
		t.Fatalf("Failed to connect to database service: %v", err)
	}
	defer client.Close()

	numRequests := 1_000_000
	log.Printf("Starting RPC performance test with %d requests", numRequests)

	//collect RTT measurements
	var rtts []time.Duration
	testData := types.SensorData{
		SensorID:  "rpc-perf-test",
		Timestamp: time.Now(),
		Value:     42.0,
		Unit:      "test",
	}

	//measure RTT for each RPC call
	for i := range numRequests {
		start := time.Now()

		err := client.AddDataPoint(testData)
		if err != nil {
			t.Errorf("RPC call %d failed: %v", i, err)
			continue
		}

		rtt := time.Since(start)
		rtts = append(rtts, rtt)
	}

	//calculate statistics
	stats := calculateRPCStatistics(rtts)

	log.Printf("RPC Performance Test Results:")
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

	//write results to file
	err = writeRPCResultsToFile(stats, "rpc_performance_results.txt")
	if err != nil {
		t.Errorf("Failed to write results to file: %v", err)
	}
}

// RPCStatistics contains statistical measures for RPC RTT measurements
type RPCStatistics struct {
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

// calculateRPCStatistics calculates statistical measures from RPC RTT measurements
func calculateRPCStatistics(rtts []time.Duration) RPCStatistics {
	if len(rtts) == 0 {
		return RPCStatistics{}
	}

	//sort for percentile calculations
	sort.Slice(rtts, func(i, j int) bool {
		return rtts[i] < rtts[j]
	})

	count := len(rtts)
	min := rtts[0]
	max := rtts[count-1]

	//calculate mean
	var sum time.Duration
	for _, rtt := range rtts {
		sum += rtt
	}
	mean := sum / time.Duration(count)

	//median
	var median time.Duration
	if count%2 == 0 {
		median = (rtts[count/2-1] + rtts[count/2]) / 2
	} else {
		median = rtts[count/2]
	}

	//sd
	var sumSquaredDifferences float64
	for _, rtt := range rtts {
		diff := float64(rtt - mean)
		sumSquaredDifferences += diff * diff
	}
	variance := sumSquaredDifferences / float64(count)
	stdDev := time.Duration(math.Sqrt(variance))

	//percentiles like in the other tst
	p90Index := int(float64(count) * 0.9)
	p95Index := int(float64(count) * 0.95)
	p99Index := int(float64(count) * 0.99)

	percentile90 := rtts[p90Index]
	percentile95 := rtts[p95Index]
	percentile99 := rtts[p99Index]

	//requests per second
	totalDuration := sum
	requestsPerSecond := float64(count) / totalDuration.Seconds()

	return RPCStatistics{
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

// writeRPCResultsToFile writes RPC test results to a file
func writeRPCResultsToFile(stats RPCStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("RPC Performance Test Results\n")
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
	fmt.Fprintf(file, "Total duration:     %v\n", stats.TotalDuration)

	return nil
}

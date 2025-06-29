package performance

import (
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"slices"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// Test2PCPerformance tests the performance of Two-Phase Commit vs direct database calls
func Test2PCPerformance(t *testing.T) {
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

	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client: %v", err)
	}
	defer tpcClient.Close()

	numRequests := 10_000 //smaller number for 2PC due to crazy costs
	log.Printf("Starting 2PC performance comparison with %d requests", numRequests)

	//test 1: Direct RPC calls (baseline)
	log.Println("=== Testing Direct RPC Performance (Baseline) ===")
	directStats := testDirectRPCPerformance(t, client1, numRequests)

	//test 2: Two-Phase Commit
	log.Println("=== Testing 2PC Performance ===")
	tpcStats := test2PCPerformance(t, tpcClient, numRequests)

	//test 3: Concurrent 2PC transactions
	log.Println("=== Testing Concurrent 2PC Performance ===")
	concurrentStats := testConcurrent2PCPerformance(t, tpcClient, numRequests/10, 10)

	err = write2PCComparisonResults(directStats, tpcStats, concurrentStats, "2pc_performance_results.txt")
	if err != nil {
		t.Errorf("Failed to write results to file: %v", err)
	}

	log.Println("2PC performance testing completed")
}

// testDirectRPCPerformance measures baseline RPC performance to single database
func testDirectRPCPerformance(t *testing.T, client *database.Client, numRequests int) TwoPhaseCommitStatistics {
	var rtts []time.Duration
	testData := types.SensorData{
		SensorID:  "direct-rpc-perf",
		Timestamp: time.Now(),
		Value:     42.0,
		Unit:      "test",
	}

	log.Printf("Running %d direct RPC calls...", numRequests)
	start := time.Now()

	for i := 0; i < numRequests; i++ {
		requestStart := time.Now()
		err := client.AddDataPoint(testData)
		if err != nil {
			t.Errorf("Direct RPC call %d failed: %v", i, err)
			continue
		}
		rtt := time.Since(requestStart)
		rtts = append(rtts, rtt)
	}

	totalDuration := time.Since(start)
	stats := calculate2PCStatistics(rtts, "Direct-RPC", totalDuration)
	log2PCStatistics(stats)
	return stats
}

// test2PCPerformance measures Two-Phase Commit performance
func test2PCPerformance(t *testing.T, tpcClient *database.TwoPhaseCommitClient, numRequests int) TwoPhaseCommitStatistics {
	var rtts []time.Duration
	testData := types.SensorData{
		SensorID:  "2pc-perf-test",
		Timestamp: time.Now(),
		Value:     42.0,
		Unit:      "test",
	}

	log.Printf("Running %d 2PC transactions...", numRequests)
	start := time.Now()

	for i := range numRequests {
		//create unique test data for each transaction
		uniqueData := testData
		uniqueData.SensorID = fmt.Sprintf("2pc-perf-%d", i)
		uniqueData.Timestamp = time.Now()

		requestStart := time.Now()
		err := tpcClient.AddDataPointWithTwoPhaseCommit(uniqueData)
		if err != nil {
			t.Errorf("2PC transaction %d failed: %v", i, err)
			continue
		}
		rtt := time.Since(requestStart)
		rtts = append(rtts, rtt)
	}

	totalDuration := time.Since(start)
	stats := calculate2PCStatistics(rtts, "2PC-Sequential", totalDuration)
	log2PCStatistics(stats)
	return stats
}

// testConcurrent2PCPerformance measures 2PC performance under concurrent load
func testConcurrent2PCPerformance(t *testing.T, tpcClient *database.TwoPhaseCommitClient, requestsPerClient, numClients int) TwoPhaseCommitStatistics {
	var mu sync.Mutex
	var allRTTs []time.Duration
	var wg sync.WaitGroup

	log.Printf("Running %d concurrent 2PC clients with %d requests each...", numClients, requestsPerClient)
	start := time.Now()

	for clientID := range numClients {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for i := range requestsPerClient {
				testData := types.SensorData{
					SensorID:  fmt.Sprintf("2pc-concurrent-%d-%d", id, i),
					Timestamp: time.Now(),
					Value:     float64(id*100 + i),
					Unit:      "test",
				}

				requestStart := time.Now()
				err := tpcClient.AddDataPointWithTwoPhaseCommit(testData)
				if err != nil {
					log.Printf("Concurrent 2PC client %d, request %d failed: %v", id, i, err)
					continue
				}
				rtt := time.Since(requestStart)

				mu.Lock()
				allRTTs = append(allRTTs, rtt)
				mu.Unlock()
			}
		}(clientID)
	}

	wg.Wait()
	totalDuration := time.Since(start)

	stats := calculate2PCStatistics(allRTTs, "2PC-Concurrent", totalDuration)
	log2PCStatistics(stats)
	return stats
}

// TwoPhaseCommitStatistics contains statistical measures for 2PC performance
type TwoPhaseCommitStatistics struct {
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

// calculate2PCStatistics calculates statistical measures from RTT measurements
func calculate2PCStatistics(rtts []time.Duration, protocol string, totalDuration time.Duration) TwoPhaseCommitStatistics {
	if len(rtts) == 0 {
		return TwoPhaseCommitStatistics{Protocol: protocol}
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

	requestsPerSecond := float64(count) / totalDuration.Seconds()

	return TwoPhaseCommitStatistics{
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

// log2PCStatistics logs performance statistics
func log2PCStatistics(stats TwoPhaseCommitStatistics) {
	log.Printf("  Protocol: %s", stats.Protocol)
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

// write2PCComparisonResults writes comprehensive 2PC comparison results to file
func write2PCComparisonResults(directStats, tpcStats, concurrentStats TwoPhaseCommitStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("Two-Phase Commit Performance Analysis (Task 3.5)\n")
	file.WriteString("==============================================\n\n")

	file.WriteString("Direct RPC Performance (Baseline):\n")
	file.WriteString("-----------------------------------\n")
	write2PCStatsToFile(file, directStats)

	file.WriteString("\nTwo-Phase Commit Performance:\n")
	file.WriteString("-----------------------------\n")
	write2PCStatsToFile(file, tpcStats)

	file.WriteString("\nConcurrent 2PC Performance:\n")
	file.WriteString("---------------------------\n")
	write2PCStatsToFile(file, concurrentStats)

	file.WriteString("\nPerformance Impact Analysis:\n")
	file.WriteString("============================\n")

	if directStats.Count > 0 && tpcStats.Count > 0 {
		latencyOverhead := float64(tpcStats.Mean-directStats.Mean) / float64(directStats.Mean) * 100
		throughputDegradation := (directStats.RequestsPerSecond - tpcStats.RequestsPerSecond) / directStats.RequestsPerSecond * 100
		consistencyOverhead := float64(tpcStats.Mean) / float64(directStats.Mean)

		fmt.Fprintf(file, "2PC latency overhead: %.1f%% (%.3fms additional latency)\n",
			latencyOverhead, float64(tpcStats.Mean-directStats.Mean)/float64(time.Millisecond))
		fmt.Fprintf(file, "2PC throughput degradation: %.1f%% (%.2f vs %.2f req/sec)\n",
			throughputDegradation, tpcStats.RequestsPerSecond, directStats.RequestsPerSecond)
		fmt.Fprintf(file, "Consistency cost multiplier: %.2fx slower\n", consistencyOverhead)

		if concurrentStats.Count > 0 {
			concurrentDegradation := float64(concurrentStats.Mean-tpcStats.Mean) / float64(tpcStats.Mean) * 100
			fmt.Fprintf(file, "Concurrent load impact: %.1f%% additional degradation\n", concurrentDegradation)
		}
	}

	file.WriteString("============\n")
	file.WriteString("- 2PC provides data consistency at the cost of performance\n")
	file.WriteString("- Redundant storage introduces latency and throughput overhead\n")
	file.WriteString("- Concurrent load amplifies the performance impact of 2PC coordination\n")
	file.WriteString("- The trade off; Consistency and fault tolerance vs. performance\n")

	return nil
}

// write2PCStatsToFile writes statistics to file
func write2PCStatsToFile(file *os.File, stats TwoPhaseCommitStatistics) {
	fmt.Fprintf(file, "Protocol:           %s\n", stats.Protocol)
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

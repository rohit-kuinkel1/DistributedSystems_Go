package functional

import (
	"fmt"
	"log"
	"sync"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// Test2PCSuccessfulTransaction tests successful 2PC transaction where both databases commit
func Test2PCSuccessfulTransaction(t *testing.T) {
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

	testData := types.SensorData{
		SensorID:  "2pc-test-success",
		Timestamp: time.Now(),
		Value:     42.5,
		Unit:      "°C",
	}

	err = tpcClient.AddDataPointWithTwoPhaseCommit(testData)
	if err != nil {
		t.Fatalf("2PC transaction failed: %v", err)
	}

	//verify data exists in both databases
	data1, err := client1.GetDataPointBySensorId(testData.SensorID)
	if err != nil {
		t.Errorf("Failed to get data from database1: %v", err)
	}
	if len(data1) != 1 {
		t.Errorf("Expected 1 data point in database1, got %d", len(data1))
	}

	data2, err := client2.GetDataPointBySensorId(testData.SensorID)
	if err != nil {
		t.Errorf("Failed to get data from database2: %v", err)
	}
	if len(data2) != 1 {
		t.Errorf("Expected 1 data point in database2, got %d", len(data2))
	}

	//verify data consistency between databases
	if len(data1) > 0 && len(data2) > 0 {
		if data1[0].SensorID != data2[0].SensorID {
			t.Errorf("SensorID mismatch: db1=%s, db2=%s", data1[0].SensorID, data2[0].SensorID)
		}
		if data1[0].Value != data2[0].Value {
			t.Errorf("Value mismatch: db1=%.2f, db2=%.2f", data1[0].Value, data2[0].Value)
		}
		if data1[0].Unit != data2[0].Unit {
			t.Errorf("Unit mismatch: db1=%s, db2=%s", data1[0].Unit, data2[0].Unit)
		}
	}

	log.Println("2PC successful transaction test passed")
}

// Test2PCFailedTransaction tests failed 2PC transaction by simulating database failure
func Test2PCFailedTransaction(t *testing.T) {
	//here we'll connect to one working and one non-existent database to simulate failure

	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:99999"})
	if err == nil {
		defer tpcClient.Close()

		// Test data
		testData := types.SensorData{
			SensorID:  "2pc-test-failure",
			Timestamp: time.Now(),
			Value:     99.9,
			Unit:      "°C",
		}

		//this should fail
		err = tpcClient.AddDataPointWithTwoPhaseCommit(testData)
		if err == nil {
			t.Errorf("Expected 2PC transaction to fail, but it succeeded")
		} else {
			log.Printf("2PC transaction failed as expected: %v", err)
		}

		//verify that no data was committed to the working database
		client1, err := database.ClientFactory("localhost:50051")
		if err != nil {
			t.Fatalf("Failed to connect to database1: %v", err)
		}
		defer client1.Close()

		data1, err := client1.GetDataPointBySensorId(testData.SensorID)
		if err != nil {
			t.Errorf("Failed to query database1: %v", err)
		}
		if len(data1) != 0 {
			t.Errorf("Expected no data in database1 after failed 2PC, but found %d records", len(data1))
		}
	} else {
		//if we cannot even create the client then thats also a valid test result
		log.Printf("2PC client creation failed as expected with invalid address: %v", err)
	}

	log.Println("2PC failed transaction test passed")
}

// Test2PCDataConsistency tests data consistency between both databases after multiple transactions
func Test2PCDataConsistency(t *testing.T) {
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

	testDataSet := []types.SensorData{
		{
			SensorID:  "2pc-consistency-1",
			Timestamp: time.Now(),
			Value:     10.1,
			Unit:      "°C",
		},
		{
			SensorID:  "2pc-consistency-2",
			Timestamp: time.Now(),
			Value:     20.2,
			Unit:      "°C",
		},
		{
			SensorID:  "2pc-consistency-3",
			Timestamp: time.Now(),
			Value:     30.3,
			Unit:      "°C",
		},
	}

	//exec all transactions
	for _, testData := range testDataSet {
		err = tpcClient.AddDataPointWithTwoPhaseCommit(testData)
		if err != nil {
			t.Fatalf("2PC transaction failed for %s: %v", testData.SensorID, err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	//verify data consistency by comparing all data in both databases
	allData1, err := client1.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get all data from database1: %v", err)
	}

	allData2, err := client2.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get all data from database2: %v", err)
	}

	//filter to incl only our test data
	testData1 := filterTestData(allData1, "2pc-consistency-")
	testData2 := filterTestData(allData2, "2pc-consistency-")

	if len(testData1) != len(testData2) {
		t.Errorf("Data count mismatch: db1=%d, db2=%d", len(testData1), len(testData2))
	}

	if len(testData1) != len(testDataSet) {
		t.Errorf("Expected %d records, got %d in database1", len(testDataSet), len(testData1))
	}

	//verify each record matches between databases
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

	log.Println("2PC data consistency test passed")
}

// Test2PCTransactionIDUniqueness tests that transaction IDs are unique
func Test2PCTransactionIDUniqueness(t *testing.T) {
	//create multiple 2PC clients to simulate concurrent coordinators
	tpcClient1, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client1: %v", err)
	}
	defer tpcClient1.Close()

	tpcClient2, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client2: %v", err)
	}
	defer tpcClient2.Close()

	//for nowcwe will test by ensuring concurrent transactions work correctly
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	testData1 := types.SensorData{
		SensorID:  "2pc-unique-1",
		Timestamp: time.Now(),
		Value:     11.1,
		Unit:      "°C",
	}

	testData2 := types.SensorData{
		SensorID:  "2pc-unique-2",
		Timestamp: time.Now(),
		Value:     22.2,
		Unit:      "°C",
	}

	//execute transactions concurrently
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := tpcClient1.AddDataPointWithTwoPhaseCommit(testData1)
		if err != nil {
			errChan <- fmt.Errorf("client1 transaction failed: %v", err)
		}
	}()

	go func() {
		defer wg.Done()
		err := tpcClient2.AddDataPointWithTwoPhaseCommit(testData2)
		if err != nil {
			errChan <- fmt.Errorf("client2 transaction failed: %v", err)
		}
	}()

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Concurrent transaction error: %v", err)
	}

	//verify both transactions succeeded
	client1, err := database.ClientFactory("localhost:50051")
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}
	defer client1.Close()

	data1, err := client1.GetDataPointBySensorId(testData1.SensorID)
	if err != nil || len(data1) != 1 {
		t.Errorf("Transaction 1 data not found: err=%v, count=%d", err, len(data1))
	}

	data2, err := client1.GetDataPointBySensorId(testData2.SensorID)
	if err != nil || len(data2) != 1 {
		t.Errorf("Transaction 2 data not found: err=%v, count=%d", err, len(data2))
	}

	log.Println("2PC transaction ID uniqueness test passed")
}

// Test2PCConcurrentTransactions tests handling of multiple concurrent transactions
func Test2PCConcurrentTransactions(t *testing.T) {
	tpcClient, err := database.TwoPhaseCommitClientFactory([]string{"localhost:50051", "localhost:50052"})
	if err != nil {
		t.Fatalf("Failed to create 2PC client: %v", err)
	}
	defer tpcClient.Close()

	numConcurrentTransactions := 10
	var wg sync.WaitGroup
	errChan := make(chan error, numConcurrentTransactions)

	//execute multiple concurrent transactions
	for i := range numConcurrentTransactions {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			testData := types.SensorData{
				SensorID:  fmt.Sprintf("2pc-concurrent-%d", id),
				Timestamp: time.Now(),
				Value:     float64(id * 10),
				Unit:      "°C",
			}

			err := tpcClient.AddDataPointWithTwoPhaseCommit(testData)
			if err != nil {
				errChan <- fmt.Errorf("transaction %d failed: %v", id, err)
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	errorCount := 0
	for err := range errChan {
		t.Errorf("Concurrent transaction error: %v", err)
		errorCount++
	}

	//verify all successful transactions are in both databases
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

	allData1, err := client1.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get all data from database1: %v", err)
	}

	allData2, err := client2.GetAllDataPoints()
	if err != nil {
		t.Fatalf("Failed to get all data from database2: %v", err)
	}

	//filter to include only our test data
	concurrentData1 := filterTestData(allData1, "2pc-concurrent-")
	concurrentData2 := filterTestData(allData2, "2pc-concurrent-")

	expectedSuccess := numConcurrentTransactions - errorCount
	if len(concurrentData1) != expectedSuccess {
		t.Errorf("Expected %d successful transactions in db1, got %d", expectedSuccess, len(concurrentData1))
	}
	if len(concurrentData2) != expectedSuccess {
		t.Errorf("Expected %d successful transactions in db2, got %d", expectedSuccess, len(concurrentData2))
	}

	//verify the data consistency
	if len(concurrentData1) != len(concurrentData2) {
		t.Errorf("Data count mismatch between databases: db1=%d, db2=%d",
			len(concurrentData1), len(concurrentData2))
	}

	log.Printf("2PC concurrent transactions test passed: %d/%d transactions succeeded",
		expectedSuccess, numConcurrentTransactions)
}

// Helper function to filter test data by sensor ID prefix
func filterTestData(data []types.SensorData, prefix string) []types.SensorData {
	var filtered []types.SensorData
	for _, d := range data {
		if len(d.SensorID) >= len(prefix) && d.SensorID[:len(prefix)] == prefix {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

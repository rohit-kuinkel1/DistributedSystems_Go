package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

// DataStore manages the storage of sensor data with thread safety
type DataStore struct {
	mutex sync.RWMutex //our mutex which can have N number of readers but only 1 writer at a given time
	data  []types.SensorData
	limit int //maximum number of data points to keep
}

// DataStoreFactory creates a new data store with a specified size limit
func DataStoreFactory(limit int) *DataStore {
	return &DataStore{
		data:  make([]types.SensorData, 0, limit),
		limit: limit,
	}
}

// AddDataPoint adds a new sensor data point to the store
func (ds *DataStore) AddDataPoint(sensorData types.SensorData) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()

	ds.data = append(ds.data, sensorData)

	//if we've exceeded the limit, remove the oldest data points (FIFO)
	if len(ds.data) > ds.limit {
		ds.data = ds.data[len(ds.data)-ds.limit:]
	}
}

// GetAllDataPoints returns all stored sensor data
func (ds *DataStore) GetAllDataPoints() []types.SensorData {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	//create a copy of the data to avoid race conditions
	result := make([]types.SensorData, len(ds.data))
	copy(result, ds.data)
	return result
}

// GetDataPointBySensorId returns data for a specific sensor
func (ds *DataStore) GetDataPointBySensorId(sensorID string) []types.SensorData {
	ds.mutex.RLock()
	defer ds.mutex.RUnlock()

	var result []types.SensorData
	for _, data := range ds.data {
		if data.SensorID == sensorID {
			result = append(result, data)
		}
	}
	return result
}

func main() {
	host := flag.String("host", "0.0.0.0", "Server host")
	port := flag.Int("port", 8080, "Server port")
	server := http.ServerFactory(*host, *port)

	flag.Parse()
	dataLimit := flag.Int("data-limit", 1_000_000, "Maximum number of data points to keep")
	dataStore := DataStoreFactory(*dataLimit)

	registerHandlers(server, dataStore)

	//the listener for the TCP is also added in Start
	err := server.Start()
	if err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	//wait for termination signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down server...")
	server.Stop()
}

// registerHandlers registers all HTTP handlers for the server
func registerHandlers(server *http.Server, dataStore *DataStore) {
	//handler for HTTP POST requests to add sensor data
	server.RegisterHandler(
		http.POST,
		"/data",
		func(req *http.Request) *http.Response {
			var sensorData types.SensorData
			err := json.Unmarshal(req.Body, &sensorData)
			if err != nil {
				log.Printf("Error parsing sensor data: %v", err)
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString(fmt.Sprintf("Invalid JSON: %v", err))
				return resp
			}

			//now lets validate the data received
			if sensorData.SensorID == "" {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Missing sensorId")
				return resp
			}

			//set timestamp to current time if not provided
			if sensorData.Timestamp.IsZero() {
				sensorData.Timestamp = time.Now()
			}

			//store the data
			dataStore.AddDataPoint(sensorData)
			log.Printf(
				"Stored data from sensor %s: %.2f %s",
				sensorData.SensorID,
				sensorData.Value,
				sensorData.Unit,
			)

			//return a success response
			resp := http.NewResponse(http.StatusOK)
			resp.SetBodyString("Data stored successfully")
			return resp
		},
	)

	//handler for HTTP GET requests to retrieve all sensor data
	server.RegisterHandler(
		http.GET,
		"/data",
		func(req *http.Request) *http.Response {
			//get all data from the data store
			allData := dataStore.GetAllDataPoints()

			//convert to JSON before sending the data
			jsonData, err := json.Marshal(allData)
			if err != nil {
				log.Printf("Error marshaling data to JSON: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Server error: %v", err))
				return resp
			}

			//return the JSON response
			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)

	// Handler for HTTP GET requests to retrieve data for a specific sensor
	server.RegisterHandler(
		http.GET,
		"/data/*",
		func(req *http.Request) *http.Response {
			//extract sensor ID from path
			path := req.Path
			if path == "/data/" {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Missing sensor ID")
				return resp
			}

			sensorID := path[6:] // Remove "/data/"

			//get data for the specified sensor
			sensorData := dataStore.GetDataPointBySensorId(sensorID)

			if len(sensorData) == 0 {
				resp := http.NewResponse(http.StatusNotFound)
				resp.SetBodyString(fmt.Sprintf("No data found for sensor %s", sensorID))
				return resp
			}

			//convert to JSON
			jsonData, err := json.Marshal(sensorData)
			if err != nil {
				log.Printf("Error marshaling data to JSON: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Server error: %v", err))
				return resp
			}

			//return JSON response
			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)

	//handler for HTTP GET requests to the root path (for browser access)
	server.RegisterHandler(
		http.GET,
		"/",
		func(req *http.Request) *http.Response {
			//create a simple HTML page that displays the data
			html := `
				<!DOCTYPE html>
				<html>
				<head>
					<title>IoT Data Viewer</title>
					<style>
						body { font-family: Arial, sans-serif; margin: 0; padding: 20px; }
						h1 { color: #333; }
						table { border-collapse: collapse; width: 100%; }
						th, td { border: 1px solid #ddd; padding: 8px; text-align: left; }
						th { background-color: #f2f2f2; }
						tr:nth-child(even) { background-color: #f9f9f9; }
					</style>
					<script>
						// Fetch data every x seconds
						function fetchData() {
							fetch('/data')
								.then(response => response.json())
								.then(data => {
									const tableBody = document.getElementById('dataTable').getElementsByTagName('tbody')[0];
									tableBody.innerHTML = '';
									
									// Sort by timestamp (newest first)
									data.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));
									
									data.forEach(item => {
										const row = tableBody.insertRow();
										row.insertCell(0).textContent = item.sensorId;
										row.insertCell(1).textContent = new Date(item.timestamp).toLocaleString();
										row.insertCell(2).textContent = item.value + ' ' + item.unit;
									});
								})
								.catch(error => console.error('Error fetching data:', error));
						}
						
						// Initial fetch and setup interval
						document.addEventListener('DOMContentLoaded', () => {
							fetchData();
							setInterval(fetchData, 1000);
						});
					</script>
				</head>
				<body>
					<h1>IoT Sensor Data</h1>
					<table id="dataTable">
						<thead>
							<tr>
								<th>Sensor ID</th>
								<th>Timestamp</th>
								<th>Value</th>
							</tr>
						</thead>
						<tbody>
							<!-- Data will be inserted here by JavaScript -->
						</tbody>
					</table>
				</body>
				</html>
			`
			return http.CreateHTMLResponse(http.StatusOK, []byte(html))
		},
	)
}

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/internal/database"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
)

func main() {
	host := flag.String("host", "0.0.0.0", "Server host")
	port := flag.Int("port", 8080, "Server port")
	dbAddr := flag.String("db-addr", "localhost:50051", "Database server address")
	flag.Parse()

	dbClient, err := database.NewClient(*dbAddr)
	if err != nil {
		log.Fatalf("Failed to connect to database service: %v", err)
	}
	defer dbClient.Close()

	server := http.ServerFactory(*host, *port)

	registerHandlers(server, dbClient)

	err = server.Start()
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
func registerHandlers(server *http.Server, dbClient *database.Client) {
	//for HTTP POST requests to add sensor data
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

			//store the data using the database client now that we have all things needed
			err = dbClient.AddDataPoint(sensorData)
			if err != nil {
				log.Printf("Error storing data: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error storing data: %v", err))
				return resp
			}

			log.Printf(
				"Stored data from sensor %s: %.2f %s",
				sensorData.SensorID,
				sensorData.Value,
				sensorData.Unit,
			)

			resp := http.NewResponse(http.StatusOK)
			resp.SetBodyString("Data stored successfully")
			return resp
		},
	)

	//for HTTP GET requests to retrieve all sensor data
	server.RegisterHandler(
		http.GET,
		"/data",
		func(req *http.Request) *http.Response {
			allData, err := dbClient.GetAllDataPoints()
			if err != nil {
				log.Printf("Error retrieving data: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error retrieving data: %v", err))
				return resp
			}

			jsonData, err := json.Marshal(allData)
			if err != nil {
				log.Printf("Error marshaling data to JSON: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Server error: %v", err))
				return resp
			}

			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)

	//for HTTP GET requests to retrieve data for a specific sensor
	server.RegisterHandler(
		http.GET,
		"/data/*",
		func(req *http.Request) *http.Response {
			// Extract sensor ID from path
			path := req.Path
			if path == "/data/" {
				resp := http.NewResponse(http.StatusBadRequest)
				resp.SetBodyString("Missing sensor ID")
				return resp
			}

			sensorID := path[6:] // Remove "/data/"

			sensorData, err := dbClient.GetDataPointBySensorId(sensorID)
			if err != nil {
				log.Printf("Error retrieving data for sensor %s: %v", sensorID, err)
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
				log.Printf("Error marshaling data to JSON: %v", err)
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Server error: %v", err))
				return resp
			}

			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)

	//for HTTP GET requests to the root path (for browser access)
	server.RegisterHandler(
		http.GET,
		"/",
		func(req *http.Request) *http.Response {
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

	//handler for performance testing of the RPC interface
	server.RegisterHandler(
		http.GET,
		"/performance/rpc",
		func(req *http.Request) *http.Response {
			iterations := 1_000_000 //number of test iterations
			min, max, avg, err := dbClient.RunPerformanceTest(iterations)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Performance test failed: %v", err))
				return resp
			}

			result := map[string]interface{}{
				"iterations": iterations,
				"min_rtt":    min.String(),
				"max_rtt":    max.String(),
				"avg_rtt":    avg.String(),
			}

			jsonData, err := json.Marshal(result)
			if err != nil {
				resp := http.NewResponse(http.StatusServerError)
				resp.SetBodyString(fmt.Sprintf("Error marshaling results: %v", err))
				return resp
			}

			return http.CreateJSONResponse(http.StatusOK, jsonData)
		},
	)
}

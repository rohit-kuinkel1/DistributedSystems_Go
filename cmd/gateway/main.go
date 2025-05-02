package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/http"
)

// Sensor represents a specific type of sensor that will pick up the sensor data
type Sensor struct {
	ID                     string  //sensor type identifier
	Name                   string  //name that can be read by us
	MinValue               float64 //minimum value the sensor can produce
	MaxValue               float64 //maximum value the sensor can produce
	Unit                   string  //unit of measurement used in the sensor
	NoiseLevel             float64 //how much noise to add to base value (percentage)
	DataGenerationInterval int     //data generation interval in milliseconds
}

// SensorData represents a sensor reading, meaning each data point picked up by the sensors will have these values
type SensorData struct {
	SensorID  string    `json:"sensorId"`  //from which sensor did we get this value?
	Timestamp time.Time `json:"timestamp"` //at what time was this value picked up?
	Value     float64   `json:"value"`     //the actual value that was picked up, we consider it to be smth like temperature, for now at least
	Unit      string    `json:"unit"`      //the unit of the value picked up
}

// Gateway represents the IoT Gateway that will send the data using POST over HTTPS using a TCP Socket.
type Gateway struct {
	ServerURL        string   //the URL for the server that receives the data
	Sensors          []Sensor //the collection of sensors that send data using this gateway
	InstancesPerType int
	Client           *http.Client
	StopChan         chan struct{} //channel for concurrent communication
	WaitGroup        sync.WaitGroup
}

var sensors = []Sensor{
	{
		ID:                     "temp",
		Name:                   "Temperature Sensor",
		MinValue:               -40.0,
		MaxValue:               130.0,
		Unit:                   "°C",
		NoiseLevel:             0.05,
		DataGenerationInterval: 1000,
	},
	{
		ID:                     "humid",
		Name:                   "Humidity Sensor",
		MinValue:               30.0,
		MaxValue:               80.0,
		Unit:                   "%",
		NoiseLevel:             0.05,
		DataGenerationInterval: 2000,
	},
	{
		ID:                     "press",
		Name:                   "Pressure Sensor",
		MinValue:               980.0,
		MaxValue:               1020.0,
		Unit:                   "hPa",
		NoiseLevel:             0.01,
		DataGenerationInterval: 2000,
	},
	{
		ID:                     "light",
		Name:                   "Light Sensor",
		MinValue:               0.0,
		MaxValue:               1000.0,
		Unit:                   "cd",
		NoiseLevel:             0.10,
		DataGenerationInterval: 1500,
	},
}

// GatewayFactory creates a new IoT Gateway and returns the fresh instance
func GatewayFactory(serverURL string, instancesPerType int) *Gateway {
	return &Gateway{
		ServerURL:        serverURL,
		Sensors:          sensors,
		InstancesPerType: instancesPerType,
		Client:           http.NewClient(5 * time.Second),
		StopChan:         make(chan struct{}),
	}
}

// Start starts the IoT Gateway
func (g *Gateway) Start() {
	log.Printf(
		"Starting IoT Gateway with %d sensor types, %d instances each",
		len(g.Sensors),
		g.InstancesPerType,
	)

	//start sensor data simulation for each sensor type and instance
	for _, sensorType := range g.Sensors {
		for i := 0; i < g.InstancesPerType; i++ {
			sensorID := fmt.Sprintf("%s-%d", sensorType.ID, i+1)
			g.WaitGroup.Add(1)
			go g.simulateSensor(sensorType, sensorID)
		}
	}
}

// Stop stops the IoT Gateway
func (g *Gateway) Stop() {
	log.Println("Stopping IoT Gateway...")

	close(g.StopChan)
	g.WaitGroup.Wait()

	log.Println("IoT Gateway stopped")
}

// simulateSensor simulates a sensor and sends data to the server
func (g *Gateway) simulateSensor(sensorType Sensor, sensorID string) {
	defer g.WaitGroup.Done()

	//create a ticker for periodic data generation
	ticker := time.NewTicker(time.Duration(sensorType.DataGenerationInterval) * time.Millisecond)
	defer ticker.Stop()

	//initialize base value somewhere in the sensor's range
	baseValue := sensorType.MinValue + rand.Float64()*(sensorType.MaxValue-sensorType.MinValue)

	//track round-trip times for performance measurement
	var rtts []time.Duration
	var rttMutex sync.Mutex

	log.Printf("Started sensor simulation for %s (%s)", sensorID, sensorType.Name)

	for {
		select {
		case <-g.StopChan:
			//calculate and log RTT statistics if available
			if len(rtts) > 0 {
				g.logRTTStatistics(sensorID, rtts)
			}
			return
		case <-ticker.C:
			//generate sensor data with some noise and drift
			value := g.generateSensorValue(baseValue, sensorType)
			data := SensorData{
				SensorID:  sensorID,
				Timestamp: time.Now(),
				Value:     value,
				Unit:      sensorType.Unit,
			}

			//send the data to server
			startTime := time.Now()
			err := g.sendData(data)
			if err != nil {
				log.Printf("Error sending data from sensor %s: %v", sensorID, err)
				continue
			}

			//calculate and store RTT
			rtt := time.Since(startTime)
			rttMutex.Lock()
			rtts = append(rtts, rtt)
			rttMutex.Unlock()

			//apply random drift to base value for next reading
			baseValue = g.applyDrift(baseValue, sensorType)
		}
	}
}

// generateSensorValue generates a sensor value based on a base value
func (g *Gateway) generateSensorValue(baseValue float64, sensorType Sensor) float64 {
	//add noise to the base value
	noise := (rand.Float64()*2 - 1) * sensorType.NoiseLevel * baseValue
	value := baseValue + noise

	//ensure that the value is within sensor range
	if value < sensorType.MinValue {
		value = sensorType.MinValue
	} else if value > sensorType.MaxValue {
		value = sensorType.MaxValue
	}

	return value
}

// applyDrift applies a small random drift to the base value
func (g *Gateway) applyDrift(baseValue float64, sensorType Sensor) float64 {
	//small random drift (0.1% of range)
	driftRange := (sensorType.MaxValue - sensorType.MinValue) * 0.001
	drift := (rand.Float64()*2 - 1) * driftRange

	newValue := baseValue + drift

	//ensure value stays within allowed range
	if newValue < sensorType.MinValue {
		newValue = sensorType.MinValue
	} else if newValue > sensorType.MaxValue {
		newValue = sensorType.MaxValue
	}

	return newValue
}

// sendData sends sensor data to the server
func (g *Gateway) sendData(data SensorData) error {
	//convert data to JSON
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling data to JSON: %w", err)
	}

	//send HTTP POST request
	resp, err := g.Client.PostJSON(g.ServerURL+"/data", jsonData)
	if err != nil {
		return fmt.Errorf("error sending data to server: %w", err)
	}

	//check response status
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status: %d %s", resp.StatusCode, resp.StatusText)
	}

	return nil
}

// logRTTStatistics calculates and logs RTT statistics
func (g *Gateway) logRTTStatistics(sensorID string, rtts []time.Duration) {
	if len(rtts) == 0 {
		return
	}

	//calculate min, max, mean
	var min, max, sum time.Duration = rtts[0], rtts[0], 0
	for _, rtt := range rtts {
		if rtt < min {
			min = rtt
		}
		if rtt > max {
			max = rtt
		}
		sum += rtt
	}
	mean := sum / time.Duration(len(rtts))

	//calculate median and standard deviation
	sortedRTTs := make([]time.Duration, len(rtts))
	copy(sortedRTTs, rtts)
	sortDurations(sortedRTTs)

	var median time.Duration
	if len(sortedRTTs)%2 == 0 {
		median = (sortedRTTs[len(sortedRTTs)/2-1] + sortedRTTs[len(sortedRTTs)/2]) / 2
	} else {
		median = sortedRTTs[len(sortedRTTs)/2]
	}

	//calculate standard deviation
	var variance float64
	for _, rtt := range rtts {
		diff := float64(rtt - mean)
		variance += diff * diff
	}
	variance /= float64(len(rtts))
	stdDev := time.Duration(float64(time.Microsecond) * float64(variance))

	log.Printf("RTT Statistics for %s (%d samples):", sensorID, len(rtts))
	log.Printf("  Min: %v", min)
	log.Printf("  Max: %v", max)
	log.Printf("  Mean: %v", mean)
	log.Printf("  Median: %v", median)
	log.Printf("  Std Dev: %v", stdDev)
}

// sortDurations sorts a slice of time.Duration values using insertion sort
func sortDurations(durations []time.Duration) {
	for i := 1; i < len(durations); i++ {
		key := durations[i]
		j := i - 1
		for j >= 0 && durations[j] > key {
			durations[j+1] = durations[j]
			j--
		}
		durations[j+1] = key
	}
}

func main() {
	//parse command line arguments
	serverHost := flag.String("server-host", "localhost", "Server hostname")
	serverPort := flag.Int("server-port", 8080, "Server port")
	instancesPerType := flag.Int("instances", 1, "Number of instances per sensor type")
	duration := flag.Int("duration", 0, "Run duration in seconds (0 = run until interrupted)")
	flag.Parse()

	//set up random source
	rand.Seed(time.Now().UnixNano())

	//create the server URL
	serverURL := fmt.Sprintf("http://%s:%d", *serverHost, *serverPort)

	//create and start the gateway
	gateway := GatewayFactory(serverURL, *instancesPerType)
	gateway.Start()

	//set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	//wait for termination signal or duration
	if *duration > 0 {
		log.Printf("Gateway will run for %d seconds", *duration)
		select {
		case <-sigChan:
			log.Println("Received termination signal")
		case <-time.After(time.Duration(*duration) * time.Second):
			log.Println("Run duration reached")
		}
	} else {
		log.Println("Gateway running until interrupted")
		<-sigChan
		log.Println("Received termination signal")
	}

	//stop the gateway
	gateway.Stop()
}

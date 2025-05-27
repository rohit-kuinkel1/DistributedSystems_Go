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

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// SensorSimulator represents a single sensor that publishes data to MQTT
type SensorSimulator struct {
	SensorType types.Sensor
	SensorID   string
	MQTTClient mqtt.Client
	StopChan   chan struct{}
	WaitGroup  *sync.WaitGroup
}

// SensorManager manages multiple sensor simulators
type SensorManager struct {
	BrokerURL      string
	Sensors        []types.Sensor
	SensorsPerType int
	Duration       int
	Simulators     []*SensorSimulator
	WaitGroup      sync.WaitGroup
}

var sensors = []types.Sensor{
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
		DataGenerationInterval: 500,
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

// NewSensorManager creates a new sensor manager
func NewSensorManager(brokerURL string, sensorsPerType, duration int) *SensorManager {
	return &SensorManager{
		BrokerURL:      brokerURL,
		Sensors:        sensors,
		SensorsPerType: sensorsPerType,
		Duration:       duration,
		Simulators:     make([]*SensorSimulator, 0),
	}
}

// Start starts all sensor simulators
func (sm *SensorManager) Start() error {
	log.Printf("Starting sensor manager with %d sensor types, %d instances each",
		len(sm.Sensors), sm.SensorsPerType)

	//create sensor simulators
	for _, sensorType := range sm.Sensors {
		for i := 0; i < sm.SensorsPerType; i++ {
			sensorID := fmt.Sprintf("%s-%d", sensorType.ID, i+1)
			simulator, err := sm.createSensorSimulator(sensorType, sensorID)
			if err != nil {
				return fmt.Errorf("failed to create sensor %s: %w", sensorID, err)
			}
			sm.Simulators = append(sm.Simulators, simulator)
		}
	}

	//start all simulators
	for _, simulator := range sm.Simulators {
		sm.WaitGroup.Add(1)
		go simulator.Start(&sm.WaitGroup)
	}

	return nil
}

// Stop stops all sensor simulators
func (sm *SensorManager) Stop() {
	log.Println("Stopping all sensor simulators...")

	for _, simulator := range sm.Simulators {
		close(simulator.StopChan)
	}

	sm.WaitGroup.Wait()

	//disconn MQTT clients
	for _, simulator := range sm.Simulators {
		if simulator.MQTTClient.IsConnected() {
			simulator.MQTTClient.Disconnect(250)
		}
	}

	log.Println("All sensor simulators stopped")
}

// createSensorSimulator creates and connects a sensor simulator to MQTT
func (sm *SensorManager) createSensorSimulator(sensorType types.Sensor, sensorID string) (*SensorSimulator, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s", sm.BrokerURL))
	opts.SetClientID(fmt.Sprintf("sensor-%s", sensorID))
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Printf("Sensor %s connected to MQTT broker", sensorID)
	})
	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("Sensor %s lost connection to MQTT broker: %v", sensorID, err)
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return &SensorSimulator{
		SensorType: sensorType,
		SensorID:   sensorID,
		MQTTClient: client,
		StopChan:   make(chan struct{}),
	}, nil
}

// Start starts the sensor simulation
func (s *SensorSimulator) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(time.Duration(s.SensorType.DataGenerationInterval) * time.Millisecond)
	defer ticker.Stop()

	//init with base value
	baseValue := s.SensorType.MinValue + rand.Float64()*(s.SensorType.MaxValue-s.SensorType.MinValue)

	log.Printf("Started sensor simulation for %s (%s)", s.SensorID, s.SensorType.Name)

	for {
		select {
		case <-s.StopChan:
			log.Printf("Stopping sensor %s", s.SensorID)
			return
		case <-ticker.C:
			value := s.generateSensorValue(baseValue)
			data := types.SensorData{
				SensorID:  s.SensorID,
				Timestamp: time.Now(),
				Value:     value,
				Unit:      s.SensorType.Unit,
			}

			//publish to MQTT
			if err := s.publishData(data); err != nil {
				log.Printf("Error publishing data from sensor %s: %v", s.SensorID, err)
			}

			//apply drift for next reading
			baseValue = s.applyDrift(baseValue)
		}
	}
}

// generateSensorValue generates a sensor value with noise
func (s *SensorSimulator) generateSensorValue(baseValue float64) float64 {
	noise := (rand.Float64()*2 - 1) * s.SensorType.NoiseLevel * baseValue
	value := baseValue + noise

	//ensure value is within sensor range
	if value < s.SensorType.MinValue {
		value = s.SensorType.MinValue
	} else if value > s.SensorType.MaxValue {
		value = s.SensorType.MaxValue
	}

	return value
}

// applyDrift applies random drift to the base value
func (s *SensorSimulator) applyDrift(baseValue float64) float64 {
	driftRange := (s.SensorType.MaxValue - s.SensorType.MinValue) * 0.001
	drift := (rand.Float64()*2 - 1) * driftRange

	newValue := baseValue + drift

	//wnsure the value stays within range
	if newValue < s.SensorType.MinValue {
		newValue = s.SensorType.MinValue
	} else if newValue > s.SensorType.MaxValue {
		newValue = s.SensorType.MaxValue
	}

	return newValue
}

// publishData publishes sensor data to MQTT topic
func (s *SensorSimulator) publishData(data types.SensorData) error {
	topic := fmt.Sprintf("sensors/%s/%s", s.SensorType.ID, s.SensorID)

	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal sensor data: %w", err)
	}

	//publish topci to MQTT
	token := s.MQTTClient.Publish(topic, 0, false, jsonData)
	token.Wait()

	if token.Error() != nil {
		return fmt.Errorf("failed to publish to topic %s: %w", topic, token.Error())
	}

	log.Printf("Published data from %s: %.2f %s to topic %s",
		s.SensorID, data.Value, data.Unit, topic)

	return nil
}

func main() {
	brokerHost := flag.String("mqtt-host", "localhost", "MQTT broker hostname")
	brokerPort := flag.Int("mqtt-port", 1883, "MQTT broker port")
	instancesPerType := flag.Int("instances", 3, "Number of instances per sensor type")
	duration := flag.Int("duration", 0, "Run duration in seconds (0 = run until interrupted)")
	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	brokerURL := fmt.Sprintf("%s:%d", *brokerHost, *brokerPort)
	manager := NewSensorManager(brokerURL, *instancesPerType, *duration)

	if err := manager.Start(); err != nil {
		log.Fatalf("Failed to start sensor manager: %v", err)
	}

	//set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	//wait for termination signal or duration
	if *duration > 0 {
		log.Printf("Sensors will run for %d seconds", *duration)
		select {
		case <-sigChan:
			log.Println("Received termination signal")
		case <-time.After(time.Duration(*duration) * time.Second):
			log.Println("Run duration reached")
		}
	} else {
		log.Println("Sensors running until interrupted")
		<-sigChan
		log.Println("Received termination signal")
	}

	manager.Stop()
}

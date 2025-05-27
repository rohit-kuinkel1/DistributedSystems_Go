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
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Gateway represents the IoT Gateway that receives data via MQTT and forwards via HTTP
type Gateway struct {
	ServerURL     string           // HTTP server URL to forward data to
	MQTTBrokerURL string           // MQTT broker URL
	Client        *http.HttpClient // HTTP client for forwarding data
	MQTTClient    mqtt.Client      // MQTT client for receiving sensor data
	StopChan      chan struct{}    // Channel for graceful shutdown
	WaitGroup     sync.WaitGroup   // Ensures clean shutdown
	MessageCount  int64            // Count of processed messages
	mutex         sync.Mutex       // Protects message count
}

// GatewayFactory creates a new IoT Gateway
func GatewayFactory(serverURL, mqttBrokerURL string) *Gateway {
	return &Gateway{
		ServerURL:     serverURL,
		MQTTBrokerURL: mqttBrokerURL,
		Client:        http.HttpClientFactory(5 * time.Second),
		StopChan:      make(chan struct{}),
		MessageCount:  0,
	}
}

// Start starts the IoT Gateway
func (g *Gateway) Start() error {
	log.Printf("Starting IoT Gateway")
	log.Printf("HTTP Server: %s", g.ServerURL)
	log.Printf("MQTT Broker: %s", g.MQTTBrokerURL)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s", g.MQTTBrokerURL))
	opts.SetClientID("iot-gateway")
	opts.SetCleanSession(true)
	opts.SetAutoReconnect(true)
	opts.SetKeepAlive(60 * time.Second)
	opts.SetPingTimeout(10 * time.Second)

	// Connection handlers
	opts.SetOnConnectHandler(func(client mqtt.Client) {
		log.Println("Gateway connected to MQTT broker")
		g.subscribeToTopics(client)
	})

	opts.SetConnectionLostHandler(func(client mqtt.Client, err error) {
		log.Printf("Gateway lost connection to MQTT broker: %v", err)
	})

	g.MQTTClient = mqtt.NewClient(opts)

	//connect to MQTT broker
	if token := g.MQTTClient.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	log.Println("Gateway started successfully")
	return nil
}

// subscribeToTopics subscribes to all sensor topics
func (g *Gateway) subscribeToTopics(client mqtt.Client) {
	//subscribe to all sensor topics using wildcard
	topic := "sensors/+/+"

	token := client.Subscribe(topic, 0, g.messageHandler)
	token.Wait()

	if token.Error() != nil {
		log.Printf("Failed to subscribe to topic %s: %v", topic, token.Error())
	} else {
		log.Printf("Successfully subscribed to topic: %s", topic)
	}
}

// messageHandler handles incoming MQTT messages
func (g *Gateway) messageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Received message from topic %s", msg.Topic())

	var sensorData types.SensorData
	if err := json.Unmarshal(msg.Payload(), &sensorData); err != nil {
		log.Printf("Error parsing sensor data from topic %s: %v", msg.Topic(), err)
		return
	}

	//forward data to HTTP server
	g.WaitGroup.Add(1)
	go func() {
		defer g.WaitGroup.Done()

		startTime := time.Now()
		if err := g.forwardData(sensorData); err != nil {
			log.Printf("Error forwarding data from sensor %s: %v", sensorData.SensorID, err)
		} else {
			rtt := time.Since(startTime)
			log.Printf("Successfully forwarded data from %s (RTT: %v)", sensorData.SensorID, rtt)

			//update message count
			g.mutex.Lock()
			g.MessageCount++
			if g.MessageCount%100 == 0 {
				log.Printf("Processed %d messages", g.MessageCount)
			}
			g.mutex.Unlock()
		}
	}()
}

// forwardData forwards sensor data to the HTTP server
func (g *Gateway) forwardData(data types.SensorData) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("error marshaling data to JSON: %w", err)
	}

	resp, err := g.Client.PostJSON(g.ServerURL+"/data", jsonData)
	if err != nil {
		return fmt.Errorf("error sending data to server: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned non-OK status: %d %s", resp.StatusCode, resp.StatusText)
	}

	return nil
}

// Stop stops the IoT Gateway
func (g *Gateway) Stop() {
	log.Println("Stopping IoT Gateway...")

	//signal all goroutines to stop
	close(g.StopChan)

	//wait for all message processing to complete
	g.WaitGroup.Wait()

	//disconn from MQTT broker
	if g.MQTTClient != nil && g.MQTTClient.IsConnected() {
		g.MQTTClient.Disconnect(250)
		log.Println("Disconnected from MQTT broker")
	}

	g.mutex.Lock()
	finalCount := g.MessageCount
	g.mutex.Unlock()

	log.Printf("IoT Gateway stopped. Total messages processed: %d", finalCount)
}

// GetMessageCount returns the current message count (thread-safe)
func (g *Gateway) GetMessageCount() int64 {
	g.mutex.Lock()
	defer g.mutex.Unlock()
	return g.MessageCount
}

func main() {
	serverHost := flag.String("server-host", "localhost", "Server hostname")
	serverPort := flag.Int("server-port", 8080, "Server port")
	mqttHost := flag.String("mqtt-host", "localhost", "MQTT broker hostname")
	mqttPort := flag.Int("mqtt-port", 1883, "MQTT broker port")
	duration := flag.Int("duration", 0, "Run duration in seconds (0 = run until interrupted)")
	flag.Parse()

	serverURL := fmt.Sprintf("http://%s:%d", *serverHost, *serverPort)
	mqttBrokerURL := fmt.Sprintf("%s:%d", *mqttHost, *mqttPort)

	gateway := GatewayFactory(serverURL, mqttBrokerURL)

	if err := gateway.Start(); err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}

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

	gateway.Stop()
}

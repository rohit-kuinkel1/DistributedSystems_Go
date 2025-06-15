package performance

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE/pkg/types"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// TestMQTTPerformance tests MQTT throughput and latency
func TestMQTTPerformance(t *testing.T) {
	brokerURL := "tcp://localhost:1883"
	testDuration := 120 * time.Second
	publishersCount := 1000
	publishInterval := 100 * time.Millisecond //1000 messages per second per publisher

	log.Printf("Starting MQTT performance test")
	log.Printf("Duration: %v, Publishers: %d, Interval: %v", testDuration, publishersCount, publishInterval)

	//setup the subscriber to count messages
	subscriber := &MQTTSubscriber{
		BrokerURL:    brokerURL,
		MessageCount: 0,
		StartTime:    time.Now(),
	}

	err := subscriber.Connect()
	if err != nil {
		t.Fatalf("Failed to connect subscriber: %v", err)
	}
	defer subscriber.Disconnect()

	var wg sync.WaitGroup
	stopChan := make(chan struct{})

	for i := range publishersCount {
		wg.Add(1)
		go func(publisherID int) {
			defer wg.Done()
			publisher := &MQTTPublisher{
				BrokerURL:   brokerURL,
				PublisherID: publisherID,
			}

			err := publisher.Connect()
			if err != nil {
				log.Printf("Publisher %d failed to connect: %v", publisherID, err)
				return
			}
			defer publisher.Disconnect()

			publisher.PublishLoop(stopChan, publishInterval)
		}(i)
	}

	//run test for specified duration
	time.Sleep(testDuration)
	close(stopChan)
	wg.Wait()

	//calculate statistics
	subscriber.mutex.Lock()
	totalMessages := subscriber.MessageCount
	actualDuration := time.Since(subscriber.StartTime)
	subscriber.mutex.Unlock()

	stats := MQTTStatistics{
		TotalMessages:     totalMessages,
		Duration:          actualDuration,
		Publishers:        publishersCount,
		MessagesPerSecond: float64(totalMessages) / actualDuration.Seconds(),
		MessagesPerMinute: float64(totalMessages) / actualDuration.Minutes(),
	}

	log.Printf("MQTT Performance Test Results:")
	log.Printf("  Total messages:     %d", stats.TotalMessages)
	log.Printf("  Test duration:      %v", stats.Duration)
	log.Printf("  Publishers:         %d", stats.Publishers)
	log.Printf("  Messages per second: %.2f", stats.MessagesPerSecond)
	log.Printf("  Messages per minute: %.2f", stats.MessagesPerMinute)

	err = writeMQTTResultsToFile(stats, "mqtt_performance_results.txt")
	if err != nil {
		t.Errorf("Failed to write results to file: %v", err)
	}
}

type MQTTStatistics struct {
	TotalMessages     int64
	Duration          time.Duration
	Publishers        int
	MessagesPerSecond float64
	MessagesPerMinute float64
}

type MQTTSubscriber struct {
	BrokerURL    string
	Client       mqtt.Client
	MessageCount int64
	StartTime    time.Time
	mutex        sync.Mutex
}

type MQTTPublisher struct {
	BrokerURL   string
	PublisherID int
	Client      mqtt.Client
}

func (s *MQTTSubscriber) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(s.BrokerURL)
	opts.SetClientID("mqtt-perf-subscriber")
	opts.SetCleanSession(true)

	s.Client = mqtt.NewClient(opts)
	token := s.Client.Connect()
	token.Wait()

	if token.Error() != nil {
		return token.Error()
	}

	//subscribe to all sensor topics
	token = s.Client.Subscribe("sensors/+/+", 0, s.messageHandler)
	token.Wait()

	return token.Error()
}

func (s *MQTTSubscriber) messageHandler(client mqtt.Client, msg mqtt.Message) {
	s.mutex.Lock()
	s.MessageCount++
	if s.MessageCount%1000 == 0 {
		log.Printf("Received %d messages", s.MessageCount)
	}
	s.mutex.Unlock()
}

func (s *MQTTSubscriber) Disconnect() {
	if s.Client != nil && s.Client.IsConnected() {
		s.Client.Disconnect(250)
	}
}

func (p *MQTTPublisher) Connect() error {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(p.BrokerURL)
	opts.SetClientID(fmt.Sprintf("mqtt-perf-publisher-%d", p.PublisherID))
	opts.SetCleanSession(true)

	p.Client = mqtt.NewClient(opts)
	token := p.Client.Connect()
	token.Wait()

	return token.Error()
}

func (p *MQTTPublisher) PublishLoop(stopChan chan struct{}, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stopChan:
			return
		case <-ticker.C:
			data := types.SensorData{
				SensorID:  fmt.Sprintf("perf-test-%d", p.PublisherID),
				Timestamp: time.Now(),
				Value:     23.5,
				Unit:      "°C",
			}

			jsonData, _ := json.Marshal(data)
			topic := fmt.Sprintf("sensors/temp/perf-test-%d", p.PublisherID)

			token := p.Client.Publish(topic, 0, false, jsonData)
			token.Wait()
		}
	}
}

func (p *MQTTPublisher) Disconnect() {
	if p.Client != nil && p.Client.IsConnected() {
		p.Client.Disconnect(250)
	}
}

func writeMQTTResultsToFile(stats MQTTStatistics, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	file.WriteString("MQTT Performance Test Results\n")
	file.WriteString("=============================\n\n")

	fmt.Fprintf(file, "Total messages:     %d\n", stats.TotalMessages)
	fmt.Fprintf(file, "Test duration:      %v\n", stats.Duration)
	fmt.Fprintf(file, "Publishers:         %d\n", stats.Publishers)
	fmt.Fprintf(file, "Messages per second: %.2f\n", stats.MessagesPerSecond)
	fmt.Fprintf(file, "Messages per minute: %.2f\n", stats.MessagesPerMinute)

	return nil
}

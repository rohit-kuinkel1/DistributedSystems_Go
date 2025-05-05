package types

import "time"

// SensorData represents the data received from sensors
type SensorData struct {
	SensorID  string    `json:"sensorId"`
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
}

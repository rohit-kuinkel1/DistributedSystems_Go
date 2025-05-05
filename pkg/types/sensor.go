package types

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

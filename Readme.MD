# Introduction
This README covers setup, build, test, and run instructions for a complete Industrial IoT system with redundant storage and Two-Phase Commit (2PC) consistency guarantees.

## System Architecture

The system implements a complete Industrial IoT stack with the following components:

```
[Sensors] --MQTT--> [Gateway] --HTTP--> [Server] --2PC--> [Database1]
                                                          [Database2]
```

### Overview
- **Redundant Storage**: Data is stored across two database instances using Two-Phase Commit protocol
- **High Availability**: System continues operation if one database fails
- **Data Consistency**: 2PC ensures atomic commits across both databases
- **Multiple Protocols**: MQTT (sensors), HTTP (gateway-server), gRPC (server-databases)
- **Containerized Deployment**: Full Docker Compose support

## Prerequisites

### Go Installation
#### macOS
```bash
brew install go
brew install protobuf
```

#### Linux (Ubuntu/Debian)
```bash
sudo apt update
sudo apt install golang-go
sudo apt install -y protobuf-compiler
```

#### Windows
```bash 
choco install golang
choco install protoc
```

#### All Platforms 
``` bash 
go get google.golang.org/protobuf/cmd/protoc-gen-go
go get google.golang.org/grpc/cmd/protoc-gen-go-grpc

go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

go get google.golang.org/grpc
go get google.golang.org/protobuf
go get google.golang.org/genproto
```

### Make Installation

#### macOS (if not already installed)
```bash
brew install make
```

#### Linux (Ubuntu/Debian)
```bash
sudo apt install make
```

#### Windows
Choose one of these options:
- **Git Bash**: Comes with make
- **Chocolatey**: `choco install make`
- **WSL**: Install Windows Subsystem for Linux, then install make with apt

### Docker Installation

#### macOS
- Download [Docker Desktop](https://www.docker.com/products/docker-desktop) and install

#### Linux (Ubuntu/Debian)
```bash
sudo apt install docker.io docker-compose
```

#### Windows
- Download [Docker Desktop](https://www.docker.com/products/docker-desktop) and install

## Getting Started

### Clone the Repository
```bash
git clone https://code.fbi.h-da.de/distributed-systems/praktika/lab-for-distributed-systems-2025-sose/moore/Mo-4X-TeamE.git
```
```bash
cd Mo-4X-TeamE
```

## Building and Running

### Quick Start with Docker (Recommended)
```bash
make docker-build
make docker-run

#access the web interface
open http://localhost:8080
```

### Manual Build and Run
```bash
make build

#start the complete 2PC system
make run-2pc-system

#or start with MQTT sensors
make run-mqtt-system
```

### Individual Component Testing
```bash
#run all tests (functional + performance + 2PC)
make test-all

#run only 2PC tests
make test-2pc-functional
make test-2pc-perf

#clean build artifacts
make clean
```

## System Components

### 1. Database Services (Redundant Storage)
Two identical database instances provide redundant storage:
```bash
#Primary
./bin/database -port 50051 -data-limit 1000000

#Redundant / Secondary
./bin/database -port 50052 -data-limit 1000000
```

### 2. HTTP Server with 2PC Coordinator
The server coordinates Two-Phase Commit transactions across both databases:
```bash
./bin/server -host 0.0.0.0 -port 8080 \
  -db-addr1 localhost:50051 \
  -db-addr2 localhost:50052
```

**Key Endpoints:**
- `POST /data` - Store sensor data using 2PC (atomic across both databases)
- `GET /data` - Retrieve all sensor data 
- `GET /data/{sensorId}` - Retrieve data for specific sensor
- `GET /` - Web interface for viewing data
- `GET /performance/2pc` - Run 2PC performance test

### 3. IoT Gateway
Receives MQTT messages from sensors and forwards via HTTP:
```bash
./bin/gateway -server-host localhost -server-port 8080 \
  -mqtt-host localhost -mqtt-port 1883
```

### 4. Sensor Simulators
Generate realistic sensor data published via MQTT:
```bash
./bin/sensor -mqtt-host localhost -mqtt-port 1883 \
  -instances 3 -duration 300
```

## Two-Phase Commit Implementation

### Working
1. **Prepare Phase**: Server sends `PrepareTransaction` to both databases
2. **Decision Phase**: 
   - If both databases vote "YES" → Send `CommitTransaction` to both
   - If any database votes "NO" → Send `AbortTransaction` to both

### Transaction Safety
- **Atomicity**: Either both databases store the data or neither does
- **Consistency**: Both databases always contain identical data
- **Durability**: Committed data persists in both databases
- **Timeout Handling**: Prepared transactions expire after 30 seconds

### Failure Scenarios
- **Single Database Failure**: Transaction aborts, no data corruption
- **Network Partition**: Prepared transactions timeout and rollback
- **Server Crash**: Databases cleanup expired prepared transactions

## Testing

### Functional Tests
```bash
#test HTTP + 2PC integration
make test-2pc-functional

#test complete system functionality  
go test -v ./tests/functional/...
```

### Performance Tests
```bash
make test-performance-all

make test-http-perf    #HTTP performance
make test-rpc-perf     #RPC performance  
make test-2pc-perf     #2PC overhead analysis
make test-mqtt-perf    #MQTT throughput
```

## Docker Deployment

### Complete System
```bash
#start all services with redundant storage
docker-compose up

#scale sensor simulators
docker-compose up --scale sensors=5

#view logs
docker-compose logs -f server
docker-compose logs -f database
```

### Services Overview
- **database + database2**: Redundant gRPC database services
- **server**: HTTP server with 2PC coordination
- **mqtt-broker**: Eclipse Mosquitto MQTT broker
- **gateway**: MQTT→HTTP bridge
- **sensors**: Sensor data simulators (scalable)

## Development

### Code Generation
```bash
#regenerate gRPC code after protocol changes
make generate
```
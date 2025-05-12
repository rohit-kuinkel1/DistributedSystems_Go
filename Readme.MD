# Distributed Systems Project
This README covers setup, build, test, and run instructions across different operating systems (haven't tested it on Windows so take it with a pinch of salt).

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

### Building the Project

```bash
#build all components
make build

#clean build artifacts
make clean

#run functional tests
make test-functional

#run performance tests
make test-performance
```

### Docker Development

```bash
#build the Docker images
make docker-build

#run with Docker Compose
make docker-run
```

## Planned Project Structure throughout the Practica;

```
├── cmd                # Main applications
│   ├── server         # Server component
│   ├── gateway        # IoT Gateway component
│   └── sensor         # Sensor simulator
├── internal           # Private application code
│   └── database       # Database implementation
├── pkg                # Public libraries
│   ├── http           # HTTP implementation
│   ├── rpc            # RPC implementation
│   └── mom            # Message-Oriented Middleware
├── tests              # Test suite
│   ├── functional     # Functional tests
│   └── performance    # Performance tests
├── bin                # Compiled binaries (generated)
├── Dockerfile         # Docker configuration
├── docker-compose.yml # Docker Compose configuration
├── Makefile           # Build automation
└── README.md          # This file
```

## Running Components Individually

### Run the Server
```bash
./bin/server    #on Unix systems
bin\server.exe  #on Windows
```

### Run the Gateway
```bash
./bin/gateway    #on Unix systems
bin\gateway.exe  #on Windows
```

### Run the Sensor Simulator
```bash
./bin/sensor    #on Unix systems
bin\sensor.exe  #on Windows
```
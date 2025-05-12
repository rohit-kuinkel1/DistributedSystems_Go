ifeq ($(OS),Windows_NT)
	RM := rmdir /s /q
	MKDIR := mkdir
	BINARY_EXT := .exe
	PATHSEP := \\
else
	RM := rm -rf
	MKDIR := mkdir -p
	BINARY_EXT :=
	PATHSEP := /
endif

#ensure bin directory exists before building
$(shell $(MKDIR) bin 2>/dev/null)

.PHONY: build test-functional test-performance clean docker-build docker-run all test generate

all: build

#create a new generate target for protobuf/gRPC
generate:
	@echo "Generating gRPC code..."
	$(MKDIR) pkg$(PATHSEP)generated$(PATHSEP)rpc
	protoc --go_out=. --go-grpc_out=. pkg/rpc/database.proto

#update build to depend on generate
build: generate
	go build -o bin$(PATHSEP)server$(BINARY_EXT) ./cmd/server
	go build -o bin$(PATHSEP)gateway$(BINARY_EXT) ./cmd/gateway
	go build -o bin$(PATHSEP)sensor$(BINARY_EXT) ./cmd/sensor
	@echo "Build completed successfully"

test-functional:
	go test -v ./tests/functional/...

test-performance:
	go test -v ./tests/performance/...

clean:
ifeq ($(OS),Windows_NT)
	if exist bin $(RM) bin
	if exist pkg\generated $(RM) pkg\generated
else
	$(RM) bin
	$(RM) pkg/generated
endif
	@echo "Clean completed successfully"

test: clean build test-functional test-performance
	@echo "All tests completed successfully"

docker-build:
	docker-compose build

docker-run:
	docker-compose up
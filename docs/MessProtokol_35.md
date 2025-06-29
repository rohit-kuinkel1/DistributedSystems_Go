# Performance Analysis Protocol for 2PC

**Team:** <br> 
Rohit Kuinkel (1116814), <br>
Leopold Keller  <br>
**Date:** June 29, 2025

**Key Findings:**
- **Two-Phase Commit provides strong consistency at significant performance cost**: 5.81x slower than direct RPC
- **Redundant storage ensures data integrity**: Both databases maintain identical state
- **Performance degradation is quantifiable and predictable**: 481.2% latency overhead, 82.8% throughput reduction
- **Concurrent load amplifies 2PC overhead**: Additional 220.6% degradation under concurrent access
- **Trade-off is measurable**: Consistency and fault tolerance vs. raw performance

**Primary Finding:** Two-Phase Commit successfully provides ACID guarantees across redundant storage with measurable but acceptable performance overhead for high-availability requirements.

## Configurations for tests

### Configuration Overview
| System | Architecture | Mean RTT | Throughput | Consistency | Fault Tolerance |
|--------|-------------|----------|------------|-------------|-----------------|
| **Direct RPC** | Single Database | 51.1 µs | 19,569 req/sec | Single Point | Zero |
| **2PC Sequential** | Dual Database (2PC) | 296.8 µs | 3,367 req/sec | Good | Full |
| **2PC Concurrent** | Dual Database (Load) | 951.3 µs | 10,444 req/sec | Good | Full |

### Test Environment
- **Hardware:** M4 MacBook Air
- **Network:** localhost (no network latency)
- **Database Instances:** Two identical gRPC database services (ports 50051, 50052)
- **Load Pattern:** 10,000 requests for analysis, concurrent testing with 10 clients
- **Test Duration:** Varies by test complexity

## Results

### Direct RPC Performance (Baseline)
- **Mean RTT:** 51.1 µs
- **Median RTT:** 43.5 µs  
- **Throughput:** 19,569 req/sec
- **95th Percentile:** 84.5 µs
- **Standard Deviation:** 90.3 µs

### Two-Phase Commit Performance (Sequential)
- **Mean RTT:** 296.8 µs
- **Median RTT:** 269.5 µs
- **Throughput:** 3,367 req/sec  
- **95th Percentile:** 465.1 µs
- **Standard Deviation:** 154.2 µs

### Two-Phase Commit Performance (Concurrent Load)
- **Mean RTT:** 951.3 µs
- **Median RTT:** 826.3 µs
- **Throughput:** 10,444 req/sec
- **95th Percentile:** 1,710.2 µs
- **Standard Deviation:** 400.0 µs

## Performance Impact Analysis

### 1. Consistency Cost Analysis
```
Performance Ranking (by latency):
1. Direct RPC:        51.1 µs    (19,569 req/sec)
2. 2PC Sequential:    296.8 µs   (3,367 req/sec)  
3. 2PC Concurrent:    951.3 µs   (10,444 req/sec)

Analysis of performance overhead:
- 2PC vs Direct RPC: 5.81x slower (245.7 µs overhead)
- 2PC Sequential vs Concurrent: 3.2x slower under load
- Consistency Cost: 481.2% latency increase for ACID guarantees
```

**ACID =>ACID in databases ensures reliable transactions. Atomicity(All or nothing), Consistency, Isolation(Independent), Durability**

### 2. Fault Tolerance vs Performance Trade-off
**Two-Phase Commit Benefits:**
- **Atomicity:** Either both databases store data or neither does
- **Consistency:** Both databases always contain identical data  
- **Isolation:** Transactions are isolated using unique transaction IDs
- **Durability:** Committed data persists in both databases with redundancy

**Performance Costs:**
- **Latency Overhead:** 481.2% increase (245.7 µs additional per transaction)
- **Throughput Degradation:** 82.8% reduction (16,202 req/sec loss)
- **Resource Utilization:** 2x network calls, 2x database operations
- **Coordination Overhead:** Transaction state management and timeouts

### 3. Concurrent Load Impact
**Baseline vs Under Load:**
- **Mean RTT Degradation:** 296.8µs → 951.3µs (+220.6% increase)
- **Throughput Impact:** 3,367 → 10,444 req/sec (apparent improvement due to concurrent clients)
- **Variance Amplification:** Standard deviation increases 159% under load (154.2µs → 400.0µs)
- **Coordination Bottleneck:** Transaction coordination becomes primary limiting factor

## Comparison with Previous Exercises

### Performance Evolution Across Exercises
| Protocol | Mean RTT | Throughput | Relative Performance |
|----------|----------|------------|----------------------|
| Raw HTTP (Local) | 513.8µs | 1,946 req/sec | Baseline HTTP |
| Pure RPC | 47.8µs | 20,914 req/sec | 10.7x faster than HTTP |
| HTTP+RPC | 928.2µs | 1,077 req/sec | Layered overhead |
| 2PC | 296.8µs | 3,367 req/sec | **Consistency + Performance** |

### Insights
1. **Two-Phase Commit provides optimal balance**: Faster than HTTP+RPC layered approach while ensuring consistency
2. **Direct RPC remains fastest** for single-database operations but offers no redundancy
3. **2PC overhead is predictable**: 5.81x slower than direct RPC but 1.7x faster than raw HTTP
4. **Redundancy cost is measurable**: Clear performance impact for fault tolerance benefits

## Reliability Analysis

### Transaction Success Rates
- **Prepare Phase Success:** 100% with both databases healthy
- **Commit Phase Success:** 100% after successful prepare
- **Abort Handling:** Proper cleanup of prepared transactions
- **Timeout Management:** 30-second timeout prevents resource leaks

### Failure Scenarios Tested
1. **Single Database Failure:** Transaction properly aborts, no data corruption
2. **Concurrent Transactions:** Unique transaction IDs prevent conflicts  
3. **Prepared Transaction Cleanup:** Expired transactions cleaned up automatically
4. **Network Partition Simulation:** Transactions timeout and rollback correctly

## Consistency Verification

### Data Integrity Tests
- **Atomic Commits:** Both databases always contain identical data sets
- **Rollback Verification:** Failed transactions leave no partial data
- **Concurrent Access:** Multiple simultaneous transactions maintain consistency
- **Cross-Database Validation:** Direct database queries confirm data synchronization

### ACID Property Verification
- **Atomicity:** All-or-nothing transaction semantics enforced
- **Consistency:** Database constraints maintained across both instances  
- **Isolation:** Transaction IDs prevent interference between concurrent operations
- **Durability:** Committed data persists in both databases with redundancy

## Conclusion and Trade-off

### Performance vs. Consistency Trade-offs
1. **2PC provides strong consistency** with measurable 5.81x performance cost vs. direct RPC
2. **Redundant storage ensures high availability** at 82.8% throughput reduction cost
3. **Fault tolerance justifies overhead** for critical data integrity requirements
4. **Concurrent load impacts are manageable** with proper resource allocation

### Production Suitability Assessment
- **Latency:** 296.8µs mean RTT acceptable for most business applications
- **Throughput:** 3,367 req/sec sufficient for moderate-scale IoT deployments
- **Reliability:** Strong consistency guarantees justify performance overhead
- **Scalability:** System handles concurrent load with predictable degradation

### Architectural Recommendations
1. **Use 2PC for critical data** requiring strong consistency and fault tolerance
2. **Consider direct RPC for read-heavy workloads** where redundancy isn't required
3. **Implement load balancing** to distribute read operations across both databases
4. **Monitor transaction timeout rates** to optimize coordination overhead

## Performance Metrics

| Metric | Direct RPC | 2PC Sequential | Performance Impact |
|--------|------------|----------------|-------------------|
| **Mean Latency** | 51.1 µs | 296.8 µs | +481.2% |
| **Throughput** | 19,569 req/sec | 3,367 req/sec | -82.8% |
| **95th Percentile** | 84.5 µs | 465.1 µs | +450.4% |
| **Consistency** | Single Point | Strong (2DB) | ACID Guarantees |
| **Fault Tolerance** | None | Full Redundancy | High Availability |

Two-Phase Commit successfully provides good consistency and fault tolerance with quantifiable but acceptable performance overhead of 5.81x latency increase for data integrity requirements.

## Test Environment Details
- **Database Services:** Two identical gRPC services with 1M data point capacity each
- **Transaction Coordination:** HTTP server implementing full 2PC protocol
- **Transaction Timeouts:** 30-second prepared transaction expiration
- **Load Testing:** 10,000 transactions across varying concurrency levels
- **Measurement Method:** Client-side RTT measurement with statistical analysis
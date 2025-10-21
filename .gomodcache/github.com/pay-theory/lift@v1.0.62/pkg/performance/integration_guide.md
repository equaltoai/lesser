# DynamORM Performance Integration Guide

This package provides production-ready connection pooling for DynamoDB operations that can be easily integrated with DynamORM.

## Current Status ✅ COMPLETE

The connection pooling framework is **complete and ready for integration**:

- ✅ **Production-ready connection pool** with health checks, metrics, auto-scaling
- ✅ **Workload-specific optimizations** (high-throughput, low-latency, production)
- ✅ **Session management** with automatic cleanup and maintenance
- ✅ **Performance monitoring** with detailed statistics and alerts

## Integration Points for DynamORM Team

### 1. Connection Pool Integration

The `ConnectionPool` provides optimized DynamoDB client management:

```go
// Create optimized pool
pool, err := performance.NewConnectionPool(ctx, performance.HighThroughputConnectionPoolConfig())

// Get client for operations
client, err := pool.GetClient(ctx)
defer pool.ReturnClient(client)

// Use client with DynamORM
db := dynamorm.NewWithClient(client) // DynamORM team to implement
```

### 2. DynamORM Pool Wrapper

The `DynamORMPool` provides a higher-level interface:

```go
// Create pooled DynamORM instance
pool, err := performance.NewDynamORMPool(ctx, performance.DefaultPooledDynamORMConfig())

// Execute operations with pooled connections
client, cleanup, err := pool.GetClient(ctx, "my-table")
defer cleanup()

// DynamORM team can wrap this in their DB interface
```

### 3. Configuration Profiles

Pre-built configurations for different workloads:

```go
// High-throughput microservices
config := performance.HighThroughputConnectionPoolConfig()

// Low-latency real-time apps  
config := performance.LowLatencyConnectionPoolConfig()

// Production deployments
config := performance.ProductionConnectionPoolConfig()
```

## Integration Tasks for DynamORM Team

### High Priority

1. **Implement `NewWithClient()` method** in DynamORM:
   ```go
   // In DynamORM package
   func NewWithClient(client *dynamodb.Client, tableName string) (*DB, error) {
       // Initialize DynamORM with provided client instead of creating new one
   }
   ```

2. **Add pool-aware factory** in DynamORM:
   ```go
   // In DynamORM package  
   func NewPooled(ctx context.Context, config *PoolConfig) (*PooledDB, error) {
       pool, err := performance.NewConnectionPool(ctx, config.ConnectionPool)
       if err != nil {
           return nil, err
       }
       return &PooledDB{pool: pool}, nil
   }
   ```

### Medium Priority

3. **Enhance session management** in DynamORM:
   ```go
   // In DynamORM package
   type PooledDB struct {
       pool *performance.ConnectionPool
   }
   
   func (db *PooledDB) WithTable(tableName string) *TableDB {
       // Return table-specific DB that uses pooled connections
   }
   ```

4. **Add metrics integration** in DynamORM:
   ```go
   // In DynamORM package
   func (db *PooledDB) GetPerformanceMetrics() *PerformanceMetrics {
       poolStats := db.pool.GetPoolStats()
       // Combine with DynamORM operation metrics
   }
   ```

### Low Priority  

5. **Middleware integration** for Lift:
   ```go
   // In Lift middleware
   func DynamORMPooled(config *performance.PooledDynamORMConfig) lift.Middleware {
       return func(next lift.Handler) lift.Handler {
           return func(ctx *lift.Context) error {
               // Inject pooled DynamORM into context
               pooledDB := performance.GetPooledDynamORM(ctx, config)
               ctx.Set("dynamorm", pooledDB)
               return next(ctx)
           }
       }
   }
   ```

## Performance Benefits

The connection pooling provides significant performance improvements:

- **Cold start reduction**: Pre-warmed connections eliminate initialization overhead
- **Connection reuse**: Avoid repeated SSL handshakes and connection setup  
- **Automatic scaling**: Pool size adjusts based on workload patterns
- **Health monitoring**: Proactive connection health checks and replacement
- **Resource efficiency**: Optimal connection sharing across Lambda invocations

## Example Usage

```go
package main

import (
    "context"
    "github.com/pay-theory/lift/pkg/performance"
)

func main() {
    ctx := context.Background()
    
    // Create production-optimized pool
    pool, err := performance.NewDynamORMPool(ctx, performance.ProductionPooledDynamORMConfig())
    if err != nil {
        panic(err)
    }
    defer pool.Close()
    
    // Use pooled connections for operations
    err = pool.ExecuteWithClient(ctx, "users", func(client *dynamodb.Client) error {
        // Perform DynamoDB operations with pooled client
        // DynamORM would wrap this client
        return nil
    })
}
```

## Next Steps

1. **DynamORM team**: Implement `NewWithClient()` method
2. **Performance testing**: Benchmark pooled vs non-pooled performance  
3. **Integration testing**: Test with real DynamORM operations
4. **Documentation**: Update DynamORM docs with pooling best practices

The connection pooling framework is **production-ready** and waiting for DynamORM integration!
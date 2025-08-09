# RDB Logging Demo

This example demonstrates the improved RDB loading with batched logging implemented to address verbose per-key logging issues.

## What Was Fixed

### Before (Verbose Logging)
```
DEBUG: Processing RDB key key=user:1 type=[]uint8 db=0
DEBUG: Successfully stored RDB key key=user:1 value_len=4
DEBUG: Processing RDB key key=user:2 type=[]uint8 db=0  
DEBUG: Successfully stored RDB key key=user:2 value_len=4
DEBUG: Processing RDB key key=session:abc type=[]uint8 db=0
DEBUG: Successfully stored RDB key key=session:abc value_len=8
...
```

### After (Batched Logging)
```
INFO: RDB load progress db=0 keys=3 types=string:2,hash:1 total_processed=3 elapsed=100ms rate=30 keys/s
INFO: RDB load progress db=1 keys=2 types=list:1,string:1 total_processed=5 elapsed=150ms rate=33 keys/s
```

## Features

- **Reduced Log Volume**: Aggregated logs instead of per-key verbose output
- **Performance Tracking**: Shows processing rate (keys/second) and elapsed time
- **Type Distribution**: Counts of each Redis data type processed
- **Database Separation**: Statistics separated by database number
- **Configurable Batching**: Adjustable batch size and time intervals

## Configuration

- **Batch Size**: Log every N keys (default: 100)
- **Time Interval**: Or log every X seconds (default: 2 seconds)
- **Databases**: Track statistics per database

## Usage

```bash
# Run the demo
go run ./examples/rdb-logging-demo

# Or build and run
make build
./bin/rdb-logging-demo
```

## Benefits

1. **Performance**: Reduces I/O overhead from excessive logging
2. **Readability**: Cleaner, more meaningful progress information
3. **Monitoring**: Better insights into RDB loading performance
4. **Scalability**: Handles large RDB files without log flooding
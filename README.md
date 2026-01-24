# pxctl - pxGrid Direct Test Data Generator and Loader

A CLI tool for generating and loading test data for Cisco ISE pxGrid Direct connectors. It connects to ISE, retrieves connector configurations, generates sample data according to the connector's schema, and can load that data into ISE via push connectors. It also provides utilities for managing connectors including deletion and recreation.

## Requirements

- Go 1.25 or greater
- Access to a Cisco ISE instance with pxGrid Direct configured
- Valid ISE credentials

## Building

This project uses [Task](https://taskfile.dev/) for build automation.

### Install Dependencies

```bash
task install-deps
```

### Build

```bash
task build
```

The binary will be created in `./bin/pxctl`.

### Clean

```bash
task clean
```

## Usage

### Global Options

All commands support the following global options:

- `-v, --verbose`: Enable verbose logging to stderr. When enabled, detailed information about command progression is logged, including:
  - HTTP request/response details (method, URL, status code, duration)
  - Retry and backoff events for rate limiting
  - Configuration parameters
  - Data processing steps
  - File operations

Example with verbose logging:
```bash
./bin/pxctl --verbose generate --connector SNOW_CMDB --count 10
```

### Generate Test Data

Generate test data for a pxGrid Direct connector:

```bash
./bin/pxctl generate \
  --host <ISE_FQDN_OR_IP> \
  --username <ISE_USERNAME> \
  --password <ISE_PASSWORD> \
  --connector <CONNECTOR_NAME> \
  --count <NUMBER_OF_ELEMENTS> \
  --output <OUTPUT_FILE>
```

### Command Options

- `-H, --host`: ISE FQDN or IP address (required, can use env: `PXCTL_ISE_HOST`)
- `-u, --username`: ISE username (required, can use env: `PXCTL_ISE_USERNAME`)
- `-p, --password`: ISE password (required, can use env: `PXCTL_ISE_PASSWORD`)
- `-c, --connector`: Name of the pxGrid Direct connector (required)
- `-n, --count`: Number of random data elements to create (default: 10)
- `-o, --output`: Output file name in JSON format (default: testdata.json)

### Environment Variables

For convenience and security, you can set ISE credentials using environment variables instead of command-line flags:

- `PXCTL_ISE_HOST`: ISE FQDN or IP address
- `PXCTL_ISE_USERNAME`: ISE username
- `PXCTL_ISE_PASSWORD`: ISE password

Environment variables are particularly useful for:
- Avoiding credentials in shell history
- CI/CD pipelines
- Scripting and automation

**Priority order**: Command-line flags override environment variables, which override config file values.

### Examples

#### Using Command-Line Flags

```bash
./bin/pxctl generate \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector SNOW_CMDB \
  --count 50 \
  --output snow_testdata.json
```

#### Using Environment Variables

```bash
# Set environment variables
export PXCTL_ISE_HOST=ise.example.com
export PXCTL_ISE_USERNAME=admin
export PXCTL_ISE_PASSWORD=password123

# Run command (no need to specify host, username, password)
./bin/pxctl generate \
  --connector SNOW_CMDB \
  --count 50 \
  --output snow_testdata.json
```

#### Mixed Approach

```bash
# Use environment variables for credentials
export PXCTL_ISE_USERNAME=admin
export PXCTL_ISE_PASSWORD=password123

# Override host with flag
./bin/pxctl generate \
  --host ise-prod.example.com \
  --connector SNOW_CMDB \
  --count 50 \
  --output snow_testdata.json
```

#### With Verbose Logging

```bash
# Enable verbose logging to see detailed HTTP interactions and progress
./bin/pxctl --verbose generate \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector SNOW_CMDB \
  --count 50 \
  --output snow_testdata.json
```

The verbose flag will output detailed information to stderr, such as:
```
[2024-01-23 10:15:30.123] Configuration: host=ise.example.com, username=admin, connector=SNOW_CMDB, count=50, output=snow_testdata.json
[2024-01-23 10:15:30.125] Creating ISE API client for host: ise.example.com
[2024-01-23 10:15:30.126] Fetching connector configuration for: SNOW_CMDB
[2024-01-23 10:15:30.127] HTTP Request: GET https://ise.example.com/api/v1/pxgrid-direct/connector-config/SNOW_CMDB
[2024-01-23 10:15:30.456] HTTP Response: 200 OK (took 329ms)
[2024-01-23 10:15:30.457] Connector type: PUSH, enabled: true
...
```

### Load Test Data

Load previously generated test data into ISE via a pxGrid Direct push connector:

```bash
./bin/pxctl load-data \
  --host <ISE_FQDN_OR_IP> \
  --username <ISE_USERNAME> \
  --password <ISE_PASSWORD> \
  --connector <PUSH_CONNECTOR_NAME> \
  --input <INPUT_JSON_FILE> \
  --batch-size <BATCH_SIZE>
```

#### Command Options

- `-H, --host`: ISE FQDN or IP address (required, can use env: `PXCTL_ISE_HOST`)
- `-u, --username`: ISE username (required, can use env: `PXCTL_ISE_USERNAME`)
- `-p, --password`: ISE password (required, can use env: `PXCTL_ISE_PASSWORD`)
- `-c, --connector`: Name of the pxGrid Direct push connector (required)
- `-i, --input`: Input JSON file containing test data (required)
- `-b, --batch-size`: Number of objects to submit per API call (default: 100)
- `-r, --backoff`: Seconds to wait on 429 rate limit (default: 0.5, min: 0.001, max: 120)
- `--empty-correlation-id`: Deliberately empty the correlation ID field to create bad requests (for testing)

#### How It Works

1. Reads the JSON test data file (typically generated by the `generate` command)
2. Extracts the data array from the top-level JSON object
3. Splits the data into batches based on `--batch-size`
4. Submits each batch serially to the `/api/v1/pxgrid-direct/push/{connectorName}/bulk` API
5. If a 429 (Too Many Requests) response is received, waits for the specified `--backoff` duration and retries
6. Reports timing for each batch loaded
7. Stops immediately if any batch fails with a non-429 error and reports the error

#### Examples

```bash
# Load data with default batch size (100)
./bin/pxctl load-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --input testdata.json

# Load data with custom batch size
./bin/pxctl load-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --input testdata.json \
  --batch-size 50

# Using environment variables
export PXCTL_ISE_HOST=ise.example.com
export PXCTL_ISE_USERNAME=admin
export PXCTL_ISE_PASSWORD=password123

./bin/pxctl load-data \
  --connector PUSH_CONNECTOR \
  --input testdata.json

# Custom backoff time for rate limiting
./bin/pxctl load-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --input testdata.json \
  --batch-size 50 \
  --backoff 1.0

# With verbose logging to see HTTP interactions and retry events
./bin/pxctl --verbose load-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --input testdata.json \
  --batch-size 50

# Testing error handling by deliberately emptying the correlation ID
./bin/pxctl load-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --input testdata.json \
  --empty-correlation-id
```

The command outputs a table showing the progress:
```
Batch      Objects         Duration        Status
-----      -------         --------        ------
1          100             850ms           success
2          100             1.1s            success
3          50              620ms           success

Successfully loaded 250 objects to connector 'PUSH_CONNECTOR'
```

When verbose logging is enabled, you'll see detailed progress information including retry events:
```
[2024-01-23 10:20:15.123] Configuration: host=ise.example.com, username=admin, connector=PUSH_CONNECTOR, input=testdata.json, batch-size=50, backoff=0.500s
[2024-01-23 10:20:15.125] Reading input file: testdata.json
[2024-01-23 10:20:15.130] Read 125643 bytes from input file
[2024-01-23 10:20:15.135] Extracted 500 data objects from input file
[2024-01-23 10:20:15.136] Processing 500 objects in 10 batches
[2024-01-23 10:20:15.137] Batch 1: submitting objects 0-49
[2024-01-23 10:20:15.138] Prepared bulk push request with 50 objects
[2024-01-23 10:20:15.139] HTTP Request: POST https://ise.example.com/api/v1/pxgrid-direct/push/PUSH_CONNECTOR/bulk
[2024-01-23 10:20:15.567] HTTP Response: 200 OK (took 428ms)
[2024-01-23 10:20:15.568] Batch 1 completed successfully: success
[2024-01-23 10:20:15.569] HTTP Request: POST https://ise.example.com/api/v1/pxgrid-direct/push/PUSH_CONNECTOR/bulk
[2024-01-23 10:20:15.789] HTTP Response: 429 Too Many Requests (took 220ms)
[2024-01-23 10:20:15.790] Retry: received 429 Too Many Requests - backing off for 0.500 seconds
[2024-01-23 10:20:16.291] HTTP Request: POST https://ise.example.com/api/v1/pxgrid-direct/push/PUSH_CONNECTOR/bulk
[2024-01-23 10:20:16.512] HTTP Response: 200 OK (took 221ms)
...
```

### Delete All Data

Delete all objects held by a pxGrid Direct push connector:

```bash
./bin/pxctl delete-data \
  --host <ISE_FQDN_OR_IP> \
  --username <ISE_USERNAME> \
  --password <ISE_PASSWORD> \
  --connector <PUSH_CONNECTOR_NAME>
```

#### Command Options

- `-H, --host`: ISE FQDN or IP address (required, can use env: `PXCTL_ISE_HOST`)
- `-u, --username`: ISE username (required, can use env: `PXCTL_ISE_USERNAME`)
- `-p, --password`: ISE password (required, can use env: `PXCTL_ISE_PASSWORD`)
- `-c, --connector`: Name of the pxGrid Direct push connector (required)
- `-b, --batch-size`: Number of objects to delete per API call (optional, defaults to 5MB payload limit only)
- `-r, --backoff`: Seconds to wait on 429 rate limit (default: 0.5, min: 0.001, max: 120)

#### How It Works

1. Retrieves all objects from the specified push connector using pagination
2. Extracts unique identifiers from the objects
3. Splits the objects into batches:
   - If `--batch-size` is specified: uses both the object count limit AND 5MB size limit (whichever is hit first)
   - If `--batch-size` is NOT specified: uses only the 5MB size limit for batching
4. Submits each batch serially to the `/api/v1/pxgrid-direct/push/{connectorName}/bulk` API with operation="delete"
5. If a 429 (Too Many Requests) response is received, waits for the specified `--backoff` duration and retries
6. Each delete request is guaranteed to be <= 5MB in size
7. Reports timing for each batch deleted
8. Stops immediately if any batch fails with a non-429 error and reports the error

#### Examples

```bash
# Delete all data using only 5MB payload limit (no object count limit)
./bin/pxctl delete-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR

# Delete with custom batch size (50 objects) and backoff
./bin/pxctl delete-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR \
  --batch-size 50 \
  --backoff 1.0

# Using environment variables
export PXCTL_ISE_HOST=ise.example.com
export PXCTL_ISE_USERNAME=admin
export PXCTL_ISE_PASSWORD=password123

./bin/pxctl delete-data --connector PUSH_CONNECTOR

# With verbose logging
./bin/pxctl --verbose delete-data \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR
```

The command outputs a table showing the progress:
```
Batch      Objects         Size            Duration        Status
-----      -------         ----            --------        ------
1          100             234.5 KB        1.2s            success
2          100             231.8 KB        980ms           success
3          50              115.3 KB        650ms           success

Successfully deleted 250 objects from connector 'PUSH_CONNECTOR'
```

### Recycle Connector

Delete and recreate a pxGrid Direct connector with the same configuration, or create a copy of an existing connector. This is useful for resetting a connector's state while preserving its configuration, or for duplicating a connector setup.

```bash
./bin/pxctl recycle-connector \
  --host <ISE_FQDN_OR_IP> \
  --username <ISE_USERNAME> \
  --password <ISE_PASSWORD> \
  --connector <CONNECTOR_NAME>
```

#### Command Options

- `-H, --host`: ISE FQDN or IP address (required, can use env: `PXCTL_ISE_HOST`)
- `-u, --username`: ISE username (required, can use env: `PXCTL_ISE_USERNAME`)
- `-p, --password`: ISE password (required, can use env: `PXCTL_ISE_PASSWORD`)
- `-c, --connector`: Name of the pxGrid Direct connector (required)
- `--copy`: Create a copy of the connector instead of deleting and recreating (optional)

#### How It Works

**Default Mode (Recycle):**
1. Retrieves the complete configuration of the named connector, including credentials
2. Attempts to delete the connector
3. If deletion succeeds, recreates the connector with the same configuration
4. Updates any schedule parameters (fullsyncSchedule, deltasyncSchedule) to be at least 30 minutes in the future
5. For PULL connectors (urlfetcher, vmware), triggers a full sync-now operation after recreation
6. For PUSH connectors (urlpusher), no sync is triggered

**Copy Mode (with --copy flag):**
1. Retrieves the complete configuration of the named connector, including credentials
2. Creates a new connector with a timestamped name (e.g., `CONNECTOR_NAME_copy_20260123_143025`)
3. Updates any schedule parameters (fullsyncSchedule, deltasyncSchedule) to be at least 30 minutes in the future
4. For PULL connectors (urlfetcher, vmware), triggers a full sync-now operation after creation
5. Original connector remains untouched

#### Use Cases

- Reset a connector that is in an error state
- Clear out accumulated data while preserving configuration
- Test connector recreation workflows
- Update schedule times to the future after cloning configurations
- Create a copy of a connector for testing or backup purposes

#### Examples

```bash
# Recycle a PULL connector
./bin/pxctl recycle-connector \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector SNOW_CMDB

# Recycle a PUSH connector
./bin/pxctl recycle-connector \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector PUSH_CONNECTOR

# Using environment variables
export PXCTL_ISE_HOST=ise.example.com
export PXCTL_ISE_USERNAME=admin
export PXCTL_ISE_PASSWORD=password123

./bin/pxctl recycle-connector --connector SNOW_CMDB

# With verbose logging to see detailed HTTP interactions
./bin/pxctl --verbose recycle-connector \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector SNOW_CMDB

# Copy a connector instead of recycling
./bin/pxctl recycle-connector \
  --host ise.example.com \
  --username admin \
  --password password123 \
  --connector SNOW_CMDB \
  --copy
```

Expected output for recycle mode:
```
Retrieving configuration for connector 'SNOW_CMDB'...
Retrieved connector configuration (type: urlfetcher)
Deleting connector 'SNOW_CMDB'...
Successfully deleted connector 'SNOW_CMDB'
Waiting 5 seconds before recreating connector...
Recreating connector 'SNOW_CMDB'...
Successfully recreated connector 'SNOW_CMDB'
Triggering sync-now for PULL connector 'SNOW_CMDB'...
Successfully triggered sync-now for connector 'SNOW_CMDB'

Connector 'SNOW_CMDB' recycled successfully!
```

Expected output for copy mode:
```
Retrieving configuration for connector 'SNOW_CMDB'...
Retrieved connector configuration (type: urlfetcher)
Copy mode: New connector will be named 'SNOW_CMDB_copy_20260123_143025'
Creating copy of connector as 'SNOW_CMDB_copy_20260123_143025'...
Successfully created connector copy 'SNOW_CMDB_copy_20260123_143025'
Triggering sync-now for PULL connector 'SNOW_CMDB_copy_20260123_143025'...
Successfully triggered sync-now for connector 'SNOW_CMDB_copy_20260123_143025'

Connector 'SNOW_CMDB' copied successfully as 'SNOW_CMDB_copy_20260123_143025'!
```

## Test Data Generation

The tool generates test data according to the following rules:

- **Correlation IDs**: Unique MAC addresses in the "locally administered" (LA) range
- **Unique identifiers**: UUID4 format
- **Version identifiers**: UTC timestamps in ISO 8601 format (RFC3339)
- **Other fields**: Random ASCII strings of random length between 8 and 64 characters

### JSON Path Handling

The connector configuration from ISE may include JSON path expressions with the `$.` prefix (e.g., `$.macAddress`, `$.sys_id`). The tool automatically strips this prefix when generating test data, so the output JSON will contain clean attribute names without the JSON path notation.

**Example:**
- Connector config attribute: `$.macAddress`
- Generated JSON attribute: `macAddress`

## Configuration

You can use a configuration file to store default values. By default, the tool looks for `~/.pxctl.yaml`, but you can specify a custom config file:

```bash
./bin/pxctl --config /path/to/config.yaml generate ...
```

## Development

### Project Structure

```
.
├── bin/                    # Compiled binaries
├── cmd/
│   └── pxctl/             # Main application entry point
├── internal/
│   ├── api/               # ISE API client
│   ├── cmd/               # Cobra commands
│   └── generator/         # Test data generation logic
├── go.mod                 # Go module definition
├── Taskfile.yml          # Task definitions for building
├── pxgrid-direct.yaml    # OpenAPI specification
└── SPECIFICATIONS.md     # Project specifications
```

### Libraries Used

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [UUID](https://github.com/google/uuid) - UUID generation

## License

This is a test tool for Cisco ISE pxGrid Direct.

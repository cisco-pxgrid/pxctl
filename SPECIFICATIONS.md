# SPECIFICATIONS

## Global Requirements & Information

* Use Go 1.25 or greater
* OpenAPI specification is in `pxgrid-direct.yaml`
* Call go module `github.com/einarnn/pxctl`
* Call Go application `pxctl`
* Generate go application to `./bin`
* Use Cobra and Viper libraries for all CLI and config
* Create a taskfile for took `task` to build and clean
* Please create a README.md for all operations supported by `pxctl`
* Support environment variables for all server, username and password parameters.
* Provide a `--verbose` flag that logs to stderr detailed information about the progression of all commands. Include a summary of HTTP interactions, any backoff/retry, etc.

## List Connectors

* Command `list-connectors`
* Take as input:
  * Name of a connector
  * ISE FQDN or IP address
  * Username
  * Password
* List all connectors, output as a prettified JSON array of strings

## Dump Connector Configuration

* Command `dump-connector`
* Take as input:
  * Name of a connector
  * ISE FQDN or IP address
  * Username
  * Password
* Provide options to:
  * Dump out the config as prettified JSON (default)
  * Dump out config in same YAML format as accepted by `create-push-connector`

## New Push Connector Creation

* Command `create-push-connector`
* Take as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * A YAML file with the parameters to create a new pxGrid Direct **push connector**
* Define a sample YAML file for creating a new pxGrid Direct push connector based on the OpenAPI schema in `pxgrid-direct.yaml`.
* Implementatiopn notes:
  * Flexible URL section MUST have no content

## Delete Connector

* Command `delete-connector`
* Take as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * Name of a push or pull connector
* Delete the named connector

## Test Data Generation

* Take as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * The name of a pxGrid Direct connector
  * The number of random data elements to create
  * The name of the output file to create in JSON format
* Retrieve connector config using API
* Connector config will have a JSON path for attributes that includes a `$.`. The `$.` must not be included in the sample data generated.
* Generate sample data:
  * Correlation Ids MUST be unique MAC addresses that are in the "locally administered" (LA) range
  * Unique identifier MUST be a UUID4
  * Version identifier MUST be a UTC timestamp in a standard ISO format
  * All other fields MUST be populated with random ASCII strings of random lebgth between 8 and 64 characters in length

## Delete And Recreate A Named Connector

* Create a command `recycle-connector`
* Take as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * The name of a pxGrid Direct connector
  * Option parameter `--copy` that takes the name to copy the source connector to
* Retrieve the named connector config, including credentials.
* Extract the FULL object with JSON path `$.response.connector`.
* If the `--copy` parameter IS provided, create a copy of the original named connector
  * Ensure that any schedule parameters are updated to be at least 30 minutes in the future
  * Use the connector config extracted above with the JSON payload setting extracted JSON object to the "connector" attribute in the POST
* If the `--copy` parameter IS NOT provided:
  * Attempt to delete the named connector
  * If deletion succeeds, fully recreate the connector
    * Ensure that any schedule parameters are updated to be at least 30 minutes in the future
    * Use the connector config extracted above with the JSON payload setting extracted JSON object to the "connector" attribute in the POST
* If connector creation fails with the error "API request failed with status 400: When a connector with huge number of endpoints is deleted using the Delete option, even though the connector is deleted from the GUI immediately, it might take few more seconds to delete all the endpoints associated with that connector. You will not be able to create a new connector with the same name before all the associated endpoints are deleted.", then retry the creation after 5 seconds, then 10 seconds, etc., until user hits CTRL+C.
* As a final step, if it is a PULL connector, trigger a sync-now
* If the creation of the new connector fails, display the URL POST'd to and JSON used in the POST in prettified format

## Load Data Via Push Connector

* Create a command `load-data`
* `load-data` takes as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * The name of a pxGrid Direct PUSH connector
  * Name of test data input file
  * **Optional** number of objects to create or update per call to `/api/v1/pxgrid-direct/push/{ConnectorName}/bulk` API. If not specified, keep requests <= 5MB for payload size.
  * Float specifying seconds to back off for if 429 received:
    * Default value 0.5 seconds.
    * Minimum value 0.001 seconds.
    * Maximum value 120 seconds.
  * Add an option to deliberately create a bad request by leaving the value identified as the correlation ID empty even if it is populated in the test data file.
* Test data file will be a JSON document with the following contents:
  * Top-level array property
  * Array containing list of JSON objects to load into ISE using the `/api/v1/pxgrid-direct/push/{ConnectorName}/bulk` API
* Submit requests to bulk API in serial.
* Honor error 429 and retry per backoff parameters
* After run, report how long each batch of objects took to load
  * If application is terminated via SIGINT, report statistics so far.
  * Ensure integrity of report display by flushing STDERR before displaying report.
  * Do not display report progressively, hold all reporting until either all work is completed or the user hits CTRL+C
* In verbose logging mode please display full HTTP request being sent as well as response. For larger payloads ensure the text is split prettified across multiple log lines.

## Load Data Using PUT Via Push Connector

* Create a command `load-data-put`
* `load-data-put` takes as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * The name of a pxGrid Direct PUSH connector
  * Optional JSON property name to use for `{uniqueId}`
  * Name of test data input file
  * Concurrency factor, determining how many requests can be submitted in parallel using separate http connections (default:10)
  * Float specifying inter-object delay:
    * Default 0.0 seconds.
    * Minimum value 0.0 seconds.
    * Maximum value 5.0 seconds.
  * Float specifying seconds to back off for if 429 received:
    * Default value 0.5 seconds.
    * Minimum value 0.001 seconds.
    * Maximum value 120 seconds.
* Test data file will be a JSON document with the following contents:
  * Top-level array property
  * Array containing list of JSON objects to load into ISE using a PUT operation against `/api/v1/pxgrid-direct/push/{ConnectorName}/{uniqueId}`
* Each JSON object in the array MUST be PUT against the URL `/api/v1/pxgrid-direct/push/{ConnectorName}/{uniqueId}`, where the `{uniqueId}` is extracted from the JSON object using the property name configured.
* Honor error 429 and retry per backoff parameters
* After run, report how long each batch of objects took to load
  * If application is terminated via SIGINT, report statistics so far.
  * Ensure integrity of report display by flushing STDERR before displaying report.
  * Do not display report progressively, hold all reporting until either all work is completed or the user hits CTRL+C
* In verbose logging mode please display full HTTP request being sent as well as response.

## Delete All Objects Held By A Named Push Connector

* Create a command `delete-data`
* `delete-data` takes as input:
  * ISE FQDN or IP address
  * Username
  * Password
  * The name of a pxGrid Direct PUSH connector
  * **Optional** number of objects to delete per call to `/api/v1/pxgrid-direct/push/{ConnectorName}/bulk` API. If not specified, keep requests <= 5MB for payload size.
  * Float specifying seconds to back off for if 429 received:
    * Default value 0.5 seconds.
    * Minimum value 0.001 seconds.
    * Maximum value 120 seconds.
* Retrieve data on all objects held by a named push connector
  * GET page size for retrieving details of all objects MUST be set to 1000
  * Ensure that only the unique id is kept in memory as the rest of the object details are not needed for the bulk delete operation
* Use unique identifiers retrieved to create bulk delete operations
* Submit delete requests to bulk API in serial.
* Each delete request MUST have a payload <= 5MB in size
* Honor error 429 and retry per backoff parameters
* After run, report how long each batch of objects took to delete
  * If application is terminated via SIGINT, report statistics so far.
  * Ensure integrity of report display by flushing STDERR before displaying report.
  * Do not display report progressively, hold all reporting until either all work is completed or the user hits CTRL+C

# startup
Contains startup related structs, helpers, and can be used to handle the startup and shutdown of a service.

###### Install
`go get github.com/blutspende/libs-startup`

## Startup
It is a fully configurable drop-in replacement for most of the boilerplate code in the `main()` function. It handles .env and configuration reading, database connection, initializations, and graceful shutdown.

It can be configured using the `startup.Config` struct, and used with the `startup.Startup(cfg)` method. The configuration allows for optional injection of custom initialization and shutdown functions.

## Config
Contains a base `CommonConfiguration` struct that can be embedded in other configuration structs to provide common configuration values, and a `ReadConfiguration` function to read environment variables and do basic common processing.

It also contains a `Configuration` interface that should be implemented by any service specific configuration struct to be usable in `ReadConfiguration` and in various things from this library.

Here is an example of a service specific configuration struct:
```go
import startup "github.com/blutspende/libs-startup"

type Configuration struct {
    startup.CommonConfiguration
	
    ServiceSpecific string `envconfig:"SERVICE_SPECIFIC" required:"true"`
}

func (c *Configuration) GetCommonConfig() *startup.CommonConfiguration {
    return &c.CommonConfiguration
}
```
# listener-master

## A package helps you to create a zero-downtime network service application. This package uses Master-Woker model to manager one or more fd listeners. The master creates or manages listener fds and passes them to the worker.

## Usage
```shell
import "github.com/jqqjj/listener-master"

//create listeners
listeners := master.Listeners(func() []string {
  //Codes here will not be refreshed until the application is fully restarted.
  //Usually we only send the HUP signal to restart the worker without restarting the master so the worker has ablility to run the newest codes.
  //It is recommended to return the listener addresses read from a file.
  return []string{":8080","8081"}
})

//Use with standard http package
http.Serve(listeners[1], nil)

//Use with Gin framework
r := gin.Default()
r.RunListener(listeners[1])
```

## How to restart applications gracefully
```shell
kill -HUP APP_PID
```

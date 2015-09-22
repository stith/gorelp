RELP
====

A Golang library for [RELP](http://www.rsyslog.com/doc/relp.html).


Usage
=====

Server
------

```go
// relp.NewServer(host string, port int, autoAck bool)
// host - Host to listen on 
// port - Port to listen on
// autoAck - Whether or not to acknowledge message as soon as they're recieved
relpServer, err := relp.NewServer("localhost", 3333, true)
if err != nil {
  fmt.Println("Error starting server:", err)
}
fmt.Println("Listening on localhost:3333")

for {
  message := <-relpServer.MessageChannel

  fmt.Println("Got message:", message.Data)
}

// When done:
relpServer.Close()
```

If you want to manage message acknowledgement yourself, just set `autoAck` to
`false` when you create the server, then call `message.Ack()`:

```go
relpServer, err := relp.NewServer("localhost", 3333, false)
for {
  message := <-relpServer.MessageChannel
  doSomeAwesomeProcessing(message)
  err = message.Ack()
}
```

Client
------

```go
// relp.NewClient(host string, port int)
relpClient, err := relp.NewClient("localhost", 3333)
if err != nil {
  fmt.Println("Error starting client:", err)
}

relpClient.SendString("Testing! Hooray!")

customMessage := relp.Message{
  Txn: 99,
  Command: "syslog",
  Data: "Manually constructed messages if you want that for some reason!",
}
relpClient.SendMessage(customMessage)

relpClient.Close()
```

At the moment, messages wait for a corresponding ack before they return. This
behavior will be configurable in the future.

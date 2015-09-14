RELP
----

A Golang library for [RELP](http://www.rsyslog.com/doc/relp.html).

For the moment, only impliments the server side and only emits raw strings,
ignoring info about who sent the message and such.

Usage
-----


```go
relpServer, err := relp.NewServer("localhost", 3333)
if err != nil {
  fmt.Println("Error starting server:", err)
}
fmt.Println("Listening on localhost:3333")

for {
  message := <-relpServer.MessageChannel

  fmt.Println("Got message:", message)
}

// When done:
relpServer.Close()
```

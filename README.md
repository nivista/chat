# Chat
This chat application demonstrates chat using gRPC and golang.
It uses redis as a database.
Generated code is uploaded for convenience.

## Usage
Install [Golang](https://golang.org/doc/install). 

Install with dependencies:
```
go mod download github.com/nivista/chat
```
Start server:
```
go run server/server.go -port 8080
```
Start as many clients as you want:
```
go run client/client.go -server_addr localhost:8080 -username user123 -conversation a
```
Conversation should either be 'a', 'b', or 'c'.
Type in the console to chat!
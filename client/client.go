package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/nivista/chat/.gen/pb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

var (
	serverAddr   = flag.String("server_addr", "localhost:8080", "The server address in the format of host:port")
	username     = flag.String("username", "user1", "Tell others who you are")
	conversation = flag.String("conversation", "a", "a, b, or c. There's no CRUD API for conversations yet")
)

type basicAuth struct {
	username string
}

func (b basicAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	m := make(map[string]string)
	m["username"] = b.username
	return m, nil
}

func (basicAuth) RequireTransportSecurity() bool {
	return false
}

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*serverAddr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(basicAuth{*username}))
	if err != nil {
		log.Fatalf("fail to dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewChatClient(conn)
	withMetadata := metadata.AppendToOutgoingContext(context.Background(), "conversation", *conversation)
	chatClient, err := client.Chat(withMetadata)

	if err != nil {
		panic(err)
	}

	go func() {
		for {
			msg, err := chatClient.Recv()
			if err != nil {
				return
			}

			fmt.Printf(">> %v says: %v", msg.Sender, msg.Text)
		}
	}()

	reader := bufio.NewReader(os.Stdin)
	for {
		var s string
		s, _ = reader.ReadString('\n')

		chatClient.Send(&pb.SendMessage{Text: s})
	}

}

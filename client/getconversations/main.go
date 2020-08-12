package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/nivista/chat/.gen/pb"
	bauth "github.com/nivista/chat/client/bauth"
	"google.golang.org/grpc"
)

var (
	addr   = flag.String("server address", "localhost:8080", "address of server")
	user   = flag.String("user", "default", "user you're acting as")
	convID = flag.String("conv", "", "ID of conversation your adding to")
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	req := pb.GetConversationsRequest{}

	res, err := client.GetConversations(context.Background(), &req)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("OK")

	for {
		data, err := res.Recv()
		if err != nil {
			fmt.Println("err:", err.Error())
			return
		}
		fmt.Println("conversations:")
		for _, val := range data.Headers {
			fmt.Printf("- name: %v, id: %v\n", val.Name, val.Uuid)
		}

	}
}

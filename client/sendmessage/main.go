package main

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/nivista/chat/.gen/pb"
	bauth "github.com/nivista/chat/client/bauth"
	"google.golang.org/grpc"
)

var (
	addr    = flag.String("addr", "localhost:8080", "address of server")
	user    = flag.String("user", "default", "user you're acting as")
	convID  = flag.String("conv", "", "ID of conversation your adding to")
	message = flag.String("message", "", "message you want to send")
)

func main() {
	flag.Parse()

	if len(*convID) == 0 {
		panic("convID required")
	}
	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	req := pb.SendMessageRequest{
		ConvUuid: *convID,
		Text:     *message,
	}

	_, err = client.SendMessage(ctx, &req)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("OK")
}

package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/nivista/chat/.gen/pb"
	bauth "github.com/nivista/chat/client/bauth"
	"google.golang.org/grpc"
)

var (
	addr    = flag.String("server address", "localhost:8080", "address of server")
	user    = flag.String("user", "default", "user you're acting as")
	convID  = flag.String("conv", "", "ID of conversation your adding to")
	members = flag.String("others", "", "'.' separated new users you're adding to conversation")
)

func main() {
	flag.Parse()

	if len(*members) == 0 {
		panic("others required")
	}

	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	membersArr := strings.Split(*members, ".")

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	req := pb.RemoveFromConversationRequest{
		ConvUuid: *convID,
		Users:    membersArr,
	}

	_, err = client.RemoveFromConversation(ctx, &req)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("OK")
}

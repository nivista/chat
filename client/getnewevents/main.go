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
	addr                   = flag.String("addr", "localhost:8080", "address of server")
	user                   = flag.String("user", "default", "user you're acting as")
	convID                 = flag.String("conv", "", "ID of conversation your adding to")
	latestLogicalTimestamp = flag.String("latest", "", "logical timestamp of latest message you have ")
)

func main() {
	flag.Parse()

	if len(*convID) == 0 {
		panic("conv required")
	}

	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	req := pb.GetNewEventsRequest{
		ConvUuid:                    *convID,
		LatestEventLogicalTimestamp: *latestLogicalTimestamp,
	}

	res, err := client.GetNewEvents(context.Background(), &req)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("OK")

	for {
		data, err := res.Recv()
		if err != nil {
			fmt.Println("err:", err)
			return
		}

		oneof := data.Event.EventRequest.EventRequest

		var kind string
		var info string
		var sender = data.Event.Sender

		switch event := oneof.(type) {
		case *pb.EventRequest_AddToConversation:
			kind = "AddToConversation"
			info = fmt.Sprintf("users: %v\nconvID: %v\n", event.AddToConversation.Users, event.AddToConversation.ConvUuid)
		case *pb.EventRequest_RemoveFromConversation:
			kind = "RemoveFromConversation"
			info = fmt.Sprintf("users: %v\nconvID: %v\n", event.RemoveFromConversation.Users, event.RemoveFromConversation.ConvUuid)
		case *pb.EventRequest_CreateConversation:
			kind = "CreateConversation"
			info = fmt.Sprintf("name: %v\nmembers: %v\n", event.CreateConversation.Name, event.CreateConversation.Members)
		case *pb.EventRequest_SendMessage:
			kind = "SendMessage"
			info = fmt.Sprintf("message: %v\nconvID: %v\n", event.SendMessage.Text, event.SendMessage.ConvUuid)
		}

		fmt.Printf(">> %v sent a %v request:\n", sender, kind)
		fmt.Println(info)

	}
}

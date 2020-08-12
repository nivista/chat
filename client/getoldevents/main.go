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
	addr                     = flag.String("server address", "localhost:8080", "address of server")
	user                     = flag.String("user", "default", "user you're acting as")
	convID                   = flag.String("conv", "", "ID of conversation your adding to")
	earliestLogicalTimestamp = flag.String("earliest", "", "earliest logical timestamp of messages that you have")
	count                    = flag.Int("count", 5, "number of messages you want")
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	req := pb.GetOldEventsRequest{
		ConvUuid:                      *convID,
		EarliestEventLogicalTimestamp: *earliestLogicalTimestamp,
		Count:                         int32(*count),
	}

	res, err := client.GetOldEvents(context.Background(), &req)
	if err != nil {
		fmt.Println("err:", err)
		return
	}
	fmt.Println("OK")

	for _, val := range res.Events {
		var kind string
		var info string
		var timestamp = val.LogicalTimestamp
		sender := val.Sender
		switch event := val.EventRequest.EventRequest.(type) {
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
		fmt.Println("timestamp:", timestamp)
		fmt.Println(info)

	}
}

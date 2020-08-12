package main

import (
	"fmt"

	"github.com/nivista/chat/.gen/pb"
	"google.golang.org/protobuf/proto"
)

func main() {
	var in pb.EventRequest = pb.EventRequest{
		EventRequest: &pb.EventRequest_AddToConversation{
			AddToConversation: &pb.AddToConversationRequest{
				ConvUuid: "uuid",
				Users:    []string{"u1", "u2"},
			},
		},
	}
	bytes, err := proto.Marshal(&in)
	fmt.Println(err)

	var eventRequest pb.EventRequest

	err = proto.Unmarshal(bytes, &eventRequest)
	fmt.Println(err)
	fmt.Println(eventRequest.GetEventRequest())
}

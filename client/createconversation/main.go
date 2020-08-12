package main

import (
	"context"
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/nivista/chat/.gen/pb"
	"github.com/nivista/chat/client/bauth"
	"google.golang.org/grpc"
)

var (
	addr     = flag.String("addr", "localhost:8080", "address of server")
	user     = flag.String("user", "default", "user you're acting as")
	convName = flag.String("name", "", "name of conversation you're creating")
	members  = flag.String("others", "", "'.' separated users you're adding to conversation")
)

func main() {
	flag.Parse()

	if *convName == "" {
		panic("name required")
	}

	conn, err := grpc.Dial(*addr, grpc.WithInsecure(), grpc.WithPerRPCCredentials(bauth.BasicAuth{*user}))
	if err != nil {
		panic(err)
	}

	client := pb.NewChatClient(conn)

	var membersArr []string
	if len(*members) == 0 {
		membersArr = []string{}
	} else {
		membersArr = strings.Split(*members, ".")
	}

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	req := pb.CreateConversationRequest{
		Name:    *convName,
		Members: membersArr,
	}

	res, err := client.CreateConversation(ctx, &req)
	if err != nil {
		fmt.Println("err:", err.Error())
		return
	}
	fmt.Printf("ID: %v\n", res.Uuid)
	fmt.Println("OK")
}

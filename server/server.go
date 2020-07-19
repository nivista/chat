package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"sync"

	"github.com/nivista/chat/.gen/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	port = flag.Int("port", 8080, "The server port")
)

type (
	// Conversation stores the data for a conversation, allows people to leave, enter, send and recieve messages.
	Conversation struct {
		messages  []*pb.RecieveMessage
		listeners map[string]chan<- *pb.RecieveMessage
		mux       sync.Mutex
	}

	// Server implements the proto-generated ChatServer interface.
	Server struct{}
)

// This is basically the "database" for developement purposes.
var conversations = make(map[string]*Conversation)

func main() {

	// Parse CLI args
	flag.Parse()

	// Initialize conversations
	conversations["a"] = NewConversation()
	conversations["b"] = NewConversation()
	conversations["c"] = NewConversation()

	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))

	if err != nil {
		panic(err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterChatServer(grpcServer, newServer())
	grpcServer.Serve(lis)
}

// Send persists a message in the DB and broadcasts it to all listeners.
func (c *Conversation) Send(m *pb.SendMessage, user string) {
	c.mux.Lock()
	defer c.mux.Unlock()
	recvMsg := &pb.RecieveMessage{Text: m.Text, Sender: user}
	for _, listener := range c.listeners {
		listener <- recvMsg
	}
	c.messages = append(c.messages, recvMsg)
}

// Listen registers a user as a listener to a conversation, and returns all messages that have already
// happened in the conversation.
func (c *Conversation) Listen(user string) (oldMessages []*pb.RecieveMessage, messages <-chan *pb.RecieveMessage) {
	c.mux.Lock()
	defer c.mux.Unlock()
	ch := make(chan *pb.RecieveMessage)
	messages = ch
	c.listeners[user] = ch
	oldMessages = append(oldMessages, c.messages...)

	return
}

// Unlisten unregisters a user as a listener to a conversation.
func (c *Conversation) Unlisten(user string) {
	c.mux.Lock()
	defer c.mux.Unlock()

	delete(c.listeners, user)
}

// NewConversation returns an initialized conversation.
func NewConversation() *Conversation {
	return &Conversation{messages: make([]*pb.RecieveMessage, 0), listeners: make(map[string]chan<- *pb.RecieveMessage, 0)}
}

// Chat manages a single client in a single conversation.
func (s *Server) Chat(stream pb.Chat_ChatServer) error {
	username, _ := extractHeader(stream.Context(), "username")
	conv, _ := extractHeader(stream.Context(), "conversation")

	// listen to conversation
	oldMessages, messages := conversations[conv].Listen(username)

	// make sure to unlisten when the function exits.
	defer func() {
		conversations[conv].Unlisten(username)
	}()

	// TODO: this blocks, and because we're not consuming messages the conversation lock will be held on to.
	// Send oldMessages to the user
	for _, msg := range oldMessages {
		stream.SendMsg(msg)
	}

	// Get a channel for all the messages being recieved from the client
	messagesFromClient := messageStreamToChannel(stream)

	// TODO: this for loop should capture os termination signals, SIG TERM, SIG INT
	for {
		select {
		case <-stream.Context().Done():
			fmt.Println("Context cancelled...", stream.Context().Err().Error())
			return nil

		case recvMsg := <-messages:
			// "send" a "recieve" message. Naming needs work, but a recieve message is a message from server to client,
			// this sends it from server to client
			stream.SendMsg(recvMsg)

		case sendMsg := <-messagesFromClient:
			// Here we're recieving sendMsgs
			// The client sends the server this message. It's called a send message because its sent to other clients.
			// it has to be done in a goroutine to avoid a deadlock.
			// TODO: find other way around deadlock, for example not sending messages to yourself (which is what this does)
			go conversations[conv].Send(sendMsg, username)
		}
	}
}

func messageStreamToChannel(c pb.Chat_ChatServer) <-chan *pb.SendMessage {
	ch := make(chan *pb.SendMessage)
	go func() {
		for {
			msg, err := c.Recv()
			if err != nil {
				return
			}
			ch <- msg

		}
	}()
	return ch
}

func newServer() *Server {
	return &Server{}
}

// This function extracts the value of a header from a context.
func extractHeader(ctx context.Context, header string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no headers in request")
	}

	authHeaders, ok := md[header]
	if !ok {
		return "", status.Error(codes.Unauthenticated, "no header in request")
	}

	if len(authHeaders) != 1 {
		return "", status.Error(codes.Unauthenticated, "more than 1 header in request")
	}

	return authHeaders[0], nil
}

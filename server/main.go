package main

import (
	"context"
	"flag"
	"fmt"
	"net"

	redis "github.com/go-redis/redis/v8"
	"github.com/nivista/chat/.gen/pb"
	"github.com/nivista/chat/server/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	port = flag.Int("port", 8080, "The server port")
)

func main() {

	// Parse CLI args
	flag.Parse()

	// connect to server
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		panic(fmt.Sprintf("failed to listen: %v", err))
	}

	rdc := redis.NewClient(&redis.Options{Addr: "localhost:6379"})

	grpcServer := grpc.NewServer()
	pb.RegisterChatServer(grpcServer, newServer(rdc))

	grpcServer.Serve(lis)
}

type server struct {
	db db.DB
}

func newServer(rdc *redis.Client) pb.ChatServer {
	return &server{db.NewDB(rdc)}
}

func (s *server) CreateConversation(ctx context.Context, req *pb.CreateConversationRequest) (*pb.CreateConversationResponse, error) {
	user, err := getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return s.db.CreateConversation(ctx, req, user)
}

func (s *server) GetConversations(req *pb.GetConversationsRequest, server pb.Chat_GetConversationsServer) error {
	user, err := getUserFromContext(server.Context())
	if err != nil {
		return err
	}

	ch, err := s.db.GetConversations(server.Context(), req, user)

	if err != nil {
		return err
	}

	for {
		res, more := <-ch
		if !more {
			return nil
		}
		server.Send(res)
	}

}
func (s *server) GetConversationMembers(req *pb.GetConversationMembersRequest, server pb.Chat_GetConversationMembersServer) error {
	ch, err := s.db.GetConversationMembers(server.Context(), req)

	if err != nil {
		return err
	}

	for {
		res, more := <-ch
		if !more {
			return nil
		}
		server.Send(res)
	}
}

func (s *server) GetOldEvents(ctx context.Context, req *pb.GetOldEventsRequest) (*pb.GetOldEventsResponse, error) {
	return s.db.GetOldEvents(ctx, req)
}

func (s *server) GetNewEvents(req *pb.GetNewEventsRequest, server pb.Chat_GetNewEventsServer) error {
	ch, err := s.db.GetNewEvents(server.Context(), req)

	if err != nil {
		return err
	}

	for {
		res, more := <-ch
		if !more {
			return nil
		}
		server.Send(res)
	}
}
func (s *server) SendMessage(ctx context.Context, req *pb.SendMessageRequest) (*pb.SendMessageResponse, error) {
	user, err := getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	err = s.db.SendMessage(ctx, req, user)
	if err != nil {
		return nil, err
	}
	return &pb.SendMessageResponse{}, nil
}

func (s *server) AddToConversation(ctx context.Context, req *pb.AddToConversationRequest) (*pb.AddToConversationResponse, error) {
	user, err := getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	err = s.db.AddToConversation(ctx, req, user)
	if err != nil {
		return nil, err
	}
	return &pb.AddToConversationResponse{}, nil
}

func (s *server) RemoveFromConversation(ctx context.Context, req *pb.RemoveFromConversationRequest) (*pb.RemoveFromConversationResponse, error) {
	user, err := getUserFromContext(ctx)
	if err != nil {
		return nil, err
	}

	err = s.db.RemoveFromConversation(ctx, req, user)
	if err != nil {
		return nil, err
	}
	return &pb.RemoveFromConversationResponse{}, nil
}

func getUserFromContext(ctx context.Context) (string, error) {
	return extractHeader(ctx, "user")
}

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

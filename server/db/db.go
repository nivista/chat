package db

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
	"github.com/nivista/chat/.gen/pb"
	"google.golang.org/protobuf/proto"

	redis "github.com/go-redis/redis/v8"
)

type (
	DB interface {
		CreateConversation(ctx context.Context, req *pb.CreateConversationRequest, user string) (*pb.CreateConversationResponse, error)
		GetConversations(ctx context.Context, req *pb.GetConversationsRequest, user string) (<-chan *pb.GetConversationsResponse, error)
		GetConversationMembers(ctx context.Context, req *pb.GetConversationMembersRequest) (<-chan *pb.GetConversationMembersResponse, error)
		SendMessage(ctx context.Context, req *pb.SendMessageRequest, user string) error
		AddToConversation(ctx context.Context, req *pb.AddToConversationRequest, user string) error
		RemoveFromConversation(ctx context.Context, req *pb.RemoveFromConversationRequest, user string) error
		GetOldEvents(ctx context.Context, req *pb.GetOldEventsRequest) (*pb.GetOldEventsResponse, error)
		GetNewEvents(ctx context.Context, req *pb.GetNewEventsRequest) (<-chan *pb.GetNewEventsResponse, error)
	}

	db struct {
		rdc           *redis.Client
		conversations map[string]conversation
	}

	conversation struct {
		lastBroadcastedEventID string
		cancelFn               func()

		eventSubscribers      map[*int]chan<- *pb.GetNewEventsResponse
		membershipSubscribers map[*int]chan<- *pb.GetConversationMembersResponse

		eventLock      sync.Mutex
		membershipLock sync.Mutex
	}
)

func NewDB(rdc *redis.Client) DB {
	return &db{rdc, make(map[string]conversation)}
}

// CreateConversation creates a conversation
func (db *db) CreateConversation(ctx context.Context, req *pb.CreateConversationRequest, user string) (*pb.CreateConversationResponse, error) {
	// TODO validation

	// make create conversation event
	createConversationEvent := pb.EventRequest{
		EventRequest: &pb.EventRequest_CreateConversation{req},
	}
	bytes, err := proto.Marshal(&createConversationEvent)
	if err != nil {
		return nil, err
	}

	// make uuid
	uuidBytes, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	uuidString := uuidBytes.String()

	pipelinerFunc := func(pipe redis.Pipeliner) error {
		// push create conversation event onto stream
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: chatStreamName(uuidString),
			Values: map[string]interface{}{"sender": user, "data": bytes},
		})

		// set name
		pipe.HSet(ctx, convMetadataName(uuidString), "name", req.Name)

		members := append(req.Members, user)

		// initialize membership
		pipe.SAdd(ctx, convUsersSetName(uuidString), members)

		// intialize membership and notify users of new conversation
		for _, member := range members {
			pipe.SAdd(ctx, userConvsSetName(member), uuidString)
			pipe.Publish(ctx, userConvPubSubName(member), uuidString)
		}

		return nil
	}

	_, err = db.rdc.TxPipelined(ctx, pipelinerFunc)
	if err != nil {
		return nil, err
	}

	return &pb.CreateConversationResponse{
		Uuid: uuidString,
	}, nil
}

// GetConversations returns a channel that must be consumed until it closes.
func (db *db) GetConversations(ctx context.Context, req *pb.GetConversationsRequest, user string) (<-chan *pb.GetConversationsResponse, error) {
	pubSubCh := db.rdc.Subscribe(ctx, userConvPubSubName(user)).Channel()

	getConversationMembership := func() (*pb.GetConversationsResponse, error) {
		convs, err := db.rdc.SMembers(ctx, userConvsSetName(user)).Result()
		if err != nil {
			return nil, err
		}

		headers := make([]*pb.GetConversationsResponse_ConversationHeader, len(convs))
		// TODO : this could all be pipelined
		for idx, id := range convs {
			name, err := db.rdc.HGet(ctx, convMetadataName(id), "name").Result()
			if err != nil {
				// TODO: now i have to panic, if this was in a tx I could just fail and return an error
				panic(err)
			}
			headers[idx] = &pb.GetConversationsResponse_ConversationHeader{
				Name: name,
				Uuid: id,
			}
		}

		return &pb.GetConversationsResponse{
			Headers: headers,
		}, nil
	}

	res, err := getConversationMembership()

	if err != nil {
		return nil, err
	}

	out := make(chan *pb.GetConversationsResponse, 1)
	out <- res

	go func() {
		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			case <-pubSubCh:
				res, err := getConversationMembership()

				if err != nil {
					panic(err)
				}

				out <- res
			}
		}
	}()

	return out, nil
}

// GetConversationMembers returns a channel that must be consumed until it closes.
func (db *db) GetConversationMembers(ctx context.Context, req *pb.GetConversationMembersRequest) (<-chan *pb.GetConversationMembersResponse, error) {
	// validate

	convID := req.ConvUuid

	conv := db.getConversation(convID)

	conv.membershipLock.Lock()
	defer conv.membershipLock.Unlock()

	members, err := db.rdc.SMembers(ctx, convUsersSetName(convID)).Result()

	if err != nil {
		return nil, err
	}

	resMembers := make([]*pb.GetConversationMembersResponse_ConversationMember, len(members))
	for idx, member := range members {
		resMembers[idx] = &pb.GetConversationMembersResponse_ConversationMember{
			Name: member,
		}
	}

	resConv := pb.GetConversationMembersResponse{
		Members: resMembers,
	}

	out := make(chan *pb.GetConversationMembersResponse, 1)
	out <- &resConv

	var key int
	conv.membershipSubscribers[&key] = out

	cancel := func() {

		conv.membershipLock.Lock()
		defer conv.membershipLock.Unlock()

		close(conv.membershipSubscribers[&key])
		delete(conv.membershipSubscribers, &key)
	}

	go func() {
		<-ctx.Done()
		cancel()
	}()

	return out, nil
}

func (db *db) SendMessage(ctx context.Context, req *pb.SendMessageRequest, user string) error {

	convID := req.ConvUuid

	bytes, err := proto.Marshal(&pb.EventRequest{
		EventRequest: &pb.EventRequest_SendMessage{req},
	})

	if err != nil {
		return err
	}

	_, err = db.rdc.XAdd(ctx, &redis.XAddArgs{
		Stream: chatStreamName(convID),
		Values: map[string]interface{}{"sender": user, "data": bytes},
	}).Result()

	return err
}

func (db *db) AddToConversation(ctx context.Context, req *pb.AddToConversationRequest, user string) error {
	// in a transaction, send a message and update sets
	convID := req.ConvUuid

	bytes, err := proto.Marshal(&pb.EventRequest{
		EventRequest: &pb.EventRequest_AddToConversation{req},
	})
	if err != nil {
		return err
	}

	pipeFn := func(pipe redis.Pipeliner) error {

		// update set of users in conversation
		err = pipe.SAdd(ctx, convUsersSetName(convID), req.Users).Err()
		if err != nil {
			return err
		}
		// add to event to stream
		err = pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: chatStreamName(convID),
			Values: map[string]interface{}{"sender": user, "data": bytes},
		}).Err()

		if err != nil {
			return err
		}

		// update users membership view
		for _, user := range req.Users {
			err = pipe.SAdd(ctx, userConvsSetName(user), convID).Err()
			if err != nil {
				return err
			}
			err = pipe.Publish(ctx, userConvPubSubName(user), convID).Err()
			if err != nil {
				return err
			}
		}

		return nil
	}

	_, err = db.rdc.TxPipelined(ctx, pipeFn)
	return err
}

func (db *db) RemoveFromConversation(ctx context.Context, req *pb.RemoveFromConversationRequest, user string) error {
	// in a transaction, send a message and update sets
	convID := req.ConvUuid

	bytes, err := proto.Marshal(&pb.EventRequest{
		EventRequest: &pb.EventRequest_RemoveFromConversation{req},
	})
	if err != nil {
		return err
	}

	pipeFn := func(pipe redis.Pipeliner) error {

		// update set of users in conversation
		err = pipe.SRem(ctx, convUsersSetName(convID), req.Users).Err()
		if err != nil {
			return err
		}

		// add to event to stream
		err = pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: chatStreamName(convID),
			Values: map[string]interface{}{"sender": user, "data": bytes},
		}).Err()

		if err != nil {
			return err
		}

		// update users membership view
		for _, user := range req.Users {
			err = pipe.SRem(ctx, userConvsSetName(user), convID).Err()
			if err != nil {
				return err
			}
			err = pipe.Publish(ctx, userConvPubSubName(user), convID).Err()
			if err != nil {
				return err
			}
		}

		return nil
	}

	_, err = db.rdc.TxPipelined(ctx, pipeFn)
	return err

}

func (db *db) GetOldEvents(ctx context.Context, req *pb.GetOldEventsRequest) (*pb.GetOldEventsResponse, error) {
	convID := req.ConvUuid

	var start string
	if req.EarliestEventLogicalTimestamp == "" {
		start = "+"
	} else {
		start = req.EarliestEventLogicalTimestamp
	}
	stop := "-"

	count := int64(req.Count) + 1
	val, err := db.rdc.XRevRangeN(ctx, chatStreamName(convID), start, stop, count).Result()

	if err != nil {
		return nil, err
	}

	chatEvents, err := xMsgToChatEvents(val)
	if err != nil {
		return nil, err
	}

	// we don't want to include EarliestEventLogicalTimestamp
	if len(chatEvents) > 0 && chatEvents[0].LogicalTimestamp == req.EarliestEventLogicalTimestamp {
		chatEvents = chatEvents[1:]
	}

	return &pb.GetOldEventsResponse{
		Events: chatEvents,
	}, nil
}

func (db *db) GetNewEvents(ctx context.Context, req *pb.GetNewEventsRequest) (<-chan *pb.GetNewEventsResponse, error) {
	convID := req.ConvUuid

	conv := db.getConversation(convID)

	conv.eventLock.Lock()

	var start string
	if req.LatestEventLogicalTimestamp == "" {
		start = "-"
	} else {
		start = req.LatestEventLogicalTimestamp
	}

	var stop string
	if conv.lastBroadcastedEventID == "" {
		stop = "+"
	} else {
		stop = conv.lastBroadcastedEventID
	}

	val, err := db.rdc.XRange(ctx, chatStreamName(convID), start, stop).Result()

	if err != nil {
		return nil, err
	}

	if len(val) > 0 {
		val = val[1:]
	}

	out := make(chan *pb.GetNewEventsResponse)

	chatEvents, err := xMsgToChatEvents(val)

	if err != nil {
		return nil, err
	}

	var key int

	conv.eventSubscribers[&key] = out

	cancel := func() {
		conv.eventLock.Lock()
		defer conv.eventLock.Unlock()

		close(conv.eventSubscribers[&key])
		delete(conv.eventSubscribers, &key)
	}

	go func() {
		for _, event := range chatEvents {

			out <- &pb.GetNewEventsResponse{
				Event: event,
			}
		}

		conv.eventLock.Unlock()

		<-ctx.Done()
		cancel()
	}()

	return out, nil
}

// gets conversation if it exists, otherwise creates it and returns it
func (db *db) getConversation(convID string) *conversation {
	if conv, ok := db.conversations[convID]; ok {
		return &conv
	}

	ctx, cancel := context.WithCancel(context.Background())
	conv := conversation{
		eventSubscribers:      make(map[*int]chan<- *pb.GetNewEventsResponse),
		membershipSubscribers: make(map[*int]chan<- *pb.GetConversationMembersResponse),
		cancelFn:              cancel,
	}

	go func() {
		for {

			var lastEventID string
			if conv.lastBroadcastedEventID == "" {
				lastEventID = "$"
			} else {
				lastEventID = conv.lastBroadcastedEventID
			}

			val, err := db.rdc.XRead(ctx, &redis.XReadArgs{
				Streams: []string{chatStreamName(convID), lastEventID},
			}).Result()

			if err != nil {
				// this should only happen if the err is context canceled
				// TODO handle this case explicitly, panic in othe case
				fmt.Println("Exiting listening to conversation, err:", err.Error())
				return
			}

			events, err := xMsgToChatEvents(val[0].Messages)
			if err != nil {
				panic(err)
			}
			event := events[0]
			resEvent := pb.GetNewEventsResponse{
				Event: event,
			}

			conv.eventLock.Lock()
			for _, ch := range conv.eventSubscribers {
				ch <- &resEvent
			}
			conv.lastBroadcastedEventID = val[0].Messages[0].ID
			conv.eventLock.Unlock()

			_, isAdd := event.EventRequest.EventRequest.(*pb.EventRequest_AddToConversation)
			_, isRemove := events[0].EventRequest.EventRequest.(*pb.EventRequest_RemoveFromConversation)
			if isAdd || isRemove {

				members, err := db.rdc.SMembers(ctx, convUsersSetName(convID)).Result()

				if err != nil {
					// this should only happen if the err is context canceled
					// TODO handle this case explicitly, panic in othe case
					fmt.Println("Exiting listening getting updated conv members, err:", err.Error())
					return
				}

				resMembers := make([]*pb.GetConversationMembersResponse_ConversationMember, len(members))
				for idx, member := range members {
					resMembers[idx] = &pb.GetConversationMembersResponse_ConversationMember{
						Name: member,
					}
				}

				resConv := pb.GetConversationMembersResponse{
					Members: resMembers,
				}

				conv.membershipLock.Lock()
				for _, ch := range conv.membershipSubscribers {
					ch <- &resConv
				}
				conv.membershipLock.Unlock()

			}
		}
	}()

	return &conv
}

func xMsgToChatEvents(xmsgs []redis.XMessage) ([]*pb.ChatEvent, error) {
	out := make([]*pb.ChatEvent, len(xmsgs))
	for idx, val := range xmsgs {
		id := val.ID
		sender := val.Values["sender"].(string)
		var req pb.EventRequest
		err := proto.Unmarshal([]byte(val.Values["data"].(string)), &req)
		if err != nil {
			return nil, err
		}
		out[idx] = &pb.ChatEvent{
			LogicalTimestamp: id,
			Sender:           sender,
			EventRequest:     &req,
		}
	}
	return out, nil
}

func chatStreamName(convUUID string) string {
	return fmt.Sprintf("<%v>chatstream", convUUID)
}

func convUsersSetName(convUUID string) string {
	return fmt.Sprintf("<%v>users", convUUID)

}

func convMetadataName(convUUID string) string {
	return fmt.Sprintf("<%v>metadata", convUUID)
}

func userConvsSetName(user string) string {
	return fmt.Sprintf("<%v>conversations", user)
}

func userConvPubSubName(user string) string {
	return fmt.Sprintf("<%v>convpubsub", user)
}

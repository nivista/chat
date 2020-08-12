package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	redis "github.com/go-redis/redis/v8"
)

const maxTrys = 10

type (
	DB struct {
		rdc           *redis.Client
		conversations map[string]dbConversation
	}

	dbConversation struct {
		subscribers map[string]chan<- map[string]interface{}
		messages    []map[string]interface{}
	}
)

// all writes will follow a similar pattern
// createconversation
// sendmessage
// addtoconversation
// removefromconversation
// they will all make a conversation event, atomically do their business

// our internal representation of the stream can contain chat events

// make chatevent

//

func (db *DB) SendMessage(ctx context.Context, conversation, user, text string) error {
	conversationUsers := fmt.Sprint("<", conversation, ">users")
	conversationChatStream := fmt.Sprint("<", conversation, ">chatstream")

	var txErr string
	tryAgain := true
	attempts := 0
	for ; attempts < maxTrys && tryAgain; attempts++ {
		tryAgain = false
		db.rdc.Watch(ctx, func(tx *redis.Tx) error {
			isMember, err := tx.SIsMember(tx.Context(), conversationUsers, user).Result()
			if err != nil {
				txErr = err.Error()
				return err
			}

			if !isMember {
				txErr = fmt.Sprintf("user '%v' is not a member of convseration '%v'", user, conversation)
				return nil
			}

			_, err = tx.TxPipelined(tx.Context(), func(pipe redis.Pipeliner) error {
				err := pipe.XAdd(tx.Context(), &redis.XAddArgs{
					Stream: conversationChatStream,
					Values: map[string]interface{}{"type": "message", "user": user, "text": text},
				}).Err()

				return err
			})

			if err != nil {
				tryAgain = true
				return err
			}

			return nil

		}, conversationUsers)
	}

	if txErr != "" {
		return errors.New(txErr)
	}

	if attempts == maxTrys {
		return errors.New("max tries reached")
	}

	return nil
}

func (db *DB) AddToConversation(ctx context.Context, conversation string, users ...string) error {
	conversationUsers := fmt.Sprint("<", conversation, ">users")
	conversationChatStream := fmt.Sprint("<", conversation, ">chatstream")

	_, err := db.rdc.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		err := pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: conversationChatStream,
			Values: map[string]interface{}{"type": "addtoconversation", "users": strings.Join(users, "_")}},
		).Err()

		if err != nil {
			return err
		}

		err = pipe.SAdd(ctx, conversationUsers, users).Err()

		if err != nil {
			return err
		}

		for _, user := range users {
			userConversations := fmt.Sprint("<", user, ">conversations")
			err = pipe.SAdd(ctx, userConversations, conversation).Err()

			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func (db *DB) RemoveFromConversation(ctx context.Context, conversation string, users ...string) error {
	conversationUsers := fmt.Sprint("<", conversation, ">users")
	conversationChatStream := fmt.Sprint("<", conversation, ">chatstream")

	_, err := db.rdc.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		err := pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: conversationChatStream,
			Values: map[string]interface{}{"type": "addtoconversation", "users": strings.Join(users, "_")}},
		).Err()

		if err != nil {
			return err
		}

		err = pipe.SRem(ctx, conversationUsers, users).Err()

		if err != nil {
			return err
		}

		for _, user := range users {
			userConversations := fmt.Sprint("<", user, ">conversations")
			err = pipe.SRem(ctx, userConversations, conversation).Err()

			if err != nil {
				return err
			}
		}

		return nil
	})

	return err
}

func (db *DB) GetMessages(ctx context.Context, conversation, user, earliestEventID string, count uint) ([]map[string]interface{}, error) {
	conversationChatStream := fmt.Sprint("<", conversation, ">chatstream")
	splitEarliestEventID := strings.Split(earliestEventID, "-")

	if len(splitEarliestEventID) != 2 {
		return nil, errors.New("invalid earliestEventID")
	}

	v1, err := strconv.Atoi(splitEarliestEventID[0])
	if err != nil {
		return nil, errors.New("invalid earliestEventID")
	}

	v2, err := strconv.Atoi(splitEarliestEventID[1])
	if err != nil {
		return nil, errors.New("invalid earliestEventID")
	}

	if v1 == 0 {
		v1 = 2 << 31
		v2--
	} else {
		v1--
	}

	end := fmt.Sprint("%v-%v", v1, v2)
	val, err := db.rdc.XRevRangeN(ctx, conversationChatStream, end, "-", int64(count)).Result()

	out := make([]map[string]interface{}, 0, len(val))
	for _, msg := range val {
		out = append(out, msg.Values)
	}

	return out, nil
}

func (db *DB) Subscribe(ctx context.Context, conversation, user, lastEventID string) (<-chan map[string]interface{}, error) {
	// TODO: check if you are part of the conversation
	// TODO: locking ???
	// TODO: get all messages after x id
	ch := make(chan map[string]interface{}) // buffer?

	var conv dbConversation
	var ok bool
	if conv, ok = db.conversations[conversation]; !ok {
		conv = dbConversation{
			subscribers: map[string]chan<- map[string]interface{}{user: ch},
			messages:    make([]map[string]interface{}, 0),
		}
		db.conversations[conversation] = conv

		if lastEventID == "" {
			lastEventID = "0"
		}

		go func() {
			// TODO: if it is a remove from conversation message, have to Unsubscribe them

			conversationChatStream := fmt.Sprint("<", conversation, ">chatstream")

			val, err := db.rdc.XRead(ctx, &redis.XReadArgs{
				Streams: []string{conversationChatStream, lastEventID},
			}).Result()

			if err != nil {
				panic(err)
			}

			redisMessages := val[0].Messages
			conv.messages = make([]map[string]interface{}, 0, len(val))
			for _, msg := range redisMessages {
				conv.messages = append(conv.messages, msg.Values)
			}

			lastEventID = redisMessages[len(redisMessages)-1].ID
			// read new messages
			for len(conv.subscribers) != 0 {
				val, err := db.rdc.XRead(ctx, &redis.XReadArgs{
					Streams: []string{conversationChatStream, lastEventID},
				}).Result()
				if err != nil {
					panic(err)
				}

				conv.messages = append(conv.messages, val[0].Messages[0].Values)
				for _, ch := range conv.subscribers {
					ch <- val[0].Messages[0].Values
				}

				lastEventID = val[0].Messages[0].ID
			}

			// TODO: fix this is not safe concurrently
			delete(db.conversations, conversation)
		}()
	} else {
		go func() {
			// TODO: LOCK
			for _, msg := range conv.messages {
				ch <- msg
			}

			conv.subscribers[user] = ch
			// TODO: UNLOCK

		}()
	}
	return ch, nil
}

func (db *DB) Unsubscribe(conversation, user string) error {
	// TODO: locking
	var conv dbConversation
	var ok bool
	if conv, ok = db.conversations[conversation]; !ok {
		return fmt.Errorf("Conversation '%v' does not exist", conversation)
	}

	if _, ok := conv.subscribers[user]; !ok {
		return fmt.Errorf("User '%v' is not subscribed to conversation '%v'", user, conversation)
	}
	close(conv.subscribers[user])
	delete(conv.subscribers, user)
	return nil
}
func New(rdc *redis.Client) *DB {
	return &DB{rdc, make(map[string]dbConversation)}
}

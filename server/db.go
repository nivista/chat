package main
/*
import (
	"context"
	"fmt"
	"sync"
)

type DB interface {
	CreateConversation(ctx context.Context, name string, members ...string) error
	AddToConversation(ctx context.Context, name string, members ...string) error
	RemoveFromConversation(ctx context.Context, name string, members ...string) error
	GetConversations(ctx context.Context, name string) ([]string, error)
}

type mockDBConversation struct {
	members map[string]struct{}
}

type mockDB struct {
	conversations map[string]mockDBConversation
	mux           sync.Mutex
}

func (db *mockDB) CreateConversation(ctx context.Context, name string, members ...string) error {
	db.mux.Lock()
	defer db.mux.Unlock()

	if _, ok := db.conversations[name]; ok {
		return fmt.Errorf("Conversation w/ name '%v' already exists.", name)
	}

	membersMap := make(map[string]struct{})
	for _, m := range members {
		membersMap[m] = struct{}{}
	}

	db.conversations[name] = mockDBConversation{membersMap}
	return nil
}

func (db *mockDB) AddToConversation(ctx context.Context, name string, members ...string) error {
	db.mux.Lock()
	defer db.mux.Unlock()

	if _, ok := db.conversations[name]; !ok {
		return fmt.Errorf("Conversation '%v' does not exist", name)
	}

	for _, m := range members {
		if _, ok := db.conversations[name].members[m]; ok {
			return fmt.Errorf("'m' is already in this conversation", m)
		}
	}

	for _, m := range members {
		db.conversations[name].members[m] = struct{}{}
	}

	return nil
}

func (db *mockDB) RemoveFromConversation(ctx context.Context, name string, members ...string) error {
	db.mux.Lock()
	defer db.mux.Unlock()

	if _, ok := db.conversations[name]; !ok {
		return fmt.Errorf("Conversation '%v' does not exist", name)
	}

	for _, m := range members {
		if _, ok := db.conversations[name].members[m]; !ok {
			return fmt.Errorf("'m' is not in this conversation", m)
		}
	}

	for _, m := range members {
		delete(db.conversations[name].members, m)
	}

	return nil
}

func (db *mockDB) GetConversations(ctx context.Context, member string) ([]string, error) {
	db.mux.Lock()
	defer db.mux.Unlock()

	out := make([]string, 0)
	for name, conv := range db.conversations {
		if _, ok := conv.members[member]; ok {
			out = append(out, name)
		}
	}

	return out, nil
}

func GetMockDB() DB {
	return &mockDB{conversations: make(map[string]mockDBConversation)}
}

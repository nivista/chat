package bauth

import "context"

type BasicAuth struct {
	User string
}

func (b BasicAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	m := make(map[string]string)
	m["user"] = b.User
	return m, nil
}

func (BasicAuth) RequireTransportSecurity() bool {
	return false
}

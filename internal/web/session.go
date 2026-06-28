package web

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// sessionStore holds valid login session tokens in memory. The product is
// single-user and single-process, so an in-memory set is sufficient and avoids
// pulling in a database purely for sessions.
type sessionStore struct {
	mu     sync.Mutex
	tokens map[string]time.Time
	ttl    time.Duration
}

// newSessionStore creates a session store whose tokens expire after ttl.
func newSessionStore(ttl time.Duration) *sessionStore {

	var store sessionStore

	store.tokens = make(map[string]time.Time)
	store.ttl = ttl

	return &store
}

// issue creates a new opaque session token and records its expiry.
func (store *sessionStore) issue() (string, error) {

	var raw []byte = make([]byte, 24)

	var _, err = rand.Read(raw)
	if nil != err {
		return "", err
	}

	var token string = "sid_" + hex.EncodeToString(raw)

	store.mu.Lock()
	defer store.mu.Unlock()

	store.tokens[token] = time.Now().Add(store.ttl)

	return token, nil
}

// valid reports whether a token exists and has not expired, pruning if expired.
func (store *sessionStore) valid(token string) bool {

	if "" == token {
		return false
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	var expiry time.Time
	var ok bool

	expiry, ok = store.tokens[token]
	if false == ok {
		return false
	}

	if time.Now().After(expiry) {
		delete(store.tokens, token)
		return false
	}

	return true
}

// revoke removes a token so its cookie can no longer authenticate.
func (store *sessionStore) revoke(token string) {

	store.mu.Lock()
	defer store.mu.Unlock()

	delete(store.tokens, token)
}

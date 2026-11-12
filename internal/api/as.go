package api

import (
	"context"
	"net/http"

	"github.com/WiFiGUY512/aeolus/internal/access"
)

// asKey is the context key of a request the manager makes of its own API,
// in its own process, as one of its accounts (0205). Only code in the
// manager can put it there: no request from outside carries it.
type asKey struct{}

// As is h, the API's handler, as the account actor: for the manager's own
// use in its process, as the advisor reads it (0205). There is no token to
// keep or leak. The account's own roles say what it may do, as for anyone,
// and it must be active.
func As(h http.Handler, actor access.AccountID) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), asKey{}, actor)))
	})
}

// asAccount is the account a request the manager made of itself is made
// as, if it is one.
func asAccount(r *http.Request) (access.AccountID, bool) {
	who, ok := r.Context().Value(asKey{}).(access.AccountID)
	return who, ok
}

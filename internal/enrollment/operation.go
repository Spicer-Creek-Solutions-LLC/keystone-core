package enrollment

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"go.keystone-core.io/keystone-core/internal/operator"
)

// OperationCreate is the operator socket's first operation (C05.md § 3.2).
const OperationCreate = "enroll.create"

// CreateRequest is the operation's request frame.
type CreateRequest struct {
	Op         string `json:"op"`
	AgentName  string `json:"agent_name"`
	TTLSeconds int64  `json:"ttl_seconds,omitempty"`
}

// CreateResponse is its response frame.
type CreateResponse struct {
	Bundle Bundle `json:"bundle"`
}

// CreateHandler answers enroll.create. A request it cannot parse, or whose
// name or lifetime is refused, is invalid_request. A failure to issue -- the
// store, or randomness -- closes the connection unanswered: the token was not
// recorded, and there is nothing true to tell the operator but that.
func (i *Issuer) CreateHandler() operator.Handler {
	return func(ctx context.Context, actor operator.Actor, body []byte) (any, string) {
		var req CreateRequest
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		if err := d.Decode(&req); err != nil || req.Op != OperationCreate {
			return nil, operator.ErrInvalidRequest
		}
		if req.TTLSeconds < 0 || req.TTLSeconds > int64(MaxTokenTTL/time.Second) {
			return nil, operator.ErrInvalidRequest
		}
		b, err := i.Issue(ctx, Actor{UID: actor.UID, UsernameSnapshot: actor.UsernameSnapshot},
			req.AgentName, time.Duration(req.TTLSeconds)*time.Second)
		switch {
		case errors.Is(err, ErrInvalidRequest):
			return nil, operator.ErrInvalidRequest
		case err != nil:
			log.Printf("%s: token not issued: %v", OperationCreate, err)
			return nil, ""
		}
		return CreateResponse{Bundle: b}, ""
	}
}

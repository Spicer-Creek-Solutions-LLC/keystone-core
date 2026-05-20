package outbound

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// ErrSubscriptionNotFound is returned by [SubscriptionStore.Update]
// / Delete when the id is unknown.
var ErrSubscriptionNotFound = errors.New("outbound: subscription not found")

// SubscriptionStore persists outbound webhook [Subscription]s and the
// per-attempt [DeliveryRecord] audit. The §4.14 spec names this one
// interface across both tables — naming kept literal; delivery
// methods documented here. Implementations: in-memory [MemoryStore]
// (the dark-until-boot default symmetric with the rollback /
// verification stores) and durable [SQLiteStore] (task 11).
//
// Get / List return (nil,false,nil) on miss — the gitops-domain store
// convention. UpdateSubscription / DeleteSubscription return
// [ErrSubscriptionNotFound] when the id is unknown (the mutating
// callers want to distinguish that case).
type SubscriptionStore interface {
	// subscriptions
	CreateSubscription(ctx context.Context, s *Subscription) error
	GetSubscription(ctx context.Context, id string) (*Subscription, bool, error)
	ListSubscriptions(ctx context.Context) ([]*Subscription, error)
	UpdateSubscription(ctx context.Context, s *Subscription) error
	DeleteSubscription(ctx context.Context, id string) error

	// deliveries (upsert on Save — same id across attempts)
	SaveDelivery(ctx context.Context, d *DeliveryRecord) error
	GetDelivery(ctx context.Context, id string) (*DeliveryRecord, bool, error)
	// ListDeliveries returns deliveries for subscriptionID
	// (empty = all). limit > 0 caps the count; <= 0 = unlimited.
	ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]*DeliveryRecord, error)
	// DeleteOldDeliveries prunes deliveries with delivered_at <
	// before. §4.14 retention enforcer — auto-invocation is the
	// v0.1.x trivial-fix gotcha. Returns the number of rows deleted.
	DeleteOldDeliveries(ctx context.Context, before time.Time) (int, error)
}

// MemoryStore is the in-memory [SubscriptionStore]. Records are
// copied on the way in/out so callers cannot mutate stored state via
// the pointer (the rollback/verification precedent).
type MemoryStore struct {
	mu       sync.RWMutex
	subs     map[string]Subscription
	subSeq   map[string]int
	subN     int
	deliv    map[string]DeliveryRecord
	delivSeq map[string]int
	delivN   int
}

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		subs:     make(map[string]Subscription),
		subSeq:   make(map[string]int),
		deliv:    make(map[string]DeliveryRecord),
		delivSeq: make(map[string]int),
	}
}

func cloneSub(s Subscription) Subscription {
	cp := s
	cp.Events = append([]string(nil), s.Events...)
	if s.Headers != nil {
		cp.Headers = make(map[string]string, len(s.Headers))
		for k, v := range s.Headers {
			cp.Headers[k] = v
		}
	}
	return cp
}

// CreateSubscription implements [SubscriptionStore].
func (s *MemoryStore) CreateSubscription(_ context.Context, sub *Subscription) error {
	if sub == nil || sub.ID == "" {
		return errors.New("outbound: create: nil subscription or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub.ID]; ok {
		return errors.New("outbound: subscription " + sub.ID + " already exists")
	}
	s.subN++
	s.subSeq[sub.ID] = s.subN
	s.subs[sub.ID] = cloneSub(*sub)
	return nil
}

// GetSubscription implements [SubscriptionStore].
func (s *MemoryStore) GetSubscription(_ context.Context, id string) (*Subscription, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sub, ok := s.subs[id]
	if !ok {
		return nil, false, nil
	}
	cp := cloneSub(sub)
	return &cp, true, nil
}

// ListSubscriptions implements [SubscriptionStore] — insertion order.
func (s *MemoryStore) ListSubscriptions(_ context.Context) ([]*Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Subscription, 0, len(s.subs))
	for _, sub := range s.subs {
		cp := cloneSub(sub)
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return s.subSeq[out[i].ID] < s.subSeq[out[j].ID] })
	return out, nil
}

// UpdateSubscription implements [SubscriptionStore]. The PATCH
// semantics (which fields to update) live in the REST handler; this
// store unconditionally overwrites the row.
func (s *MemoryStore) UpdateSubscription(_ context.Context, sub *Subscription) error {
	if sub == nil || sub.ID == "" {
		return errors.New("outbound: update: nil subscription or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[sub.ID]; !ok {
		return ErrSubscriptionNotFound
	}
	s.subs[sub.ID] = cloneSub(*sub)
	return nil
}

// DeleteSubscription implements [SubscriptionStore].
func (s *MemoryStore) DeleteSubscription(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.subs[id]; !ok {
		return ErrSubscriptionNotFound
	}
	delete(s.subs, id)
	delete(s.subSeq, id)
	return nil
}

// SaveDelivery implements [SubscriptionStore] (upsert).
func (s *MemoryStore) SaveDelivery(_ context.Context, d *DeliveryRecord) error {
	if d == nil || d.ID == "" {
		return errors.New("outbound: save delivery: nil record or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.delivSeq[d.ID]; !ok {
		s.delivN++
		s.delivSeq[d.ID] = s.delivN
	}
	s.deliv[d.ID] = *d
	return nil
}

// GetDelivery implements [SubscriptionStore].
func (s *MemoryStore) GetDelivery(_ context.Context, id string) (*DeliveryRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	d, ok := s.deliv[id]
	if !ok {
		return nil, false, nil
	}
	return &d, true, nil
}

// ListDeliveries implements [SubscriptionStore]. Filters by
// subscriptionID when non-empty; orders by insertion sequence; caps
// at limit when > 0.
func (s *MemoryStore) ListDeliveries(_ context.Context, subscriptionID string, limit int) ([]*DeliveryRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*DeliveryRecord, 0, len(s.deliv))
	for _, d := range s.deliv {
		if subscriptionID != "" && d.SubscriptionID != subscriptionID {
			continue
		}
		c := d
		out = append(out, &c)
	}
	sort.Slice(out, func(i, j int) bool { return s.delivSeq[out[i].ID] < s.delivSeq[out[j].ID] })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// DeleteOldDeliveries implements [SubscriptionStore]. before is the
// exclusive upper bound on DeliveredAt — rows at or after stay.
func (s *MemoryStore) DeleteOldDeliveries(_ context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for id, d := range s.deliv {
		if d.DeliveredAt.Before(before) {
			delete(s.deliv, id)
			delete(s.delivSeq, id)
			n++
		}
	}
	return n, nil
}

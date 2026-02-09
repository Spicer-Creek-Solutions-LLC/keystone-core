package netconf

import (
	"context"
	"fmt"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/protocols"
)

// GetConfig retrieves configuration from the specified datastore.
func (a *Adapter) GetConfig(_ context.Context, source string, filter *protocols.NetconfFilter) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return nil, fmt.Errorf("not connected")
	}

	ds := Datastore(source)
	if !ds.Valid() {
		return nil, fmt.Errorf("invalid datastore: %s", source)
	}

	op := &rpcGetConfig{
		Source: datastoreElement(ds),
		Filter: filterElement(toInternalFilter(filter)),
	}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return nil, err
	}
	if reply.Errors.HasError() {
		return nil, reply.Errors
	}
	return reply.Data, nil
}

// EditConfig modifies the specified datastore.
func (a *Adapter) EditConfig(_ context.Context, target string, config []byte, opts *protocols.NetconfEditOptions) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	ds := Datastore(target)
	if !ds.Valid() {
		return fmt.Errorf("invalid datastore: %s", target)
	}

	op := &rpcEditConfig{
		Target: datastoreElement(ds),
		Config: rawXML{Content: string(config)},
	}

	if opts != nil {
		op.DefaultOperation = opts.DefaultOperation
		op.TestOption = opts.TestOption
		op.ErrorOption = opts.ErrorOption
	}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// CopyConfig copies one datastore to another.
func (a *Adapter) CopyConfig(_ context.Context, source, target string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	srcDS := Datastore(source)
	tgtDS := Datastore(target)

	op := &rpcCopyConfig{
		Source: datastoreElement(srcDS),
		Target: datastoreElement(tgtDS),
	}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// DeleteConfig deletes the specified datastore.
func (a *Adapter) DeleteConfig(_ context.Context, target string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	ds := Datastore(target)
	if ds == Running {
		return fmt.Errorf("cannot delete running datastore")
	}

	op := &rpcDeleteConfig{Target: datastoreElement(ds)}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// Lock acquires a lock on the specified datastore.
func (a *Adapter) Lock(_ context.Context, target string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	op := &rpcLock{Target: datastoreElement(Datastore(target))}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// Unlock releases a lock on the specified datastore.
func (a *Adapter) Unlock(_ context.Context, target string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	op := &rpcUnlock{Target: datastoreElement(Datastore(target))}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// Commit commits the candidate configuration to running.
func (a *Adapter) Commit(_ context.Context) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	reply, err := a.session.SendRPC(&rpcCommit{})
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// DiscardChanges discards uncommitted candidate changes.
func (a *Adapter) DiscardChanges(_ context.Context) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	reply, err := a.session.SendRPC(&rpcDiscardChanges{})
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// Validate validates the specified datastore.
func (a *Adapter) Validate(_ context.Context, source string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	op := &rpcValidate{Source: datastoreElement(Datastore(source))}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// Get retrieves running configuration and device state data.
func (a *Adapter) Get(_ context.Context, filter *protocols.NetconfFilter) ([]byte, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return nil, fmt.Errorf("not connected")
	}

	op := &rpcGet{
		Filter: filterElement(toInternalFilter(filter)),
	}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return nil, err
	}
	if reply.Errors.HasError() {
		return nil, reply.Errors
	}
	return reply.Data, nil
}

// ServerCapabilities returns the server's advertised capabilities.
func (a *Adapter) ServerCapabilities() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.session == nil {
		return nil
	}
	caps := a.session.ServerCapabilities()
	result := make([]string, len(caps))
	for i, c := range caps {
		result[i] = string(c)
	}
	return result
}

// SessionID returns the NETCONF session ID.
func (a *Adapter) SessionID() uint32 {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.session == nil {
		return 0
	}
	return a.session.SessionID()
}

// KillSession terminates another NETCONF session.
func (a *Adapter) KillSession(_ context.Context, sessionID uint32) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.connected || a.session == nil {
		return fmt.Errorf("not connected")
	}

	op := &rpcKillSession{SessionID: sessionID}

	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

// executeCommand parses a command string and dispatches to the appropriate operation.
// Format: "operation [args] [body]"
func (a *Adapter) executeCommand(_ context.Context, command string) ([]byte, error) {
	parts := strings.SplitN(strings.TrimSpace(command), " ", 3)
	if len(parts) == 0 || parts[0] == "" {
		return nil, fmt.Errorf("empty command")
	}

	op := strings.ToLower(parts[0])
	switch op {
	case "get-config":
		source := "running"
		var f *Filter
		if len(parts) > 1 {
			source = parts[1]
		}
		if len(parts) > 2 {
			f = &Filter{Type: "subtree", Content: parts[2]}
		}
		return a.doGetConfig(Datastore(source), f)

	case "get":
		var f *Filter
		if len(parts) > 1 {
			f = &Filter{Type: "subtree", Content: strings.Join(parts[1:], " ")}
		}
		return a.doGet(f)

	case "edit-config":
		if len(parts) < 3 {
			return nil, fmt.Errorf("edit-config requires target and config XML")
		}
		return nil, a.doEditConfig(Datastore(parts[1]), []byte(parts[2]), nil)

	case "copy-config":
		if len(parts) < 3 {
			return nil, fmt.Errorf("copy-config requires source and target")
		}
		return nil, a.doCopyConfig(Datastore(parts[1]), Datastore(parts[2]))

	case "delete-config":
		if len(parts) < 2 {
			return nil, fmt.Errorf("delete-config requires target")
		}
		return nil, a.doDeleteConfig(Datastore(parts[1]))

	case "lock":
		if len(parts) < 2 {
			return nil, fmt.Errorf("lock requires target")
		}
		return nil, a.doLock(Datastore(parts[1]))

	case "unlock":
		if len(parts) < 2 {
			return nil, fmt.Errorf("unlock requires target")
		}
		return nil, a.doUnlock(Datastore(parts[1]))

	case "commit":
		return nil, a.doCommit()

	case "discard-changes":
		return nil, a.doDiscardChanges()

	case "validate":
		source := "candidate"
		if len(parts) > 1 {
			source = parts[1]
		}
		return nil, a.doValidate(Datastore(source))

	case "kill-session":
		if len(parts) < 2 {
			return nil, fmt.Errorf("kill-session requires session-id")
		}
		var id uint32
		if _, err := fmt.Sscanf(parts[1], "%d", &id); err != nil {
			return nil, fmt.Errorf("invalid session-id: %s", parts[1])
		}
		return nil, a.doKillSession(id)

	default:
		return nil, fmt.Errorf("unknown NETCONF operation: %s", op)
	}
}

// internal operation methods (no locking, called from locked contexts)

func (a *Adapter) doGetConfig(source Datastore, filter *Filter) ([]byte, error) {
	op := &rpcGetConfig{
		Source: datastoreElement(source),
		Filter: filterElement(filter),
	}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return nil, err
	}
	if reply.Errors.HasError() {
		return nil, reply.Errors
	}
	return reply.Data, nil
}

func (a *Adapter) doGet(filter *Filter) ([]byte, error) {
	op := &rpcGet{Filter: filterElement(filter)}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return nil, err
	}
	if reply.Errors.HasError() {
		return nil, reply.Errors
	}
	return reply.Data, nil
}

func (a *Adapter) doEditConfig(target Datastore, config []byte, opts *EditOptions) error {
	op := &rpcEditConfig{
		Target: datastoreElement(target),
		Config: rawXML{Content: string(config)},
	}
	if opts != nil {
		op.DefaultOperation = string(opts.DefaultOperation)
		op.TestOption = opts.TestOption
		op.ErrorOption = opts.ErrorOption
	}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doCopyConfig(source, target Datastore) error {
	op := &rpcCopyConfig{
		Source: datastoreElement(source),
		Target: datastoreElement(target),
	}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doDeleteConfig(target Datastore) error {
	if target == Running {
		return fmt.Errorf("cannot delete running datastore")
	}
	op := &rpcDeleteConfig{Target: datastoreElement(target)}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doLock(target Datastore) error {
	op := &rpcLock{Target: datastoreElement(target)}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doUnlock(target Datastore) error {
	op := &rpcUnlock{Target: datastoreElement(target)}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doCommit() error {
	reply, err := a.session.SendRPC(&rpcCommit{})
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doDiscardChanges() error {
	reply, err := a.session.SendRPC(&rpcDiscardChanges{})
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doValidate(source Datastore) error {
	op := &rpcValidate{Source: datastoreElement(source)}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func (a *Adapter) doKillSession(sessionID uint32) error {
	op := &rpcKillSession{SessionID: sessionID}
	reply, err := a.session.SendRPC(op)
	if err != nil {
		return err
	}
	if reply.Errors.HasError() {
		return reply.Errors
	}
	return nil
}

func toInternalFilter(f *protocols.NetconfFilter) *Filter {
	if f == nil {
		return nil
	}
	return &Filter{Type: f.Type, Content: f.Content}
}

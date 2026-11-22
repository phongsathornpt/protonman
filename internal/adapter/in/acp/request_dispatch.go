package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// methodCancelRequest is the ACP protocol-level notification that asks the
// receiver to abandon an in-flight JSON-RPC request by id.
const methodCancelRequest = "$/cancel_request"

// errRequestCancelled marks a request that was abandoned through
// `$/cancel_request`. It maps onto JSON-RPC error code CodeRequestCancelled.
var errRequestCancelled = errors.New("request cancelled")

// CancelRequestParams is the payload of `$/cancel_request`.
type CancelRequestParams struct {
	MetaCarrier
	RequestID json.RawMessage `json:"requestId"`
}

// inflightRequest is the cancellation handle of one admitted request.
type inflightRequest struct {
	key       string
	sessionID string
	prompt    bool
	cancel    context.CancelFunc
	cancelled atomic.Bool
}

// abort marks the request cancelled and cancels its context. It is idempotent.
func (r *inflightRequest) abort() {
	r.cancelled.Store(true)
	r.cancel()
}

// requestDispatcher runs every inbound client request off the read loop so the
// loop stays responsive to `$/cancel_request` and `session/cancel` while slow
// methods such as session/load, session/resume, or provider discovery run.
//
// Ordering contract: requests that carry the same sessionId (or no session at
// all but are not listed in unorderedMethods) start in arrival order, each
// waiting for its predecessor to finish. Prompts wait for earlier requests of
// their session but never delay later ones, because cancel, mode, and runtime
// changes must remain possible while a turn is running.
type requestDispatcher struct {
	server *Server
	ctx    context.Context
	output io.Writer
	wg     sync.WaitGroup
	errs   chan error

	mu       sync.Mutex
	inflight map[string]*inflightRequest
	prompts  map[string]map[*inflightRequest]struct{}
	tails    map[string]chan struct{}
}

func newRequestDispatcher(ctx context.Context, server *Server, output io.Writer) *requestDispatcher {
	return &requestDispatcher{
		server:   server,
		ctx:      ctx,
		output:   output,
		errs:     make(chan error, 1),
		inflight: make(map[string]*inflightRequest),
		prompts:  make(map[string]map[*inflightRequest]struct{}),
		tails:    make(map[string]chan struct{}),
	}
}

// unorderedMethods never wait on, or hold up, other requests. session/cancel in
// particular must act immediately, and session/new has no session to order by.
var unorderedMethods = map[string]struct{}{
	"initialize":     {},
	"session/new":    {},
	"session/list":   {},
	"session/cancel": {},
}

// orderingKey returns the serialization lane for a request, or "" when the
// request is not ordered against others.
func orderingKey(method, sessionID string) string {
	if _, ok := unorderedMethods[method]; ok {
		return ""
	}
	if sessionID != "" {
		return sessionID
	}
	return "*"
}

func requestSessionID(params json.RawMessage) string {
	if len(params) == 0 {
		return ""
	}
	var probe struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(params, &probe); err != nil {
		return ""
	}
	return strings.TrimSpace(probe.SessionID)
}

// canonicalRequestID normalizes a JSON-RPC id so that whitespace differences do
// not split one id into two keys. A string id and a number id stay distinct.
func canonicalRequestID(id json.RawMessage) string {
	trimmed := bytes.TrimSpace(id)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return string(trimmed)
	}
	return compact.String()
}

var errDuplicateRequestID = errors.New("request id is already in flight")

func (d *requestDispatcher) admit(id json.RawMessage, sessionID string, prompt bool) (*inflightRequest, context.Context, error) {
	reqCtx, cancel := context.WithCancel(d.ctx)
	entry := &inflightRequest{key: canonicalRequestID(id), sessionID: sessionID, prompt: prompt, cancel: cancel}
	d.mu.Lock()
	defer d.mu.Unlock()
	if entry.key != "" {
		if _, exists := d.inflight[entry.key]; exists {
			cancel()
			return nil, nil, errDuplicateRequestID
		}
		d.inflight[entry.key] = entry
	}
	if prompt {
		lane := d.prompts[sessionID]
		if lane == nil {
			lane = make(map[*inflightRequest]struct{})
			d.prompts[sessionID] = lane
		}
		lane[entry] = struct{}{}
	}
	return entry, reqCtx, nil
}

func (d *requestDispatcher) release(entry *inflightRequest) {
	d.mu.Lock()
	if entry.key != "" && d.inflight[entry.key] == entry {
		delete(d.inflight, entry.key)
	}
	if entry.prompt {
		if lane := d.prompts[entry.sessionID]; lane != nil {
			delete(lane, entry)
			if len(lane) == 0 {
				delete(d.prompts, entry.sessionID)
			}
		}
	}
	d.mu.Unlock()
	entry.cancel()
}

// order returns the predecessor completion channel of lane key and, when
// advance is set, installs a new tail that the returned release func closes.
func (d *requestDispatcher) order(key string, advance bool) (<-chan struct{}, func()) {
	d.mu.Lock()
	prev := d.tails[key]
	if !advance {
		d.mu.Unlock()
		return prev, func() {}
	}
	done := make(chan struct{})
	d.tails[key] = done
	d.mu.Unlock()
	return prev, func() {
		close(done)
		d.mu.Lock()
		if d.tails[key] == done {
			delete(d.tails, key)
		}
		d.mu.Unlock()
	}
}

// awaitTurn blocks until the predecessor finished or the request was cancelled.
func awaitTurn(ctx context.Context, prev <-chan struct{}) bool {
	if prev == nil {
		return ctx.Err() == nil
	}
	select {
	case <-prev:
		return ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}

func (d *requestDispatcher) report(err error) {
	if err == nil {
		return
	}
	select {
	case d.errs <- err:
	default:
	}
}

// failed returns the first asynchronous write failure, if any.
func (d *requestDispatcher) failed() error {
	select {
	case err := <-d.errs:
		return err
	default:
		return nil
	}
}

// cancelRequest implements `$/cancel_request`. Unknown ids are ignored: the
// request may already have completed.
func (d *requestDispatcher) cancelRequest(params json.RawMessage) {
	var cancel CancelRequestParams
	if err := json.Unmarshal(params, &cancel); err != nil {
		return
	}
	key := canonicalRequestID(cancel.RequestID)
	if key == "" {
		return
	}
	d.mu.Lock()
	entry := d.inflight[key]
	d.mu.Unlock()
	if entry == nil {
		return
	}
	entry.abort()
	if entry.prompt {
		if sess, ok := d.server.lookupSession(entry.sessionID); ok {
			sess.Cancel()
		}
	}
}

// cancelQueuedPrompts aborts prompts of a session that were admitted but may not
// have activated yet, so a session/cancel that races a just-submitted prompt is
// not lost.
func (d *requestDispatcher) cancelQueuedPrompts(sessionID string) {
	if sessionID == "" {
		return
	}
	d.mu.Lock()
	entries := make([]*inflightRequest, 0, len(d.prompts[sessionID]))
	for entry := range d.prompts[sessionID] {
		entries = append(entries, entry)
	}
	d.mu.Unlock()
	for _, entry := range entries {
		entry.abort()
	}
}

// submit admits one non-prompt request and runs it on its own goroutine.
func (d *requestDispatcher) submit(request RPCRequest) error {
	sessionID := requestSessionID(request.Params)
	if request.Method == "session/cancel" {
		d.cancelQueuedPrompts(sessionID)
	}
	entry, reqCtx, err := d.admit(request.ID, sessionID, false)
	if err != nil {
		return d.server.writeResponse(d.output, request.ID, nil, nil, invalidRequestError{err})
	}
	prev, finish := func() (<-chan struct{}, func()) {
		if key := orderingKey(request.Method, sessionID); key != "" {
			return d.order(key, true)
		}
		return nil, func() {}
	}()
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer d.release(entry)
		defer finish()
		if !awaitTurn(reqCtx, prev) {
			if entry.cancelled.Load() {
				d.report(d.server.writeResponse(d.output, request.ID, nil, nil, errRequestCancelled))
			}
			return
		}
		result, notify, routeErr := d.server.route(reqCtx, request, d.output)
		if routeErr != nil && entry.cancelled.Load() {
			routeErr = errRequestCancelled
		}
		d.report(d.server.writeResponse(d.output, request.ID, result, notify, routeErr))
	}()
	return nil
}

// submitPrompt admits one validated session/prompt request.
func (d *requestDispatcher) submitPrompt(request RPCRequest, sess *Session, blocks []ContentBlock) error {
	entry, reqCtx, err := d.admit(request.ID, sess.ID(), true)
	if err != nil {
		return d.server.writeResponse(d.output, request.ID, nil, nil, invalidRequestError{err})
	}
	prev, _ := d.order(sess.ID(), false)
	d.wg.Add(1)
	go func() {
		defer d.wg.Done()
		defer d.release(entry)
		if !awaitTurn(reqCtx, prev) {
			if entry.cancelled.Load() {
				d.report(d.server.writeResponse(d.output, request.ID, SessionPromptResult{StopReason: StopReasonCancelled}, nil, nil))
			}
			return
		}
		notifier := func(notification RPCNotification) error {
			return WriteJSON(d.output, &d.server.writeMu, notification)
		}
		result, promptErr := sess.ExecutePrompt(reqCtx, blocks, notifier)
		d.report(d.server.writeResponse(d.output, request.ID, result, nil, promptErr))
	}()
	return nil
}

// cancelOutboundRequest tells the client that a server-initiated request (a
// permission or elicitation prompt) is no longer awaited, so it can dismiss the
// matching UI instead of leaving an orphaned dialog open.
func (s *Server) cancelOutboundRequest(id uint64) {
	s.mu.Lock()
	output := s.output
	s.mu.Unlock()
	if output == nil {
		return
	}
	_ = WriteJSON(output, &s.writeMu, RPCNotification{
		JSONRPC: "2.0",
		Method:  methodCancelRequest,
		Params:  CancelRequestParams{RequestID: json.RawMessage(strconv.FormatUint(id, 10))},
	})
}

// wait blocks until every admitted request finished.
func (d *requestDispatcher) wait() { d.wg.Wait() }

// invalidRequestError carries a JSON-RPC invalid-request failure through
// writeResponse's error mapping.
type invalidRequestError struct{ error }

func (e invalidRequestError) Unwrap() error { return e.error }

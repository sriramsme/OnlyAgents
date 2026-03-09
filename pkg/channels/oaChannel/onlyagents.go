// Package oaChannel implements the WebSocket-based GUI channel for OnlyAgents.
// It replaces both the SSE /v1/events stream and the HTTP POST /v1/chat endpoint.
//
// One WebSocket connection carries:
//   - Chat messages (UI → agent via MessageReceived, agent → UI via Send())
//   - War room events (agent activity, tool calls, delegation) via UIBus subscription
//   - Notifications and proactive agent messages via Send()
//   - Voice chunks (when voice mode is added)
//
// Route: GET /v1/ws?session_id=<uuid>&agent_id=<id>
package oaChannel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/google/uuid"
	"github.com/mitchellh/mapstructure"

	"github.com/sriramsme/OnlyAgents/pkg/asec/vault"
	"github.com/sriramsme/OnlyAgents/pkg/channels"
	"github.com/sriramsme/OnlyAgents/pkg/core"
)

const version = "1.0.0"

func init() {
	channels.Register("onlyagents", NewChannel)
}

// client represents a single connected browser tab.
type client struct {
	id        string // unique per connection, used as UIBus subscriber key
	sessionID string
	agentID   string
	send      chan WSMessage // outbound messages queued for this client
	conn      *websocket.Conn
}

// OAChannel implements channels.Channel for the OnlyAgents web UI.
type OAChannel struct {
	eventBus  chan<- core.Event
	subscribe func(id string) (<-chan core.UIEvent, func()) // injected by kernel

	clients sync.Map // sessionID → *client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewChannel satisfies the channels.ChannelConstructor signature.
func NewChannel(
	ctx context.Context,
	rawConfig map[string]interface{},
	v vault.Vault,
	eventBus chan<- core.Event,
) (channels.Channel, error) {
	var cfg Config
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		WeaklyTypedInput: true,
		TagName:          "yaml",
	})
	if err != nil {
		return nil, fmt.Errorf("oaChannel: create decoder: %w", err)
	}
	if err := decoder.Decode(rawConfig); err != nil {
		return nil, fmt.Errorf("oaChannel: decode config: %w", err)
	}

	chanCtx, cancel := context.WithCancel(ctx)
	return &OAChannel{
		eventBus: eventBus,
		ctx:      chanCtx,
		cancel:   cancel,
		logger:   slog.With("channel", "oaChannel"),
	}, nil
}

// SetSubscribe injects the kernel's Subscribe function.
// Must be called by the kernel after construction, before any client connects.
func (g *OAChannel) SetSubscribe(fn func(id string) (<-chan core.UIEvent, func())) {
	g.subscribe = fn
}

// ── channels.Channel interface ────────────────────────────────────────────────

func (g *OAChannel) PlatformName() string { return "onlyagents" }
func (g *OAChannel) Version() string      { return version }
func (g *OAChannel) Connect() error       { return nil }
func (g *OAChannel) Disconnect() error    { g.cancel(); return nil }
func (g *OAChannel) Start() error         { return nil }

func (g *OAChannel) Stop() error {
	g.cancel()
	g.wg.Wait()
	return nil
}

func (g *OAChannel) HealthCheck() (bool, error) {
	var count int
	g.clients.Range(func(_, _ any) bool { count++; return true })
	return true, nil
}

// Send is called by the kernel when an agent has a response or proactive message
// for the UI. This is the only path for agent → UI communication.
func (g *OAChannel) Send(ctx context.Context, msg channels.OutgoingMessage) error {
	sessionID := msg.Channel.ChatID
	v, ok := g.clients.Load(sessionID)
	if !ok {
		g.logger.Warn("oaChannel: no ws client for session — message dropped",
			"session_id", sessionID,
			"preview", truncate(msg.Content, 80))
		// TODO: persist to offline queue and deliver on next connect
		return nil
	}
	return g.writeToClient(v.(*client), WSMessage{
		Type:      WSMsgAgentText,
		SessionID: sessionID,
		Timestamp: time.Now(),
		Payload: AgentTextPayload{
			Content: msg.Content,
			IsFinal: true,
		},
	})
}

// ── WebSocket handler ─────────────────────────────────────────────────────────

// WSHandler upgrades the HTTP connection to WebSocket and drives the client loop.
// Registered as: GET /v1/ws
//
// Query params:
//
//	session_id — optional; omit to start a new session, pass existing to resume
//	agent_id   — optional; defaults to "executive"
func (g *OAChannel) WSHandler(w http.ResponseWriter, r *http.Request) {
	if g.subscribe == nil {
		http.Error(w, "channel not ready", http.StatusServiceUnavailable)
		return
	}

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		g.logger.Error("oaChannel: ws upgrade failed", "err", err)
		return
	}

	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		agentID = "executive"
	}

	// Resolve session — use existing if provided, else ask kernel to create one
	sessionID := r.URL.Query().Get("session_id")
	if sessionID == "" {
		replyCh := make(chan core.Event, 1)
		g.eventBus <- core.Event{
			Type:    core.SessionNew,
			ReplyTo: replyCh,
			Payload: core.SessionNewPayload{
				Channel: "onlyagents",
				AgentID: agentID,
			},
		}
		select {
		case reply := <-replyCh:
			sessionID, _ = reply.Payload.(string)
		case <-r.Context().Done():
			err = conn.Close(websocket.StatusInternalError, "session init timeout")
			g.logger.Error("oaChannel: session init timeout", "err", err)
			return
		}
	}

	if sessionID == "" {
		err = conn.Close(websocket.StatusInternalError, "session init failed")
		g.logger.Error("oaChannel: session init failed", "err", err)
		return
	}

	c := &client{
		id:        uuid.NewString(),
		sessionID: sessionID,
		agentID:   agentID,
		send:      make(chan WSMessage, 64),
		conn:      conn,
	}
	g.clients.Store(sessionID, c)
	defer g.clients.Delete(sessionID)

	g.logger.Info("oaChannel: client connected",
		"session_id", sessionID,
		"agent_id", agentID)
	defer g.logger.Info("oaChannel: client disconnected", "session_id", sessionID)

	connCtx, connCancel := context.WithCancel(r.Context())
	defer connCancel()

	// Subscribe to UIBus — war room events flow through here
	uiCh, unsubscribe := g.subscribe(c.id)
	defer unsubscribe()

	// Single writer goroutine — merges c.send and uiCh into one WS connection.
	// IMPORTANT: websocket.Conn must never have concurrent writers.
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		g.writeLoop(connCtx, c, uiCh, connCancel)
	}()

	// Reader loop blocks until client disconnects or context is cancelled.
	g.readLoop(connCtx, c, connCancel)
}

// ── Internal loops ────────────────────────────────────────────────────────────

// readLoop reads inbound frames and dispatches them.
// Cancels connCtx on exit so writeLoop shuts down too.
func (g *OAChannel) readLoop(ctx context.Context, c *client, cancel context.CancelFunc) {
	defer cancel()
	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			return // client disconnected or context cancelled
		}
		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			g.logger.Warn("oaChannel: malformed ws message", "err", err)
			continue
		}
		g.handleInbound(ctx, c, msg)
	}
}

// writeLoop is the single writer for the WebSocket connection.
// It drains both c.send (chat responses, notifications) and uiCh (war room events).
// Cancels connCtx on exit so readLoop shuts down too.
func (g *OAChannel) writeLoop(ctx context.Context, c *client, uiCh <-chan core.UIEvent, cancel context.CancelFunc) {
	defer cancel()
	for {
		select {
		case msg := <-c.send:
			g.writeWS(ctx, c.conn, msg)
		case evt := <-uiCh:
			wsMsg := uiEventToWSMessage(evt)
			if wsMsg.Type != "" {
				g.writeWS(ctx, c.conn, wsMsg)
			}
		case <-ctx.Done():
			return
		}
	}
}

// ── Inbound message handling ──────────────────────────────────────────────────

func (g *OAChannel) handleInbound(ctx context.Context, c *client, msg WSMessage) {
	var err error
	switch msg.Type {
	case WSMsgChat:
		g.handleMessage(ctx, c, msg)
	case WSMsgVoiceChunk:
		g.handleVoiceChunk(ctx, c, msg)
	case WSMsgVoiceEnd:
		// signal that voice turn is complete — wire into voice pipeline when ready
	case WSMsgNewSession:
		err = g.handleNewSession(ctx, c)
	case WSMsgPing:
		err = g.writeToClient(c, WSMessage{Type: WSMsgPong, Timestamp: time.Now()})
	default:
		g.logger.Warn("oaChannel: unknown inbound message type", "type", msg.Type)
	}
	if err != nil {
		g.logger.Error("oaChannel: handleInbound ", "err", err)
	}
}

// handleMessage fires a MessageReceived event — identical pattern to Telegram.
// Response arrives asynchronously via Send() called by kernel.
func (g *OAChannel) handleMessage(_ context.Context, c *client, msg WSMessage) {
	var p ChatPayload
	err := remarshal(msg.Payload, &p)
	if err != nil {
		g.logger.Error("oaChannel: remarshal failed", "err", err)
	}
	if p.Message == "" {
		return
	}

	agentID := p.AgentID
	if agentID == "" {
		agentID = c.agentID
	}

	g.eventBus <- core.Event{
		Type:          core.MessageReceived,
		CorrelationID: uuid.NewString(),
		Payload: core.MessageReceivedPayload{
			Channel: &core.ChannelMetadata{
				SessionID: c.sessionID,
				ChatID:    c.sessionID,
				Name:      "onlyagents",
				UserID:    "user",
				Username:  "user",
			},
			Content: p.Message,
			Metadata: map[string]string{
				"target_agent": agentID,
			},
		},
	}
	// No replyCh, no waiting goroutine — response arrives via Send()
}

func (g *OAChannel) handleVoiceChunk(_ context.Context, _ *client, _ WSMessage) {
	// Placeholder — wire into voice pipeline when ready.
	// VoiceChunkPayload carries base64 audio + encoding + sample rate.
}

func (g *OAChannel) handleNewSession(_ context.Context, c *client) error {
	// End current session
	g.eventBus <- core.Event{
		Type:    core.SessionEnd,
		Payload: core.SessionEndPayload{SessionID: c.sessionID},
	}

	// Request a new one
	replyCh := make(chan core.Event, 1)
	g.eventBus <- core.Event{
		Type:    core.SessionNew,
		ReplyTo: replyCh,
		Payload: core.SessionNewPayload{Channel: "oaChannel", AgentID: c.agentID},
	}

	var newSessionID string
	select {
	case reply := <-replyCh:
		newSessionID, _ = reply.Payload.(string)
	case <-g.ctx.Done():
		return nil
	}

	g.clients.Delete(c.sessionID)
	c.sessionID = newSessionID
	g.clients.Store(newSessionID, c)

	return g.writeToClient(c, WSMessage{
		Type:      WSMsgNewSession,
		SessionID: newSessionID,
		Timestamp: time.Now(),
		Payload:   NewSessionPayload{SessionID: newSessionID},
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// writeToClient is a non-blocking enqueue into c.send.
// Drops the message if the buffer is full rather than blocking the caller.
func (g *OAChannel) writeToClient(c *client, msg WSMessage) error {
	select {
	case c.send <- msg:
		return nil
	default:
		g.logger.Warn("oaChannel: client send buffer full — message dropped",
			"session_id", c.sessionID,
			"type", msg.Type)
		return fmt.Errorf("oaChannel writeToeClient: client buffer full")
	}
}

// writeWS serialises and writes a single frame to the WebSocket connection.
// Must only be called from writeLoop (single writer guarantee).
func (g *OAChannel) writeWS(ctx context.Context, conn *websocket.Conn, msg WSMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		g.logger.Error("oaChannel: marshal ws message", "err", err)
		return
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		g.logger.Debug("oaChannel: ws write error", "err", err)
	}
}

// uiEventToWSMessage maps internal UIEvent types to outbound WebSocket types.
// Returns a zero-value WSMessage (empty Type) for unmapped events — callers skip those.
func uiEventToWSMessage(evt core.UIEvent) WSMessage {
	typeMap := map[core.UIEventType]WSMessageType{
		core.UIEventAgentActivated: WSMsgAgentActivated,
		core.UIEventAgentIdle:      WSMsgAgentIdle,
		core.UIEventAgentError:     WSMsgAgentError,
		core.UIEventToolCalled:     WSMsgToolCalled,
		core.UIEventToolResult:     WSMsgToolResult,
		core.UIEventDelegation:     WSMsgDelegation,
		core.UIEventSnapshotAgent:  WSMsgSnapshot,
	}
	wsType, ok := typeMap[evt.Type]
	if !ok {
		return WSMessage{}
	}

	return WSMessage{
		Type:      wsType,
		AgentID:   evt.AgentID,
		Timestamp: evt.Timestamp,
		Payload:   evt.Payload,
	}
}

// remarshal round-trips through JSON to decode msg.Payload (any/interface{})
// into a concrete struct without reflection gymnastics.
func remarshal(src any, dst any) error {
	b, err := json.Marshal(src)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, dst)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

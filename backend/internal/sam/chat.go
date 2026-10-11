package sam

import (
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/toufiqqureshi/seomarine/backend/internal/auth"
	"github.com/toufiqqureshi/seomarine/backend/internal/platform/httpx"
)

// Chat serves the conversation routes of a SAM session.
type Chat struct {
	Agent    *Agent
	Loader   SessionLoader
	Messages MessageStore
	turns    *turns
}

// NewChat wires the conversation routes.
func NewChat(agent *Agent, loader SessionLoader, messages MessageStore) *Chat {
	return &Chat{Agent: agent, Loader: loader, Messages: messages, turns: newTurns()}
}

type sessionRequest struct {
	SessionID string `json:"sessionId"`
}

type chatRequest struct {
	SessionID string `json:"sessionId"`
	Message   struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"message"`
}

type rewindRequest struct {
	SessionID string `json:"sessionId"`
	MessageID string `json:"messageId"`
}

func validID(id string) bool { return id != "" && len(id) <= maxIDChars }

// session resolves the caller's own, active session or writes the failure.
func (d Deps) session(w http.ResponseWriter, r *http.Request, sessionID string) (SessionContext, bool) {
	user, ok := auth.UserFromContext(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusUnauthorized, "unauthenticated", "Sign in to continue.")
		return SessionContext{}, false
	}
	if d.Chat == nil {
		d.unavailable(w)
		return SessionContext{}, false
	}
	sc, err := d.Chat.Loader.Load(r.Context(), r.PathValue("projectId"), user.ID, sessionID)
	if err != nil {
		d.fail(w, r, "load session", err)
		return SessionContext{}, false
	}
	return sc, true
}

// messages returns the transcript the chat view starts from.
func (d Deps) messages(w http.ResponseWriter, r *http.Request) {
	var in sessionRequest
	if !decodeJSON(w, r, &in) || !validID(in.SessionID) {
		writeInvalid(w)
		return
	}
	sc, ok := d.session(w, r, in.SessionID)
	if !ok {
		return
	}
	messages, err := d.Chat.Messages.List(r.Context(), sc.SessionID)
	if err != nil {
		d.fail(w, r, "list messages", err)
		return
	}
	if messages == nil {
		messages = []Message{}
	}
	httpx.WriteJSON(w, http.StatusOK, messages)
}

// chat stores the user's message and streams SAM's reply as server-sent events.
func (d Deps) chat(w http.ResponseWriter, r *http.Request) {
	var in chatRequest
	if !decodeJSONLimit(w, r, &in, maxChatBodyBytes) || !validID(in.SessionID) || !validID(in.Message.ID) {
		writeInvalid(w)
		return
	}
	text := strings.TrimSpace(in.Message.Text)
	if text == "" || utf8.RuneCountInString(text) > maxMessageChars {
		httpx.WriteError(w, http.StatusBadRequest, "invalid_message", "Write a message of up to 20,000 characters.")
		return
	}
	sc, ok := d.session(w, r, in.SessionID)
	if !ok {
		return
	}
	ctx, release, ok := d.Chat.turns.begin(r.Context(), sc.SessionID)
	if !ok {
		httpx.WriteError(w, http.StatusConflict, "sam_turn_in_progress", "SAM is still answering. Wait for the reply or stop it first.")
		return
	}
	defer release()

	userMessage := Message{ID: in.Message.ID, Role: "user", Parts: []Part{{Type: "text", Text: text}}}
	if err := d.Chat.Messages.Append(ctx, sc.SessionID, userMessage); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			httpx.WriteError(w, http.StatusConflict, "sam_duplicate_message", "That message was already sent.")
			return
		}
		d.fail(w, r, "store message", err)
		return
	}
	sink, ok := newSSESink(w)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "Streaming is not available.")
		return
	}
	defer sink.Close()
	if err := d.Chat.Agent.Run(ctx, sc, userMessage, sink); err != nil {
		d.Logger.ErrorContext(ctx, "SAM turn failed", "session", sc.SessionID, "err", err)
	}
}

// rewind deletes a message and everything after it, backing the client's undo
// and edit. A running turn is stopped first, or its loop would keep streaming
// and store a reply to a message that no longer exists.
func (d Deps) rewind(w http.ResponseWriter, r *http.Request) {
	var in rewindRequest
	if !decodeJSON(w, r, &in) || !validID(in.SessionID) || !validID(in.MessageID) {
		writeInvalid(w)
		return
	}
	sc, ok := d.session(w, r, in.SessionID)
	if !ok {
		return
	}
	if !d.Chat.turns.cancelAndWait(sc.SessionID, rewindWait) {
		httpx.WriteError(w, http.StatusConflict, "sam_turn_in_progress", "SAM is still stopping. Try again in a moment.")
		return
	}
	found, err := d.Chat.Messages.DeleteFrom(r.Context(), sc.SessionID, in.MessageID)
	if err != nil {
		d.fail(w, r, "rewind", err)
		return
	}
	if !found {
		httpx.WriteError(w, http.StatusNotFound, "sam_message_not_found", "Message not found.")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

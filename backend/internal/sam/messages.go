package sam

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Part is one piece of a chat message, shaped like an AI SDK UIMessage part so
// the React client can render stored and streamed messages the same way.
//
// Type is "text", "reasoning", "step-start" or "tool-<name>". A tool part is
// created once its input is known and always ends as output-available or
// output-error.
type Part struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	State      string          `json:"state,omitempty"`
	ToolCallID string          `json:"toolCallId,omitempty"`
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	ErrorText  string          `json:"errorText,omitempty"`
}

// Tool part states.
const (
	stateOutputAvailable = "output-available"
	stateOutputError     = "output-error"
	stateDone            = "done"
)

// Message is one stored chat message.
type Message struct {
	ID    string `json:"id"`
	Role  string `json:"role"`
	Parts []Part `json:"parts"`
	// Seq orders messages within the store; it is not sent to clients.
	Seq int64 `json:"-"`
}

// Summary stands in for the oldest messages of a long chat.
type Summary struct {
	Text       string
	ThroughSeq int64
}

// MessageStore persists a session's transcript.
type MessageStore interface {
	// List returns the session's messages oldest first.
	List(ctx context.Context, sessionID string) ([]Message, error)
	// Append stores one message at the end of the transcript.
	Append(ctx context.Context, sessionID string, message Message) error
	// DeleteFrom removes the message and everything after it. It reports false
	// when the message is not in the session.
	DeleteFrom(ctx context.Context, sessionID, messageID string) (bool, error)
	// Summary returns the session's rolling summary, if any.
	Summary(ctx context.Context, sessionID string) (Summary, bool, error)
	SaveSummary(ctx context.Context, sessionID string, summary Summary) error
}

// MessageRepository is the Postgres MessageStore.
type MessageRepository struct{ DB querier }

// List returns the session's messages oldest first.
func (r MessageRepository) List(ctx context.Context, sessionID string) ([]Message, error) {
	rows, err := r.DB.Query(ctx, `SELECT seq, id, role, parts FROM go_sam_messages
		WHERE session_id=$1 ORDER BY seq`, sessionID)
	if err != nil {
		return nil, fmt.Errorf("query SAM messages: %w", err)
	}
	messages, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Message, error) {
		var (
			m   Message
			raw []byte
		)
		if err := row.Scan(&m.Seq, &m.ID, &m.Role, &raw); err != nil {
			return Message{}, err
		}
		if err := json.Unmarshal(raw, &m.Parts); err != nil {
			return Message{}, fmt.Errorf("decode parts of message %s: %w", m.ID, err)
		}
		return m, nil
	})
	if err != nil {
		return nil, fmt.Errorf("read SAM messages: %w", err)
	}
	if messages == nil {
		messages = []Message{}
	}
	return messages, nil
}

// Append stores one message at the end of the transcript.
func (r MessageRepository) Append(ctx context.Context, sessionID string, message Message) error {
	parts, err := json.Marshal(message.Parts)
	if err != nil {
		return fmt.Errorf("encode SAM message parts: %w", err)
	}
	parts = stripNULEscapes(parts)
	if _, err := r.DB.Exec(ctx, `INSERT INTO go_sam_messages (id, session_id, role, parts)
		VALUES ($1, $2, $3, $4)`, message.ID, sessionID, message.Role, parts); err != nil {
		return fmt.Errorf("append SAM message: %w", err)
	}
	return nil
}

// DeleteFrom removes the message and everything after it.
func (r MessageRepository) DeleteFrom(ctx context.Context, sessionID, messageID string) (bool, error) {
	tag, err := r.DB.Exec(ctx, `DELETE FROM go_sam_messages
		WHERE session_id=$1 AND seq >= (SELECT seq FROM go_sam_messages WHERE session_id=$1 AND id=$2)`,
		sessionID, messageID)
	if err != nil {
		return false, fmt.Errorf("rewind SAM messages: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	// A summary that covered the deleted messages describes a conversation that
	// no longer exists.
	if _, err := r.DB.Exec(ctx, `DELETE FROM go_sam_session_summaries
		WHERE session_id=$1 AND through_seq > COALESCE((SELECT MAX(seq) FROM go_sam_messages WHERE session_id=$1), 0)`,
		sessionID); err != nil {
		return true, fmt.Errorf("drop stale SAM summary: %w", err)
	}
	return true, nil
}

// Summary returns the session's rolling summary, if any.
func (r MessageRepository) Summary(ctx context.Context, sessionID string) (Summary, bool, error) {
	var s Summary
	err := r.DB.QueryRow(ctx, `SELECT summary, through_seq FROM go_sam_session_summaries WHERE session_id=$1`, sessionID).
		Scan(&s.Text, &s.ThroughSeq)
	if errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, false, nil
	}
	if err != nil {
		return Summary{}, false, fmt.Errorf("read SAM summary: %w", err)
	}
	return s, true, nil
}

// SaveSummary replaces the session's rolling summary.
func (r MessageRepository) SaveSummary(ctx context.Context, sessionID string, s Summary) error {
	if _, err := r.DB.Exec(ctx, `INSERT INTO go_sam_session_summaries (session_id, summary, through_seq)
		VALUES ($1, $2, $3)
		ON CONFLICT (session_id) DO UPDATE SET summary=EXCLUDED.summary, through_seq=EXCLUDED.through_seq, updated_at=now()`,
		sessionID, s.Text, s.ThroughSeq); err != nil {
		return fmt.Errorf("save SAM summary: %w", err)
	}
	return nil
}

// stripNULEscapes removes \u0000 escapes from encoded JSON. Postgres jsonb
// rejects them, and scraped pages and tool output can contain a NUL byte; one
// would otherwise cost the user the whole reply. Escapes are consumed in pairs,
// so an escaped backslash followed by the text "u0000" is left alone.
func stripNULEscapes(encoded []byte) []byte {
	const nul = `\u0000`
	out := make([]byte, 0, len(encoded))
	for i := 0; i < len(encoded); {
		if encoded[i] != '\\' || i+1 >= len(encoded) {
			out = append(out, encoded[i])
			i++
			continue
		}
		if i+len(nul) <= len(encoded) && string(encoded[i:i+len(nul)]) == nul {
			i += len(nul)
			continue
		}
		out = append(out, encoded[i], encoded[i+1])
		i += 2
	}
	return out
}

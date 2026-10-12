import { useChat } from "@ai-sdk/react";
import { useQueryClient, useSuspenseQuery } from "@tanstack/react-query";
import { DefaultChatTransport } from "ai";
import type { UIMessage } from "ai";
import { useEffect, useMemo, useRef } from "react";
import { findLast } from "remeda";
import { toast } from "sonner";
import { ChatComposer } from "@/client/features/sam/ChatComposer";
import { rewindSamSession, samChatUrl } from "@/client/features/sam/samApi";
import {
  invalidateSamSessions,
  samMessagesQueryOptions,
} from "@/client/features/sam/samQueries";
import { captureClientEvent } from "@/client/lib/posthog";
import { ErrorState } from "@/client/components/ErrorState";
import { Button } from "@/client/components/ui/button";
import {
  ChatMessage,
  messageHasVisibleContent,
} from "@/client/components/chat/ChatMessage";
import { useStickToBottom } from "@/client/components/chat/useStickToBottom";

const SUGGESTIONS = [
  "What keywords should I focus on next?",
  "Who are my top SERP competitors?",
  "How is my Search Console traffic trending?",
  "Find quick-win keywords I already rank for",
];

function messageText(message: UIMessage): string {
  return message.parts
    .filter((part) => part.type === "text")
    .map((part) => part.text)
    .join("\n");
}

/**
 * Loads the stored transcript, then hands it to the live chat. useChat needs
 * its initial messages at mount, so the load suspends here (the route wraps
 * this in a Suspense boundary) instead of inside the chat.
 */
export function SamConversation({
  projectId,
  sessionId,
}: {
  projectId: string;
  sessionId: string;
}) {
  const { data: initialMessages } = useSuspenseQuery(
    samMessagesQueryOptions(projectId, sessionId),
  );
  return (
    <SamChatView
      projectId={projectId}
      sessionId={sessionId}
      initialMessages={initialMessages}
    />
  );
}

function SamChatView({
  projectId,
  sessionId,
  initialMessages,
}: {
  projectId: string;
  sessionId: string;
  initialMessages: UIMessage[];
}) {
  const queryClient = useQueryClient();
  // The conversation lives on the Go server; each send is one HTTP request
  // whose response streams the reply. The server keeps the transcript, so the
  // request carries only the new message, not the history.
  const transport = useMemo(
    () =>
      new DefaultChatTransport({
        api: samChatUrl(projectId),
        prepareSendMessagesRequest: ({ messages }) => {
          const last = messages[messages.length - 1];
          return {
            body: {
              sessionId,
              message: { id: last.id, text: messageText(last) },
            },
          };
        },
      }),
    [projectId, sessionId],
  );
  // SAM streams dense tool-input deltas; unthrottled per-chunk store fanout
  // re-renders the transcript per delta.
  const { messages, sendMessage, setMessages, status, stop } = useChat({
    id: sessionId,
    messages: initialMessages,
    transport,
    experimental_throttle: 50,
  });

  const isBusy = status === "submitted" || status === "streaming";
  const { scrollRef, onScroll, pinToBottom } = useStickToBottom(
    messages,
    status,
  );
  // Client-side send counts, to set against the server's turns: a send with
  // no matching turn is a message that never reached the server.
  const sendText = (
    text: string,
    source: "composer" | "suggestion" | "edit" | "retry" = "composer",
  ) => {
    pinToBottom();
    captureClientEvent("sam:message_send", {
      session_id: sessionId,
      project_id: projectId,
      source,
      chars: text.length,
    });
    void sendMessage({ text });
  };

  useEffect(() => {
    if (status !== "error") return;
    captureClientEvent("sam:client_error", {
      session_id: sessionId,
      project_id: projectId,
      kind: "turn",
    });
  }, [status, sessionId, projectId]);

  // Rewind the server-side conversation to before `messageId`: the server
  // stops any in-flight turn, then deletes the message and everything after
  // it. Sync the local view from the server afterwards rather than slicing
  // locally: a stopped turn may have stored (or removed) more than we can see.
  const rewindTo = async (messageId: string) => {
    void stop();
    try {
      await rewindSamSession({ data: { projectId, sessionId, messageId } });
      const fresh = await queryClient.fetchQuery(
        samMessagesQueryOptions(projectId, sessionId),
      );
      setMessages(fresh);
      return true;
    } catch {
      toast.error("Couldn't change the conversation. Try again.");
      return false;
    }
  };

  const undoFrom = (messageId: string) => void rewindTo(messageId);
  const editAndResend = async (
    messageId: string,
    newText: string,
    source: "edit" | "retry" = "edit",
  ) => {
    if (await rewindTo(messageId)) sendText(newText, source);
  };
  // Retry after a failed turn goes through the same rewind-then-resend path
  // as edit, so the failed partial reply is deleted server-side before the
  // new one streams in. Each click is one human-gated turn; the server never
  // re-runs a turn on its own.
  const lastUserMessage = findLast(messages, (m) => m.role === "user");
  const retryLast = () => {
    if (!lastUserMessage) return;
    void editAndResend(
      lastUserMessage.id,
      messageText(lastUserMessage),
      "retry",
    );
  };

  // The server names the session from its first message during the turn, so refresh
  // the side-panel once the turn settles (busy -> idle) to pick up the title.
  const wasBusyRef = useRef(false);
  useEffect(() => {
    if (isBusy) {
      wasBusyRef.current = true;
      return;
    }
    if (wasBusyRef.current) {
      wasBusyRef.current = false;
      void invalidateSamSessions(projectId);
    }
  }, [isBusy, projectId]);

  const lastMessage = messages[messages.length - 1];
  const showTyping =
    isBusy &&
    (lastMessage?.role !== "assistant" ||
      !messageHasVisibleContent(lastMessage));
  const showSuggestions = messages.length === 0 && !isBusy;

  return (
    <div className="relative flex min-w-0 flex-1 flex-col">
      <div
        ref={scrollRef}
        onScroll={onScroll}
        className="flex-1 overflow-y-auto px-5 py-6"
      >
        <div className="mx-auto max-w-2xl space-y-6">
          {messages.length === 0 ? (
            <div className="space-y-2 text-sm text-foreground/80">
              <p>
                Hey, I’m SAM — your in-app SEO agent. I can research keywords,
                size up competitors, read your SERPs, backlinks, rank tracking
                and Search Console, and turn it into next steps for this
                project.
              </p>
              <p>Ask me anything, or start with one of these:</p>
            </div>
          ) : null}

          {messages.map((message, index) => (
            <ChatMessage
              key={message.id}
              message={message}
              streaming={
                isBusy &&
                index === messages.length - 1 &&
                message.role === "assistant"
              }
              onUndo={
                // Allowed even mid-turn: rewind aborts the in-flight turn
                // server-side, so undo doubles as "stop and take it back".
                message.role === "user" ? () => undoFrom(message.id) : undefined
              }
              onEdit={
                message.role === "user"
                  ? (newText) => void editAndResend(message.id, newText)
                  : undefined
              }
            />
          ))}

          {showTyping ? (
            <div className="flex items-center gap-2 pt-1 text-muted-foreground">
              <span className="flex items-center gap-1.5">
                <span className="size-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.3s]" />
                <span className="size-1.5 animate-bounce rounded-full bg-current [animation-delay:-0.15s]" />
                <span className="size-1.5 animate-bounce rounded-full bg-current" />
              </span>
            </div>
          ) : null}

          {status === "error" ? (
            <ErrorState
              variant="inline"
              message="SAM stopped before finishing this reply."
              onRetry={lastUserMessage ? retryLast : undefined}
              isRetrying={isBusy}
            />
          ) : null}

          {showSuggestions ? (
            <div className="flex flex-wrap gap-2">
              {SUGGESTIONS.map((question) => (
                <Button
                  key={question}
                  variant="outline"
                  size="sm"
                  className="rounded-full text-xs text-muted-foreground"
                  onClick={() => sendText(question, "suggestion")}
                >
                  {question}
                </Button>
              ))}
            </div>
          ) : null}
        </div>
      </div>

      <div className="flex-shrink-0 border-t border-border px-5 py-3">
        <div className="mx-auto w-full max-w-2xl">
          <ChatComposer
            busy={isBusy}
            onSend={sendText}
            onStop={() => void stop()}
          />
        </div>
      </div>
    </div>
  );
}

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";
import { Button } from "@/client/components/ui/button";
import { Input } from "@/client/components/ui/input";
import { Textarea } from "@/client/components/ui/textarea";
import {
  contentAuthorsQueryOptions,
  deleteContentAuthor,
  saveContentAuthor,
  validateAuthorInput,
  type AuthorInput,
  type ContentAuthor,
} from "./authorsApi";

const bioLimit = 300;

/**
 * AuthorProfiles manages the project's author profiles on the Integrations
 * settings page. Any member may edit them: profiles are editorial data, not
 * accounts. They feed the WordPress byline and Article schema.
 */
export function AuthorProfiles({ projectId }: { projectId: string }) {
  const queryClient = useQueryClient();
  const [editing, setEditing] = useState<AuthorInput | null>(null);
  const [error, setError] = useState("");
  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: contentAuthorsQueryOptions(projectId).queryKey,
    });

  const save = useMutation({
    mutationFn: (input: AuthorInput) => saveContentAuthor(projectId, input),
    onSuccess: (_author, input) => {
      toast.success(
        input.id ? "Author profile updated." : "Author profile added.",
      );
      setEditing(null);
      setError("");
      void invalidate();
    },
    onError: (cause) =>
      setError(
        cause instanceof Error
          ? cause.message
          : "Could not save this author profile.",
      ),
  });
  const remove = useMutation({
    mutationFn: (authorId: string) => deleteContentAuthor(projectId, authorId),
    onSuccess: () => {
      toast.success("Author profile deleted. Drafts keep working without it.");
      void invalidate();
    },
    onError: (cause) =>
      toast.error(
        cause instanceof Error
          ? cause.message
          : "Could not delete this author.",
      ),
  });

  const submit = () => {
    if (!editing) return;
    const problem = validateAuthorInput(editing);
    if (problem) {
      setError(problem);
      return;
    }
    setError("");
    save.mutate({
      ...editing,
      role: editing.role.trim(),
      bio: editing.bio.trim(),
      photoUrl: editing.photoUrl?.trim() || undefined,
      profileUrl: editing.profileUrl?.trim() || undefined,
    });
  };

  return (
    <div className="space-y-3 rounded-xl border bg-card p-4">
      {!editing ? (
        <AddButton onNew={() => setEditing({ name: "", role: "", bio: "" })} />
      ) : (
        <div className="space-y-2">
          <Input
            aria-label="Author name"
            placeholder="Full name, e.g. Aman Sharma"
            maxLength={120}
            value={editing.name}
            onChange={(event) =>
              setEditing({ ...editing, name: event.target.value })
            }
          />
          <Input
            aria-label="Author role"
            placeholder="Role, e.g. SEO Writer"
            maxLength={120}
            value={editing.role}
            onChange={(event) =>
              setEditing({ ...editing, role: event.target.value })
            }
          />
          <div>
            <Textarea
              aria-label="Author bio"
              placeholder={`One or two lines about the author (${bioLimit} characters).`}
              rows={3}
              maxLength={bioLimit}
              value={editing.bio}
              onChange={(event) =>
                setEditing({ ...editing, bio: event.target.value })
              }
            />
            <p className="mt-1 text-xs text-muted-foreground">
              {editing.bio.length}/{bioLimit}
            </p>
          </div>
          <Input
            aria-label="Author photo link"
            placeholder="Photo link (optional, https)"
            maxLength={500}
            value={editing.photoUrl ?? ""}
            onChange={(event) =>
              setEditing({ ...editing, photoUrl: event.target.value })
            }
          />
          <Input
            aria-label="Author profile link"
            placeholder="Profile link (optional, https)"
            maxLength={500}
            value={editing.profileUrl ?? ""}
            onChange={(event) =>
              setEditing({ ...editing, profileUrl: event.target.value })
            }
          />
          {error ? (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          ) : null}
          <div className="flex gap-2">
            <Button size="sm" onClick={submit} disabled={save.isPending}>
              {save.isPending
                ? "Saving…"
                : editing.id
                  ? "Update author"
                  : "Add author"}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => {
                setEditing(null);
                setError("");
              }}
            >
              Cancel
            </Button>
          </div>
        </div>
      )}
      <AuthorList
        projectId={projectId}
        onEdit={(author) =>
          setEditing({
            id: author.id,
            name: author.name,
            role: author.role,
            bio: author.bio,
            photoUrl: author.photoUrl,
            profileUrl: author.profileUrl,
          })
        }
        onDelete={(author) => {
          if (window.confirm(`Delete the profile of "${author.name}"?`)) {
            remove.mutate(author.id);
          }
        }}
        deletePending={remove.isPending}
      />
    </div>
  );
}

function AddButton({ onNew }: { onNew: () => void }) {
  return (
    <div className="flex items-center justify-between gap-3">
      <p className="text-sm text-muted-foreground">
        Author profiles appear bylined at the end of published articles.
      </p>
      <Button size="sm" variant="outline" onClick={onNew}>
        Add author
      </Button>
    </div>
  );
}

function AuthorList({
  projectId,
  onEdit,
  onDelete,
  deletePending,
}: {
  projectId: string;
  onEdit: (author: ContentAuthor) => void;
  onDelete: (author: ContentAuthor) => void;
  deletePending: boolean;
}) {
  const authors = contentAuthorsQueryOptions(projectId);
  const list = useStableAuthors(authors);
  void projectId;
  if (list.length === 0) return null;
  return (
    <ul className="divide-y">
      {list.map((author) => (
        <li
          key={author.id}
          className="flex items-center justify-between gap-3 py-2"
        >
          <div className="min-w-0">
            <p className="truncate text-sm font-medium">
              {author.name}
              {author.role ? (
                <span className="text-muted-foreground"> · {author.role}</span>
              ) : null}
            </p>
            {author.bio ? (
              <p className="truncate text-xs text-muted-foreground">
                {author.bio}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 gap-2">
            <Button size="sm" variant="outline" onClick={() => onEdit(author)}>
              Edit
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={deletePending}
              onClick={() => onDelete(author)}
            >
              Delete
            </Button>
          </div>
        </li>
      ))}
    </ul>
  );
}

// useStableAuthors keeps the list read on one query hook; TanStack Query
// caches per project so two cards never drift.
function useStableAuthors(
  options: ReturnType<typeof contentAuthorsQueryOptions>,
) {
  return useQuery(options).data ?? [];
}

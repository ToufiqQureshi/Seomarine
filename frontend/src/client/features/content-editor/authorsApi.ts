import { z } from "zod";
import { apiRequest } from "@/client/lib/uplumoApi";

// Author profiles live on the project and feed the publish-time byline and
// Article schema. They are editorial data: any workspace member manages them.

const authorSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  name: z.string(),
  role: z.string(),
  bio: z.string(),
  photoUrl: z.string().optional(),
  profileUrl: z.string().optional(),
  createdAt: z.iso.datetime({ offset: true }),
  updatedAt: z.iso.datetime({ offset: true }),
});
export type ContentAuthor = z.infer<typeof authorSchema>;

export type AuthorInput = {
  id?: string;
  name: string;
  role: string;
  bio: string;
  photoUrl?: string;
  profileUrl?: string;
};

const projectPath = (projectId: string) =>
  `/api/v1/projects/${encodeURIComponent(projectId)}`;

export const contentAuthorsQueryOptions = (projectId: string) => ({
  queryKey: ["contentAuthors", projectId],
  queryFn: () =>
    apiRequest(
      `${projectPath(projectId)}/content/authors/list`,
      z.array(authorSchema),
      "POST",
      {},
    ),
});

export function saveContentAuthor(projectId: string, input: AuthorInput) {
  return apiRequest(
    `${projectPath(projectId)}/content/authors/save`,
    authorSchema,
    "POST",
    input,
  );
}

export function deleteContentAuthor(projectId: string, authorId: string) {
  return apiRequest(
    `${projectPath(projectId)}/content/authors/${encodeURIComponent(authorId)}/delete`,
    z.object({ success: z.boolean() }),
    "POST",
    {},
  );
}

// subscription-shaped client validation shared by the card and the editor.
const authorBioLimit = 300;

export function validateAuthorInput(input: AuthorInput): string | null {
  if (!input.name.trim()) return "Enter the author's name.";
  if (input.name.length > 120)
    return "The name must be at most 120 characters.";
  if (input.role.length > 120)
    return "The role must be at most 120 characters.";
  if (input.bio.length > authorBioLimit)
    return `The bio must be at most ${authorBioLimit} characters.`;
  for (const link of [input.photoUrl ?? "", input.profileUrl ?? ""]) {
    if (!link) continue;
    if (!/^https?:\/\//iu.test(link)) {
      return "Photo and profile links must start with https:// or http://.";
    }
    // A link with user:pass@host would leak credentials into the article's
    // public JSON-LD; the server refuses it and so does the form.
    if (/^[a-z]+:\/\/[^/]+@/iu.test(link)) {
      return "Photo and profile links cannot contain credentials.";
    }
  }
  return null;
}

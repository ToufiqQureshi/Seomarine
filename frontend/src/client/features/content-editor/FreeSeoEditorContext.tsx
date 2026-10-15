import type { Editor } from "@tiptap/react";
import { createContext, useContext, useState, type ReactNode } from "react";
import type { ContentDraftInput } from "./scoring";

type DraftField = keyof ContentDraftInput;

type FreeSeoEditorValue = {
  editor: Editor | null;
  setEditor: (editor: Editor | null) => void;
  setDraftField: (field: DraftField, value: string) => void;
};

const FreeSeoEditorContext = createContext<FreeSeoEditorValue | null>(null);

export function FreeSeoEditorProvider({
  children,
  onDraftFieldChange,
}: {
  children: ReactNode;
  onDraftFieldChange: (field: DraftField, value: string) => void;
}) {
  const [editor, setEditor] = useState<Editor | null>(null);
  return (
    <FreeSeoEditorContext.Provider
      value={{ editor, setEditor, setDraftField: onDraftFieldChange }}
    >
      {children}
    </FreeSeoEditorContext.Provider>
  );
}

export function useFreeSeoEditor() {
  const value = useContext(FreeSeoEditorContext);
  if (!value) throw new Error("Free SEO editor context is missing");
  return value;
}

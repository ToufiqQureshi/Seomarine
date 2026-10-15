import { createContext, useContext, useMemo, useState } from "react";
import type { Editor } from "@tiptap/react";
import type { ReactNode } from "react";

type EditorSession = {
  editor: Editor | null;
  setEditor: (editor: Editor | null) => void;
};

const EditorSessionContext = createContext<EditorSession | null>(null);

export function EditorSessionProvider({ children }: { children: ReactNode }) {
  const [editor, setEditor] = useState<Editor | null>(null);
  const value = useMemo(() => ({ editor, setEditor }), [editor]);
  return (
    <EditorSessionContext.Provider value={value}>
      {children}
    </EditorSessionContext.Provider>
  );
}

export function useEditorSession() {
  const value = useContext(EditorSessionContext);
  if (!value) throw new Error("Content editor session provider is missing.");
  return value;
}

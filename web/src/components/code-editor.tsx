import * as React from "react";
import Editor, { type OnMount } from "@monaco-editor/react";
import { useTheme } from "@/hooks/use-theme";
import {
  monaco,
  configureJsLanguage,
} from "@/lib/monaco-setup";

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  /** Ambient .d.ts describing the plugin runtime, for IntelliSense. */
  typeDefs?: string;
  height?: number | string;
  readOnly?: boolean;
}

const EXTRA_LIB_URI = "ts:loom-plugin-runtime.d.ts";

// CodeEditor is a Monaco-backed JavaScript editor wired with the Loom plugin
// runtime type definitions. Lazy-load this component so Monaco stays out of the
// main bundle.
export default function CodeEditor({
  value,
  onChange,
  typeDefs,
  height = 360,
  readOnly = false,
}: CodeEditorProps) {
  const { resolvedTheme } = useTheme();
  const monacoTheme =
    resolvedTheme === "dark" || resolvedTheme === "amoled"
      ? "vs-dark"
      : "light";

  const libRef = React.useRef<monaco.IDisposable | null>(null);
  const mountedRef = React.useRef(false);

  const applyTypeDefs = React.useCallback((dts: string | undefined) => {
    libRef.current?.dispose();
    libRef.current = null;
    if (dts) {
      libRef.current =
        monaco.languages.typescript.javascriptDefaults.addExtraLib(
          dts,
          EXTRA_LIB_URI,
        );
    }
  }, []);

  const handleMount: OnMount = React.useCallback(
    (_editor, _monaco) => {
      configureJsLanguage();
      mountedRef.current = true;
      applyTypeDefs(typeDefs);
    },
    [applyTypeDefs, typeDefs],
  );

  // Re-apply when the type defs arrive/change after the editor has mounted.
  React.useEffect(() => {
    if (mountedRef.current) {
      applyTypeDefs(typeDefs);
    }
  }, [typeDefs, applyTypeDefs]);

  React.useEffect(
    () => () => {
      mountedRef.current = false;
      libRef.current?.dispose();
      libRef.current = null;
    },
    [],
  );

  return (
    <div className="overflow-hidden rounded-md border border-border">
      <Editor
        height={height}
        language="javascript"
        theme={monacoTheme}
        value={value}
        onChange={(v) => onChange(v ?? "")}
        onMount={handleMount}
        options={{
          readOnly,
          minimap: { enabled: false },
          fontSize: 13,
          lineNumbers: "on",
          scrollBeyondLastLine: false,
          tabSize: 2,
          automaticLayout: true,
          wordWrap: "on",
          fontFamily:
            "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace",
        }}
      />
    </div>
  );
}

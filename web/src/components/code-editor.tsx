import * as React from "react";
import Editor, { useMonaco, type Monaco } from "@monaco-editor/react";
import { useTheme } from "@/hooks/use-theme";
import { configureJsLanguage } from "@/lib/monaco-setup";

export interface CodeEditorProps {
  value: string;
  onChange: (value: string) => void;
  /** Ambient .d.ts describing the plugin runtime, for IntelliSense. */
  typeDefs?: string;
  height?: number | string;
  readOnly?: boolean;
}

const EXTRA_LIB_URI = "ts:loom-plugin-runtime.d.ts";

type Disposable = { dispose: () => void };

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
  const monaco = useMonaco();
  const monacoTheme =
    resolvedTheme === "dark" || resolvedTheme === "amoled"
      ? "vs-dark"
      : "light";

  const libRef = React.useRef<Disposable | null>(null);

  const applyTypeDefs = React.useCallback(
    (m: Monaco, dts: string | undefined) => {
      libRef.current?.dispose();
      libRef.current = null;
      if (dts) {
        libRef.current = m.languages.typescript.javascriptDefaults.addExtraLib(
          dts,
          EXTRA_LIB_URI,
        );
      }
    },
    [],
  );

  // Configure the JS language and (re)apply the type defs once Monaco has
  // loaded and whenever the defs change.
  React.useEffect(() => {
    if (!monaco) return;
    configureJsLanguage(monaco);
    applyTypeDefs(monaco, typeDefs);
  }, [monaco, typeDefs, applyTypeDefs]);

  React.useEffect(
    () => () => {
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

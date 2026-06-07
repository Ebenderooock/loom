// Monaco bootstrap: must run at module load, BEFORE any <Editor/> mounts.
//
// 1. Wire the language web workers via Vite "?worker" imports so Monaco never
//    tries to fetch them from a CDN (the @monaco-editor/react default).
// 2. Point @monaco-editor/react at our bundled monaco instance.
// 3. Configure the JavaScript language service ONCE (ES5 to match the goja
//    runtime) with low-strictness diagnostics.

import { loader } from "@monaco-editor/react";
import * as monaco from "monaco-editor";
import editorWorker from "monaco-editor/esm/vs/editor/editor.worker?worker";
import tsWorker from "monaco-editor/esm/vs/language/typescript/ts.worker?worker";

self.MonacoEnvironment = {
  getWorker(_workerId: string, label: string) {
    if (label === "typescript" || label === "javascript") {
      return new tsWorker();
    }
    return new editorWorker();
  },
};

loader.config({ monaco });

let compilerConfigured = false;

// configureJsLanguage sets ES5 compiler options (goja parity) and enables
// diagnostics. Guarded so it only runs once even if multiple editors mount.
export function configureJsLanguage() {
  if (compilerConfigured) return;
  compilerConfigured = true;

  const js = monaco.languages.typescript.javascriptDefaults;
  js.setCompilerOptions({
    target: monaco.languages.typescript.ScriptTarget.ES5,
    lib: ["es5"],
    allowNonTsExtensions: true,
    checkJs: true,
    noImplicitAny: false,
    noLib: false,
  });
  js.setDiagnosticsOptions({
    noSemanticValidation: false,
    noSyntaxValidation: false,
    noSuggestionDiagnostics: false,
  });
}

export { monaco };

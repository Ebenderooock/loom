// Monaco bootstrap.
//
// Instead of bundling Monaco from source (which makes the bundler process
// thousands of modules and OOMs constrained CI/Docker build nodes), we load
// Monaco's prebuilt AMD assets at runtime from our own origin. The assets are
// copied to /monaco/vs by vite-plugin-static-copy (see vite.config.ts), so this
// stays fully offline / self-hosted — no CDN.

import { loader, type Monaco } from "@monaco-editor/react";

// Use an absolute URL (with origin) so Monaco's web workers — which run from a
// blob and therefore have no document base — can resolve the nested worker
// scripts (e.g. language/typescript/tsWorker.js). A root-relative path breaks
// inside the worker.
loader.config({
  paths: {
    vs: new URL(`${import.meta.env.BASE_URL}monaco/vs`, window.location.origin)
      .href,
  },
});

let compilerConfigured = false;

// configureJsLanguage sets ES5 compiler options (goja parity) and enables
// diagnostics. Guarded so it only runs once even if multiple editors mount.
export function configureJsLanguage(monaco: Monaco) {
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

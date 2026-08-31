import { lazy, Suspense } from 'react'
// Importing types only: these disappear at compile time and therefore do NOT
// pull monaco into the entry chunk.
import type { DiffEditorProps, EditorProps } from '@monaco-editor/react'

// monacoBootstrap holds the one-time dynamic import of the editor stack.
// Previously main.tsx imported monaco-editor, its worker and the react
// bindings eagerly, which forced the whole editor (~3MB parsed) into the
// entry chunk even for users who never open an editor. Everything is now
// loaded on first actual use.
let monacoBootstrap: Promise<typeof import('@monaco-editor/react')> | null =
  null

// ensureMonaco loads monaco-editor, the editor worker and the react bindings
// in parallel, then points the react loader at the bundled monaco instance so
// no CDN fetch ever happens. The promise is memoized: subsequent editors
// reuse the same module instances.
export function ensureMonaco() {
  if (!monacoBootstrap) {
    monacoBootstrap = Promise.all([
      import('monaco-editor'),
      import('@monaco-editor/react'),
      import('monaco-editor/esm/vs/editor/editor.worker?worker'),
    ]).then(
      ([monaco, reactBinding, editorWorker]) => {
        // The worker constructor lives on the default export of the ?worker
        // module; monaco needs it to spawn the editor worker thread.
        self.MonacoEnvironment = {
          getWorker() {
            return new editorWorker.default()
          },
        }
        reactBinding.loader.config({ monaco })
        return reactBinding
      },
      (error) => {
        // A failed chunk download (network hiccup, or a stale chunk hash
        // after a redeploy) must not be cached: reset so the next editor
        // mount retries instead of failing until a full page reload.
        monacoBootstrap = null
        throw error
      }
    )
  }
  return monacoBootstrap
}

// Lazy component variants: each resolves only after ensureMonaco() has
// configured the loader, so Editor/DiffEditor never mount against an
// unconfigured (CDN-seeking) loader.
const LazyMonacoEditor = lazy(() =>
  ensureMonaco().then((binding) => ({ default: binding.default }))
)
const LazyMonacoDiffEditor = lazy(() =>
  ensureMonaco().then((binding) => ({ default: binding.DiffEditor }))
)

// MonacoLoadingFallback keeps the layout stable while the editor chunk is
// being fetched and instantiated.
function MonacoLoadingFallback() {
  return (
    <div className="flex h-full min-h-[120px] w-full items-center justify-center">
      <div className="h-4 w-4 animate-spin rounded-full border-2 border-gray-300 border-t-blue-600" />
    </div>
  )
}

// MonacoEditor / MonacoDiffEditor are drop-in replacements for the
// @monaco-editor/react components that include their own Suspense boundary,
// so callers do not need to change anything beyond the import source.
export function MonacoEditor(props: EditorProps) {
  return (
    <Suspense fallback={<MonacoLoadingFallback />}>
      <LazyMonacoEditor {...props} />
    </Suspense>
  )
}

export function MonacoDiffEditor(props: DiffEditorProps) {
  return (
    <Suspense fallback={<MonacoLoadingFallback />}>
      <LazyMonacoDiffEditor {...props} />
    </Suspense>
  )
}

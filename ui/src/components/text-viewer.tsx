import { useEffect, useRef, useState } from 'react'
// Type-only import: erased at compile time, keeps monaco out of this chunk.
import type { editor as monacoEditor } from 'monaco-editor'

import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useAppearance } from '@/components/appearance-provider'
// Lazily-loaded editor wrapper; the monaco bundle loads on first render.
import { MonacoEditor as Editor } from '@/lib/monaco-loader'

interface TextViewerProps {
  value: string
  title?: string
  className?: string
  fillHeight?: boolean
}

export function TextViewer({
  value,
  title = 'Text',
  className,
  fillHeight = false,
}: TextViewerProps) {
  const [editorValue, setEditorValue] = useState(value)
  const { actualTheme } = useAppearance()

  const editorRef = useRef<monacoEditor.IStandaloneCodeEditor | null>(null)

  // Update editor value when value prop changes
  useEffect(() => {
    setEditorValue(value)
  }, [value])

  const handleEditorDidMount = (editor: monacoEditor.IStandaloneCodeEditor) => {
    editorRef.current = editor
  }

  return (
    <Card className={className}>
      <CardHeader className="flex flex-row items-center justify-between">
        <div className="space-y-1">
          <CardTitle>{title}</CardTitle>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <div
            className={
              fillHeight
                ? 'h-full min-h-[24rem] overflow-hidden'
                : 'h-[calc(100vh-300px)] overflow-hidden'
            }
          >
            <Editor
              language="yaml"
              theme={actualTheme === 'dark' ? 'custom-dark' : 'custom-vs'}
              value={editorValue}
              beforeMount={(monaco) => {
                monaco.editor.defineTheme('custom-dark', {
                  base: 'vs-dark',
                  inherit: true,
                  rules: [],
                  colors: {
                    'editor.background': '#18181b',
                  },
                })
                monaco.editor.defineTheme('custom-vs', {
                  base: 'vs',
                  inherit: true,
                  rules: [],
                  colors: {
                    'editor.background': '#ffffff',
                  },
                })
              }}
              onMount={handleEditorDidMount}
              options={{
                readOnly: true,
                minimap: { enabled: false },
                scrollBeyondLastLine: false,
                automaticLayout: true,
                wordWrap: 'on',
                lineNumbers: 'on',
                folding: true,
                tabSize: 2,
                insertSpaces: true,
                fontSize: 14,
                fontFamily:
                  "'Maple Mono',Monaco, 'Cascadia Code', 'Roboto Mono', Consolas, 'Courier New', monospace",
                acceptSuggestionOnCommitCharacter: false,
                acceptSuggestionOnEnter: 'off',
                quickSuggestions: false,
                suggestOnTriggerCharacters: false,
                wordBasedSuggestions: 'off',
                // Disable unnecessary features for YAML editing
                parameterHints: { enabled: false },
                hover: { enabled: false },
                contextmenu: false,
                // Better scrolling behavior
                smoothScrolling: true,
                cursorSmoothCaretAnimation: 'on',
                multiCursorModifier: 'alt',
                accessibilitySupport: 'off',
                quickSuggestionsDelay: 500,
                links: false,
                colorDecorators: false,
              }}
            />
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

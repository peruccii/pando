import { Check, Copy } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import type { BundledLanguage, BundledTheme } from 'shiki'

interface ReactAICodeBlockProps {
  code: string
  language: BundledLanguage
  theme: BundledTheme
  className?: string
}

let shikiRuntimePromise: Promise<typeof import('shiki')> | null = null

export function ReactAICodeBlock({ code, language, theme, className }: ReactAICodeBlockProps) {
  const [html, setHtml] = useState('')
  const [isCopied, setIsCopied] = useState(false)
  const resetCopyTimeoutRef = useRef<number | null>(null)

  useEffect(() => {
    let cancelled = false
    if (!shikiRuntimePromise) {
      shikiRuntimePromise = import('shiki')
    }

    void shikiRuntimePromise
      .then((shiki) => {
        const diffTransformer: import('shiki').ShikiTransformer = {
          name: 'diff-transformer',
          line(node) {
            if (language !== 'diff') {
              return
            }
            // node.children[0] geralmente é o texto se não houver tokens complexos,
            // mas shiki quebra em tokens. Precisamos checar o texto "bruto" da linha ou o primeiro token.
            // Para simplificar, checaremos se a linha começa com os marcadores.
            // No entanto, 'line' é o número da linha. O conteúdo não vem fácil aqui sem acesso ao code split.
            // Shiki transformer 'line' recebe o AST da linha.
            // Vamos iterar sobre os tokens para achar o conteúdo textual inicial.
            
            let content = ''
            for (const child of node.children) {
              if (child.type === 'element' && child.children && child.children.length > 0) {
                 const textNode = child.children[0]
                 if (textNode.type === 'text') {
                   content += textNode.value
                 }
              }
              if (child.type === 'text') {
                content += child.value
              }
            }
            
            const trimmed = content.trimStart()
            
            if (trimmed.startsWith('+') && !trimmed.startsWith('+++')) {
              this.addClassToHast(node, 'diff-add')
            } else if (trimmed.startsWith('-') && !trimmed.startsWith('---')) {
              this.addClassToHast(node, 'diff-remove')
            } else if (trimmed.startsWith('@@')) {
              this.addClassToHast(node, 'diff-header')
            }
          },
        }

        return shiki.codeToHtml(code, {
          lang: language,
          theme,
          transformers: [diffTransformer],
        })
      })
      .then((nextHTML) => {
        if (!cancelled) {
          setHtml(nextHTML)
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHtml('')
        }
      })

    return () => {
      cancelled = true
    }
  }, [code, language, theme])

  useEffect(() => {
    return () => {
      if (resetCopyTimeoutRef.current !== null) {
        window.clearTimeout(resetCopyTimeoutRef.current)
      }
    }
  }, [])

  const handleCopy = async () => {
    if (!navigator?.clipboard?.writeText) {
      return
    }
    try {
      await navigator.clipboard.writeText(code)
      setIsCopied(true)
      if (resetCopyTimeoutRef.current !== null) {
        window.clearTimeout(resetCopyTimeoutRef.current)
      }
      resetCopyTimeoutRef.current = window.setTimeout(() => {
        setIsCopied(false)
      }, 1800)
    } catch {
      // noop: falha de clipboard nao deve quebrar render.
    }
  }

  const classes = ['git-panel-ai-code-block', className].filter(Boolean).join(' ')

  return (
    <div className={classes}>
      <button
        type="button"
        className="git-panel-ai-code-block__copy"
        onClick={() => { void handleCopy() }}
        aria-label={isCopied ? 'Código copiado' : 'Copiar código'}
        title={isCopied ? 'Copiado' : 'Copiar'}
      >
        {isCopied ? <Check size={13} /> : <Copy size={13} />}
      </button>

      {html ? (
        <div className="git-panel-ai-code-block__content" dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre className="git-panel-ai-code-block__fallback">
          <code>{code}</code>
        </pre>
      )}
    </div>
  )
}

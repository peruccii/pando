import { useEffect, useRef, useCallback, useState, useMemo } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebglAddon } from '@xterm/addon-webgl'
import { SearchAddon } from '@xterm/addon-search'
import '@xterm/xterm/css/xterm.css'
import { useLayoutStore } from '../stores/layoutStore'
import { useAppStore } from '../../../stores/appStore'
import { useWorkspaceStore } from '../../../stores/workspaceStore'
import { TERMINAL_THEMES } from '../types/layout'
import { getResumeCommand } from '../../../utils/cli-resume'
import './TerminalPane.css'

interface TerminalPaneProps {
  paneId: string
  isActive: boolean
}

interface RemoteCursor {
  userID: string
  userName: string
  userColor: string
  column: number
  row: number
  isTyping: boolean
  updatedAt: number
}

const MAX_TERMINAL_RING_BYTES = 64 * 1024

/**
 * TerminalPane — painel de terminal com xterm.js.
 * Integra PTY via Wails Events para streaming de I/O.
 */
export function TerminalPane({ paneId, isActive }: TerminalPaneProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const terminalRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const webglAddonRef = useRef<WebglAddon | null>(null)
  const sessionIDRef = useRef<string | null>(null)
  const eventOffsRef = useRef<Array<() => void>>([])
  const outputQueueRef = useRef<string[]>([])
  const queuedBytesRef = useRef(0)
  const flushTimerRef = useRef<number | null>(null)
  const historyAppliedRef = useRef(false)
  const preserveSessionOnUnmountRef = useRef(false)

  const decoderRef = useRef(new TextDecoder())
  const [isReady, setIsReady] = useState(false)
  const [remoteCursors, setRemoteCursors] = useState<Record<string, RemoteCursor>>({})
  const [containerWidth, setContainerWidth] = useState(0)

  const theme = useAppStore((s) => s.theme)
  const terminalFontSize = useAppStore((s) => s.terminalFontSize)
  const paneCount = useLayoutStore((s) => s.paneOrder.length)
  const setPaneSessionID = useLayoutStore((s) => s.setPaneSessionID)
  const updatePaneStatus = useLayoutStore((s) => s.updatePaneStatus)
  const pane = useLayoutStore((s) => s.panes[paneId])
  const historyByAgentId = useWorkspaceStore((s) => s.historyByAgentId)

  const isMinimized = pane?.isMinimized ?? false
  const isHighDensity = paneCount >= 10
  const agentHistoryBuffer = pane?.agentDBID ? (historyByAgentId[pane.agentDBID] || '') : ''

  const encodeInputToBase64 = useCallback((data: string): string => {
    const bytes = new TextEncoder().encode(data)
    const binaryString = Array.from(bytes, (byte) => String.fromCharCode(byte)).join('')
    return btoa(binaryString)
  }, [])

  const flushOutput = useCallback(() => {
    const terminal = terminalRef.current
    if (!terminal || outputQueueRef.current.length === 0) {
      return
    }

    const merged = outputQueueRef.current.join('')
    outputQueueRef.current = []
    queuedBytesRef.current = 0
    terminal.write(merged)
  }, [])

  const scheduleFlush = useCallback(() => {
    if (flushTimerRef.current !== null) {
      return
    }

    const delay = isActive ? 16 : isHighDensity ? 220 : 80
    flushTimerRef.current = window.setTimeout(() => {
      flushTimerRef.current = null
      flushOutput()
    }, delay)
  }, [flushOutput, isActive, isHighDensity])

  const enqueueOutput = useCallback((data: string) => {
    if (!data) return

    outputQueueRef.current.push(data)
    queuedBytesRef.current += data.length

    while (queuedBytesRef.current > MAX_TERMINAL_RING_BYTES && outputQueueRef.current.length > 0) {
      const shifted = outputQueueRef.current.shift() || ''
      queuedBytesRef.current -= shifted.length
    }

    if (isActive && !isHighDensity) {
      flushOutput()
      return
    }

    scheduleFlush()
  }, [flushOutput, isActive, isHighDensity, scheduleFlush])

  // Limpeza de timer de flush
  useEffect(() => {
    return () => {
      if (flushTimerRef.current !== null) {
        window.clearTimeout(flushTimerRef.current)
        flushTimerRef.current = null
      }
    }
  }, [])

  // Expira cursores remotos antigos
  useEffect(() => {
    const timer = window.setInterval(() => {
      const now = Date.now()
      setRemoteCursors((prev) => {
        const next: Record<string, RemoteCursor> = {}
        Object.entries(prev).forEach(([key, cursor]) => {
          if (now - cursor.updatedAt < 2500) {
            next[key] = cursor
          }
        })

        if (Object.keys(next).length === Object.keys(prev).length) {
          return prev
        }
        return next
      })
    }, 400)

    return () => window.clearInterval(timer)
  }, [])

  // Recebe cursor awareness remoto
  useEffect(() => {
    const handler = (event: Event) => {
      const payload = (event as CustomEvent<RemoteCursor>).detail
      if (!payload?.userID) return

      setRemoteCursors((prev) => ({
        ...prev,
        [payload.userID]: payload,
      }))
    }

    window.addEventListener('session:cursor-awareness:remote', handler)
    return () => window.removeEventListener('session:cursor-awareness:remote', handler)
  }, [])

  /** Inicializar terminal xterm.js */
  useEffect(() => {
    if (!containerRef.current) return

    const termTheme = TERMINAL_THEMES[theme] || TERMINAL_THEMES.dark

    const terminal = new Terminal({
      cursorBlink: true,
      cursorStyle: 'bar',
      fontFamily: "'JetBrains Mono', 'SF Mono', 'Fira Code', monospace",
      fontSize: terminalFontSize,
      lineHeight: 1.4,
      letterSpacing: 0,
      allowProposedApi: true,
      scrollback: 10000,
      theme: termTheme,
      allowTransparency: false,
      drawBoldTextInBrightColors: false,
    })

    const fitAddon = new FitAddon()
    const searchAddon = new SearchAddon()

    terminal.loadAddon(fitAddon)
    terminal.loadAddon(searchAddon)

    // Shift+Enter: força line feed sem enviar Enter normal.
    terminal.attachCustomKeyEventHandler((e) => {
      if (e.key === 'Enter' && e.shiftKey) {
        e.preventDefault()
        e.stopPropagation()

        // Escreve apenas no keydown para evitar duplicidade.
        if (e.type === 'keydown') {
          const sessionID = sessionIDRef.current
          if (sessionID) {
            const encoded = encodeInputToBase64('\n')
            window.go?.main?.App?.WriteTerminal(sessionID, encoded).catch(() => { })
          }

          window.dispatchEvent(new CustomEvent('session:shared-input:append', {
            detail: { input: '\n' },
          }))

          const row = terminal.buffer.active.cursorY
          const column = terminal.buffer.active.cursorX
          window.dispatchEvent(new CustomEvent('session:cursor-awareness:local', {
            detail: {
              row,
              column,
              isTyping: false,
            },
          }))
        }

        // Bloqueia também keypress/keyup para não entrar Enter padrão.
        return false
      }
      return true
    })

    terminal.open(containerRef.current)

    // Fit inicial com atraso mínimo para garantir que o container DOM esteja estável
    setTimeout(() => {
      if (fitAddonRef.current) {
        fitAddonRef.current.fit()
        if (containerRef.current) {
          setContainerWidth(containerRef.current.clientWidth)
        }
        const dims = fitAddonRef.current.proposeDimensions()
        createPTYSession(terminal, fitAddonRef.current, dims?.cols || 80, dims?.rows || 24)
      }
    }, 50)

    terminalRef.current = terminal
    fitAddonRef.current = fitAddon

    setIsReady(true)
    preserveSessionOnUnmountRef.current = Boolean(pane?.agentDBID)

    return () => {
      // Em panes ligados a AgentSession, o processo permanece vivo ao trocar de aba/workspace.
      if (!preserveSessionOnUnmountRef.current && sessionIDRef.current && window.go?.main?.App) {
        window.go.main.App.DestroyTerminal(sessionIDRef.current).catch(() => { })
      }

      // Cleanup listeners de eventos Wails
      eventOffsRef.current.forEach((off) => {
        try {
          off()
        } catch {
          // ignore
        }
      })
      eventOffsRef.current = []

      // Cleanup xterm addons
      searchAddon.dispose()
      fitAddon.dispose()
      terminal.dispose()

      terminalRef.current = null
      fitAddonRef.current = null
      webglAddonRef.current = null
      sessionIDRef.current = null
    }
  }, []) // Only run once on mount

  /** Criar sessão PTY e configurar streaming */
  const createPTYSession = useCallback(async (terminal: Terminal, fitAddon: FitAddon, cols?: number, rows?: number) => {
    // Verificar se temos acesso ao Wails
    if (!window.go?.main?.App) {
      // Dev mode — simular terminal
      terminal.write('\r\n  \x1b[1;36m⚡ ORCH Terminal\x1b[0m\r\n')
      terminal.write('  \x1b[90mDev mode — Wails backend não disponível\x1b[0m\r\n\r\n')
      terminal.write('  \x1b[33mInicie com `wails dev` para terminal real\x1b[0m\r\n\r\n')
      return
    }

    try {
      // Configurar modo (Docker vs Local) e CWD com base no snapshot de restauração se existir
      const snapshot = pane?.config?.restoreSnapshot // Type: TerminalSnapshotDTO
      const useDocker = snapshot?.useDocker ?? !!pane?.config?.useDocker
      const initialCwd = snapshot?.cwd || pane?.config?.cwd || ''
      // Local sem snapshot: backend resolve shell via Settings/auto-detect.
      // Docker sem snapshot: backend usa shell padrão do container.
      const initialShell = snapshot?.shell || ''

      // Criar ou reutilizar terminal no backend
      const sessionID = pane?.agentDBID && window.go?.main?.App?.CreateTerminalForAgent
        ? await window.go.main.App.CreateTerminalForAgent(pane.agentDBID, initialShell, initialCwd, useDocker, cols || 80, rows || 24)
        : await window.go?.main?.App?.CreateTerminal?.(initialShell, initialCwd, useDocker, cols || 80, rows || 24)
      if (!sessionID) throw new Error('Failed to create terminal session')

      sessionIDRef.current = sessionID
      setPaneSessionID(paneId, sessionID)
      updatePaneStatus(paneId, 'running')

      if (agentHistoryBuffer && !historyAppliedRef.current) {
        terminal.write(agentHistoryBuffer)
        historyAppliedRef.current = true
      }

      // Escutar output do terminal
      if (window.runtime) {
        const offOutput = window.runtime.EventsOn('terminal:output', (msg: { sessionID?: string; data?: string }) => {
          if (msg.sessionID === sessionID && msg.data) {
            try {
              // Decodificar base64 preservando UTF-8 via stream decoder
              const binaryString = atob(msg.data)
              const bytes = new Uint8Array(binaryString.length)
              for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i)
              }
              const decoded = decoderRef.current.decode(bytes, { stream: true })
              enqueueOutput(decoded)
            } catch (err) {
              console.error('[Terminal] Decode error:', err)
              enqueueOutput(msg.data)
            }
          }
        })
        eventOffsRef.current.push(offOutput)

        const offAIChunk = window.runtime.EventsOn('ai:response:chunk', (msg: { sessionID?: string; chunk?: string }) => {
          if (msg?.sessionID === sessionID && msg?.chunk) {
            enqueueOutput(msg.chunk)
          }
        })
        eventOffsRef.current.push(offAIChunk)

        const offAIDone = window.runtime.EventsOn('ai:response:done', (msg: { sessionID?: string }) => {
          if (msg?.sessionID === sessionID) {
            enqueueOutput('\r\n')
          }
        })
        eventOffsRef.current.push(offAIDone)
      }

      // Enviar input do terminal para o backend
      terminal.onData((data: string) => {
        if (sessionIDRef.current) {
          const encoded = encodeInputToBase64(data)

          window.go?.main?.App?.WriteTerminal(sessionIDRef.current, encoded).catch((err: Error) => {
            console.error('[Terminal] Write error:', err)
          })
        }

        window.dispatchEvent(new CustomEvent('session:shared-input:append', {
          detail: { input: data },
        }))

        const row = terminal.buffer.active.cursorY
        const column = terminal.buffer.active.cursorX
        window.dispatchEvent(new CustomEvent('session:cursor-awareness:local', {
          detail: {
            row,
            column,
            isTyping: data.trim().length > 0,
          },
        }))
      })

      // Enviar resize
      terminal.onResize(({ cols, rows }) => {
        if (sessionIDRef.current) {
          window.go?.main?.App?.ResizeTerminal(sessionIDRef.current, cols, rows).catch(() => { })
        }
      })

      // Fit inicial com as dimensões corretas
      requestAnimationFrame(() => {
        fitAddon.fit()
      })

      // Se havia uma CLI ativa no snapshot, enviar comando de resume
      if (snapshot?.cliType) {
        setTimeout(async () => {
          const resumeCmd = getResumeCommand(snapshot.cliType || '')
          if (resumeCmd && sessionIDRef.current === sessionID) {
            console.log(`[Terminal] Resuming CLI session: ${resumeCmd}`)
            const encoded = btoa(unescape(encodeURIComponent(resumeCmd + '\n')))
            try {
              await window.go?.main?.App?.WriteTerminal?.(sessionID, encoded)
            } catch (err) {
              console.error('[Terminal] Failed to send resume command:', err)
            }
          }
        }, 800) // Delay para garantir shell pronto
      }

    } catch (err) {
      console.error('[Terminal] Failed to create PTY session:', err)
      terminal.write('\r\n  \x1b[31mErro ao criar sessão de terminal\x1b[0m\r\n')
      updatePaneStatus(paneId, 'error')
    }
  }, [agentHistoryBuffer, encodeInputToBase64, enqueueOutput, pane, paneId, setPaneSessionID, updatePaneStatus])

  /** ResizeObserver para refit automático */
  useEffect(() => {
    if (!containerRef.current || !fitAddonRef.current) return

    const observer = new ResizeObserver(() => {
      requestAnimationFrame(() => {
        try {
          fitAddonRef.current?.fit()
          if (containerRef.current) {
            setContainerWidth(containerRef.current.clientWidth)
          }
        } catch {
          // Ignore fit errors during transitions
        }
      })
    })

    observer.observe(containerRef.current)

    return () => observer.disconnect()
  }, [isReady])

  /** Atualizar tema do terminal quando o tema do app muda */
  useEffect(() => {
    if (!terminalRef.current) return
    const termTheme = TERMINAL_THEMES[theme] || TERMINAL_THEMES.dark
    terminalRef.current.options.theme = termTheme
  }, [theme])

  /** Atualizar font size do terminal (zoom) */
  useEffect(() => {
    if (!terminalRef.current) return
    terminalRef.current.options.fontSize = terminalFontSize
    requestAnimationFrame(() => {
      fitAddonRef.current?.fit()
      if (containerRef.current) {
        setContainerWidth(containerRef.current.clientWidth)
      }
    })
  }, [terminalFontSize])

  /** Focar o terminal quando o painel fica ativo */
  useEffect(() => {
    if (isActive && terminalRef.current) {
      requestAnimationFrame(() => {
        terminalRef.current?.focus()
      })
    }
  }, [isActive])

  // Flush imediato ao ativar painel
  useEffect(() => {
    if (isActive) {
      flushOutput()
    }
  }, [flushOutput, isActive])

  // Reidrata histórico do terminal para evitar tela preta ao trocar de workspace/aba.
  useEffect(() => {
    if (!terminalRef.current) return
    if (!agentHistoryBuffer) return
    if (historyAppliedRef.current) return

    terminalRef.current.write(agentHistoryBuffer)
    historyAppliedRef.current = true
  }, [agentHistoryBuffer])

  const visibleRemoteCursors = useMemo(() => {
    return Object.values(remoteCursors).slice(0, 6)
  }, [remoteCursors])

  return (
    <div
      className={`terminal-pane ${isActive ? 'terminal-pane--active' : 'terminal-pane--inactive'}`}
      id={`terminal-${paneId}`}
      style={isMinimized ? { display: 'none' } : undefined}
    >
      <div
        ref={containerRef}
        className="terminal-pane__container"
      />

      {visibleRemoteCursors.map((cursor) => {
        const left = Math.max(8, Math.min(containerWidth - 80, 8 + cursor.column * 8))
        return (
          <div
            key={cursor.userID}
            className="terminal-pane__remote-cursor"
            style={{ left, borderColor: cursor.userColor }}
            title={`${cursor.userName}${cursor.isTyping ? ' (typing...)' : ''}`}
          >
            <span className="terminal-pane__remote-cursor-label" style={{ backgroundColor: cursor.userColor }}>
              {cursor.userName}{cursor.isTyping ? '...' : ''}
            </span>
          </div>
        )
      })}
    </div>
  )
}

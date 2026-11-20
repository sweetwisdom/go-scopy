// Adapted from escrcpy; Apache-2.0. See sources.json and licenses/escrcpy-Apache-2.0.txt.
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import * as themes from './theme.js'
import { ref, shallowRef, computed, watch, nextTick, onMounted, onBeforeUnmount } from 'vue'

export function useTerminal({ theme = 'github', themeStore, config, bridge, active }) {
  const terminalRef = ref(null)
  const terminal = shallowRef(null)
  const fitAddon = shallowRef(null)
  const sessionId = ref(null)
  const connected = ref(false)
  const newline = config.platform === 'windows' ? '\r\n' : '\n'
  const disposeCallbacks = ref(null)
  let unwatchTheme = null
  let resizeTimer = null
  let fitFrame = 0
  let disposed = false
  const sessionStatus = ref('')

  const terminalConfig = computed(() => config)
  const platform = { darwin: 'macos', windows: 'windows', win32: 'windows' }[config.platform] || config.platform

  function getCurrentTheme() {
    const isDark = themeStore.isDark
    return themes[theme][isDark ? 'dark' : 'light']
  }

  function handleInput(val) {
    if (!sessionId.value || !active() || config.mode !== 'shell') {
      return false
    }

    // xterm emits CR for Enter/paste. Pipe-based adb on Windows needs LF
    // as a delimiter; preserve control sequences and existing CRLF intact.
    const command = config.platform === 'windows' ? val.replace(/\r(?!\n)/g, newline) : val

    if (['exit', 'error'].includes(sessionStatus.value)) {
      return false
    }

    bridge.write(sessionId.value, command).catch(() => {})
  }

  function handleResize({ cols, rows }) {
    if (!sessionId.value)
      return
    clearTimeout(resizeTimer)
    resizeTimer = setTimeout(() => {
      bridge.resize(sessionId.value, cols, rows)
    }, 16)
  }

  function fit() {
    cancelAnimationFrame(fitFrame)
    fitFrame = requestAnimationFrame(() => {
      if (disposed || !terminalRef.value?.clientWidth || !terminalRef.value?.clientHeight) return
      fitAddon.value?.fit()
      if (terminal.value) handleResize({ cols: terminal.value.cols, rows: terminal.value.rows })
    })
  }

  async function initTerminal() {
    disposeCallbacks.value?.()
    disposeCallbacks.value = null

    const platformFontMap = {
      windows: '"Cascadia Mono", "Consolas", "Courier New", monospace',
      macos: '"Menlo", "Monaco", "Courier New", monospace',
      linux: 'Monospace',
    }

    const fontFamily = platformFontMap[platform] || 'Monospace'

    terminal.value = new Terminal({
      fontFamily,
      fontSize: 14,
      lineHeight: 1.4,
      cursorBlink: true,
      cursorStyle: 'bar',
      cursorWidth: 2,
      convertEol: true,
      allowTransparency: true,
      scrollback: 5000,
      smoothScrollDuration: 150,
      macOptionIsMeta: true,
      theme: getCurrentTheme(),
    })

    fitAddon.value = new FitAddon()
    terminal.value.loadAddon(fitAddon.value)
    terminal.value.open(terminalRef.value)
    terminal.value.clear()
    terminal.value.element.addEventListener('focus', () => terminal.value.textarea?.focus())

    await nextTick()
    fit()
    const resizeObserver = new ResizeObserver(fit)
    resizeObserver.observe(terminalRef.value)
    const unwatchActive = watch(active, enabled => {
      terminal.value.options.disableStdin = !enabled || config.mode !== 'shell'
      if (enabled) nextTick(() => { fit(); if (config.mode === 'shell') terminal.value?.focus() })
    }, { immediate: true })
    disposeCallbacks.value = () => { resizeObserver.disconnect(); unwatchActive(); cancelAnimationFrame(fitFrame) }

    terminal.value.onData(handleInput)
    terminal.value.onResize(handleResize)

    unwatchTheme?.()
    unwatchTheme = watch(
      () => themeStore.isDark,
      () => {
        terminal.value.options.theme = getCurrentTheme()
      },
      { immediate: true },
    )

    if (active() && config.mode === 'shell') terminal.value.focus()
  }

  async function connectSession() {
    try {
      const actualDimensions = { cols: terminal.value.cols, rows: terminal.value.rows }
      const result = await bridge.create({
        type: terminalConfig.value.type,
        instanceId: terminalConfig.value.instanceId,
        options: { deviceId: config.serial, ...actualDimensions },
        mode: config.mode,
        onData: (data, ack) => {
          sessionStatus.value = 'data'
          if (terminal.value) terminal.value.write(data, ack)
          else ack()
        },
        onExit: (code) => {
          sessionStatus.value = 'exit'
          connected.value = false
          terminal.value?.writeln(`${newline}\x1B[33mProcess exited with code ${code}\x1B[0m`)
        },
        onError: (err) => {
          sessionStatus.value = 'error'
          connected.value = false
          terminal.value?.writeln(`${newline}\x1B[31mError: ${err.message || err}\x1B[0m`)
        },
      })

      if (!result.success) {
        terminal.value.writeln(`${newline}\x1B[31mError: ${result.error}\x1B[0m`)
        return
      }

      if (disposed) { bridge.destroy(result.sessionId); result.dispose?.(); return }
      sessionId.value = result.sessionId

      if (result.dispose) {
        const prevDispose = disposeCallbacks.value
        disposeCallbacks.value = () => {
          prevDispose?.()
          result.dispose()
        }
      }

      connected.value = !['exit', 'error'].includes(sessionStatus.value)


    }
    catch (error) {
      sessionStatus.value = 'error'
      connected.value = false
      if (!disposed) terminal.value?.writeln(`${newline}\x1B[31mFailed to connect: ${error.message}\x1B[0m`)
    }
  }

  function cleanup() {
    if (disposed) return
    disposed = true
    unwatchTheme?.()
    clearTimeout(resizeTimer)
    sessionId.value && bridge.destroy(sessionId.value)
    disposeCallbacks.value?.()
    disposeCallbacks.value = null
    terminal.value?.dispose()
    terminal.value = null
  }

  onMounted(async () => {
    await initTerminal()
    if (disposed) return
    await connectSession()
    if (!disposed) window.addEventListener('beforeunload', cleanup)
  })

  onBeforeUnmount(() => {
    cleanup()
    window.removeEventListener('beforeunload', cleanup)
  })

  return { terminalRef, terminalConfig, terminal, connected, sessionStatus, sessionId, cleanup, handleInput, fit }
}

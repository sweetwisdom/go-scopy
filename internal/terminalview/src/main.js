import { createApp, defineComponent, ref, reactive, watch, h, nextTick } from 'vue'
import { useTerminal } from './use-terminal.js'
import { deviceBridge, setContextHandler } from './bridge.js'
import './workspace.css'

const context = reactive({ serial: '', name: '', theme: 'light', platform: '', online: false, active: false })
const themeStore = reactive({ isDark: false })
const mode = ref('shell')
const showCommands = ref(false)
const message = ref(null)
let messageTimer
function dismissMessage() {
  clearTimeout(messageTimer)
  message.value = null
}
function showMessage(text, type = 'error') {
  dismissMessage()
  message.value = { text, type }
  messageTimer = setTimeout(dismissMessage, 3000)
}
const panes = reactive([])
let epoch = 0
const help = [
  { title: '设备型号', shell: 'getprop ro.product.model', note: '读取设备型号' },
  { title: '系统版本', shell: 'getprop ro.build.version.release', note: '读取 Android 版本' },
  { title: '应用列表', shell: 'pm list packages -3', note: '列出第三方应用包名' },
  { title: '前台应用', shell: 'dumpsys activity activities', note: '查看 Activity 与任务信息' },
  { title: '屏幕尺寸', shell: 'wm size', note: '读取物理与覆盖分辨率' },
  { title: '屏幕密度', shell: 'wm density', note: '读取设备 DPI' },
  { title: '电池状态', shell: 'dumpsys battery', note: '查看电量、温度与供电状态' },
  { title: '存储空间', shell: 'df -h /sdcard', note: '查看共享存储用量' },
]
function ensurePane() {
  if (!context.serial || !context.online || !context.active) return
  if (!panes.some(p => p.serial === context.serial && p.mode === mode.value))
    panes.push({ serial: context.serial, mode: mode.value, platform: context.platform, epoch: ++epoch })
}
setContextHandler(value => {
  Object.assign(context, value)
  themeStore.isDark = context.theme === 'dark'
  document.documentElement.dataset.theme = context.theme
  ensurePane()
})
watch(mode, ensurePane)

const TerminalPane = defineComponent({
  props: ['config'],
  setup(props) {
    const config = props.config
    const active = () => context.active && context.online && context.serial === config.serial && mode.value === config.mode
    const term = useTerminal({ themeStore, config, bridge: deviceBridge(config.serial), active })
    // The textarea remains xterm.js-owned (IME, paste, control keys, selection).
    return () => h('section', { class: 'session', style: { display: active() ? 'flex' : 'none' }, 'data-mode': config.mode, 'data-session': term.sessionId.value || '', 'data-connected': String(term.connected.value) }, [
      h('div', { class: 'terminal-surface', ref: term.terminalRef, 'aria-label': config.mode === 'shell' ? '设备 Shell 终端' : '设备 Logcat 日志' }),
      h('footer', { class: 'session-footer' }, [
        h('span', { class: ['session-dot', { online: term.connected.value }] }),
        h('span', term.connected.value ? (config.mode === 'shell' ? '已连接 · Android Shell' : '实时日志 · 只读') : (term.sessionStatus.value === 'exit' ? '会话已结束 · 可重新连接' : term.sessionStatus.value === 'error' ? '连接异常 · 可重新连接' : '正在连接…')),
        h('span', { class: 'spacer' }),
        h('span', { class: 'key-hint' }, config.mode === 'shell' ? 'Ctrl+C 中断 · Ctrl+Shift+C 复制' : '滚动查看历史'),
      ]),
    ])
  },
})

function reconnect() {
  dismissMessage()
  const index = panes.findIndex(p => p.serial === context.serial && p.mode === mode.value)
  if (index >= 0) panes.splice(index, 1)
  nextTick(ensurePane)
}
async function copy(text) {
  try {
    // WebView2 custom schemes are secure contexts; retain an offline fallback.
    if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(text)
    else {
      const input = document.createElement('textarea'); input.value = text; document.body.append(input); input.select()
      try { if (!document.execCommand('copy')) throw new Error('系统剪贴板不可用，请手动选择命令复制') }
      finally { input.remove() }
    }
    showMessage('已复制命令，粘贴后按回车执行', 'success')
  } catch (e) { showMessage(e?.message || '复制失败，请重试') }
}

createApp({
  setup() {
    return () => h('main', { class: 'workspace' }, [
      h('header', { class: 'toolbar' }, [
        h('div', { class: 'tabs', role: 'tablist', 'aria-label': '终端类型' }, ['shell', 'logcat'].map(key => h('button', { class: ['tab', { selected: mode.value === key }], role: 'tab', 'aria-selected': mode.value === key, onClick: () => { mode.value = key } }, key === 'shell' ? 'Shell' : 'Logcat'))),
        h('span', { class: 'device-label', title: context.serial }, context.serial || '设备终端'),
        h('span', { class: 'spacer' }),
        h('button', { class: 'tool', disabled: !context.online, title: '结束当前会话并重新连接', onClick: reconnect }, '重新连接'),
        h('button', { class: ['tool', { selected: showCommands.value }], 'aria-expanded': showCommands.value, onClick: () => { showCommands.value = !showCommands.value } }, '常用命令'),
      ]),
      h('div', { class: 'workspace-body' }, [
        h('div', { class: 'terminal-stack' }, [
          ...panes.map(config => h(TerminalPane, { key: config.epoch, config })),
          !context.online ? h('div', { class: 'empty' }, [h('strong', '设备未连接'), h('span', '连接设备后即可打开终端，已有输出会保留。')]) : null,
        ]),
        showCommands.value ? h('aside', { class: 'commands', 'aria-label': '常用 ADB 命令' }, [
          h('div', { class: 'commands-heading' }, [h('div', [h('span', { class: 'eyebrow' }, 'QUICK REFERENCE'), h('h2', '常用命令')]), h('button', { class: 'close', 'aria-label': '收起常用命令', onClick: () => { showCommands.value = false } }, '×')]),
          h('p', { class: 'commands-note' }, '这里已进入设备 Shell，无需再加 adb shell。复制不会自动执行。'),
          ...help.map(item => h('article', { class: 'command' }, [h('div', { class: 'command-title' }, [h('strong', item.title), h('button', { class: 'copy', 'aria-label': `复制${item.title}命令`, onClick: () => copy(item.shell) }, '复制')]), h('code', item.shell), h('p', item.note)])),
          h('p', { class: 'commands-note' }, '电脑端连接、安装、推送命令需在电脑终端执行，不能直接在此执行 adb / fastboot。'),
        ]) : null,
      ]),
      h('div', { class: 'message-region', role: 'status', 'aria-live': 'polite', 'aria-atomic': 'true' }, [
        message.value ? h('div', { class: ['toast', `toast-${message.value.type}`], onClick: dismissMessage }, [
          h('span', { class: 'toast-icon', 'aria-hidden': 'true' }, message.value.type === 'success' ? '✓' : '!'),
          h('span', message.value.text),
        ]) : null,
      ]),
    ])
  },
}).mount('#app')
window.addEventListener('unhandledrejection', event => { showMessage(event.reason?.message || '终端请求失败'); event.preventDefault() })
window.viva.postMessage(JSON.stringify({ action: 'ready' }))

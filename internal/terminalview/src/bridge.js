// Only the narrow, device-scoped Go bridge is exposed to the offline page.
const pending = new Map()
const sessions = new Map()
const early = new Map()
let next = 0
export let onContext = () => {}
export function setContextHandler(fn) { onContext = fn }
export function request(action, values = {}) {
  const id = ++next
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('设备请求超时，请重试')) }, 8000)
    pending.set(id, { resolve, reject, timer })
    try { window.viva.postMessage(JSON.stringify({ id, action, ...values })) }
    catch (err) { clearTimeout(timer); pending.delete(id); reject(err) }
  })
}
function deliver(message, callbacks) {
  if (message.type === 'data') {
    const data = Uint8Array.from(atob(message.data), c => c.charCodeAt(0))
    callbacks.onData(data, () => window.viva.postMessage(JSON.stringify({ action: 'ack', session: message.session })))
  } else if (message.type === 'exit') {
    callbacks.onExit(message.code)
    if (message.error) callbacks.onError({ message: message.error })
  } else if (message.type === 'input-error') callbacks.onError({ message: message.error })
}
window.viva.onmessage = raw => {
  let m
  try { m = JSON.parse(typeof raw === 'string' ? raw : raw.data) } catch { return }
  if (m.type === 'context') { onContext(m); return }
  if (m.type === 'response') {
    const p = pending.get(m.id)
    if (p) { pending.delete(m.id); clearTimeout(p.timer); m.error ? p.reject(new Error(m.error)) : p.resolve(m.data) }
    return
  }
  const callbacks = sessions.get(m.session)
  if (callbacks) deliver(m, callbacks)
  else if (m.session && (early.has(m.session) || early.size < 8) && (m.type === 'data' || m.type === 'exit')) {
    // Creation response and first output can arrive in one UI-thread turn.
    const queue = early.get(m.session) || []
    if (queue.length < 3) queue.push(m)
    early.set(m.session, queue)
  }
}
export function deviceBridge(serial) {
  return {
    async create(options) {
      const result = await request('create', { serial, mode: options.mode })
      sessions.set(result.sessionId, options)
      for (const m of early.get(result.sessionId) || []) deliver(m, options)
      early.delete(result.sessionId)
      return { ...result, success: true, dispose: () => { sessions.delete(result.sessionId); early.delete(result.sessionId) } }
    },
    write(session, data) {
      return request('write', { serial, session, data }).catch(error => {
        sessions.get(session)?.onError({ message: error.message })
        throw error
      })
    },
    resize() {}, // Remote PTY sizing matches escrcpy's pipe-based provider.
    destroy(session) {
      sessions.delete(session); early.delete(session)
      return request('destroy', { serial, session }).catch(() => {})
    },
  }
}

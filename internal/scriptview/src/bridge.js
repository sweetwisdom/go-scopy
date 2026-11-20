let sequence = 0
const pending = new Map()
let contextHandler = () => {}
export function onContext(handler) { contextHandler = handler }
export function notify(action, data = {}) { window.viva.postMessage(JSON.stringify({ action, ...data })) }
export function request(action, data = {}) {
  const id = ++sequence
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => { pending.delete(id); reject(new Error('请求超时，请重试')) }, action === 'open' || action === 'python' ? 300000 : 30000)
    pending.set(id, { resolve, reject, timer })
    notify(action, { ...data, id })
  })
}
window.viva.onmessage = event => {
  let message
  try { message = JSON.parse(typeof event === 'string' ? event : event.data) } catch { return }
  if (message.type === 'context') { contextHandler(message); return }
  const item = pending.get(message.id)
  if (!item) return
  clearTimeout(item.timer); pending.delete(message.id)
  message.error ? item.reject(new Error(message.error)) : item.resolve(message.data)
}

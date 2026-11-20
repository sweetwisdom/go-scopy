import test from 'node:test'
import assert from 'node:assert/strict'

const sent = []
globalThis.window = { viva: { postMessage(raw) { sent.push(JSON.parse(raw)) } } }
const { deviceBridge, request, setContextHandler } = await import('./bridge.js')
function deliver(message, event = true) {
 const raw = JSON.stringify(message)
 window.viva.onmessage(event ? { data: raw } : raw)
}

test('native MessageEvent and raw context strings both work', () => {
 const contexts = []
 setContextHandler(value => contexts.push(value))
 deliver({ type: 'context', serial: 'device-a' })
 deliver({ type: 'context', serial: 'device-b' }, false)
 assert.deepEqual(contexts.map(c => c.serial), ['device-a', 'device-b'])
})

test('creation response/early byte output and ACK preserve UTF-8', async () => {
 const bridge = deviceBridge('device-a')
 const chunks = []
 let exits = 0
 const session = bridge.create({ mode: 'shell', onData(data,ack) { chunks.push(data); ack() }, onExit() { exits++ }, onError() {} })
 const create = sent.at(-1)
 assert.equal(create.action, 'create')
 assert.equal(create.serial, 'device-a')
 deliver({ type: 'response', id: create.id, data: { sessionId: 'test-1' } })
 const utf8 = Buffer.from('中文输出')
 deliver({ type: 'data', session: 'test-1', data: utf8.subarray(0,4).toString('base64') })
 const result = await session
 deliver({ type: 'data', session: 'test-1', data: utf8.subarray(4).toString('base64') })
 assert.equal(Buffer.concat(chunks).toString(), '中文输出')
 assert.equal(sent.at(-1).action, 'ack')
 const write = bridge.write(result.sessionId, '\x03')
 const m = sent.at(-1)
 assert.equal(m.data, '\x03')
 assert.equal(m.session, 'test-1')
 deliver({ type: 'response', id: m.id, data: null, error: '' })
 await write
 deliver({ type: 'exit', session: 'test-1', code: 0 })
 assert.equal(exits, 1)
 const destroy = bridge.destroy('test-1')
 deliver({ type: 'response', id: sent.at(-1).id, data: null })
 await destroy
 result.dispose()
})

test('device errors reject requests instead of pretending commands succeeded', async () => {
 const value = request('create', { serial: 'offline', mode: 'shell' })
 deliver({ type: 'response', id: sent.at(-1).id, error: '设备已离线' })
 await assert.rejects(value, /设备已离线/)
})

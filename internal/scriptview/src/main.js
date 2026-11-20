import { createEditor } from './editor.js'
import { notify, request, onContext } from './bridge.js'
import { templates, activeRun, runLabel, canRun } from './model.js'
import './workspace.css'

const $ = id => document.getElementById(id)
let documentFile = null, savedText = '', dirty = false, busy = false, loading = false
let files = [], devices = [], runs = [], target = '', runID = '', active = false, initialized = false, listed = false, dark = false
let messageTimer, lastError = '', deviceOptionsKey = '', historyOptionsKey = ''
function message(text, error = false) {
  clearTimeout(messageTimer)
  const card = document.createElement('div'); card.className = error ? 'message error' : 'message'; card.textContent = `${error ? '!' : '✓'}  ${text}`
  card.onclick = () => $('message-region').replaceChildren()
  $('message-region').replaceChildren(card)
  messageTimer = setTimeout(() => $('message-region').replaceChildren(), error ? 6000 : 3000)
}
const editor = createEditor($('editor'), {
  onChange() {
    if (loading) return
    $('trusted').checked = false
    updateDirty(); renderControls()
  },
  onPosition(line, column) { $('position').textContent = `行 ${line}，列 ${column}` },
  onSave() { if (documentFile && !busy) attempt(save) },
})
function updateDirty() {
  dirty = !!documentFile && editor.view.state.doc.toString() !== savedText
  notify('dirty', { dirty })
  $('filename').textContent = documentFile ? `${documentFile.name}${dirty ? ' · 未保存' : ''}` : '选择或新建一个脚本'
}
function renderControls() {
  $('save').disabled = !documentFile || busy || !active
  $('run').disabled = !canRun({ path: documentFile?.path, target, devices, trusted: $('trusted').checked, busy, runs, active })
  $('run').textContent = dirty ? '▶ 保存并运行' : '▶ 运行'
  const current = runs.find(r => r.id === runID)
  $('stop').disabled = !activeRun(current) || current?.status === 'stopping' || busy || !active
  $('output-folder').disabled = !current || busy || !active
  for (const id of ['new', 'open', 'refresh', 'folder', 'guide', 'choose-python', 'empty-new']) $(id).disabled = busy || !active
}
async function attempt(fn) {
  if (busy || !active) return
  busy = true; renderControls()
  try { await fn() } catch (error) { message(error.message || '操作失败', true) }
  finally { busy = false; renderControls() }
}
async function refresh() {
  const listing = await request('list')
  listed = true; files = listing.files; $('directory').textContent = listing.directory; $('directory').title = listing.directory
  renderFiles()
}
function renderFiles() {
  const query = $('search').value.toLocaleLowerCase()
  const list = files.filter(file => file.name.toLocaleLowerCase().includes(query))
  const nodes = list.map(file => {
    const button = document.createElement('button'); button.className = `file${file.path === documentFile?.path ? ' selected' : ''}`; button.title = file.path
    const icon = document.createElement('span'); icon.className = 'file-type'; icon.textContent = 'PY'
    const info = document.createElement('span'); info.className = 'file-info'
    const title = document.createElement('strong'); title.textContent = file.name
    const note = document.createElement('small'); note.textContent = file.external ? '外部文件 · 原位保存' : '本地脚本'
    info.append(title, note); button.append(icon, info)
    button.onclick = () => attempt(async () => {
      if (file.path === documentFile?.path && !dirty) return
      if (await leaveDocument()) load(await request('read', { path: file.path }))
    })
    return button
  })
  if (!nodes.length) { const empty = document.createElement('p'); empty.className = 'library-empty'; empty.textContent = query ? '没有匹配的脚本' : '还没有脚本，点击“新建”开始。'; nodes.push(empty) }
  $('files').replaceChildren(...nodes)
}
function load(doc) {
  loading = true; documentFile = doc
  editor.load(doc.content); savedText = editor.view.state.doc.toString(); editor.setEditable(true)
  loading = false; $('empty').hidden = true; $('filepath').textContent = doc.path; $('filepath').title = doc.path
  $('trusted').checked = false; updateDirty(); renderFiles(); renderControls(); editor.view.focus()
}
async function save() {
  if (!documentFile) return
  const content = editor.view.state.doc.toString()
  const doc = await request('save', { path: documentFile.path, version: documentFile.version, content })
  documentFile = doc; savedText = content; updateDirty(); message('脚本已保存到本地文件')
}
async function leaveDocument() {
  if (!dirty) return true
  const dialog = $('dirty-dialog')
  dialog.showModal()
  const choice = await new Promise(resolve => {
    let settled = false
    const finish = value => { if (settled) return; settled = true; dialog.close(); dialog.oncancel = null; resolve(value) }
    $('dirty-cancel').onclick = () => finish('cancel')
    $('dirty-discard').onclick = () => finish('discard')
    $('dirty-save').onclick = () => finish('save')
    dialog.oncancel = event => { event.preventDefault(); finish('cancel') }
  })
  if (choice === 'save') { await save(); return !dirty }
  return choice === 'discard'
}
function renderRuns() {
  if (!runs.some(r => r.id === runID)) runID = runs[0]?.id || ''
  const selected = runs.find(r => r.id === runID)
  const optionsKey = JSON.stringify(runs.map(run => [run.id, run.status, run.started, run.request.path, run.request.serial]))
  if (historyOptionsKey !== optionsKey) {
    historyOptionsKey = optionsKey
    const options = runs.map(run => {
      const option = document.createElement('option'); option.value = run.id
      const time = new Date(run.started).toLocaleTimeString('zh-CN', { hour12: false })
      option.textContent = `${time} · ${run.request.path.split(/[\\/]/).pop()} · ${run.request.serial} · ${runLabel(run.status)}`
      return option
    })
    if (!options.length) { const option = document.createElement('option'); option.value = ''; option.textContent = '暂无执行记录'; options.push(option) }
    $('history').replaceChildren(...options)
  }
  $('history').value = runID
  $('run-status').textContent = selected ? `${runLabel(selected.status)}${activeRun(selected) ? '' : ` · 退出码 ${selected.exitCode}`}` : '尚未执行'
  $('run-status').dataset.status = selected?.status || ''
  const text = selected ? `${selected.log || ''}${selected.error ? `\n[错误] ${selected.error}` : ''}` : '运行结果将在这里显示。选择设备并确认脚本可信后运行。'
  const output = $('log'), atBottom = output.scrollHeight - output.scrollTop - output.clientHeight < 40
  if (output.textContent !== text) { output.textContent = text; if (atBottom) output.scrollTop = output.scrollHeight }
}
function renderDevices() {
  const optionsKey = JSON.stringify(devices)
  if (deviceOptionsKey !== optionsKey) {
    deviceOptionsKey = optionsKey
    const options = [Object.assign(document.createElement('option'), { value: '', textContent: devices.length ? '请选择已连接设备' : '暂无已连接设备' })]
    for (const device of devices) options.push(Object.assign(document.createElement('option'), { value: device.serial, textContent: `${device.name} · ${device.serial}` }))
    $('device').replaceChildren(...options)
  }
  $('device').value = target
}
onContext(context => {
  active = context.active
  if (dark !== (context.theme === 'dark')) { dark = context.theme === 'dark'; editor.setTheme(dark) }
  document.documentElement.dataset.theme = context.theme
  devices = context.devices || []; runs = context.runs || []
  if (target && !devices.some(d => d.serial === target)) target = ''
  if (!initialized) {
    initialized = true
    target = devices.some(d => d.serial === context.selected) ? context.selected : ''
    $('python').value = context.last?.python || ''; $('timeout').value = context.last?.timeoutSeconds || 600
  }
  if (active && !listed && !busy) attempt(refresh)
  renderDevices(); renderRuns(); renderControls()
  if (context.error && context.error !== lastError) message(context.error, true)
  lastError = context.error
})
$('search').oninput = renderFiles
$('device').onchange = () => { target = $('device').value; renderControls() }
$('trusted').onchange = renderControls
$('history').onchange = () => { runID = $('history').value; renderRuns(); renderControls() }
$('refresh').onclick = () => attempt(refresh)
$('save').onclick = () => attempt(save)
$('folder').onclick = () => attempt(() => request('folder'))
$('guide').onclick = () => attempt(() => request('guide'))
$('choose-python').onclick = () => attempt(async () => { const path = await request('python'); if (path) $('python').value = path })
$('open').onclick = () => attempt(async () => { if (!await leaveDocument()) return; const doc = await request('open'); if (doc) { load(doc); await refresh() } })
const newScript = () => attempt(async () => { if (await leaveDocument()) { $('new-error').textContent = ''; $('new-dialog').showModal(); $('new-name').focus(); $('new-name').select() } })
$('new').onclick = newScript; $('empty-new').onclick = newScript
$('new-cancel').onclick = () => $('new-dialog').close()
$('new-form').onsubmit = event => {
  event.preventDefault()
  attempt(async () => {
    try {
      const doc = await request('new', { name: $('new-name').value, content: templates[$('template').value] })
      $('new-dialog').close(); load(doc); await refresh(); message('已创建本地脚本')
    } catch (error) { $('new-error').textContent = error.message }
  })
}
$('run').onclick = () => attempt(async () => {
  if (dirty) await save()
  if (dirty || !$('trusted').checked) throw new Error('脚本有新修改，请保存并重新确认信任后执行')
  const timeout = Number($('timeout').value)
  if (!Number.isInteger(timeout) || timeout < 1 || timeout > 86400) throw new Error('超时应为 1–86400 秒')
  runID = await request('run', { path: documentFile.path, version: documentFile.version, serial: target, python: $('python').value.trim(), timeout, trusted: true })
  message('脚本已启动')
})
$('stop').onclick = () => attempt(() => request('stop', { runId: runID }))
$('output-folder').onclick = () => attempt(() => request('output', { runId: runID }))
document.addEventListener('keydown', event => {
  if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's' && !event.defaultPrevented) {
    event.preventDefault()
    if (documentFile && !busy) attempt(save)
  }
})
window.addEventListener('beforeunload', event => { if (dirty) { event.preventDefault(); event.returnValue = '' } })
notify('ready')

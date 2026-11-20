import { basicSetup } from 'codemirror'
import { EditorView, keymap, placeholder } from '@codemirror/view'
import { EditorState, Compartment } from '@codemirror/state'
import { python, pythonLanguage } from '@codemirror/lang-python'
import { indentUnit } from '@codemirror/language'
import { indentWithTab } from '@codemirror/commands'
import { snippetCompletion } from '@codemirror/autocomplete'
import { oneDark } from '@codemirror/theme-one-dark'

const methods = {
  goscrcpy: [
    { label: 'connect', type: 'function', apply: 'connect()', detail: '预置 · 连接所选设备' },
    { label: 'device_ids', type: 'function', apply: 'device_ids()', detail: '预置 · 所选设备序列号' },
    { label: 'output_path', type: 'function', apply: 'output_path("screen.png")', detail: '预置 · 本次产物路径' },
  ],
  d: [
    { label: 'info', type: 'property', detail: '预置 · 设备信息' },
    { label: 'app_current', type: 'function', apply: 'app_current()', detail: '预置 · 当前应用' },
    { label: 'screenshot', type: 'function', apply: 'screenshot(str(goscrcpy.output_path("screen.png")))', detail: '预置 · 截图' },
    { label: 'dump_hierarchy', type: 'function', apply: 'dump_hierarchy()', detail: '预置 · 页面 XML' },
    { label: 'press', type: 'function', apply: 'press("back")', detail: '预置 · 按键' },
  ],
}
export function automationCompletion(context) {
  const member = context.matchBefore(/\b(?:goscrcpy|d)\.\w*/)
  if (member) {
    const [object, prefix] = member.text.split('.')
    return { from: member.to - prefix.length, options: methods[object], validFor: /^\w*$/ }
  }
  const word = context.matchBefore(/\w*/)
  if (!word || (!context.explicit && word.from === word.to)) return null
  return { from: word.from, options: [
    snippetCompletion('import goscrcpy\n\nd = goscrcpy.connect()\n${}', { label: 'goscrcpy_connect', detail: '模板 · 连接所选设备', type: 'text' }),
    snippetCompletion('d.screenshot(str(goscrcpy.output_path("${screen.png}")))', { label: 'goscrcpy_screenshot', detail: '模板 · 保存截图', type: 'text' }),
    snippetCompletion('d(text="${文本}").click(timeout=${10})', { label: 'u2_click_text', detail: '模板 · 按文本点击', type: 'text' }),
  ], validFor: /^\w*$/ }
}
export function createEditor(parent, { onChange, onPosition, onSave }) {
  const theme = new Compartment(), editable = new Compartment()
  let dark = false, writable = false
  const extensions = () => [
    basicSetup, python(), pythonLanguage.data.of({ autocomplete: automationCompletion }),
    indentUnit.of('    '), keymap.of([{ key: 'Mod-s', run: () => { onSave(); return true } }, indentWithTab]),
    placeholder('在左侧选择或新建一个 .py 脚本'),
    theme.of(dark ? oneDark : []), editable.of([EditorView.editable.of(writable), EditorState.readOnly.of(!writable)]),
    EditorView.updateListener.of(update => {
      if (update.docChanged) onChange(update.state.doc.toString())
      if (update.docChanged || update.selectionSet) {
        const pos = update.state.selection.main.head, line = update.state.doc.lineAt(pos)
        onPosition(line.number, pos - line.from + 1)
      }
    }),
    EditorView.theme({ '&': { height: '100%' }, '.cm-scroller': { overflow: 'auto', fontFamily: 'Consolas, "Cascadia Mono", monospace', fontSize: '13px', lineHeight: '1.7' }, '.cm-content': { padding: '14px 0' }, '.cm-line': { padding: '0 16px' }, '.cm-gutters': { border: 'none', backgroundColor: 'transparent', color: '#8b949e' }, '&.cm-focused': { outline: 'none' } }),
  ]
  const view = new EditorView({ parent, state: EditorState.create({ extensions: extensions() }) })
  return {
    view,
    load(content) {
      // A fresh state isolates undo history between files.
      view.setState(EditorState.create({ doc: content, extensions: extensions() }))
    },
    setTheme(value) { dark = value; view.dispatch({ effects: theme.reconfigure(dark ? oneDark : []) }) },
    setEditable(value) { writable = value; view.dispatch({ effects: editable.reconfigure([EditorView.editable.of(value), EditorState.readOnly.of(!value)]) }) },
  }
}

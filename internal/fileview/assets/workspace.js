import {validName, parentPath, joinPath, formatSize, kindOf, normalizeEntries, visibleEntries} from './model.mjs';

if (!window.viva) window.viva={postMessage(){throw new Error('请从 Go-Scopy 打开文件工作台');}};
const $ = id => document.getElementById(id);
const paths = {
 back:'m14 5-7 7 7 7', forward:'m10 5 7 7-7 7', up:'m5 12 7-7 7 7M12 5v14', refresh:'M20 7v5h-5M4 17v-5h5M6.1 7a7 7 0 0 1 11.6-2L20 8M4 16l2.3 3a7 7 0 0 0 11.6-2',
 arrow:'M5 12h14m-6-6 6 6-6 6', search:'m20 20-5-5M17 10a7 7 0 1 1-14 0 7 7 0 0 1 14 0', list:'M9 6h12M9 12h12M9 18h12M3 6h.1M3 12h.1M3 18h.1', grid:'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z', close:'m6 6 12 12M6 18 18 6',
 folder:'M3 7V5a1 1 0 0 1 1-1h6l2 3h8a1 1 0 0 1 1 1v11a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1V7z', file:'M14 2H5a1 1 0 0 0-1 1v18a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V8zM14 2v6h6M8 13h8M8 17h6',
 image:'M14 2H5a1 1 0 0 0-1 1v18a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V8zM14 2v6h6M7 18l4-5 3 3 2-2 2 4M8 9h.1', video:'M14 2H5a1 1 0 0 0-1 1v18a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V8zM14 2v6h6m-10 4 5 3-5 3z', audio:'M9 18V6l10-2v12M9 10l10-2M9 18a3 2 0 1 1-6 0 3 2 0 0 1 6 0M19 16a3 2 0 1 1-6 0 3 2 0 0 1 6 0', eye:'M2 12s4-7 10-7 10 7 10 7-4 7-10 7S2 12 2 12M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0', error:'M12 3 2 21h20zM12 9v5m0 3v1'
};
function icon(name) { const svg = document.createElementNS('http://www.w3.org/2000/svg','svg'); svg.setAttribute('viewBox','0 0 24 24'); svg.setAttribute('fill','none'); svg.setAttribute('stroke','currentColor'); svg.setAttribute('stroke-width','1.5'); svg.setAttribute('stroke-linecap','round'); svg.setAttribute('stroke-linejoin','round'); svg.setAttribute('aria-hidden','true'); const p = document.createElementNS(svg.namespaceURI,'path'); p.setAttribute('d',paths[name] || paths.file); svg.append(p); return svg; }
for (const [id,name] of Object.entries({back:'back',forward:'forward',up:'up',refresh:'refresh','search-icon':'search','view-list':'list','view-grid':'grid','close-preview':'close','close-tasks':'close'})) $(id).append(icon(name));
document.querySelector('#path-form button').append(icon('arrow'));
function el(tag, cls, text) { const e = document.createElement(tag); if (cls) e.className=cls; if (text !== undefined) e.textContent=text; return e; }

let context = {serial:'',online:false,theme:'light'}, dir='/sdcard', entries=[], selected=new Set(), anchor=-1, view='list', history=[], historyAt=-1, loading=false, listReady=false;
let seq=0, generation=0, previewGeneration=0, objectURL='', toastTimer, refreshTimer, modalResolve;
const pending=new Map(), tasks=new Map();
function request(action, extra={}) {
 const id=++seq;
 return new Promise((resolve,reject) => {
  const timer=setTimeout(() => { pending.delete(id); reject(new Error('操作超时，请刷新目录后重试')); }, ['upload','download'].includes(action)?600000:65000);
  pending.set(id,{resolve,reject,timer});
  try { window.viva.postMessage(JSON.stringify({id,action,serial:context.serial,path:dir,...extra})); }
  catch(error) { clearTimeout(timer); pending.delete(id); reject(error); }
 });
}
function notify(text, error=false) { $('toast').textContent=text; $('toast').classList.toggle('error',error); $('toast').hidden=false; clearTimeout(toastTimer); toastTimer=setTimeout(() => {$('toast').hidden=true;}, error?6500:3500); }
function report(error) { notify(error.message || String(error),true); }
function picked() { return entries.filter(e => selected.has(e.name)); }
function visible() { return visibleEntries(entries,$('search').value,$('sort').value); }
function updateActions() {
 const items=picked(), one=items.length===1, online=context.online && !loading && listReady;
 $('selection-count').textContent=items.length ? `已选 ${items.length} 项` : '选择文件';
 $('preview').disabled=!online || !one || items[0].isDir; $('rename').disabled=!online || !one;
 $('download').disabled=!online || !items.length; $('delete').disabled=!online || !items.length;
 for (const id of ['upload','mkdir']) $(id).disabled=!online; $('refresh').disabled=!context.online; $('up').disabled=!context.online || dir==='/';
 $('back').disabled=historyAt<=0; $('forward').disabled=historyAt>=history.length-1;
 const list=visible(), count=list.filter(e=>selected.has(e.name)).length;
 $('select-all').checked=list.length>0 && count===list.length; $('select-all').indeterminate=count>0 && count<list.length; $('select-all').disabled=loading || !context.online || !list.length;
}
function metadata(entry) { const m=el('div','preview-meta'); m.append(el('strong','',entry.name),el('span','',entry.isDir?'文件夹':formatSize(entry.size)),el('span','',entry.date || '修改时间未知')); return m; }
function placeholder(title='选择一个文件', description='双击文件，或选中后点击「预览」。\n图片、文本和音视频可在此查看。', name='eye') { const box=el('div','preview-placeholder'); box.append(icon(name),el('strong','',title)); const p=el('p','',description); p.style.whiteSpace='pre-line'; box.append(p); $('preview-content').replaceChildren(box); return box; }
function revokePreview() { if (objectURL) { URL.revokeObjectURL(objectURL); objectURL=''; } }
function clearPreview(send=true) { previewGeneration++; revokePreview(); const media=$('preview-content').querySelector('video,audio'); if (media) { media.pause(); media.removeAttribute('src'); media.load(); } $('preview-pane').classList.remove('open'); placeholder(); if(send && context.serial) request('cancelPreview').catch(()=>{}); }
function showInfo(entry) {
 clearPreview();
 if(!entry) return;
 const box=placeholder(entry.name,entry.isDir?'双击打开文件夹':`${formatSize(entry.size)}\n${entry.date || '修改时间未知'}`,entry.isDir?'folder':kindOf(entry.name));
 if(!entry.isDir) { const b=el('button','','预览文件'); b.onclick=()=>preview(entry); box.append(b); }
}
function render() {
 const list=visible(), container=$('files'); container.classList.toggle('grid',view==='grid'); document.querySelector('.column-head').hidden=view==='grid';
 const fragment=document.createDocumentFragment();
 for(const [i,e] of list.entries()) {
  const row=el('div',`file ${e.isDir?'folder':kindOf(e.name)}${selected.has(e.name)?' selected':''}`); row.dataset.name=e.name; row.tabIndex=0; row.setAttribute('aria-label',e.name+(e.isDir?'，文件夹':'')); row.setAttribute('aria-selected',String(selected.has(e.name)));
  const check=el('input'); check.type='checkbox'; check.checked=selected.has(e.name); check.setAttribute('aria-label','选择 '+e.name); check.disabled=loading || !context.online;
  check.onclick=event=>{event.stopPropagation(); toggle(e.name); anchor=i; render(); showInfo(picked().length===1?picked()[0]:null);};
  const name=el('div','file-name'); name.append(icon(e.isDir?'folder':kindOf(e.name)),el('span','name',e.name)); row.title=e.name;
  row.append(check,name,el('span','file-size',e.isDir?'—':formatSize(e.size)),el('span','file-date',e.date));
  row.onclick=event=>{
   if(event.shiftKey && anchor>=0) { if(!event.ctrlKey && !event.metaKey) selected.clear(); for(let n=Math.min(anchor,i);n<=Math.max(anchor,i);n++) selected.add(list[n].name); }
   else if(event.ctrlKey || event.metaKey) { toggle(e.name); anchor=i; }
   else { selected=new Set([e.name]); anchor=i; }
   render(); showInfo(picked().length===1?picked()[0]:null);
  };
  row.ondblclick=()=>openEntry(e);
  row.onkeydown=event=>{if(event.key==='Enter'){event.stopPropagation();openEntry(e);}};
  fragment.append(row);
 }
 container.replaceChildren(fragment);
 $('summary').textContent=loading?'正在读取目录…':`${entries.length} 项${list.length!==entries.length?` · 筛选出 ${list.length} 项`:''}${selected.size?` · 已选 ${selected.size} 项`:''}`;
 $('empty').hidden=!!list.length || loading;
 if(!list.length && !loading) { const box=$('empty'); box.replaceChildren(icon('folder'),el('strong','',entries.length?'没有匹配的文件':'这个文件夹是空的'),el('span','',entries.length?'试试其他名称':'上传文件，或新建一个文件夹。')); }
 updateActions();
}
function toggle(name) { if(selected.has(name)) selected.delete(name); else selected.add(name); }
async function load(path=dir, navigation='push') {
 if(!context.online) return;
 const gen=++generation, serial=context.serial;
 loading=true; listReady=false; selected.clear(); anchor=-1; clearPreview(); $('files').setAttribute('aria-busy','true'); $('empty').hidden=true; updateActions(); $('summary').textContent='正在读取目录…';
 try {
  const result=await request('list',{path}); if(gen!==generation || serial!==context.serial) return;
  listReady=true; dir=result.path; entries=normalizeEntries(result.entries); $('path').value=dir;
  if(typeof navigation==='number') historyAt=navigation;
  if(navigation==='push' && history[historyAt]!==dir) { history=history.slice(0,historyAt+1); history.push(dir); historyAt=history.length-1; }
  for(const b of document.querySelectorAll('[data-path]')) b.classList.toggle('active',b.dataset.path===dir);
  loading=false; render();
 } catch(error) {
  if(gen!==generation) return; loading=false; entries=[]; render(); const box=$('empty');box.hidden=false;box.replaceChildren(icon('error'),el('strong','','无法读取目录'),el('span','',error.message)); const retry=el('button','','重试');retry.onclick=()=>load(path,navigation);box.append(retry); $('summary').textContent='读取失败';
 } finally { if(gen===generation) {$('files').setAttribute('aria-busy','false');updateActions();} }
}
function openEntry(e) { if(loading || !context.online) return; if(e.isDir) load(joinPath(dir,e.name)); else preview(e); }
async function preview(entry) {
 if(!entry || entry.isDir || !context.online || loading) return;
 clearPreview(); const gen=++previewGeneration; $('preview-pane').classList.add('open');
 const box=placeholder('正在准备预览',`${entry.name}\n${formatSize(entry.size)} · 从设备读取中…`); const cancel=el('button','preview-cancel','取消');cancel.onclick=()=>clearPreview();box.append(cancel);
 try {
  const data=await request('preview',{name:entry.name}); if(gen!==previewGeneration) return;
  const response=await fetch(data.url); if(!response.ok) throw new Error('预览缓存已失效，请重试');
  const blob=await response.blob(); if(gen!==previewGeneration) return;
  const container=$('preview-content'); container.replaceChildren(metadata(entry));
  if(data.kind==='text') { const text=await blob.text(); if(gen!==previewGeneration) return;container.append(el('pre','preview-code',text)); }
  else { objectURL=URL.createObjectURL(blob);const media=el(data.kind==='image'?'img':data.kind);media.src=objectURL;
   if(data.kind==='image') media.alt=entry.name;else {media.controls=true;media.preload='metadata';}
   media.onerror=()=>{if(gen===previewGeneration) placeholder('无法解码此文件','系统 WebView 不支持此编码，请下载后打开。','error');};
   const frame=el('div','preview-media');frame.append(media);container.append(frame);
  }
  container.append(el('div','preview-note',data.kind==='text'?'只读 UTF-8 预览 · 设备文件中的 HTML／脚本不会执行':'预览内容不会自动播放 · 解码能力取决于系统 WebView'));
 } catch(error) { if(gen!==previewGeneration) return; const box=placeholder('暂时无法预览',error.message,'file');const download=el('button','','下载后打开');download.onclick=()=>downloadEntries([entry]);box.append(download); }
}
function modal({title,description='',value,submit='确定',danger=false,validate}) {
 if(modalResolve) modalResolve(null);
 $('modal-title').textContent=title;$('modal-description').textContent=description;$('modal-input').hidden=value===undefined;$('modal-input').value=value||'';$('modal-submit').textContent=submit;$('modal-submit').classList.toggle('danger',danger);$('modal-error').textContent='';
 $('modal').showModal();
 if(value!==undefined) {$('modal-input').focus();$('modal-input').select();}
 return new Promise(resolve=>{modalResolve=resolve;$('modal-form').onsubmit=event=>{event.preventDefault();const answer=value===undefined?true:$('modal-input').value;const error=validate?.(answer);if(error){$('modal-error').textContent=error;return;}finishModal(answer);};});
}
function finishModal(value) {const resolve=modalResolve;modalResolve=null;$('modal').close();resolve?.(value);}
$('modal-cancel').onclick=()=>finishModal(null);$('modal').addEventListener('cancel',event=>{event.preventDefault();finishModal(null);});
function validateName(name) {if(!validName(name))return '名称不能为空，不能包含 / 或换行，也不能是 . 或 ..';if(entries.some(e=>e.name===name))return '当前目录已存在同名项目';return '';}
async function mkdir() {const name=await modal({title:'新建文件夹',description:'将在当前目录创建文件夹。',value:'新建文件夹',submit:'创建',validate:validateName});if(name===null)return;try{await request('mkdir',{name});notify('创建任务已提交');}catch(error){report(error);}}
async function rename() {const e=picked()[0];if(!e)return;const name=await modal({title:'重命名',description:e.name,value:e.name,submit:'保存名称',validate:n=>n===e.name?'':validateName(n)});if(name===null||name===e.name)return;try{await request('rename',{name:e.name,target:name});notify('重命名任务已提交');}catch(error){report(error);}}
async function remove() {const items=picked();if(!items.length)return;const yes=await modal({title:`删除 ${items.length} 个项目？`,description:items.slice(0,5).map(e=>e.name+(e.isDir?'（包含全部内容）':'')).join('\n')+(items.length>5?'\n…':'')+'\n\n直接从设备删除，无法在本应用中撤销。',submit:'确认删除',danger:true});if(!yes)return;try{await request('delete',{names:items.map(e=>e.name),confirmed:true});notify('删除任务已提交');selected.clear();render();}catch(error){report(error);}}
async function upload() {const yes=await modal({title:'上传到当前目录',description:dir+'\n\n下一步选择本地文件。同名设备文件将被覆盖，请确认目标目录。',submit:'选择文件'});if(!yes)return;try{await request('upload',{confirmed:true});}catch(error){report(error);}}
async function downloadEntries(items=picked()) {if(!items.length)return;try{await request('download',{names:items.map(e=>e.name)});}catch(error){report(error);}}
// Coalesce a batch of completed transfers into one directory refresh.
function scheduleRefresh() {const serial=context.serial,path=dir;clearTimeout(refreshTimer);refreshTimer=setTimeout(()=>{if(context.serial===serial && dir===path && context.online && context.active)load(dir,'replace');},120);}
function renderTasks() {const list=[...tasks.values()].slice(-12).reverse();$('task-count').textContent=list.filter(t=>t.Status==='running').length;$('task-list').replaceChildren(...list.map(t=>{const row=el('div','task'),detail=el('div');detail.append(el('strong','',t.Title),el('p','',t.Detail || (t.Status==='running'?'正在执行，请稍候…':'操作完成')));row.append(detail,el('span','task-status '+t.Status,t.Status==='running'?'进行中':t.Status==='error'?'失败':'完成'));return row;}));if(!list.length)$('task-list').append(el('p','preview-note','还没有传输或文件操作。'));}

window.viva.onmessage=event=>{
 const raw = typeof event === "string" ? event : event.data;
 let message;try{message=JSON.parse(raw);}catch{return;}
 if(message.type==='response') {const p=pending.get(message.id);if(!p)return;clearTimeout(p.timer);pending.delete(message.id);if(message.error)p.reject(new Error(message.error));else p.resolve(message.data || {});return;}
 if(message.type==='context') {
  const changed=context.serial!==message.serial, cameOnline=!context.online && message.online, becameActive=!context.active && message.active;context=message;document.documentElement.dataset.theme=message.theme;
  $('connection').textContent=message.online?'设备已连接':'设备离线';$('connection').classList.toggle('offline',!message.online);
  if(changed) {clearTimeout(refreshTimer);listReady=false;generation++;clearPreview(false);if(modalResolve)finishModal(null);for(const p of pending.values()){clearTimeout(p.timer);p.reject(new Error('设备已切换'));}pending.clear();dir='/sdcard';entries=[];selected.clear();history=[];historyAt=-1;$('search').value='';tasks.clear();renderTasks();render();if(message.online && message.active)load(dir);}
  else if((cameOnline || becameActive) && message.active)load(dir,'replace');
  if(!message.active) {generation++;loading=false;clearPreview(false);$('files').setAttribute('aria-busy','false');}
  if(!message.online) {listReady=false;generation++;previewGeneration++;loading=false;clearPreview(false);$('files').setAttribute('aria-busy','false');updateActions();$('summary').textContent='设备离线 · 重新连接后可继续';}
  updateActions();return;
 }
 if(message.type==='task' && message.serial===context.serial) {tasks.set(message.task.ID,message.task);renderTasks();if(message.task.Status!=='running'){if(message.task.Status==='error')notify(message.task.Detail,true);if(message.refresh && message.path===dir && context.online && context.active)scheduleRefresh();}}
};
$('path-form').onsubmit=e=>{e.preventDefault();load($('path').value);};
$('refresh').onclick=()=>load(dir,'replace');$('up').onclick=()=>load(parentPath(dir));
$('back').onclick=()=>{if(historyAt>0)load(history[historyAt-1],historyAt-1);};$('forward').onclick=()=>{if(historyAt<history.length-1)load(history[historyAt+1],historyAt+1);};
for(const b of document.querySelectorAll('[data-path]'))b.onclick=()=>load(b.dataset.path);
$('search').oninput=()=>{selected.clear();anchor=-1;clearPreview();render();};$('sort').onchange=render;
for(const mode of ['list','grid'])$('view-'+mode).onclick=()=>{view=mode;for(const m of ['list','grid']){$('view-'+m).classList.toggle('active',m===mode);$('view-'+m).setAttribute('aria-pressed',String(m===mode));}render();};
$('select-all').onchange=()=>{const names=visible().map(e=>e.name);if($('select-all').checked)selected=new Set(names);else selected.clear();render();showInfo(picked().length===1?picked()[0]:null);};
$('preview').onclick=()=>preview(picked()[0]);$('close-preview').onclick=()=>clearPreview();$('mkdir').onclick=mkdir;$('rename').onclick=rename;$('delete').onclick=remove;$('upload').onclick=upload;$('download').onclick=()=>downloadEntries();
$('show-tasks').onclick=()=>{$('tasks').hidden=!$('tasks').hidden;renderTasks();};$('close-tasks').onclick=()=>{$('tasks').hidden=true;};
document.addEventListener('keydown',e=>{
 if($('modal').open)return;
 const editing=/^(INPUT|TEXTAREA|SELECT)$/.test(document.activeElement?.tagName);
 if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='l'){e.preventDefault();$('path').focus();$('path').select();return;}
 if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='r'){e.preventDefault();load(dir,'replace');return;}
 if(editing)return;
 if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='a'){e.preventDefault();if(!loading && context.online){selected=new Set(visible().map(e=>e.name));render();}return;}
 if(e.key==='Escape'){selected.clear();clearPreview();render();}
 if(e.key==='Backspace'){e.preventDefault();load(parentPath(dir));}
 if(e.key==='F2' && picked().length===1){e.preventDefault();rename();}
 if(e.key==='Delete' && picked().length){e.preventDefault();remove();}
});
window.addEventListener('pagehide',()=>{clearTimeout(refreshTimer);revokePreview();for(const p of pending.values())clearTimeout(p.timer);});
placeholder();render();renderTasks();
if(window.viva) window.viva.postMessage(JSON.stringify({id:0,action:'ready'}));
else {$('summary').textContent='请从 Go-Scopy 打开文件工作台';}

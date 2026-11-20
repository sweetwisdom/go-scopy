import { pathToFileURL, fileURLToPath } from 'node:url'
import { createServer } from 'node:http'
import { readFile, mkdir, writeFile } from 'node:fs/promises'
import assert from 'node:assert/strict'
const { chromium } = await import(process.env.GOSCOPY_PLAYWRIGHT ? pathToFileURL(process.env.GOSCOPY_PLAYWRIGHT).href : new URL('../../artifacts/fileview-browser/node_modules/playwright/index.mjs', import.meta.url).href)
const artifacts = new URL('../../artifacts/script-workspace/', import.meta.url)
await mkdir(artifacts, {recursive:true})
const server=createServer(async(req,res)=>{
 try {
  const name=req.url==='/'?'index.html':req.url.slice(1)
  if(name.includes('/')||name.includes('..'))throw Error('invalid path')
  const file=await readFile(new URL(`./assets/${name}`,import.meta.url))
  res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(file)
 }catch{res.statusCode=404;res.end()}
})
await new Promise(r=>server.listen(0,'127.0.0.1',r))
const browser=await chromium.launch({channel:'msedge',headless:true})
const errors=[],checks=[]
try{
 for(const [width,height,dpr,theme] of [[1164,748,1,'light'],[924,628,1,'light'],[1400,850,1.5,'dark']]){
  const context=await browser.newContext({viewport:{width,height},deviceScaleFactor:dpr})
  const page=await context.newPage();page.on('pageerror',e=>errors.push(e.message))
  await page.addInitScript(({theme})=>{
   const directory='C:\\Test\\Go-Scopy\\scripts'
   let version=1
   const make=(name,content,external=false)=>({path:external?'C:\\Other\\'+name:directory+'\\'+name,name,content,version:String(version++),external,trusted:false})
   const docs=[make('device_info.py','import goscrcpy\n\nprint(goscrcpy.device_ids(), flush=True)\n'),make('second.py','print("Second script")\n')]
   window.docs=docs;window.sent=[];window.context={type:'context',theme,active:true,selected:'device-a',devices:[{serial:'device-a',name:'设备 A'},{serial:'device-b',name:'设备 B'}],runs:[],last:{},error:'',dependencySupport:true,dependencies:null}
   const deliver=m=>window.viva.onmessage?.(new MessageEvent('message',{data:JSON.stringify(m)}))
   window.deliverContext=()=>deliver(window.context)
   window.viva={postMessage(raw){const m=JSON.parse(raw);window.sent.push(m);queueMicrotask(()=>{
    if(m.action==='ready'){window.deliverContext();return}
    if(m.action==='dirty')return
    let data=null,error=''
    const doc=docs.find(d=>d.path===m.path)
    switch(m.action){
     case 'list':data={directory,files:docs.map(d=>({path:d.path,name:d.name,external:d.external}))};break
     case 'read':data=doc;break
     case 'trust':doc.trusted=m.trusted;data=doc;break
     case 'new':
      if(docs.some(d=>d.name===m.name)){error='文件已存在';break}
      data=make(m.name,m.content);docs.push(data);break
     case 'save':
      if(window.forceConflict){error='文件已被外部修改，请重新打开';break}
      if(doc.version!==m.version){error='版本冲突';break}
      if(doc.content!==m.content)doc.trusted=false
      doc.content=m.content;doc.version=String(version++);data=doc;break
     case 'open':data=make('external.py','print("external")\n',true);docs.push(data);break
     case 'python':data='C:\\Python\\python.exe';break
     case 'guide':window.guideOpened=true;break
     case 'dependencies-check':
      window.context.dependencies={requestedPython:m.python,python:m.python||'C:\\Python\\python.exe',operation:'check',status:'success',ready:true,log:'锁定依赖检查通过',error:''};break
     case 'dependencies-install':
      window.installCalls=(window.installCalls||0)+1
      window.context.dependencies={requestedPython:m.python,python:m.python||'C:\\Python\\python.exe',operation:'install',status:'running',ready:false,log:'pip install --require-hashes -r tools/python/requirements.lock -i https://pypi.tuna.tsinghua.edu.cn/simple',error:''}
      setTimeout(()=>{Object.assign(window.context.dependencies,{status:'success',ready:true});window.deliverContext()},250);break
     case 'dependencies-stop':Object.assign(window.context.dependencies,{status:'cancelled',ready:false});break
     case 'run':
      doc.trusted=true
      data='run-'+version++
      window.context.runs.unshift({id:data,request:{path:m.path,serial:m.serial},status:'running',started:new Date().toISOString(),exitCode:-1,log:'[runner] Selected device: '+m.serial+'\nBROWSER_SCRIPT_OK\n',error:''})
      break
     case 'stop':{const run=window.context.runs.find(r=>r.id===m.runId);run.status='cancelled';run.exitCode=-1;break}
    }
    deliver({type:'response',id:m.id,data,error})
    if(['run','stop','dependencies-check','dependencies-install','dependencies-stop'].includes(m.action))window.deliverContext()
   })}}
  },{theme})
  await page.goto(`http://127.0.0.1:${server.address().port}/`)
  await page.getByRole('button',{name:'device_info.py',exact:false}).click()
  const content=page.locator('.cm-content')
  await page.waitForFunction(()=>document.querySelector('.cm-content')?.getAttribute('contenteditable')==='true')
  async function replace(text){await content.click();await page.keyboard.press('Control+a');await page.keyboard.insertText(text)}
  // Built-ins and current-file names are the official Python language completions.
  await replace('pri');await page.keyboard.press('Control+Space')
  await page.getByRole('option',{name:'print',exact:false}).first().waitFor()
  await page.keyboard.press('Escape')
  await replace('my_device = 1\nmy_d');await page.keyboard.press('Control+Space')
  await page.getByRole('option',{name:'my_device',exact:false}).first().waitFor()
  await page.keyboard.press('Escape')
  await replace('import goscrcpy\ngoscrcpy.');await page.keyboard.press('Control+Space')
  await page.getByRole('option',{name:'device_ids',exact:false}).first().waitFor()
  await page.keyboard.press('Escape')
  await replace('print("BROWSER_SCRIPT_OK")\n')
  assert((await page.locator('#filename').textContent()).includes('未保存'))
  await page.keyboard.press('Control+s')
  await page.waitForFunction(()=>!document.getElementById('filename').textContent.includes('未保存'))
  assert.equal(await page.evaluate(()=>window.docs[0].content),'print("BROWSER_SCRIPT_OK")\n')
  await page.locator('#trusted').check()
  await content.click();await page.keyboard.press('Control+End');await page.keyboard.insertText('# edited\n')
  assert.equal(await page.locator('#trusted').isChecked(),false)
  await page.getByRole('button',{name:'second.py',exact:false}).click()
  await page.locator('#dirty-dialog').waitFor({state:'visible'})
  await page.locator('#dirty-cancel').click()
  assert((await page.locator('#filename').textContent()).includes('device_info.py'))
  // Conflict leaves the dirty buffer intact.
  await page.evaluate(()=>window.forceConflict=true)
  await page.locator('#save').click()
  await page.getByRole('status').getByText(/文件已被外部修改/).waitFor()
  assert((await page.locator('#filename').textContent()).includes('未保存'))
  await page.evaluate(()=>window.forceConflict=false)
  await page.locator('#save').click()
  await page.waitForFunction(()=>!document.getElementById('filename').textContent.includes('未保存'))
  await page.locator('#device').selectOption('device-b');await page.locator('#trusted').check()
  await page.locator('#run').click()
  await page.waitForFunction(()=>document.getElementById('log').textContent.includes('BROWSER_SCRIPT_OK'))
  assert.equal(await page.evaluate(()=>window.sent.find(m=>m.action==='run').serial),'device-b')
  await page.locator('#stop').click();await page.waitForFunction(()=>document.getElementById('run-status').textContent.includes('已取消'))
  await page.locator('#guide').click();await page.waitForFunction(()=>window.guideOpened)
  // Trust follows the saved file revision, not a transient checkbox.
  await page.getByRole('button',{name:'second.py',exact:false}).click()
  await page.getByRole('button',{name:'device_info.py',exact:false}).click()
  assert.equal(await page.locator('#trusted').isChecked(),true)
  await page.locator('#trusted').uncheck()
  await page.waitForFunction(()=>window.docs[0].trusted===false)
  await page.getByRole('button',{name:'second.py',exact:false}).click()
  await page.getByRole('button',{name:'device_info.py',exact:false}).click()
  assert.equal(await page.locator('#trusted').isChecked(),false)
  // Missing dependencies never install automatically; only the explicit button.
  assert.equal(await page.evaluate(()=>window.installCalls||0),0)
  await page.evaluate(()=>{Object.assign(window.context.dependencies,{ready:false,status:'failed',log:'uiautomator2: 未安装'});window.deliverContext()})
  assert(await page.locator('#run').isDisabled())
  page.once('dialog',dialog=>dialog.accept())
  await page.locator('#install-dependencies').click()
  await page.waitForFunction(()=>window.installCalls===1)
  assert(await page.locator('#run').isDisabled())
  await page.waitForFunction(()=>window.context.dependencies.ready===true)
  await page.getByText('Python 依赖已就绪',{exact:true}).waitFor()
  assert((await page.locator('#dependency-log').textContent()).includes('--require-hashes'))
  await page.locator('#dependency-details > summary').click()
  // Native context updates must remove disconnected devices, never pick another silently.
  await page.evaluate(()=>{window.context.devices=window.context.devices.filter(d=>d.serial!=='device-b');window.deliverContext()})
  assert.equal(await page.locator('#device').inputValue(),'')
  assert(await page.locator('#run').isDisabled())
  await page.locator('#open').click()
  await page.waitForFunction(()=>document.getElementById('filename').textContent==='external.py')
  await page.getByRole('button',{name:'device_info.py',exact:false}).click()
  await page.waitForFunction(()=>document.getElementById('filename').textContent==='device_info.py')
  await page.locator('#new').click();await page.locator('#new-name').fill('new_demo.py');await page.getByRole('button',{name:'创建脚本',exact:true}).click()
  await page.waitForFunction(()=>document.getElementById('filename').textContent==='new_demo.py')
  await page.locator('#search').fill('new_demo')
  assert.equal(await page.locator('#files .file').count(),1)
  await page.locator('#search').fill('')
  const layout=await page.evaluate(()=>{
   const e=document.getElementById('editor').getBoundingClientRect(),l=document.getElementById('log').getBoundingClientRect()
   return {editor:{x:e.x,y:e.y,width:e.width,height:e.height,right:e.right,bottom:e.bottom},log:{height:l.height,bottom:l.bottom},overflow:document.documentElement.scrollWidth>innerWidth||document.documentElement.scrollHeight>innerHeight}
  })
  assert(!layout.overflow&&layout.editor.height>100&&layout.log.height>40,JSON.stringify(layout))
  assert(layout.editor.right<=width&&layout.log.bottom<=height,JSON.stringify(layout))
  await page.screenshot({path:fileURLToPath(new URL(`workspace-${theme}-${width}-${dpr}x.png`,artifacts)),animations:'disabled'})
  checks.push({width,height,dpr,theme,layout,completion:['builtin','local-name','preset-api'],saved:true,explicitDevice:true,conflictProtected:true,guide:true,persistentTrust:true,dependencyInstallButton:true})
  await context.close()
 }
 assert.deepEqual(errors,[])
 await writeFile(new URL('browser-verification.json',artifacts),JSON.stringify({checks,errors},null,2))
 console.log(JSON.stringify({checks,errors},null,2))
}finally{await browser.close();server.close()}

import { pathToFileURL, fileURLToPath } from 'node:url'
const { chromium } = await import(process.env.GOSCOPY_PLAYWRIGHT ? pathToFileURL(process.env.GOSCOPY_PLAYWRIGHT).href : new URL('../../artifacts/fileview-browser/node_modules/playwright/index.mjs', import.meta.url).href)
import { createServer } from 'node:http'
import { readFile } from 'node:fs/promises'
import assert from 'node:assert/strict'
const assets = new URL('./assets/', import.meta.url)
const server = createServer(async (req,res) => {
 try {const name=req.url==='/'?'index.html':req.url.slice(1);const file=await readFile(new URL(name,assets));res.setHeader('Content-Type',name.endsWith('.js')?'text/javascript':name.endsWith('.css')?'text/css':'text/html');res.end(file)} catch {res.statusCode=404;res.end()}
})
await new Promise(r=>server.listen(0,'127.0.0.1',r))
const browser=await chromium.launch({channel:'msedge',headless:true})
const { mkdir } = await import('node:fs/promises')
await mkdir(new URL('../../artifacts/terminal-web/', import.meta.url), { recursive: true })
const errors=[]
const checks=[]
try {
 for (const [width,height,dpr,theme] of [[420,300,1,'light'],[852,500,1,'light'],[1280,700,1.5,'dark']]) {
  const context=await browser.newContext({viewport:{width,height},deviceScaleFactor:dpr})
  const page=await context.newPage()
  page.on('pageerror',err=>errors.push(err.message))
  await page.addInitScript(({theme})=>{
   Object.defineProperty(navigator,'clipboard',{value:{async writeText(text){
    if(window.clipboardFailure)throw new Error('剪贴板写入失败')
    window.copiedText=text
   }},configurable:true})
   window.sent=[];let seq=0
   const deliver=m=>window.viva.onmessage?.({data:JSON.stringify(m)})
   window.viva={postMessage(raw){const m=JSON.parse(raw);window.sent.push(m)
    if(m.action==='ready')queueMicrotask(()=>deliver({type:'context',serial:'127.0.0.1:5555',name:'模拟设备',online:true,active:true,theme,platform:'windows'}))
    if(m.action==='create'){
     const session='mock-'+(++seq)
     deliver({type:'response',id:m.id,data:{sessionId:session}})
     deliver({type:'data',session,data:btoa('GOSCOPY_BROWSER_OK\r\nPD2362:/ $ ')})
    }
    if(m.action==='write'||m.action==='destroy')deliver({type:'response',id:m.id,data:{}})
   }}
  },{theme})
  await page.goto(`http://127.0.0.1:${server.address().port}/`)
  await page.waitForSelector('.session[data-connected=true]')
  await page.waitForFunction(()=>document.querySelector('.xterm-screen')?.clientWidth>100)
  await page.waitForTimeout(100)
  const metrics=()=>page.evaluate(()=>{const screen=document.querySelector('.session[data-mode=shell] .xterm-screen').getBoundingClientRect();const surface=document.querySelector('.session[data-mode=shell] .terminal-surface').getBoundingClientRect();return {width:screen.width,height:screen.height,rows:document.querySelector('.session[data-mode=shell]').querySelectorAll('.xterm-rows>div').length,screen:{x:screen.x,y:screen.y,right:screen.right,bottom:screen.bottom},surface:{x:surface.x,y:surface.y,right:surface.right,bottom:surface.bottom},overflow:screen.right>surface.right+1||screen.bottom>surface.bottom+1,horizontal:document.documentElement.scrollWidth>innerWidth}})
  let before=await metrics();assert(!before.overflow&&!before.horizontal,JSON.stringify(before))
  const input=page.locator('.session[data-mode=shell] .xterm-helper-textarea')
  await input.focus();await page.keyboard.type('echo test');await page.keyboard.press('Enter')
  await page.waitForFunction(()=>window.sent.some(m=>m.action==='write'&&m.data==='\r\n'))
  await page.keyboard.press('Control+c');await page.waitForFunction(()=>window.sent.some(m=>m.action==='write'&&m.data==='\x03'))
  await page.getByRole('button',{name:'常用命令',exact:true}).click()
  await page.waitForTimeout(150)
  const withSidebar=await metrics();assert(!withSidebar.overflow&&!withSidebar.horizontal)
  await page.clock.install()
  const copyButton=page.getByRole('button',{name:'复制设备型号命令',exact:true})
  await copyButton.click()
  await page.clock.runFor(200)
  assert.equal(await page.evaluate(()=>window.copiedText),'getprop ro.product.model')
  assert.equal(await page.locator('.toast-success').count(),1)
  const messageLayout=await page.locator('.toast').evaluate(el=>{
   const box=el.getBoundingClientRect(),toolbar=document.querySelector('.toolbar').getBoundingClientRect()
   return {x:box.x,y:box.y,width:box.width,right:box.right,bottom:box.bottom,toolbarBottom:toolbar.bottom,viewportWidth:innerWidth,zIndex:Number(getComputedStyle(el.parentElement).zIndex)}
  })
  assert(messageLayout.y>=messageLayout.toolbarBottom&&messageLayout.y<80,JSON.stringify(messageLayout))
  assert(Math.abs(messageLayout.x+messageLayout.width/2-width/2)<2,JSON.stringify(messageLayout))
  assert(messageLayout.x>=12&&messageLayout.right<=width-12&&messageLayout.zIndex>3)
  await page.screenshot({path:fileURLToPath(new URL(`../../artifacts/terminal-web/message-${theme}-${width}-${dpr}x.png`,import.meta.url)),animations:'disabled'})
  // Repeating identical text must restart the timer, not disappear on the first deadline.
  await page.clock.fastForward(2000)
  await copyButton.click()
  await page.clock.fastForward(1000)
  assert.equal(await page.locator('.toast-success').count(),1)
  await page.clock.fastForward(2100)
  assert.equal(await page.locator('.toast').count(),0)
  await page.evaluate(()=>{window.clipboardFailure=true})
  await copyButton.click()
  assert.equal(await page.locator('.toast-error').textContent(),'!剪贴板写入失败')
  await page.locator('.toast').click()
  assert.equal(await page.locator('.toast').count(),0)
  await page.evaluate(()=>{window.clipboardFailure=false})
  await page.clock.resume()
  await page.getByRole('button',{name:'收起常用命令'}).click()
  await page.getByRole('tab',{name:'Logcat'}).click();await page.waitForSelector('.session[data-mode=logcat][data-connected=true]')
  await page.getByRole('tab',{name:'Shell',exact:true}).click()
  await page.waitForTimeout(120)
  assert((await page.locator('.session[data-mode=shell] .xterm-rows').textContent()).includes('GOSCOPY_BROWSER_OK'))
  const originalSession=await page.locator('.session[data-mode=shell]').getAttribute('data-session')
  await page.evaluate(({theme})=>window.viva.onmessage({data:JSON.stringify({type:'context',serial:'device-b',name:'B',online:true,active:true,theme,platform:'windows'})}),{theme})
  await page.waitForFunction(()=>Array.from(document.querySelectorAll('.session')).filter(p=>p.style.display!=='none').length===1&&document.querySelector('.device-label').textContent==='device-b')
  await page.evaluate(({theme})=>window.viva.onmessage({data:JSON.stringify({type:'context',serial:'127.0.0.1:5555',name:'A',online:true,active:true,theme,platform:'windows'})}),{theme})
  await page.waitForTimeout(120)
  assert.equal(await page.locator(`.session[data-session="${originalSession}"]`).getAttribute('data-connected'),'true')
  await page.evaluate(({theme})=>window.viva.onmessage({data:JSON.stringify({type:'context',serial:'127.0.0.1:5555',name:'A',online:true,active:false,theme,platform:'windows'})}),{theme})
  await page.waitForTimeout(100)
  const beforeHidden=await page.evaluate(()=>window.sent.filter(m=>m.action==='write').length)
  await page.keyboard.type('must-not-send')
  assert.equal(await page.evaluate(()=>window.sent.filter(m=>m.action==='write').length),beforeHidden)
  await page.evaluate(({theme})=>window.viva.onmessage({data:JSON.stringify({type:'context',serial:'127.0.0.1:5555',name:'A',online:true,active:true,theme,platform:'windows'})}),{theme})
  await page.waitForTimeout(150)

  await page.setViewportSize({width:Math.max(360,width-180),height:Math.max(260,height-100)})
  await page.waitForTimeout(160)
  let smaller=await metrics();assert(!smaller.overflow&&!smaller.horizontal)
  assert(smaller.rows<=before.rows)
  await page.screenshot({path:fileURLToPath(new URL(`../../artifacts/terminal-web/terminal-${theme}-${width}-${dpr}x.png`,import.meta.url))})
  checks.push({width,height,dpr,theme,before,withSidebar,smaller,messageLayout})
  await context.close()
 }
 assert.deepEqual(errors,[])
 const {writeFile}=await import('node:fs/promises')
 await writeFile(new URL('../../artifacts/terminal-web/browser-verification.json',import.meta.url),JSON.stringify({checks,errors},null,2))
 console.log(JSON.stringify({checks,errors},null,2))
} finally {await browser.close();server.close()}

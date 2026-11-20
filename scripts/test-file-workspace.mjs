// Optional browser regression: npm install --prefix artifacts/fileview-browser --no-save playwright
// Production builds require neither Node nor Playwright.
import assert from 'node:assert/strict';
import http from 'node:http';
import fs from 'node:fs/promises';
import path from 'node:path';
import {pathToFileURL, fileURLToPath} from 'node:url';
const root=path.resolve(path.dirname(fileURLToPath(import.meta.url)),'..');
const module=process.env.PLAYWRIGHT_MODULE || path.join(root,'artifacts/fileview-browser/node_modules/playwright/index.mjs');
const {chromium}=await import(pathToFileURL(module));
const assets=path.join(root,'internal/fileview/assets'), output=path.join(root,'artifacts/file-workspace');
await fs.mkdir(output,{recursive:true});
const server=http.createServer(async(req,res)=>{
 try {
  const name=new URL(req.url,'http://localhost').pathname.slice(1) || 'index.html';
  if(name.includes('..')){res.writeHead(404);res.end();return;}
  if(name==='preview/text'){res.setHeader('Content-Type','text/plain; charset=utf-8');res.end('<script>window.__deviceScriptExecuted=true</script>\n{"device":"Pixel 9 Pro","preview":"只读，不执行脚本"}');return;}
  if(name==='preview/image'){res.setHeader('Content-Type','image/png');res.end(await fs.readFile(path.join(root,'internal/brand/assets/icon.png')));return;}
  const types={'.html':'text/html','.js':'text/javascript','.mjs':'text/javascript','.css':'text/css'};res.setHeader('Content-Type',types[path.extname(name)]||'application/octet-stream');res.end(await fs.readFile(path.join(assets,name)));
 }catch{res.writeHead(404);res.end();}
});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
const address=`http://127.0.0.1:${server.address().port}`;
let browser;
try {
 const exe=process.env.BROWSER_EXECUTABLE || (process.platform==='win32'?'C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe':undefined);
 browser=await chromium.launch({headless:true,...(exe?{executablePath:exe}:{})});
 const page=await browser.newPage({viewport:{width:838,height:492},deviceScaleFactor:1});
 const errors=[];page.on('pageerror',e=>errors.push(e.message));
 await page.addInitScript(()=>{
  window.__requests=[];window.__task=0;
  const file=(Name,Size,ModTime='2026-11-20 09:42')=>({Name,Size,ModTime,IsDir:false});
  const directory=Name=>({Name,Size:4096,ModTime:'2026-11-19 18:20',IsDir:true});
  window.__entries=[directory('DCIM'),directory('Documents'),directory('Download'),directory('Pictures'),directory('Music'),file('Go-Scopy.png',262144),file('device-config.json',3148),file('page.html',180),file('notes.txt',2400),file('recording.mp4',18234567),file('unsupported.svg',120),file('<img src=x onerror=alert(1)>.txt',80)];
  window.__context={type:'context',serial:'USB-TEST',name:'Pixel 9 Pro',online:true,active:true,theme:'light'};
  window.__send=message=>window.viva.onmessage({data:JSON.stringify(message)});
  window.viva={postMessage(raw){
   const m=JSON.parse(raw);window.__requests.push(m);
   const respond=(data,error)=>window.__send({type:'response',id:m.id,data,error});
   const complete=(title,mutate)=>{const id=++window.__task;respond({queued:true,taskID:id});setTimeout(()=>{window.__send({type:'task',serial:m.serial,path:m.path,refresh:true,task:{ID:id,Title:title,Status:'running'}});},5);setTimeout(()=>{mutate?.();window.__send({type:'task',serial:m.serial,path:m.path,refresh:true,task:{ID:id,Title:title,Status:'done',Detail:'操作完成'}});},25);};
   if(m.action==='ready'){setTimeout(()=>window.__send(window.__context),0);return;}
   setTimeout(()=>{
    if(m.action==='list'){if(m.path==='/missing')respond(null,'目录不存在或没有访问权限');else respond({path:m.path,entries:m.path==='/sdcard'?window.__entries:[]});}
    else if(m.action==='preview'){if(m.name.endsWith('.svg'))respond(null,'此格式暂不支持内嵌预览，请下载后打开');else respond({kind:m.name.endsWith('.png')?'image':'text',url:m.name.endsWith('.png')?'./preview/image':'./preview/text'});}
    else if(m.action==='cancelPreview')respond({cancelled:true});
    else if(m.action==='mkdir')complete('新建文件夹 · '+m.name,()=>window.__entries.push(directory(m.name)));
    else if(m.action==='rename')complete('重命名 · '+m.name,()=>{window.__entries.find(e=>e.Name===m.name).Name=m.target;});
    else if(m.action==='delete')complete('删除项目',()=>{window.__entries=window.__entries.filter(e=>!m.names.includes(e.Name));});
    else respond({cancelled:true});
   },m.path==='/slow'?120:8);
  }};
 });
 await page.goto(address);await page.locator('.file').first().waitFor();assert.equal(await page.locator('.file').count(),12);
 // Navigation history is committed only after a successful directory read.
 await page.locator('[data-path="/sdcard/DCIM"]').click();await page.waitForFunction(()=>document.querySelector('#path').value==='/sdcard/DCIM');
 await page.locator('#back').click();await page.locator('.file').first().waitFor();assert.equal(await page.locator('#path').inputValue(),'/sdcard');
 await page.locator('#forward').click();await page.waitForFunction(()=>document.querySelector('#path').value==='/sdcard/DCIM');
 await page.locator('#back').click();await page.locator('.file').first().waitFor();
 // Numeric-aware sorting and literal file names: no device markup becomes DOM.
 assert.equal(await page.locator('#files img').count(),0);
 await page.locator('#search').fill('CONFIG');assert.equal(await page.locator('.file').count(),1);await page.locator('#search').fill('');
 await page.locator('.file[data-name="page.html"]').dblclick();await page.locator('.preview-code').waitFor();assert.match(await page.locator('.preview-code').textContent(),/<script>/);assert.equal(await page.evaluate(()=>window.__deviceScriptExecuted),undefined);
 await page.screenshot({path:path.join(output,'files-light-text.png')});
 await page.locator('.file[data-name="Go-Scopy.png"]').dblclick();await page.locator('.preview-media img').waitFor();await page.waitForFunction(()=>document.querySelector('.preview-media img')?.naturalWidth>0);
 await page.screenshot({path:path.join(output,'files-light-image.png')});
 await page.getByRole('button',{name:'网格视图',exact:true}).click();assert.equal(await page.locator('.files.grid').count(),1);
 await page.screenshot({path:path.join(output,'files-grid.png')});
 await page.getByRole('button',{name:'列表视图',exact:true}).click();
 await page.locator('.file[data-name="notes.txt"]').click();await page.keyboard.press('F2');await page.locator('#modal-input').fill('renamed.txt');await page.locator('#modal-submit').click();await page.locator('.file[data-name="renamed.txt"]').waitFor();
 await page.getByRole('button',{name:'新建文件夹',exact:true}).click();await page.locator('#modal-input').fill('测试目录');await page.locator('#modal-submit').click();await page.locator('.file[data-name="测试目录"]').waitFor();
 await page.locator('.file[data-name="renamed.txt"]').click();await page.locator('#delete').click();await page.locator('#modal-cancel').click();assert.equal(await page.evaluate(()=>window.__requests.filter(m=>m.action==='delete').length),0);
 await page.locator('#delete').click();await page.locator('#modal-submit').click();await page.waitForFunction(()=>!document.querySelector('.file[data-name="renamed.txt"]'));
 // Out-of-order directory reads cannot overwrite the latest navigation.
 await page.locator('#path').fill('/slow');await page.locator('#path').press('Enter');await page.locator('#path').fill('/sdcard');await page.locator('#path').press('Enter');await page.waitForTimeout(180);assert.equal(await page.locator('#path').inputValue(),'/sdcard');assert.ok(await page.locator('.file').count()>0);
 await page.locator('#path').fill('/missing');await page.locator('#path').press('Enter');await page.getByText('无法读取目录',{exact:true}).waitFor();assert.equal(await page.locator('#mkdir').isDisabled(),true);
 await page.locator('[data-path="/sdcard"]').click();await page.locator('.file').first().waitFor();
 await page.locator('.file[data-name="unsupported.svg"]').dblclick();await page.getByText('暂时无法预览',{exact:true}).waitFor();assert.equal(await page.locator('#preview-content svg script').count(),0);
 await page.locator('.file[data-name="Go-Scopy.png"]').dblclick();await page.locator('.preview-media img').waitFor();
 await page.evaluate(()=>{window.__context.theme='dark';window.__send(window.__context);});await page.waitForTimeout(180);await page.locator('#toast').evaluate(e=>e.hidden=true);await page.screenshot({path:path.join(output,'files-dark.png')});
 await page.setViewportSize({width:540,height:500});await page.screenshot({path:path.join(output,'files-compact.png')});assert.equal(await page.evaluate(()=>document.body.scrollWidth>innerWidth),false);
 await page.locator('#close-preview').click();assert.equal(await page.locator('#preview-pane').isVisible(),false);
 await page.evaluate(()=>{window.__context.online=false;window.__send(window.__context);});assert.equal(await page.locator('#upload').isDisabled(),true);assert.match(await page.locator('#summary').textContent(),/离线/);
 await page.evaluate(()=>{window.__context.online=true;window.__send(window.__context);});await page.locator('.file').first().waitFor();
 assert.deepEqual(errors,[]);console.log('PASS: browser file workspace (navigation, search, sort, grid, text/image preview, inert HTML, create/rename/delete, cancellation, offline recovery, light/dark/compact).');
 console.log(`Screenshots: ${output}`);
} finally {await browser?.close();await new Promise(resolve=>server.close(resolve));}

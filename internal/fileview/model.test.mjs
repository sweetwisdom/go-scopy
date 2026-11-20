import test from 'node:test';
import assert from 'node:assert/strict';
import {validName,parentPath,joinPath,formatSize,normalizeEntries,visibleEntries,kindOf} from './assets/model.mjs';
test('names, Unicode paths and root traversal',()=>{
 for(const n of ['', '.', '..', 'a/b', 'a\n', 'a\0']) assert.equal(validName(n),false,n);
 assert.equal(validName('中文 <script>.txt'),true);assert.equal(parentPath('/'),'/');assert.equal(parentPath('/sdcard/DCIM'),'/sdcard');assert.equal(joinPath('/','中文 空格'),'/'+'中文 空格');
});
test('directories stay first for all sort modes and inputs are not mutated',()=>{
 const entries=normalizeEntries([{Name:'file10',Size:100},{Name:'..',IsDir:true},{Name:'file2',Size:20},{Name:'Folder',IsDir:true},{Name:'image.png',Size:200}]);
 assert.equal(entries.length,4);
 for(const sort of ['name','name-desc','date','size'])assert.equal(visibleEntries(entries,'',sort)[0].name,'Folder');
 assert.deepEqual(visibleEntries(entries,'FILE','name').map(e=>e.name),['file2','file10']);assert.equal(entries[0].name,'file10');
});
test('sizes and preview categories',()=>{assert.equal(formatSize(0),'0 B');assert.equal(formatSize(1048576),'1 MB');assert.equal(formatSize(-1),'—');assert.equal(kindOf('photo.PNG'),'image');assert.equal(kindOf('untrusted.svg'),'file');});

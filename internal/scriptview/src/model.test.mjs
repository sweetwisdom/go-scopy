import test from 'node:test'
import assert from 'node:assert/strict'
import { canRun, templates, activeRun } from './model.js'
test('execution requires an explicitly selected online device, trusted file and idle target', () => {
 const input = { path: 'test.py', target: 'device-a', devices: [{serial:'device-a'}], trusted: true, busy: false, runs: [], active: true }
 assert(canRun(input))
 for (const patch of [{path:''},{target:''},{target:'device-b'},{trusted:false},{busy:true},{active:false},{devices:[]},{runs:[{status:'running',request:{serial:'device-a'}}]}]) assert(!canRun({...input,...patch}))
 assert(canRun({...input,runs:[{status:'running',request:{serial:'device-b'}}]}))
 assert(activeRun({status:'stopping'})); assert(!activeRun({status:'success'}))
})
test('starter templates use the selected device and managed artifact directory',()=>{
 assert(templates.context.includes('goscrcpy.device_ids()'))
 assert(templates.model.includes('[adb, "-s", serial'))
 assert(templates.screenshot.includes('goscrcpy.output_path('))
})

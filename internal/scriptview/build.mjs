import { build } from 'esbuild'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync, readFileSync, writeFileSync, existsSync } from 'node:fs'
const root = dirname(fileURLToPath(import.meta.url))
mkdirSync(join(root, 'assets'), { recursive: true })
await build({ entryPoints: [join(root, 'src/main.js')], outfile: join(root, 'assets/workspace.js'), bundle: true, minify: true, format: 'esm', target: ['es2020'], legalComments: 'linked' })
const lock = JSON.parse(readFileSync(join(root, 'package-lock.json'), 'utf8'))
let notices = 'Go-Scopy Python editor — bundled open-source licenses\n\n'
for (const [path, pkg] of Object.entries(lock.packages)) {
  if (!path || pkg.dev) continue
  const dir = join(root, path)
  const license = ['LICENSE', 'LICENSE.txt', 'LICENSE.md'].find(name => existsSync(join(dir, name)))
  notices += `${path.replace('node_modules/', '')} ${pkg.version} (${pkg.license})\n${license ? readFileSync(join(dir, license), 'utf8') : 'See package metadata.'}\n\n`
}
writeFileSync(join(root, 'assets/licenses.txt'), notices)

writeFileSync(join(root, 'assets/python-development.md'), readFileSync(join(root, '../../docs/python-script-development.md')))

writeFileSync(join(root, '../../licenses/CodeMirror-editor-licenses.txt'), notices)

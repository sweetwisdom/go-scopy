import { build } from 'esbuild'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { mkdirSync } from 'node:fs'
const root = dirname(fileURLToPath(import.meta.url))
mkdirSync(join(root, 'assets'), { recursive: true })
await build({ entryPoints: [join(root, 'src/main.js')], outfile: join(root, 'assets/workspace.js'), bundle: true, minify: true, format: 'esm', target: ['es2020'], legalComments: 'linked', define: { 'process.env.NODE_ENV': '"production"', __VUE_OPTIONS_API__: 'false', __VUE_PROD_DEVTOOLS__: 'false', __VUE_PROD_HYDRATION_MISMATCH_DETAILS__: 'false' } })

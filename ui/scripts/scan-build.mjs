import { readdir, readFile } from 'node:fs/promises'
import path from 'node:path'

const root = process.argv[2]

if (!root) {
  console.error('usage: node ./scripts/scan-build.mjs <dist-dir>')
  process.exit(2)
}

const distRoot = path.resolve(root)
const indexPath = path.join(distRoot, 'index.html')
const html = await readFile(indexPath, 'utf8')

if (/<script(?![^>]*\bsrc=)[^>]*>/i.test(html)) {
  throw new Error('inline script detected in built index.html')
}
if (/<style[^>]*>/i.test(html)) {
  throw new Error('inline style detected in built index.html')
}

const allowlistedUrls = new Set([
  'http://www.w3.org/2000/svg',
  'http://www.w3.org/1999/xlink',
  'http://www.w3.org/1998/Math/MathML'
])
const allowlistedPrefixes = ['https://vuejs.org/error-reference/']

for (const filePath of await listFiles(distRoot)) {
  const contents = await readFile(filePath, 'utf8')
  const matches = contents.match(/https?:\/\/[^"')\s]+/g) ?? []
  const disallowed = matches.filter(
    (value) =>
      !allowlistedUrls.has(value) &&
      !allowlistedPrefixes.some((prefix) => value.startsWith(prefix))
  )
  if (disallowed.length > 0) {
    throw new Error(`external URL detected in ${path.relative(distRoot, filePath)}: ${disallowed[0]}`)
  }
}

async function listFiles(dir) {
  const entries = await readdir(dir, { withFileTypes: true })
  const files = []
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name)
    if (entry.isDirectory()) {
      files.push(...(await listFiles(fullPath)))
      continue
    }
    if (entry.isFile()) {
      files.push(fullPath)
    }
  }
  return files
}

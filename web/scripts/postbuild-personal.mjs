import fs from 'node:fs'
import path from 'node:path'

const rootDir = path.resolve(process.cwd())
const distDir = path.join(rootDir, 'dist-personal')
const src = path.join(distDir, 'index.personal.html')
const dst = path.join(distDir, 'index.html')

if (!fs.existsSync(distDir)) {
  throw new Error(`dist-personal 不存在: ${distDir}`)
}
if (!fs.existsSync(src)) {
  throw new Error(`缺少 personal 前端入口: ${src}`)
}

fs.copyFileSync(src, dst)


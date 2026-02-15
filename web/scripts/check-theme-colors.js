import fs from 'node:fs/promises';
import path from 'node:path';
import process from 'node:process';
import { fileURLToPath } from 'node:url';

const scriptDir = path.dirname(fileURLToPath(import.meta.url));
const webRoot = path.resolve(scriptDir, '..');
const srcRoot = path.join(webRoot, 'src');
const assetsRoot = path.join(srcRoot, 'assets');

const banned = [
  { label: '#0d6efd', re: /#0d6efd/gi },
  { label: 'rgb(13, 110, 253)', re: /rgb\(\s*13\s*,\s*110\s*,\s*253\s*\)/gi },
  { label: '13, 110, 253', re: /13\s*,\s*110\s*,\s*253/gi },
  { label: '99, 102, 241', re: /99\s*,\s*102\s*,\s*241/gi },
  { label: '59, 130, 246', re: /59\s*,\s*130\s*,\s*246/gi },
  { label: '#3b82f6', re: /#3b82f6/gi },
  { label: '#6366f1', re: /#6366f1/gi },
];

const textExtensions = new Set([
  '.css',
  '.html',
  '.js',
  '.json',
  '.jsx',
  '.md',
  '.scss',
  '.ts',
  '.tsx',
  '.txt',
  '.yml',
  '.yaml',
]);

function toPosix(p) {
  return p.split(path.sep).join('/');
}

function normalizeSnippet(line, maxLen = 220) {
  const s = line.replace(/\t/g, '  ').trimEnd();
  if (s.length <= maxLen) return s;
  return `${s.slice(0, maxLen - 1)}…`;
}

async function* walkFiles(dir) {
  const entries = await fs.readdir(dir, { withFileTypes: true });
  for (const entry of entries) {
    const fullPath = path.join(dir, entry.name);
    if (fullPath === assetsRoot || fullPath.startsWith(`${assetsRoot}${path.sep}`)) continue;
    if (entry.isDirectory()) {
      yield* walkFiles(fullPath);
      continue;
    }
    if (!entry.isFile()) continue;
    yield fullPath;
  }
}

async function readUtf8IfText(filePath) {
  const ext = path.extname(filePath).toLowerCase();
  if (!textExtensions.has(ext)) return null;
  const buf = await fs.readFile(filePath);
  if (buf.includes(0)) return null;
  return buf.toString('utf8');
}

function findViolations(text, relPath) {
  const violations = [];
  const lines = text.split(/\r?\n/);
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    for (const { label, re } of banned) {
      re.lastIndex = 0;
      let match;
      while ((match = re.exec(line)) !== null) {
        violations.push({
          file: relPath,
          line: i + 1,
          column: match.index + 1,
          label,
          snippet: normalizeSnippet(line),
        });
      }
    }
  }
  return violations;
}

async function main() {
  const violations = [];

  for await (const filePath of walkFiles(srcRoot)) {
    const relPath = toPosix(path.relative(webRoot, filePath));
    const text = await readUtf8IfText(filePath);
    if (!text) continue;
    violations.push(...findViolations(text, relPath));
  }

  if (violations.length === 0) {
    process.stdout.write('check:theme: ok (no banned bright-blue literals found in src/)\n');
    return;
  }

  process.stderr.write('check:theme: found banned bright-blue literals:\n');
  for (const v of violations) {
    process.stderr.write(`- ${v.file}:${v.line}:${v.column} ${v.label}  ${v.snippet}\n`);
  }
  process.exitCode = 1;
}

await main();

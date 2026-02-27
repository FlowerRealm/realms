import crypto from 'node:crypto';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { readFile, writeFile } from 'node:fs/promises';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const webRoot = path.resolve(__dirname, '..');
const publicRoot = path.join(webRoot, 'public');

const mode = process.argv.includes('--write') ? 'write' : 'check';

function sha384Base64(buf) {
  return crypto.createHash('sha384').update(buf).digest('base64');
}

async function computeIntegrity(urlPath) {
  if (!urlPath.startsWith('/')) {
    throw new Error(`expected absolute URL path, got: ${urlPath}`);
  }
  const fsPath = path.join(publicRoot, urlPath);
  const data = await readFile(fsPath);
  return `sha384-${sha384Base64(data)}`;
}

function escapeRegExp(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function findTag(html, kind, urlPath) {
  const attr = kind === 'link' ? 'href' : 'src';
  const re = new RegExp(`<${kind}([^>]*?)\\s${attr}="${escapeRegExp(urlPath)}"([^>]*)>`, 'i');
  const m = html.match(re);
  if (!m) return null;
  return { re, full: m[0] };
}

function getAttr(tag, name) {
  const re = new RegExp(`\\s${name}="([^"]*)"`, 'i');
  const m = tag.match(re);
  return m ? m[1] : '';
}

function setAttr(tag, name, value) {
  const re = new RegExp(`(\\s${name}=")([^"]*)(")`, 'i');
  if (re.test(tag)) {
    return tag.replace(re, `$1${value}$3`);
  }
  const endMatch = tag.match(/\s*\/?>\s*$/);
  if (!endMatch) {
    throw new Error(`invalid tag (no closing '>'): ${tag}`);
  }
  const insertAt = tag.length - endMatch[0].length;
  const before = tag.slice(0, insertAt).trimEnd();
  const after = tag.slice(insertAt);
  return `${before} ${name}="${value}"${after}`;
}

function stripMisplacedSolidus(tag) {
  // Recover from the common mistake: `<link ... / crossorigin="anonymous">` (solidus must be before `>` only).
  // Only remove a standalone solidus that appears before an attribute assignment.
  return tag.replace(/\s\/\s+(?=[a-zA-Z_:][-a-zA-Z0-9_:.]*=)/g, ' ');
}

async function updateHTMLFile(htmlPath, resources) {
  const absPath = path.join(webRoot, htmlPath);
  const original = await readFile(absPath, 'utf8');
  let next = original;

  const problems = [];
  for (const r of resources) {
    const tagInfo = findTag(next, r.kind, r.urlPath);
    if (!tagInfo) {
      problems.push(`${htmlPath}: missing <${r.kind}> for ${r.urlPath}`);
      continue;
    }
    const malformedSolidus = /\s\/\s+(?=[a-zA-Z_:][-a-zA-Z0-9_:.]*=)/.test(tagInfo.full);
    if (malformedSolidus) {
      if (mode === 'write') {
        const fixed = stripMisplacedSolidus(tagInfo.full);
        next = next.replace(tagInfo.full, fixed);
      } else {
        problems.push(`${htmlPath}: malformed tag for ${r.urlPath} (misplaced '/')`);
      }
    }

    const currentTagInfo = findTag(next, r.kind, r.urlPath);
    if (!currentTagInfo) {
      problems.push(`${htmlPath}: missing <${r.kind}> for ${r.urlPath} after rewrite`);
      continue;
    }
    let tag = stripMisplacedSolidus(currentTagInfo.full);

    const expectedIntegrity = await computeIntegrity(r.urlPath);
    const currentIntegrity = getAttr(tag, 'integrity');
    const currentCrossOrigin = getAttr(tag, 'crossorigin');

    if (currentIntegrity !== expectedIntegrity) {
      if (mode === 'write') {
        tag = setAttr(tag, 'integrity', expectedIntegrity);
      } else {
        problems.push(`${htmlPath}: ${r.urlPath} integrity mismatch (expected ${expectedIntegrity}, got ${currentIntegrity || 'missing'})`);
      }
    }

    if (currentCrossOrigin !== 'anonymous') {
      if (mode === 'write') {
        tag = setAttr(tag, 'crossorigin', 'anonymous');
      } else {
        problems.push(`${htmlPath}: ${r.urlPath} missing crossorigin=\"anonymous\"`);
      }
    }

    if (mode === 'write') {
      next = next.replace(currentTagInfo.full, tag);
    }
  }

  if (mode === 'write') {
    if (next !== original) {
      await writeFile(absPath, next, 'utf8');
    }
  }

  return problems;
}

async function main() {
  const resources = [
    { kind: 'link', urlPath: '/vendor/fonts/fonts.css' },
    { kind: 'link', urlPath: '/vendor/bootstrap/bootstrap.min.css' },
    { kind: 'link', urlPath: '/vendor/flatpickr/flatpickr.min.css' },
    { kind: 'link', urlPath: '/vendor/remixicon/fonts/remixicon.css' },
    { kind: 'script', urlPath: '/vendor/flatpickr/flatpickr.min.js' },
    { kind: 'script', urlPath: '/vendor/flatpickr/zh.js' },
    { kind: 'script', urlPath: '/vendor/chart.js/chart.umd.min.js' },
    { kind: 'script', urlPath: '/vendor/bootstrap/bootstrap.bundle.min.js' },
  ];

  const problems = [];
  problems.push(...(await updateHTMLFile('index.html', resources)));
  problems.push(...(await updateHTMLFile('index.personal.html', resources)));

  if (problems.length > 0) {
    const header =
      mode === 'write'
        ? 'SRI update completed with notes:'
        : 'SRI check failed. Fix by running: npm run fix:sri';
    throw new Error([header, '', ...problems].join('\n'));
  }
}

main().catch((e) => {
  console.error(e instanceof Error ? e.message : String(e));
  process.exit(1);
});

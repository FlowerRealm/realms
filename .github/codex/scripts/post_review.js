const fs = require('fs');

function readText(path) {
  return fs.readFileSync(path, 'utf8');
}

function extractJSONBlock(markdown) {
  const begin = '<!-- CODEX_REVIEW_JSON_V1_BEGIN';
  const end = 'CODEX_REVIEW_JSON_V1_END -->';
  const bi = markdown.indexOf(begin);
  if (bi < 0) return { json: null, stripped: markdown };
  const ei = markdown.indexOf(end, bi);
  if (ei < 0) return { json: null, stripped: markdown };

  const afterBegin = bi + begin.length;
  const jsonText = markdown.slice(afterBegin, ei).trim();

  const stripped = (markdown.slice(0, bi) + markdown.slice(ei + end.length)).trim();
  return { json: jsonText, stripped };
}

function parseUnifiedDiffNewRanges(diffText) {
  const rangesByPath = new Map();
  let currentPath = null;

  for (const raw of diffText.split('\n')) {
    const line = raw.replace(/\r$/, '');

    if (line.startsWith('+++ ')) {
      const m = /^\+\+\+ b\/(.+)$/.exec(line);
      if (m && m[1] && m[1] !== '/dev/null') {
        currentPath = m[1].trim();
      } else {
        currentPath = null;
      }
      continue;
    }

    const hm = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(line);
    if (!hm || !currentPath) continue;

    const newStart = Number.parseInt(hm[3], 10);
    const newCount = hm[4] ? Number.parseInt(hm[4], 10) : 1;
    if (!Number.isFinite(newStart) || !Number.isFinite(newCount) || newCount <= 0) continue;

    const start = newStart;
    const end = newStart + newCount - 1;
    const list = rangesByPath.get(currentPath) || [];
    list.push({ start, end });
    rangesByPath.set(currentPath, list);
  }

  return rangesByPath;
}

function normalizeLine(ranges, line) {
  if (!ranges || ranges.length === 0) return null;
  if (Number.isFinite(line)) {
    for (const r of ranges) {
      if (line >= r.start && line <= r.end) return line;
    }
  }
  return ranges[0].start;
}

function safeSuggestion(s) {
  const t = (s || '').trimEnd();
  if (!t) return '';
  if (t.includes('```')) return '';
  return t;
}

async function postFallbackIssueComment({ github, context, body }) {
  await github.rest.issues.createComment({
    owner: context.repo.owner,
    repo: context.repo.repo,
    issue_number: Number(context.payload.pull_request?.number || context.payload.issue?.number),
    body,
  });
}

module.exports = async ({ github, context, core }) => {
  const prNumber = Number(process.env.PR_NUMBER || '');
  const headSha = (process.env.HEAD_SHA || '').trim();

  if (!Number.isFinite(prNumber) || prNumber <= 0) {
    throw new Error(`invalid PR_NUMBER: ${process.env.PR_NUMBER || ''}`);
  }
  if (!headSha) {
    throw new Error('missing HEAD_SHA');
  }

  const raw = readText('codex-output.md');
  const { json, stripped } = extractJSONBlock(raw);
  if (!json) {
    core.warning('missing CODEX_REVIEW_JSON_V1 block; falling back to issue comment');
    await postFallbackIssueComment({ github, context, body: raw });
    return;
  }

  let payload;
  try {
    payload = JSON.parse(json);
  } catch (e) {
    core.warning(`invalid CODEX_REVIEW_JSON_V1 JSON; falling back to issue comment: ${e instanceof Error ? e.message : String(e)}`);
    await postFallbackIssueComment({ github, context, body: raw });
    return;
  }

  const commentsIn = Array.isArray(payload?.comments) ? payload.comments : [];
  const diffText = readText('pr.diff');
  const rangesByPath = parseUnifiedDiffNewRanges(diffText);

  const inlineComments = [];
  const dropped = [];
  const maxInline = 25;

  for (const c of commentsIn) {
    if (inlineComments.length >= maxInline) {
      dropped.push({ reason: 'too many comments', comment: c });
      continue;
    }
    const path = (c?.path || '').trim();
    const body = (c?.body || '').trim();
    const lineRaw = Number.parseInt(String(c?.line || ''), 10);

    if (!path || !body) {
      dropped.push({ reason: 'missing path/body', comment: c });
      continue;
    }

    const ranges = rangesByPath.get(path);
    if (!ranges || ranges.length === 0) {
      dropped.push({ reason: 'path not in diff or no new-side hunks', comment: c });
      continue;
    }

    const line = normalizeLine(ranges, lineRaw);
    if (line == null) {
      dropped.push({ reason: 'cannot normalize line', comment: c });
      continue;
    }

    const suggestion = safeSuggestion(c?.suggestion || '');
    let finalBody = body;
    if (c?.suggestion && !suggestion) {
      dropped.push({ reason: 'suggestion contained ```; ignored', comment: { path, line: lineRaw } });
    }
    if (suggestion) {
      finalBody += `\n\n\`\`\`suggestion\n${suggestion}\n\`\`\``;
    }

    inlineComments.push({
      path,
      line,
      side: 'RIGHT',
      body: finalBody,
    });
  }

  let reviewBody = stripped.trim() || '(no summary)';
  if (dropped.length > 0) {
    const lines = dropped.slice(0, 8).map((d) => `- dropped: ${d.reason}`);
    if (dropped.length > 8) lines.push(`- dropped: ... (${dropped.length - 8} more)`);
    reviewBody += `\n\n---\n\n### Publisher notes (non-blocking)\n${lines.join('\n')}\n`;
  }

  try {
    await github.rest.pulls.createReview({
      owner: context.repo.owner,
      repo: context.repo.repo,
      pull_number: prNumber,
      commit_id: headSha,
      event: 'COMMENT',
      body: reviewBody,
      comments: inlineComments,
    });
  } catch (e) {
    core.warning(`createReview failed; falling back to issue comment: ${e instanceof Error ? e.message : String(e)}`);
    await postFallbackIssueComment({ github, context, body: raw });
  }
};


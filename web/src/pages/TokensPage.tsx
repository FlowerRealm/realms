import { useEffect, useMemo, useRef, useState } from 'react';

import {
  createUserToken,
  deleteUserToken,
  listUserTokens,
  revealUserToken,
  revokeUserToken,
  rotateUserToken,
  type UserToken,
} from '../api/tokens';
import { BootstrapModal } from '../components/BootstrapModal';
import { closeModalById } from '../components/modal';

export function TokensPage() {
  const [tokens, setTokens] = useState<UserToken[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');
  const [revealed, setRevealed] = useState<Record<number, string>>({});
  const [revealLoading, setRevealLoading] = useState<Record<number, boolean>>({});
  const [copiedID, setCopiedID] = useState<number | null>(null);

  const [name, setName] = useState('');

  const baseURL = useMemo(() => window.location.origin, []);
  const apiBaseURL = useMemo(() => `${baseURL}/v1`, [baseURL]);

  const openGeneratedTokenModalBtnRef = useRef<HTMLButtonElement | null>(null);
  const pendingGeneratedTokenRef = useRef<string | null>(null);
  const [generatedToken, setGeneratedToken] = useState('');
  const [generatedCopied, setGeneratedCopied] = useState(false);

  useEffect(() => {
    if (copiedID == null) return;
    const t = window.setTimeout(() => setCopiedID(null), 2000);
    return () => window.clearTimeout(t);
  }, [copiedID]);

  useEffect(() => {
    if (!generatedCopied) return;
    const t = window.setTimeout(() => setGeneratedCopied(false), 2000);
    return () => window.clearTimeout(t);
  }, [generatedCopied]);

  async function refresh() {
    setErr('');
    setLoading(true);
    try {
      const res = await listUserTokens();
      if (!res.success) {
        throw new Error(res.message || '加载失败');
      }
      const nextTokens = res.data || [];
      setTokens(nextTokens);
      setRevealed((prev) => {
        const active = new Set(nextTokens.filter((t) => t.status === 1).map((t) => t.id));
        const next: Record<number, string> = {};
        for (const [k, v] of Object.entries(prev)) {
          const id = Number(k);
          if (active.has(id)) next[id] = v;
        }
        return next;
      });
      setRevealLoading((prev) => {
        const active = new Set(nextTokens.filter((t) => t.status === 1).map((t) => t.id));
        const next: Record<number, boolean> = {};
        for (const [k, v] of Object.entries(prev)) {
          const id = Number(k);
          if (active.has(id)) next[id] = v;
        }
        return next;
      });
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }

  function openGeneratedTokenModal(tok: string) {
    setGeneratedCopied(false);
    setGeneratedToken(tok);
    window.setTimeout(() => openGeneratedTokenModalBtnRef.current?.click(), 0);
  }

  async function copyText(raw: string): Promise<boolean> {
    try {
      await navigator.clipboard.writeText(raw);
      return true;
    } catch {
      // fallback
    }
    try {
      const el = document.createElement('textarea');
      el.value = raw;
      el.setAttribute('readonly', 'true');
      el.style.position = 'fixed';
      el.style.top = '0';
      el.style.left = '0';
      el.style.opacity = '0';
      document.body.appendChild(el);
      el.select();
      const ok = document.execCommand('copy');
      document.body.removeChild(el);
      return ok;
    } catch {
      return false;
    }
  }

  async function copyToken(raw: string, tokenID: number) {
    const ok = await copyText(raw);
    if (ok) setCopiedID(tokenID);
  }

  async function revealToken(tokenID: number): Promise<string> {
    if (revealed[tokenID]) return revealed[tokenID];
    setRevealLoading((prev) => ({ ...prev, [tokenID]: true }));
    try {
      const res = await revealUserToken(tokenID);
      if (!res.success) {
        throw new Error(res.message || '查看失败');
      }
      const tok = (res.data?.token || '').toString();
      if (tok.trim() === '') {
        throw new Error('查看失败');
      }
      setRevealed((prev) => ({ ...prev, [tokenID]: tok }));
      return tok;
    } finally {
      setRevealLoading((prev) => ({ ...prev, [tokenID]: false }));
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="row g-4">
        <div className="col-12">
          <div className="card">
            <div className="card-body d-flex flex-column flex-md-row justify-content-between align-items-center">
              <div className="d-flex align-items-center mb-3 mb-md-0">
                <div
                  className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-3"
                  style={{ width: 48, height: 48 }}
                >
                  <span className="fs-4 material-symbols-rounded">key</span>
                </div>
                <div>
                  <h5 className="mb-1 fw-semibold">我的 API 令牌</h5>
                  <p className="mb-0 text-muted small">为安全起见，令牌默认隐藏；可在此页查看/复制。令牌撤销后无法查看。</p>
                </div>
              </div>
              <button type="button" className="btn btn-primary btn-sm" data-bs-toggle="modal" data-bs-target="#createTokenModal">
                <span className="me-1 material-symbols-rounded">add</span> 创建令牌
              </button>
            </div>
          </div>
        </div>

        {err ? (
          <div className="col-12">
            <div className="alert alert-danger mb-0" role="alert">
              <span className="me-2 material-symbols-rounded">report</span>
              {err}
            </div>
          </div>
        ) : null}

        <div className="col-lg-8">
          <div className="card h-100 overflow-hidden">
            <div className="card-body p-0">
              <div className="table-responsive">
                <table className="table table-hover align-middle mb-0">
                  <thead className="bg-light text-muted small text-uppercase">
                    <tr>
                      <th scope="col" className="fw-medium ps-4 py-3">
                        名称
                      </th>
                      <th scope="col" className="fw-medium py-3">
                        预览
                      </th>
                      <th scope="col" className="fw-medium py-3">
                        状态
                      </th>
                      <th scope="col" className="fw-medium text-end pe-4 py-3">
                        操作
                      </th>
                    </tr>
                  </thead>
                  <tbody className="border-top-0">
                    {loading ? (
                      <tr>
                        <td colSpan={4} className="text-center py-5 text-muted">
                          加载中…
                        </td>
                      </tr>
                    ) : tokens.length === 0 ? (
                      <tr>
                        <td colSpan={4} className="text-center py-5 text-muted">
                          <div className="mb-2">
                            <span className="fs-3 text-light-emphasis material-symbols-rounded">inbox</span>
                          </div>
                          暂无令牌，点击右上角按钮创建一个。
                        </td>
                      </tr>
                    ) : (
                      tokens.map((t) => (
                        <tr key={t.id}>
                          <td className="ps-4 py-3">
                            {t.name ? (
                              <span className="fw-medium text-dark">{t.name}</span>
                            ) : (
                              <span className="text-muted small fst-italic">无备注</span>
                            )}
                          </td>
                          <td className="py-3">
                            {revealed[t.id] ? (
                              <code className="bg-light px-2 py-1 rounded text-dark border user-select-all">{revealed[t.id]}</code>
                            ) : t.token_hint ? (
                              <code className="bg-light px-2 py-1 rounded text-muted border user-select-all">{`sk_…${t.token_hint}`}</code>
                            ) : (
                              <span className="text-muted small">-</span>
                            )}
                          </td>
                          <td className="py-3">
                            {t.status === 1 ? (
                              <span className="badge bg-success bg-opacity-10 text-success rounded-pill px-2">活跃</span>
                            ) : (
                              <span className="badge bg-secondary bg-opacity-10 text-secondary rounded-pill px-2">已撤销</span>
                            )}
                          </td>
                          <td className="text-end pe-4 py-3">
                            {t.status === 1 ? (
                              <>
                                <button
                                  className="btn btn-link text-secondary p-0 text-decoration-none small"
                                  type="button"
                                  disabled={loading || revealLoading[t.id]}
                                  onClick={async () => {
                                    setErr('');
                                    if (revealed[t.id]) {
                                      setRevealed((prev) => {
                                        const next = { ...prev };
                                        delete next[t.id];
                                        return next;
                                      });
                                      return;
                                    }
                                    try {
                                      await revealToken(t.id);
                                    } catch (e) {
                                      setErr(e instanceof Error ? e.message : '查看失败');
                                    }
                                  }}
                                >
                                  {revealed[t.id] ? '隐藏' : '查看'}
                                </button>

                                <span className="text-muted small mx-2">|</span>

                                <button
                                  className="btn btn-link text-secondary p-0 text-decoration-none small"
                                  type="button"
                                  disabled={loading || revealLoading[t.id]}
                                  onClick={async () => {
                                    setErr('');
                                    try {
                                      const tok = revealed[t.id] ? revealed[t.id] : await revealToken(t.id);
                                      await copyToken(tok, t.id);
                                    } catch (e) {
                                      setErr(e instanceof Error ? e.message : '复制失败');
                                    }
                                  }}
                                >
                                  {copiedID === t.id ? '已复制' : '复制'}
                                </button>

                                <span className="text-muted small mx-2">|</span>
                              </>
                            ) : null}

                            <button
                              className="btn btn-link text-primary p-0 text-decoration-none small"
                              type="button"
                              disabled={loading}
                              onClick={async () => {
                                setErr('');
                                setRevealed((prev) => {
                                  const next = { ...prev };
                                  delete next[t.id];
                                  return next;
                                });
                                try {
                                  const res = await rotateUserToken(t.id);
                                  if (!res.success) {
                                    throw new Error(res.message || '重新生成失败');
                                  }
                                  const tok = res.data?.token;
                                  await refresh();
                                  if (tok) openGeneratedTokenModal(tok);
                                } catch (e) {
                                  setErr(e instanceof Error ? e.message : '重新生成失败');
                                }
                              }}
                            >
                              重新生成
                            </button>

                            <span className="text-muted small mx-2">|</span>

                            {t.status === 1 ? (
                              <button
                                className="btn btn-link text-danger p-0 text-decoration-none small"
                                type="button"
                                disabled={loading}
                                onClick={async () => {
                                  setErr('');
                                  try {
                                    const res = await revokeUserToken(t.id);
                                    if (!res.success) {
                                      throw new Error(res.message || '撤销失败');
                                    }
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '撤销失败');
                                  }
                                }}
                              >
                                撤销
                              </button>
                            ) : (
                              <button
                                className="btn btn-link text-danger p-0 text-decoration-none small"
                                type="button"
                                disabled={loading}
                                onClick={async () => {
                                  setErr('');
                                  try {
                                    const res = await deleteUserToken(t.id);
                                    if (!res.success) {
                                      throw new Error(res.message || '删除失败');
                                    }
                                    await refresh();
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  }
                                }}
                              >
                                删除
                              </button>
                            )}
                          </td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
        </div>

        <div className="col-lg-4">
          <div className="card h-100 bg-primary bg-opacity-10 border-0">
            <div className="card-body">
              <h5 className="mb-3 fw-semibold text-primary">
                <span className="me-2 material-symbols-rounded">terminal</span>使用方式
              </h5>
              <p className="text-muted small mb-3">
                推荐：配置 <code>OPENAI_BASE_URL</code>/<code>OPENAI_API_KEY</code>，并在 Codex CLI 中将 <code>model_provider</code> 设置为{' '}
                <code>realms</code>。
              </p>

                <div className="bg-dark rounded-3 p-3 mb-3 position-relative overflow-hidden">
                  <div className="d-flex justify-content-between align-items-center mb-2">
                  <small className="text-secondary text-uppercase fw-bold smaller">终端</small>
                    <div className="d-flex gap-1">
                      <div className="rounded-circle bg-danger" style={{ width: 8, height: 8 }}></div>
                      <div className="rounded-circle bg-warning" style={{ width: 8, height: 8 }}></div>
                      <div className="rounded-circle bg-success" style={{ width: 8, height: 8 }}></div>
                    </div>
                  </div>
                <pre className="mb-0 text-light overflow-auto smaller font-monospace" style={{ whiteSpace: 'pre-wrap' }}>
                  <code>
                    {'# Linux/macOS（bash/zsh）\n'}
                    {`export OPENAI_BASE_URL="${apiBaseURL}"\n`}
                    {'export OPENAI_API_KEY="'}
                    <span className="text-warning">sk_...</span>
                    {'"\n\n'}
                    {'# Windows（PowerShell）\n'}
                    {`$env:OPENAI_BASE_URL = "${apiBaseURL}"\n`}
                    {'$env:OPENAI_API_KEY = "'}
                    <span className="text-warning">sk_...</span>
                    {'"\n\n'}
                    {'# ~/.codex/config.toml（Windows: %USERPROFILE%\\\\.codex\\\\config.toml）\n'}
                    {'model_provider = "realms"\n\n'}
                    {'[model_providers.realms]\n'}
                    {'name = "Realms"\n'}
                    {`base_url = "${apiBaseURL}"\n`}
                    {'wire_api = "responses"\n'}
                    {'requires_openai_auth = true'}
                  </code>
                </pre>
              </div>

              <div className="d-flex align-items-start small text-muted">
                <span className="me-2 mt-1 text-primary material-symbols-rounded">info</span>
                <div>
                  API 基础地址：<br />
                  <strong className="text-dark user-select-all">{apiBaseURL}</strong>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* programmatically open the generated-token modal */}
      <button ref={openGeneratedTokenModalBtnRef} type="button" className="d-none" data-bs-toggle="modal" data-bs-target="#generatedTokenModal"></button>

      <BootstrapModal
        id="generatedTokenModal"
        title="令牌已生成"
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setGeneratedCopied(false);
          setGeneratedToken('');
        }}
      >
        <div className="alert alert-warning border-0 bg-warning bg-opacity-10 d-flex align-items-start mb-3">
          <span className="me-2 mt-1 material-symbols-rounded">warning</span>
          <div className="small">
            请复制并妥善保存。也可以在令牌列表页查看/复制（默认隐藏）。令牌撤销后无法查看。
          </div>
        </div>

        <div className="mb-3">
          <label className="form-label small fw-bold text-uppercase text-muted">API 令牌</label>
          <div className="input-group input-group-lg">
            <input
              type="text"
              className="form-control font-monospace bg-light border-end-0"
              value={generatedToken}
              readOnly
              onClick={(e) => {
                try {
                  e.currentTarget.select();
                } catch {
                  // ignore
                }
              }}
            />
            <button
              className={`btn ${generatedCopied ? 'btn-success text-white' : 'btn-light'} border border-start-0 px-4`}
              type="button"
              title="点击复制"
              disabled={generatedToken.trim() === ''}
              onClick={async () => {
                setErr('');
                const ok = await copyText(generatedToken);
                if (!ok) {
                  setErr('复制失败');
                  return;
                }
                setGeneratedCopied(true);
              }}
            >
              <span className="material-symbols-rounded">{generatedCopied ? 'check' : 'content_copy'}</span>
            </button>
          </div>
          <div className={`text-success small mt-2 opacity-0 transition-opacity${generatedCopied ? ' opacity-100' : ''}`}>
            <span className="me-1 material-symbols-rounded">check</span>已成功复制到剪贴板
          </div>
        </div>

        <div className="modal-footer border-top-0 px-0 pb-0">
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            关闭
          </button>
        </div>
      </BootstrapModal>

      <BootstrapModal
        id="createTokenModal"
        title="创建新 API 令牌"
        dialogClassName="modal-dialog-centered"
        headerClassName="border-bottom-0 pb-0"
        bodyClassName="pt-4"
        onHidden={() => {
          setName('');
          const tok = pendingGeneratedTokenRef.current;
          pendingGeneratedTokenRef.current = null;
          if (tok) openGeneratedTokenModal(tok);
        }}
      >
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            try {
              const res = await createUserToken(name.trim() || undefined);
              if (!res.success) {
                throw new Error(res.message || '创建失败');
              }
              const tok = res.data?.token || '';
              pendingGeneratedTokenRef.current = tok.trim() === '' ? null : tok;
              setName('');
              closeModalById('createTokenModal');
              await refresh();
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="mb-3">
            <label className="form-label fw-medium text-dark">备注名称</label>
            <input
              name="name"
              type="text"
              className="form-control"
              placeholder="例如：我的项目、笔记本 CLI…"
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
            <div className="form-text text-muted">给令牌起个名字，方便日后管理。</div>
          </div>
          <div className="alert alert-light border mb-0 d-flex align-items-start small">
            <span className="text-primary me-2 mt-1 material-symbols-rounded">info</span>
            <div>创建成功后会弹窗展示一次；也可以在列表页查看/复制（默认隐藏）。令牌撤销后无法查看。</div>
          </div>
          <div className="modal-footer border-top-0 px-0">
            <button type="button" className="btn btn-light text-muted" data-bs-dismiss="modal">
              取消
            </button>
            <button type="submit" className="btn btn-primary px-4" disabled={loading}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>
    </div>
  );
}

import { useEffect, useRef, useState } from 'react';

import {
  createUserToken,
  deleteUserToken,
  getUserTokenGroups,
  listUserTokens,
  replaceUserTokenGroups,
  revealUserToken,
  revokeUserToken,
  rotateUserToken,
  type TokenGroupOption,
  type UserTokenGroups,
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

  const openGeneratedTokenModalBtnRef = useRef<HTMLButtonElement | null>(null);
  const pendingGeneratedTokenRef = useRef<string | null>(null);
  const [generatedToken, setGeneratedToken] = useState('');
  const [generatedCopied, setGeneratedCopied] = useState(false);

  const openTokenGroupsModalBtnRef = useRef<HTMLButtonElement | null>(null);
  const [tokenGroupsToken, setTokenGroupsToken] = useState<UserToken | null>(null);
  const [tokenGroupsData, setTokenGroupsData] = useState<UserTokenGroups | null>(null);
  const [tokenGroupsLoading, setTokenGroupsLoading] = useState(false);
  const [tokenGroupsSaving, setTokenGroupsSaving] = useState(false);
  const [tokenGroupsErr, setTokenGroupsErr] = useState('');
  const [tokenGroupsNotice, setTokenGroupsNotice] = useState('');
  const [selectedGroups, setSelectedGroups] = useState<string[]>(['default']);
  const [addGroup, setAddGroup] = useState('');

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

  function normalizeGroupOrder(inGroups: string[]): string[] {
    const out: string[] = [];
    const seen = new Set<string>();
    for (const raw of inGroups || []) {
      const name = (raw || '').trim();
      if (!name) continue;
      if (name === 'default') continue;
      if (seen.has(name)) continue;
      seen.add(name);
      out.push(name);
    }
    out.push('default');
    return out;
  }

  function addSelectedGroup(name: string) {
    const v = (name || '').trim();
    if (!v) return;
    setSelectedGroups((prev) => normalizeGroupOrder([...prev, v]));
  }

  function removeSelectedGroup(name: string) {
    const v = (name || '').trim();
    if (!v || v === 'default') return;
    setSelectedGroups((prev) => normalizeGroupOrder(prev.filter((x) => x !== v)));
  }

  function moveSelectedGroup(name: string, dir: -1 | 1) {
    const v = (name || '').trim();
    if (!v || v === 'default') return;
    setSelectedGroups((prev) => {
      const next = prev.slice();
      const idx = next.findIndex((x) => x === v);
      if (idx < 0) return prev;
      const swap = idx + dir;
      if (swap < 0) return prev;
      if (swap >= next.length) return prev;
      if (next[swap] === 'default') return prev;
      const tmp = next[idx];
      next[idx] = next[swap];
      next[swap] = tmp;
      return next;
    });
  }

  async function openTokenGroupsModal(t: UserToken) {
    setErr('');
    setTokenGroupsErr('');
    setTokenGroupsNotice('');
    setTokenGroupsToken(t);
    setTokenGroupsData(null);
    setTokenGroupsLoading(true);
    setTokenGroupsSaving(false);
    setAddGroup('');
    setSelectedGroups(['default']);
    try {
      const res = await getUserTokenGroups(t.id);
      if (!res.success) throw new Error(res.message || '加载失败');
      const d = res.data || null;
      setTokenGroupsData(d);
      const order = (d?.bindings || []).map((x) => (x.group_name || '').trim()).filter((x) => x);
      setSelectedGroups(normalizeGroupOrder(order));
    } catch (e) {
      const msg = e instanceof Error ? e.message : '加载失败';
      setTokenGroupsErr(msg);
      setSelectedGroups(['default']);
    } finally {
      setTokenGroupsLoading(false);
      window.setTimeout(() => openTokenGroupsModalBtnRef.current?.click(), 0);
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

        <div className="col-12">
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

                                <button
                                  className="btn btn-link text-secondary p-0 text-decoration-none small"
                                  type="button"
                                  disabled={loading}
                                  onClick={() => void openTokenGroupsModal(t)}
                                >
                                  路由分组
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

      </div>

      {/* programmatically open the generated-token modal */}
      <button ref={openGeneratedTokenModalBtnRef} type="button" className="d-none" data-bs-toggle="modal" data-bs-target="#generatedTokenModal"></button>

      {/* programmatically open the token-groups modal */}
      <button ref={openTokenGroupsModalBtnRef} type="button" className="d-none" data-bs-toggle="modal" data-bs-target="#tokenGroupsModal"></button>

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
        id="tokenGroupsModal"
        title={
          tokenGroupsToken
            ? `路由分组：${(tokenGroupsToken.name || '').trim() || `Token #${tokenGroupsToken.id}`}`
            : '路由分组'
        }
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setTokenGroupsToken(null);
          setTokenGroupsData(null);
          setTokenGroupsErr('');
          setTokenGroupsNotice('');
          setTokenGroupsLoading(false);
          setTokenGroupsSaving(false);
          setSelectedGroups(['default']);
          setAddGroup('');
        }}
      >
        {!tokenGroupsToken ? (
          <div className="text-muted">未选择 Token。</div>
        ) : (
          <div>
            {tokenGroupsErr ? (
              <div className="alert alert-danger d-flex align-items-center" role="alert">
                <span className="me-2 material-symbols-rounded">warning</span>
                <div>{tokenGroupsErr}</div>
              </div>
            ) : null}

            {tokenGroupsNotice ? (
              <div className="alert alert-success d-flex align-items-center" role="alert">
                <span className="me-2 material-symbols-rounded">check_circle</span>
                <div>{tokenGroupsNotice}</div>
              </div>
            ) : null}

            <div className="d-flex flex-wrap gap-2 align-items-center mb-3 small">
              <span className="badge bg-light text-dark border">
                用户分组: <span className="font-monospace">{(tokenGroupsData?.user_group || '').trim() || '-'}</span>
              </span>
              <span className="badge bg-light text-dark border">
                生效顺序:{' '}
                <span className="font-monospace">
                  {(tokenGroupsData?.effective_bindings || []).map((b) => b.group_name).filter((x) => x).join(' → ') || 'default'}
                </span>
              </span>
            </div>

            {tokenGroupsLoading ? <div className="text-muted small mb-2">加载中…</div> : null}

            <div className="row g-2 mb-3">
              <div className="col-12 col-md-8">
                <select className="form-select font-monospace" value={addGroup} onChange={(e) => setAddGroup(e.target.value)} disabled={tokenGroupsLoading || tokenGroupsSaving}>
                  <option value="">选择要添加的分组…</option>
                  {(tokenGroupsData?.allowed_groups || [])
                    .filter((g) => g.status === 1 && g.name !== 'default')
                    .slice()
                    .sort((a, b) => {
                      const pa = Number.isFinite(a.user_group_priority) ? a.user_group_priority : 0;
                      const pb = Number.isFinite(b.user_group_priority) ? b.user_group_priority : 0;
                      if (pa !== pb) return pb - pa;
                      return a.name.localeCompare(b.name, 'zh-CN');
                    })
                    .map((g) => (
                      <option key={g.name} value={g.name} disabled={selectedGroups.includes(g.name)}>
                        {g.name} · x{g.price_multiplier}
                      </option>
                    ))}
                </select>
              </div>
              <div className="col-12 col-md-4 d-grid">
                <button
                  type="button"
                  className="btn btn-outline-primary"
                  disabled={!addGroup || selectedGroups.includes(addGroup) || tokenGroupsLoading || tokenGroupsSaving}
                  onClick={() => {
                    addSelectedGroup(addGroup);
                    setAddGroup('');
                  }}
                >
                  添加
                </button>
              </div>
            </div>

            <div className="list-group mb-3">
              {selectedGroups.map((name, idx) => {
                const option: TokenGroupOption | undefined = (tokenGroupsData?.allowed_groups || []).find((x) => x.name === name);
                const isDefault = name === 'default';
                const mult = option ? `x${option.price_multiplier}` : 'x?';
                const statusLabel = !option ? '未知' : option.status === 1 ? '启用' : '禁用';
                const statusCls = !option ? 'badge bg-secondary bg-opacity-10 text-secondary border' : option.status === 1 ? 'badge bg-success bg-opacity-10 text-success border border-success-subtle' : 'badge bg-secondary bg-opacity-10 text-secondary border';
                return (
                  <div key={name} className="list-group-item d-flex align-items-center justify-content-between">
                    <div className="d-flex align-items-center gap-3" style={{ minWidth: 0 }}>
                      <span className="badge bg-light text-dark border font-monospace">{idx + 1}</span>
                      <div className="d-flex flex-column" style={{ minWidth: 0 }}>
                        <div className="d-flex align-items-center gap-2" style={{ minWidth: 0 }}>
                          <span className={`fw-semibold font-monospace text-truncate${isDefault ? ' text-primary' : ''}`} style={{ maxWidth: 260 }} title={name}>
                            {name}
                          </span>
                          <span className="badge bg-light text-dark border fw-normal">{mult}</span>
                          <span className={statusCls}>{statusLabel}</span>
                        </div>
                        {option?.description ? <div className="text-muted smaller text-truncate" style={{ maxWidth: 520 }} title={option.description || ''}>{option.description}</div> : null}
                      </div>
                    </div>
                    <div className="d-inline-flex gap-1">
                      <button type="button" className="btn btn-sm btn-light border" title="上移" disabled={isDefault || idx === 0 || tokenGroupsLoading || tokenGroupsSaving} onClick={() => moveSelectedGroup(name, -1)}>
                        <span className="material-symbols-rounded" style={{ fontSize: 18 }}>arrow_upward</span>
                      </button>
                      <button type="button" className="btn btn-sm btn-light border" title="下移" disabled={isDefault || idx >= selectedGroups.length - 2 || tokenGroupsLoading || tokenGroupsSaving} onClick={() => moveSelectedGroup(name, 1)}>
                        <span className="material-symbols-rounded" style={{ fontSize: 18 }}>arrow_downward</span>
                      </button>
                      <button type="button" className="btn btn-sm btn-light border text-danger" title={isDefault ? 'default 不可移除' : '移除'} disabled={isDefault || tokenGroupsLoading || tokenGroupsSaving} onClick={() => removeSelectedGroup(name)}>
                        <i className="ri-close-line"></i>
                      </button>
                    </div>
                  </div>
                );
              })}
            </div>

            <div className="text-muted small mb-3">提示：按顺序失败转移；计费时采用最终成功分组的倍率。</div>

            <div className="d-grid d-md-flex justify-content-md-end gap-2">
              <button type="button" className="btn btn-light" data-bs-dismiss="modal" disabled={tokenGroupsSaving}>
                关闭
              </button>
              <button
                type="button"
                className="btn btn-primary"
                disabled={tokenGroupsLoading || tokenGroupsSaving}
                onClick={async () => {
                  if (!tokenGroupsToken) return;
                  setTokenGroupsErr('');
                  setTokenGroupsNotice('');
                  setTokenGroupsSaving(true);
                  try {
                    const res = await replaceUserTokenGroups(tokenGroupsToken.id, selectedGroups);
                    if (!res.success) throw new Error(res.message || '保存失败');
                    const refreshed = await getUserTokenGroups(tokenGroupsToken.id);
                    if (refreshed.success) {
                      const d = refreshed.data || null;
                      setTokenGroupsData(d);
                      const order = (d?.bindings || []).map((x) => (x.group_name || '').trim()).filter((x) => x);
                      setSelectedGroups(normalizeGroupOrder(order));
                    }
                    setTokenGroupsNotice('已保存');
                  } catch (e) {
                    setTokenGroupsErr(e instanceof Error ? e.message : '保存失败');
                  } finally {
                    setTokenGroupsSaving(false);
                  }
                }}
              >
                {tokenGroupsSaving ? '保存中…' : '保存'}
              </button>
            </div>
          </div>
        )}
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

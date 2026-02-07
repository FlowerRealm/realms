import { useEffect, useMemo, useState } from 'react';

import { getAdminUsageEventDetail, getAdminUsagePage, type AdminUsagePage, type UsageEventDetail } from '../../api/admin/usage';

function badgeForState(cls: string): string {
  const s = (cls || '').trim();
  if (s) return `badge rounded-pill ${s}`;
  return 'badge rounded-pill bg-light text-secondary border';
}

function costSourceLabel(source: string): string {
  switch ((source || '').trim()) {
    case 'committed':
      return '已结算';
    case 'reserved':
      return '预留';
    default:
      return '事件';
  }
}

function formatDecimalPlain(raw: string): string {
  let s = (raw || '').toString().trim();
  if (!s) return '0';
  if (s.startsWith('+')) s = s.slice(1).trim();
  if (s.startsWith('$')) s = s.slice(1).trim();
  if (!s) return '0';
  if (s.includes('.')) {
    s = s.replace(/0+$/, '').replace(/\.$/, '');
  }
  if (s === '-0' || s === '') return '0';
  return s;
}

function formatUSD(raw: string): string {
  const s = formatDecimalPlain(raw);
  if (s.startsWith('-')) return `-$${s.slice(1)}`;
  return `$${s}`;
}

export function UsageAdminPage() {
  const [data, setData] = useState<AdminUsagePage | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [limit, setLimit] = useState(50);
  const [beforeID, setBeforeID] = useState<number | undefined>(undefined);
  const [afterID, setAfterID] = useState<number | undefined>(undefined);

  const [expandedID, setExpandedID] = useState<number | null>(null);
  const [detailByEventID, setDetailByEventID] = useState<Record<number, UsageEventDetail>>({});
  const [detailLoadingID, setDetailLoadingID] = useState<number | null>(null);

  async function refresh(opts?: { keepCursor?: boolean }) {
    setErr('');
    setLoading(true);
    try {
      const params: { start?: string; end?: string; limit?: number; before_id?: number; after_id?: number } = {
        start: start.trim() || undefined,
        end: end.trim() || undefined,
        limit,
      };
      if (opts?.keepCursor) {
        if (beforeID) params.before_id = beforeID;
        if (afterID) params.after_id = afterID;
      }
      const res = await getAdminUsagePage(params);
      if (!res.success) throw new Error(res.message || '加载失败');
      const d = res.data || null;
      setData(d);
      if (d) {
        if (!start.trim()) setStart(d.start || '');
        if (!end.trim()) setEnd(d.end || '');
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setData(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const window = data?.window;
  const topUsers = data?.top_users || [];
  const events = data?.events || [];

  const canPrev = useMemo(() => typeof data?.prev_after_id === 'number' && (data?.prev_after_id || 0) > 0, [data?.prev_after_id]);
  const canNext = useMemo(() => typeof data?.next_before_id === 'number' && (data?.next_before_id || 0) > 0, [data?.next_before_id]);

  async function loadDetail(eventID: number) {
    if (detailByEventID[eventID]) return;
    setDetailLoadingID(eventID);
    try {
      const res = await getAdminUsageEventDetail(eventID);
      if (!res.success) throw new Error(res.message || '加载详情失败');
      const d = res.data;
      if (d) {
        setDetailByEventID((prev) => ({ ...prev, [eventID]: d }));
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载详情失败');
    } finally {
      setDetailLoadingID(null);
    }
  }

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">全站用量统计</h3>
          <div className="text-muted small">系统级数据汇总，涵盖所有用户及上游通道。</div>
        </div>
      </div>

      {err ? (
        <div className="alert alert-danger">
          <span className="me-2 material-symbols-rounded">warning</span>
          {err}
        </div>
      ) : null}

      <form
        className="row g-2 align-items-end mb-4"
        onSubmit={(e) => {
          e.preventDefault();
          setBeforeID(undefined);
          setAfterID(undefined);
          void refresh();
        }}
      >
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">开始日期</label>
          <input className="form-control form-control-sm" type="date" value={start} onChange={(e) => setStart(e.target.value)} />
        </div>
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">结束日期</label>
          <input className="form-control form-control-sm" type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
        </div>
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">条数</label>
          <select className="form-select form-select-sm" value={limit} onChange={(e) => setLimit(Number.parseInt(e.target.value, 10) || 50)}>
            <option value={20}>20</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </div>
        <div className="col-auto d-flex gap-2">
          <button className="btn btn-sm btn-primary" type="submit" disabled={loading}>
            更新统计
          </button>
          <button
            className="btn btn-sm btn-white border text-dark"
            type="button"
            disabled={loading}
            onClick={() => {
              setStart('');
              setEnd('');
              setBeforeID(undefined);
              setAfterID(undefined);
              void refresh();
            }}
          >
            重置
          </button>
        </div>
      </form>

      {loading ? (
        <div className="text-muted">加载中…</div>
      ) : data && window ? (
        <div className="row g-4">
          <div className="col-12">
            <div className="card border-0 overflow-hidden">
              <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
                <div>
                  <span className="text-primary fw-bold text-uppercase small">{window.window}</span>
                  <span className="text-primary text-opacity-75 smaller ms-2">
                    统计区间: {window.since} ~ {window.until}
                  </span>
                </div>
                <div className="text-primary text-opacity-75 smaller">
                  <i className="ri-shield-check-line me-1"></i> 实时统计
                </div>
              </div>
              <div className="card-body p-4">
                <div className="row g-4">
                  <div className="col-lg-4 border-end">
                    <div className="mb-4">
                      <div className="text-muted smaller mb-1">总营收流水（USD）</div>
                      <h1 className="display-6 fw-bold mb-0 text-dark">{window.total_usd}</h1>
                    </div>
                    <div className="row g-0 py-3 bg-light rounded-3 px-3">
                      <div className="col-6 border-end">
                        <div className="text-muted smaller">已结算</div>
                        <div className="fw-bold h5 mb-0 text-success">{window.committed_usd}</div>
                      </div>
                      <div className="col-6 ps-3">
                        <div className="text-muted smaller">预留中</div>
                        <div className="fw-bold h5 mb-0 text-warning">{window.reserved_usd}</div>
                      </div>
                    </div>
                    <div className="mt-3 smaller text-muted">
                      <i className="ri-information-line me-1"></i> 预留中费用指尚未结束或结算中的请求估算。
                    </div>
                  </div>
                  <div className="col-lg-8 ps-lg-4">
                    <div className="row g-3">
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">全局请求数</div>
                          <div className="h4 fw-bold mb-1">{window.requests}</div>
                          <div className="text-primary smaller fw-medium">{window.rpm} RPM</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">Token 吞吐</div>
                          <div className="h4 fw-bold mb-1">{window.tokens}</div>
                          <div className="text-primary smaller fw-medium">{window.tpm} TPM</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">缓存率</div>
                          <div className="h4 fw-bold mb-1">{window.cache_ratio}</div>
                          <div className="text-muted smaller fw-medium">输入 + 输出</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">缓存 Token</div>
                          <div className="h4 fw-bold mb-1">{window.cached_tokens}</div>
                          <div className="text-muted smaller fw-medium">输入 + 输出</div>
                        </div>
                      </div>
                      <div className="col-12 mt-3">
                        <div className="bg-light p-3 rounded-3">
                          <div className="row text-center small">
                            <div className="col-6 border-end">
                              <div className="text-muted smaller">输入总计</div>
                              <div className="fw-medium">{window.input_tokens}</div>
                            </div>
                            <div className="col-6">
                              <div className="text-muted smaller">输出总计</div>
                              <div className="fw-medium">{window.output_tokens}</div>
                            </div>
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="col-12">
            <div className="card border-0 p-0 overflow-hidden">
              <div className="card-header bg-white py-3 border-bottom-0 px-4">
                <h5 className="mb-0 fw-bold">
                  <i className="ri-group-line me-2"></i>消费排行用户（统计区间）
                </h5>
              </div>
              <div className="card-body p-0">
                <div className="table-responsive">
                  <table className="table table-hover align-middle mb-0 border-0">
                    <thead className="table-light text-muted smaller uppercase">
                      <tr>
                        <th className="ps-4 border-0">用户</th>
                        <th className="border-0">状态</th>
                        <th className="text-end border-0">已结算费用</th>
                        <th className="text-end pe-4 border-0">预留中</th>
                      </tr>
                    </thead>
                    <tbody>
                      {topUsers.map((u) => (
                        <tr key={u.user_id}>
                          <td className="ps-4">
                            <div className="d-flex align-items-center">
                              <div
                                className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-3"
                                style={{ width: 32, height: 32 }}
                              >
                                {(u.email || '?').slice(0, 1)}
                              </div>
                              <div>
                                <div className="fw-bold small">{u.email}</div>
                                <div className="text-muted smaller">{u.role}</div>
                              </div>
                            </div>
                          </td>
                          <td>
                            {u.status === 1 ? (
                              <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill px-2">正常</span>
                            ) : (
                              <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill px-2">禁用</span>
                            )}
                          </td>
                          <td className="text-end font-monospace small fw-bold text-dark">{u.committed_usd}</td>
                          <td className="text-end font-monospace small text-muted pe-4">{u.reserved_usd}</td>
                        </tr>
                      ))}
                      {topUsers.length === 0 ? (
                        <tr>
                          <td colSpan={4} className="text-center py-5 text-muted small">
                            暂无用户用量数据
                          </td>
                        </tr>
                      ) : null}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </div>

          <div className="col-12">
            <div className="card border-0 p-0 overflow-hidden">
              <div className="card-header bg-white py-3 border-bottom-0 px-4 d-flex justify-content-between align-items-center">
                <h5 className="mb-0 fw-bold">
                  <i className="ri-list-check-2 me-2"></i>请求明细
                </h5>
                <div className="d-flex gap-2">
                  <button
                    type="button"
                    className="btn btn-sm btn-outline-secondary"
                    disabled={!canPrev || loading}
                    onClick={() => {
                      const id = data?.prev_after_id;
                      if (!id) return;
                      setBeforeID(undefined);
                      setAfterID(id);
                      void refresh({ keepCursor: true });
                    }}
                  >
                    上一页
                  </button>
                  <button
                    type="button"
                    className="btn btn-sm btn-outline-secondary"
                    disabled={!canNext || loading}
                    onClick={() => {
                      const id = data?.next_before_id;
                      if (!id) return;
                      setAfterID(undefined);
                      setBeforeID(id);
                      void refresh({ keepCursor: true });
                    }}
                  >
                    下一页
                  </button>
                </div>
              </div>
              <div className="card-body p-0 border-top">
                <div className="table-responsive">
                  <table className="table table-hover align-middle mb-0 border-0">
                    <thead className="table-light text-muted smaller uppercase">
                      <tr>
                        <th className="ps-4 border-0">时间</th>
                        <th className="border-0">用户</th>
                        <th className="border-0">接口 / 模型</th>
                        <th className="text-center border-0">状态码</th>
                        <th className="text-end border-0">耗时</th>
                        <th className="text-end border-0">Tokens (In/Out/Cache)</th>
                        <th className="text-end border-0">费用</th>
                        <th className="text-center border-0">状态</th>
                        <th className="text-center border-0">渠道</th>
                        <th className="pe-4 border-0">Request ID</th>
                      </tr>
                    </thead>
                    <tbody className="small">
                      {events.map((e) => (
                        <>
                          <tr
                            key={e.id}
                            role="button"
                            onClick={() => {
                              const next = expandedID === e.id ? null : e.id;
                              setExpandedID(next);
                              if (next) void loadDetail(e.id);
                            }}
                          >
                            <td className="ps-4 text-nowrap font-monospace">
                              <i className={`ri-arrow-right-s-line text-muted me-1 align-middle ${expandedID === e.id ? 'rotate-90' : ''}`}></i>
                              <span className="align-middle">{e.time}</span>
                            </td>
                            <td className="text-nowrap">
                              <div className="fw-bold small">{e.user_email}</div>
                              <div className="text-muted smaller">ID: {e.user_id}</div>
                            </td>
                            <td className="text-nowrap">
                              <div className="badge bg-light text-dark border fw-normal">{e.model}</div>
                              <div className="text-muted smaller mt-1 font-monospace">{e.endpoint}</div>
                            </td>
                            <td className="text-center">
                              {e.status_code === '200' ? (
                                <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill">200</span>
                              ) : (
                                <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill">{e.status_code}</span>
                              )}
                            </td>
                            <td className="text-end font-monospace text-muted">{e.latency_ms} ms</td>
                            <td className="text-end font-monospace">
                              <div>
                                <span className="text-muted">In:</span> {e.input_tokens}
                              </div>
                              <div>
                                <span className="text-muted">Out:</span> {e.output_tokens}
                              </div>
                              {e.cached_tokens !== '-' ? (
                                <div className="text-success smaller">
                                  <span className="material-symbols-rounded">bolt</span> {e.cached_tokens}
                                </div>
                              ) : null}
                            </td>
                            <td className="text-end font-monospace fw-bold text-dark">{e.cost_usd}</td>
                            <td className="text-center text-nowrap">
                              <span className={badgeForState(e.state_badge_class)}>{e.state_label}</span>
                              {e.is_stream ? <div className="badge bg-info-subtle text-info border border-info-subtle rounded-pill px-2 scale-90 mt-1">STREAM</div> : null}
                              {e.error ? (
                                <div className="text-danger smaller mt-1" title={e.error}>
                                  <span className="material-symbols-rounded">error</span> 错误
                                </div>
                              ) : null}
                            </td>
                            <td className="text-center text-nowrap">
                              {e.upstream_channel_name ? <span className="badge bg-light text-dark border fw-normal">{e.upstream_channel_name}</span> : <span className="text-muted">-</span>}
                            </td>
                            <td className="pe-4 font-monospace text-muted small user-select-all" style={{ maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis' }} title={e.request_id}>
                              {e.request_id}
                            </td>
                          </tr>
                          {expandedID === e.id ? (
                            <tr key={`${e.id}-detail`}>
                              <td colSpan={10} className="p-0 border-0">
                                <div className="bg-light border-top px-4 py-3">
                                  {detailLoadingID === e.id ? <div className="text-muted small">加载详情中…</div> : null}
                                  {detailByEventID[e.id] ? (
                                    <div className="row g-3 small">
                                      <div className="col-12 col-lg-4">
                                        <div className="text-muted smaller">Event ID</div>
                                        <div className="font-monospace">{e.id}</div>
                                      </div>
                                      <div className="col-12 col-lg-4">
                                        <div className="text-muted smaller">Error Class</div>
                                        <div className="font-monospace">{e.error_class || '-'}</div>
                                      </div>
                                      <div className="col-12 col-lg-4">
                                        <div className="text-muted smaller">Error Message</div>
                                        <div className="font-monospace">{e.error_message || '-'}</div>
                                      </div>

                                      {detailByEventID[e.id]?.pricing_breakdown ? (
                                        <div className="col-12">
                                          <div className="text-muted smaller">金额计算流程</div>
                                          <div className="font-monospace">
                                            <div>
                                              输入(总/缓存/计费): {detailByEventID[e.id]?.pricing_breakdown?.input_tokens_total} / {detailByEventID[e.id]?.pricing_breakdown?.input_tokens_cached} / {detailByEventID[e.id]?.pricing_breakdown?.input_tokens_billable}
                                            </div>
                                            <div>
                                              输出(总/缓存/计费): {detailByEventID[e.id]?.pricing_breakdown?.output_tokens_total} / {detailByEventID[e.id]?.pricing_breakdown?.output_tokens_cached} / {detailByEventID[e.id]?.pricing_breakdown?.output_tokens_billable}
                                            </div>
                                            <div>
                                              输入(非缓存): {detailByEventID[e.id]?.pricing_breakdown?.input_tokens_billable} × {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.input_usd_per_1m || '0')}/1M = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.input_cost_usd || '0')}
                                            </div>
                                            <div>
                                              输出(非缓存): {detailByEventID[e.id]?.pricing_breakdown?.output_tokens_billable} × {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.output_usd_per_1m || '0')}/1M = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.output_cost_usd || '0')}
                                            </div>
                                            <div>
                                              缓存输入: {detailByEventID[e.id]?.pricing_breakdown?.input_tokens_cached} × {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_input_usd_per_1m || '0')}/1M = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_input_cost_usd || '0')}
                                            </div>
                                            <div>
                                              缓存输出: {detailByEventID[e.id]?.pricing_breakdown?.output_tokens_cached} × {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_output_usd_per_1m || '0')}/1M = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_output_cost_usd || '0')}
                                            </div>
                                            <div className="mt-1">基础费用: {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.base_cost_usd || '0')}</div>
                                            <div>
                                              用户分组倍率: {(detailByEventID[e.id]?.pricing_breakdown?.user_group_factors || []).length > 0
                                                ? (detailByEventID[e.id]?.pricing_breakdown?.user_group_factors || []).map((item) => `${item.group_name}×${formatDecimalPlain(item.multiplier)}`).join(' × ')
                                                : 'default×1'}
                                            </div>
                                            <div>用户倍率合计: ×{formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.user_multiplier || '1')}</div>
                                            {detailByEventID[e.id]?.pricing_breakdown?.subscription_group ? (
                                              <div>
                                                订阅分组: {detailByEventID[e.id]?.pricing_breakdown?.subscription_group}（仅用于套餐购买权限校验，不参与计费倍率）
                                              </div>
                                            ) : null}
                                            <div>生效倍率: ×{formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.effective_multiplier || '1')}</div>
                                            <div className="mt-1">
                                              最终费用: {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.base_cost_usd || '0')} × {formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.effective_multiplier || '1')} = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.final_cost_usd || '0')}{' '}
                                              <span className="text-muted smaller">
                                                （{costSourceLabel(detailByEventID[e.id]?.pricing_breakdown?.cost_source || '')}费用: {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cost_source_usd || '0')}）
                                              </span>
                                            </div>
                                            {formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.diff_from_source_usd || '0') !== '0' ? (
                                              <div className="text-muted smaller">
                                                差值(事件费用-公式): {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.diff_from_source_usd || '0')}
                                              </div>
                                            ) : null}
                                          </div>
                                        </div>
                                      ) : null}

                                      {detailByEventID[e.id]?.available ? (
                                        <>
                                          <div className="col-12">
                                            <div className="text-muted smaller">Downstream Request Body</div>
                                            <pre className="bg-white border rounded p-2 small mb-0">
                                              <code>{detailByEventID[e.id]?.downstream_request_body || '(empty)'}</code>
                                            </pre>
                                          </div>
                                          <div className="col-12">
                                            <div className="text-muted smaller">Upstream Request Body</div>
                                            <pre className="bg-white border rounded p-2 small mb-0">
                                              <code>{detailByEventID[e.id]?.upstream_request_body || '(empty)'}</code>
                                            </pre>
                                          </div>
                                          <div className="col-12">
                                            <div className="text-muted smaller">Upstream Response Body</div>
                                            <pre className="bg-white border rounded p-2 small mb-0">
                                              <code>{detailByEventID[e.id]?.upstream_response_body || '(empty)'}</code>
                                            </pre>
                                          </div>
                                        </>
                                      ) : (
                                        <div className="col-12 text-muted small">该事件不包含可用的 body 详情。</div>
                                      )}
                                    </div>
                                  ) : (
                                    <div className="text-muted small">该事件不包含可用的 body 详情。</div>
                                  )}
                                </div>
                              </td>
                            </tr>
                          ) : null}
                        </>
                      ))}
                      {events.length === 0 ? (
                        <tr>
                          <td colSpan={10} className="text-center py-5 text-muted small">
                            暂无请求记录
                          </td>
                        </tr>
                      ) : null}
                    </tbody>
                  </table>
                </div>
              </div>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}

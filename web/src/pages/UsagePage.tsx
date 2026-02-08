import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { listUserModelsDetail, type UserManagedModel } from '../api/models';
import { listUserTokens, type UserToken } from '../api/tokens';
import { getUsageEventDetail, getUsageEvents, getUsageWindows, type UsageEvent, type UsageEventDetail, type UsageWindow } from '../api/usage';
import { formatUSD, formatUSDPlain } from '../format/money';

type UsageEventDetailState =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'loaded'; data: UsageEventDetail }
  | { status: 'error'; message: string };

function todayLocal() {
  const now = new Date();
  const yyyy = now.getFullYear();
  const mm = String(now.getMonth() + 1).padStart(2, '0');
  const dd = String(now.getDate()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd}`;
}

function formatUTCDateTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  const yyyy = d.getUTCFullYear();
  const mm = String(d.getUTCMonth() + 1).padStart(2, '0');
  const dd = String(d.getUTCDate()).padStart(2, '0');
  const hh = String(d.getUTCHours()).padStart(2, '0');
  const mi = String(d.getUTCMinutes()).padStart(2, '0');
  const ss = String(d.getUTCSeconds()).padStart(2, '0');
  return `${yyyy}-${mm}-${dd} ${hh}:${mi}:${ss}`;
}

function formatRatePerMinute(count: number, sinceISO?: string, untilISO?: string): string {
  const since = sinceISO ? Date.parse(sinceISO) : NaN;
  const until = untilISO ? Date.parse(untilISO) : NaN;
  const minutes = Number.isFinite(since) && Number.isFinite(until) ? Math.max(0, (until - since) / 1000 / 60) : 0;
  if (minutes <= 0) return '0.0';
  return (count / minutes).toFixed(1);
}

function cacheHitRate(ratio: number): string {
  if (!Number.isFinite(ratio)) return '0.0%';
  return `${(ratio * 100).toFixed(1)}%`;
}

function stateLabel(state: string): { label: string; badgeClass: string } {
  switch (state) {
    case 'committed':
      return { label: '已结算', badgeClass: 'bg-success-subtle text-success border border-success-subtle' };
    case 'reserved':
      return { label: '预留中', badgeClass: 'bg-warning-subtle text-warning border border-warning-subtle' };
    case 'void':
      return { label: '已作废', badgeClass: 'bg-secondary-subtle text-secondary border border-secondary-subtle' };
    case 'expired':
      return { label: '已过期', badgeClass: 'bg-secondary-subtle text-secondary border border-secondary-subtle' };
    default:
      return { label: state || '-', badgeClass: 'bg-secondary-subtle text-secondary border border-secondary-subtle' };
  }
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

function costLabel(ev: UsageEvent): string {
  let usd = '0';
  if (ev.state === 'committed') usd = ev.committed_usd;
  if (ev.state === 'reserved') usd = ev.reserved_usd;
  const base = formatUSD(usd);
  if (ev.state === 'reserved') return `${base} (预留)`;
  return base;
}

function tokenText(v: number | null | undefined): string {
  if (v == null) return '-';
  if (!Number.isFinite(v)) return '-';
  return String(v);
}

function cachedTokens(ev: UsageEvent): string {
  const inTok = clampCached(ev.input_tokens ?? 0, ev.cached_input_tokens ?? 0);
  const outTok = clampCached(ev.output_tokens ?? 0, ev.cached_output_tokens ?? 0);
  const n = inTok + outTok;
  if (!Number.isFinite(n) || n <= 0) return '-';
  return String(n);
}

function tokenNameFromMap(tokenByID: Record<number, UserToken>, tokenID: number): string {
  const tok = tokenByID[tokenID];
  const name = (tok?.name || '').toString().trim();
  if (name) return name;
  const hint = (tok?.token_hint || '').toString().trim();
  if (hint) return hint;
  return '-';
}

function parseDecimalToMicroInt(raw: string | number | null | undefined): bigint {
  let s = (raw == null ? '' : String(raw)).trim();
  if (!s) return 0n;
  if (s.startsWith('$')) s = s.slice(1).trim();

  let sign = 1n;
  if (s.startsWith('-')) {
    sign = -1n;
    s = s.slice(1).trim();
  } else if (s.startsWith('+')) {
    s = s.slice(1).trim();
  }
  if (!s) return 0n;

  const dot = s.indexOf('.');
  let intPart = s;
  let frac = '';
  if (dot >= 0) {
    intPart = s.slice(0, dot);
    frac = s.slice(dot + 1);
  }
  intPart = intPart.trim() || '0';
  frac = (frac || '').replace(/[^\d]/g, '');
  frac = (frac + '000000').slice(0, 6);

  const i = BigInt(intPart.replace(/[^\d]/g, '') || '0');
  const f = BigInt(frac || '0');
  return sign * (i * 1_000_000n + f);
}

function microUSDToUSDString(micro: bigint): string {
  if (micro === 0n) return '0';
  let v = micro;
  let sign = '';
  if (v < 0n) {
    sign = '-';
    v = -v;
  }
  const i = v / 1_000_000n;
  const f = v % 1_000_000n;
  if (f === 0n) return `${sign}${i.toString()}`;
  let frac = f.toString().padStart(6, '0');
  frac = frac.replace(/0+$/, '');
  return `${sign}${i.toString()}.${frac}`;
}

function microUSDToUSDWithDollar(micro: bigint): string {
  const s = microUSDToUSDString(micro);
  if (s.startsWith('-')) return `-$${s.slice(1)}`;
  return `$${s}`;
}

function formatUSDPer1M(usdPer1M: string): string {
  const micro = parseDecimalToMicroInt(usdPer1M);
  return `${microUSDToUSDWithDollar(micro)}/1M`;
}

function costMicroUSD(tokens: number, usdPer1MMicro: bigint): bigint {
  if (!Number.isFinite(tokens) || tokens <= 0) return 0n;
  if (usdPer1MMicro <= 0n) return 0n;
  return (BigInt(tokens) * usdPer1MMicro) / 1_000_000n;
}

function clampCached(total: number, cached: number): number {
  if (!Number.isFinite(total) || total <= 0) return 0;
  if (!Number.isFinite(cached) || cached <= 0) return 0;
  return Math.min(total, cached);
}

function quickRangeKey(start: string, end: string): 'today' | 'yesterday' | '7d' | null {
  const fmt = (d: Date) => {
    const y = d.getFullYear();
    const m = String(d.getMonth() + 1).padStart(2, '0');
    const dd = String(d.getDate()).padStart(2, '0');
    return `${y}-${m}-${dd}`;
  };
  const now = new Date();
  const today = fmt(now);
  const yesterdayDate = new Date(now);
  yesterdayDate.setDate(yesterdayDate.getDate() - 1);
  const yesterday = fmt(yesterdayDate);

  const last7StartDate = new Date(now);
  last7StartDate.setDate(last7StartDate.getDate() - 6);
  const last7Start = fmt(last7StartDate);

  if (start === today && end === today) return 'today';
  if (start === yesterday && end === yesterday) return 'yesterday';
  if (start === last7Start && end === today) return '7d';
  return null;
}

export function UsagePage() {
  const [start, setStart] = useState(todayLocal());
  const [end, setEnd] = useState(todayLocal());
  const [limit, setLimit] = useState(50);

  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  const [tokenByID, setTokenByID] = useState<Record<number, UserToken>>({});
  const [modelByPublicID, setModelByPublicID] = useState<Record<string, UserManagedModel>>({});

  const [window0, setWindow0] = useState<UsageWindow | null>(null);
  const [events, setEvents] = useState<UsageEvent[]>([]);
  const [nextBeforeID, setNextBeforeID] = useState<number | null>(null);
  const [beforeStack, setBeforeStack] = useState<number[]>([]);

  const [openEventID, setOpenEventID] = useState<number | null>(null);
  const [detailByID, setDetailByID] = useState<Record<number, UsageEventDetailState>>({});

  const initRef = useRef(false);

  const activeQuick = useMemo(() => quickRangeKey(start, end), [start, end]);
  const beforeID = beforeStack.length > 0 ? beforeStack[beforeStack.length - 1] : undefined;

  const refresh = useCallback(async (
    currentBeforeID: number | undefined,
    override?: {
      start?: string;
      end?: string;
    },
  ) => {
    setErr('');
    setLoading(true);
    try {
      const startValue = override?.start ?? start;
      const endValue = override?.end ?? end;

      const [w, e] = await Promise.all([
        getUsageWindows(startValue, endValue),
        getUsageEvents(limit, currentBeforeID, startValue, endValue),
      ]);
      if (!w.success) {
        throw new Error(w.message || '用量汇总加载失败');
      }
      if (!e.success) {
        throw new Error(e.message || '请求明细加载失败');
      }

      const first = w.data?.windows?.[0] ?? null;
      setWindow0(first);
      setEvents(e.data?.events || []);
      setNextBeforeID(e.data?.next_before_id || null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setWindow0(null);
      setEvents([]);
      setNextBeforeID(null);
    } finally {
      setLoading(false);
    }
  }, [end, limit, start]);

  async function ensureDetailLoaded(id: number) {
    const cur = detailByID[id];
    if (cur && (cur.status === 'loading' || cur.status === 'loaded')) return;
	    setDetailByID((m) => ({ ...m, [id]: { status: 'loading' } }));
	    try {
	      const res = await getUsageEventDetail(id);
	      const detail = res.data;
	      if (!res.success || !detail) {
	        throw new Error(res.message || '加载失败');
	      }
	      setDetailByID((m) => ({ ...m, [id]: { status: 'loaded', data: detail } }));
	    } catch (e) {
	      setDetailByID((m) => ({ ...m, [id]: { status: 'error', message: e instanceof Error ? e.message : '加载失败' } }));
	    }
	  }

  useEffect(() => {
    try {
      const s = localStorage.getItem('rlm_usage_start');
      const e = localStorage.getItem('rlm_usage_end');
      const l = localStorage.getItem('rlm_usage_limit');
      if (s) setStart(s);
      if (e) setEnd(e);
      if (l && Number.isFinite(Number(l))) setLimit(Math.max(10, Math.min(200, Number(l))));
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    try {
      localStorage.setItem('rlm_usage_start', start);
      localStorage.setItem('rlm_usage_end', end);
      localStorage.setItem('rlm_usage_limit', String(limit));
    } catch {
      // ignore
    }
  }, [end, limit, start]);

  useEffect(() => {
    if (initRef.current) return;
    initRef.current = true;
    void refresh(undefined);
  }, [refresh]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await listUserTokens();
        if (!res.success) return;
        const list = res.data || [];
        const m: Record<number, UserToken> = {};
        for (const tok of list) {
          m[tok.id] = tok;
        }
        if (cancelled) return;
        setTokenByID(m);
      } catch {
        // ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const res = await listUserModelsDetail();
        if (!res.success) return;
        const list = res.data || [];
        const m: Record<string, UserManagedModel> = {};
        for (const mm of list) {
          const pid = (mm.public_id || '').toString().trim();
          if (!pid) continue;
          m[pid] = mm;
        }
        if (cancelled) return;
        setModelByPublicID(m);
      } catch {
        // ignore
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const timer = window.setInterval(() => {
      void refresh(beforeID);
    }, 60 * 1000);
    return () => window.clearInterval(timer);
  }, [beforeID, refresh]);

  const windowSince = window0?.since;
  const windowUntil = window0?.until;
  const rpm = window0 ? formatRatePerMinute(window0.requests, windowSince, windowUntil) : '0.0';
  const tpm = window0 ? formatRatePerMinute(window0.tokens, windowSince, windowUntil) : '0.0';
  const cachedTotal = window0 ? window0.cached_input_tokens + window0.cached_output_tokens : 0;

  const cursorActive = beforeStack.length > 0;
  const canPrev = beforeStack.length > 0;
  const canNext = !!nextBeforeID;

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-start mb-4 flex-wrap gap-3">
        <div>
          <div className="d-flex align-items-center mb-1">
            <h4 className="fw-bold mb-0 text-dark">用量统计</h4>
            <span className="badge rounded-pill bg-primary bg-opacity-10 text-primary border border-primary border-opacity-10 ms-2 small fw-normal">
              UTC
            </span>
          </div>
          <p className="text-muted smaller mb-0">查看您的模型使用额度与请求明细数据。</p>
        </div>
      </div>

      <form
        className="row g-2 align-items-end mb-4"
        onSubmit={async (e) => {
          e.preventDefault();
          setBeforeStack([]);
          await refresh(undefined);
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
        <div className="col-auto d-flex gap-1">
          <button
            type="button"
            className={activeQuick === 'today' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'}
            onClick={async () => {
              const d = todayLocal();
              setStart(d);
              setEnd(d);
              setBeforeStack([]);
              await refresh(undefined, { start: d, end: d });
            }}
          >
            今天
          </button>
          <button
            type="button"
            className={activeQuick === 'yesterday' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'}
            onClick={async () => {
              const base = new Date();
              base.setDate(base.getDate() - 1);
              const y = base.getFullYear();
              const m = String(base.getMonth() + 1).padStart(2, '0');
              const dd = String(base.getDate()).padStart(2, '0');
              const d = `${y}-${m}-${dd}`;
              setStart(d);
              setEnd(d);
              setBeforeStack([]);
              await refresh(undefined, { start: d, end: d });
            }}
          >
            昨天
          </button>
          <button
            type="button"
            className={activeQuick === '7d' ? 'btn btn-sm btn-primary text-white border-primary' : 'btn btn-sm btn-white border text-dark'}
            onClick={async () => {
              const endDate = new Date();
              const startDate = new Date(endDate);
              startDate.setDate(startDate.getDate() - 6);
              const fmt = (d: Date) => {
                const y = d.getFullYear();
                const m = String(d.getMonth() + 1).padStart(2, '0');
                const dd = String(d.getDate()).padStart(2, '0');
                return `${y}-${m}-${dd}`;
              };
              const s = fmt(startDate);
              const e = fmt(endDate);
              setStart(s);
              setEnd(e);
              setBeforeStack([]);
              await refresh(undefined, { start: s, end: e });
            }}
          >
            7天
          </button>
        </div>
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">条数</label>
          <select className="form-select form-select-sm" value={limit} onChange={(e) => setLimit(Number.parseInt(e.target.value, 10) || 50)}>
            <option value={20}>20</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </div>
        <div className="col-auto">
          <button type="submit" className="btn btn-sm btn-primary" disabled={loading}>
            更新统计
          </button>
        </div>
      </form>

      {err ? (
        <div className="row g-4">
          <div className="col-12">
            <div className="alert alert-danger d-flex align-items-center" role="alert">
              <span className="me-2 material-symbols-rounded">warning</span>
              <div>{err}</div>
            </div>
          </div>
        </div>
      ) : null}

      {window0 ? (
        <div className="row g-4">
          <div className="col-12">
            <div className="card border-0 overflow-hidden">
              <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
                <div>
                  <span className="text-primary fw-bold text-uppercase small">统计区间（UTC）</span>
                  <span className="text-muted smaller ms-2">
                    统计区间: {formatUTCDateTime(window0.since)} ~ {formatUTCDateTime(window0.until)}
                  </span>
                </div>
                <div className="text-primary smaller">
                  <span className="spin-slow me-1 material-symbols-rounded">autorenew</span> 每分钟自动刷新
                </div>
              </div>

              <div className="card-body p-4">
                <div className="row g-4">
                  <div className="col-lg-4 border-end">
                  <div className="mb-4">
                      <div className="text-muted smaller mb-1">总消耗费用（USD）</div>
                      <div className="d-flex align-items-baseline">
                        <h1 className="display-5 fw-bold mb-0 text-dark">{formatUSD(window0.used_usd)}</h1>
                        {(() => {
                          const n = Number.parseFloat(String(window0.limit_usd || '0'));
                          if (!Number.isFinite(n) || n <= 0) return null;
                          return (
                            <span className="text-muted ms-2 small">/ {formatUSD(String(window0.limit_usd))}</span>
                          );
                        })()}
                      </div>
                    </div>

                    {(() => {
                      const limitN = Number.parseFloat(String(window0.limit_usd || '0'));
                      const usedN = Number.parseFloat(String(window0.used_usd || '0'));
                      if (!Number.isFinite(limitN) || limitN <= 0) return null;
                      const percentRaw = limitN > 0 && Number.isFinite(usedN) ? (usedN / limitN) * 100 : 0;
                      const percent = Math.min(100, Math.max(0, Math.floor(percentRaw)));
                      const barClass = percent > 90 ? 'bg-danger' : percent > 70 ? 'bg-warning' : 'bg-primary';
                      return (
                        <div className="mb-4">
                          <div className="d-flex justify-content-between mb-2 small">
                            <span className="text-muted">周期额度消耗</span>
                            <span className="fw-bold">{percent}%</span>
                          </div>
                          <div className="progress" style={{ height: 10, borderRadius: 5 }}>
                            <div
                              className={`progress-bar ${barClass}`}
                              role="progressbar"
                              style={{ width: `${percent}%` }}
                              aria-valuenow={percent}
                              aria-valuemin={0}
                              aria-valuemax={100}
                            ></div>
                          </div>
                        </div>
                      );
                    })()}

                    <div className="row g-0 py-3 bg-light rounded-3 px-3">
                      <div className="col-6 border-end">
                        <div className="text-muted smaller">已结算</div>
                        <div className="fw-bold h5 mb-0 text-success">{formatUSD(window0.committed_usd)}</div>
                      </div>
                      <div className="col-6 ps-3">
                        <div className="text-muted smaller">预留中</div>
                        <div className="fw-bold h5 mb-0 text-warning">{formatUSD(window0.reserved_usd)}</div>
                      </div>
                    </div>
                  </div>

                  <div className="col-lg-8 ps-lg-4">
                    <div className="row g-3">
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">请求总数</div>
                          <div className="h4 fw-bold mb-1">{window0.requests}</div>
                          <div className="text-primary smaller fw-medium">{rpm} RPM</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">总 Token</div>
                          <div className="h4 fw-bold mb-1">{window0.tokens}</div>
                          <div className="text-primary smaller fw-medium">{tpm} TPM</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">缓存命中率</div>
                          <div className="h4 fw-bold mb-1 text-success">{cacheHitRate(window0.cache_ratio)}</div>
                          <div className="text-success smaller fw-medium">命中统计</div>
                        </div>
                      </div>
                      <div className="col-sm-6 col-md-3">
                        <div className="metric-card p-3 rounded-3 border">
                          <div className="text-muted smaller mb-1">缓存 Token</div>
                          <div className="h4 fw-bold mb-1">{cachedTotal}</div>
                          <div className="text-muted smaller fw-medium">输入 + 输出</div>
                        </div>
                      </div>

                      <div className="col-12 mt-4">
                        <div className="bg-light p-3 rounded-3">
                          <div className="row text-center small">
                            <div className="col-6 border-end">
                              <div className="text-muted smaller">输入总计</div>
                              <div className="fw-medium">{window0.input_tokens}</div>
                            </div>
                            <div className="col-6">
                              <div className="text-muted smaller">输出总计</div>
                              <div className="fw-medium">{window0.output_tokens}</div>
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
            <div className="card border-0 overflow-hidden">
              <div className="card-header py-3 px-4 d-flex justify-content-between align-items-center">
                <h5 className="mb-0 fw-bold">请求明细</h5>
                <span className="text-muted smaller">按每次请求记录（倒序）· 点击一行展开明细</span>
              </div>
              <div className="card-body p-0 border-top">
                <div className="table-responsive">
                  <table className="table table-hover align-middle mb-0 border-0">
                    <thead className="bg-light text-muted smaller uppercase">
                      <tr>
                        <th className="ps-4 border-0">时间（UTC）</th>
                        <th className="border-0">接口 / 模型</th>
                        <th className="text-center border-0">状态码</th>
                        <th className="text-end border-0">耗时</th>
                        <th className="text-end border-0">Tokens (In/Out/Cache)</th>
                        <th className="text-end border-0">费用</th>
                        <th className="text-center border-0">状态</th>
                        <th className="pe-4 border-0">Request ID</th>
                      </tr>
                    </thead>
                    <tbody className="small" id="rlmUsageEvents">
                      {events.length === 0 ? (
                        <tr>
                          <td colSpan={8} className="text-center py-5 text-muted small">
                            <div className="mb-2">
                              <span className="fs-1 text-muted opacity-25 material-symbols-rounded">inbox</span>
                            </div>
                            暂无请求明细数据
                          </td>
                        </tr>
                      ) : (
	                        events.map((ev) => {
	                          const isOpen = openEventID === ev.id;
	                          const cached = cachedTokens(ev);
	                          const state = stateLabel(ev.state);
	                          const detail = detailByID[ev.id] || { status: 'idle' };
                            const tokenName = tokenNameFromMap(tokenByID, ev.token_id);
                            const pricing = ev.model ? modelByPublicID[(ev.model || '').trim()] : undefined;
	
	                          return (
	                            <FragmentUsageRow
	                              key={ev.id}
	                              ev={ev}
	                              isOpen={isOpen}
	                              onToggle={async () => {
	                                setOpenEventID((cur) => (cur === ev.id ? null : ev.id));
	                                await ensureDetailLoaded(ev.id);
	                              }}
	                              cached={cached}
	                              cost={costLabel(ev)}
	                              state={state}
                                tokenName={tokenName}
                                pricing={pricing}
	                              detailState={detail}
	                            />
	                          );
	                        })
	                      )}
                    </tbody>
                  </table>
                </div>
              </div>

              {cursorActive || canNext ? (
                <div className="card-footer bg-light py-3 px-4">
                  <nav aria-label="Usage pagination">
                    <ul className="pagination justify-content-center mb-0">
                      <li className={`page-item${cursorActive ? '' : ' disabled'}`}>
                        <button
                          className="page-link"
                          type="button"
                          onClick={async () => {
                            setBeforeStack([]);
                            setOpenEventID(null);
                            await refresh(undefined);
                          }}
                          disabled={!cursorActive || loading}
                        >
                          <span aria-hidden="true">
                            <span className="material-symbols-rounded">first_page</span> 最新
                          </span>
                        </button>
                      </li>

                      <li className={`page-item${canPrev ? '' : ' disabled'}`}>
                        <button
                          className="page-link"
                          type="button"
                          onClick={async () => {
                            setBeforeStack((s) => s.slice(0, -1));
                            setOpenEventID(null);
                            const nextStack = beforeStack.slice(0, -1);
                            const nextBefore = nextStack.length > 0 ? nextStack[nextStack.length - 1] : undefined;
                            await refresh(nextBefore);
                          }}
                          disabled={!canPrev || loading}
                        >
                          <span className="material-symbols-rounded">chevron_left</span> 上一页
                        </button>
                      </li>

                      <li className="page-item disabled">
                        <span className="page-link text-muted border-0 bg-transparent">{cursorActive ? '历史数据' : '第一页'}</span>
                      </li>

                      <li className={`page-item${canNext ? '' : ' disabled'}`}>
                        <button
                          className="page-link"
                          type="button"
                          onClick={async () => {
                            if (!nextBeforeID) return;
                            setBeforeStack((s) => [...s, nextBeforeID]);
                            setOpenEventID(null);
                            await refresh(nextBeforeID);
                          }}
                          disabled={!canNext || loading}
                        >
                          下一页 <span className="material-symbols-rounded">chevron_right</span>
                        </button>
                      </li>
                    </ul>
                  </nav>
                </div>
              ) : null}
            </div>
          </div>
        </div>
      ) : loading ? (
        <div className="text-muted">加载中…</div>
      ) : (
        <div className="text-muted">暂无统计数据。</div>
      )}
    </div>
  );
}

function FragmentUsageRow({
  ev,
  isOpen,
  onToggle,
  cached,
  cost,
  state,
  tokenName,
  pricing,
  detailState,
}: {
  ev: UsageEvent;
  isOpen: boolean;
  onToggle: () => Promise<void>;
  cached: string;
  cost: string;
  state: { label: string; badgeClass: string };
  tokenName: string;
  pricing?: UserManagedModel;
  detailState: UsageEventDetailState;
}) {
  const endpoint = (ev.endpoint || '').trim() || '-';
  const method = (ev.method || '').trim() || '-';
  const model = (ev.model || '').trim() || '-';
  const code = ev.status_code ? String(ev.status_code) : '-';

  const errText = (ev.error_class || ev.error_message || '').toString().trim();

  let detailTextDown = '（展开后自动加载）';
  let detailTextUpReq = '（展开后自动加载）';
  let detailTextUpResp = '（展开后自动加载）';
  if (detailState.status === 'loading') {
    detailTextDown = '加载中...';
    detailTextUpReq = '加载中...';
    detailTextUpResp = '加载中...';
  }
	  if (detailState.status === 'loaded') {
	    if (!detailState.data.available) {
	      detailTextDown = '（无明细：仅对失败请求保存，或该条记录未启用存储）';
	      detailTextUpReq = '（无明细：仅对失败请求保存，或该条记录未启用存储）';
	      detailTextUpResp = '-';
	    } else {
	      detailTextDown = detailState.data.downstream_request_body || '-';
	      const upstreamHidden = detailState.data.upstream_request_body === undefined && detailState.data.upstream_response_body === undefined;
	      if (upstreamHidden) {
	        detailTextUpReq = '（仅管理员可查看）';
	        detailTextUpResp = '（仅管理员可查看）';
	      } else {
	        detailTextUpReq = detailState.data.upstream_request_body || '-';
	        detailTextUpResp = detailState.data.upstream_response_body || '-';
	      }
	    }
	  }
	  if (detailState.status === 'error') {
	    detailTextDown = `加载失败：${detailState.message}`;
	    detailTextUpReq = `加载失败：${detailState.message}`;
	    detailTextUpResp = '-';
	  }

    const inTokTotal = ev.input_tokens ?? 0;
    const outTokTotal = ev.output_tokens ?? 0;
    const cachedInTok = clampCached(inTokTotal, ev.cached_input_tokens ?? 0);
    const cachedOutTok = clampCached(outTokTotal, ev.cached_output_tokens ?? 0);
    const nonCachedInTok = Math.max(0, inTokTotal - cachedInTok);
    const nonCachedOutTok = Math.max(0, outTokTotal - cachedOutTok);

    const pricingAvailable = pricing && (pricing.public_id || '').toString().trim() !== '';
    const inUSDPer1MStr = pricingAvailable ? pricing!.input_usd_per_1m : '0';
    const outUSDPer1MStr = pricingAvailable ? pricing!.output_usd_per_1m : '0';
    const cacheInUSDPer1MStr = pricingAvailable ? pricing!.cache_input_usd_per_1m : '0';
    const cacheOutUSDPer1MStr = pricingAvailable ? pricing!.cache_output_usd_per_1m : '0';

    const inUSDPer1MMicro = parseDecimalToMicroInt(inUSDPer1MStr);
    const outUSDPer1MMicro = parseDecimalToMicroInt(outUSDPer1MStr);
    const cacheInUSDPer1MMicro = parseDecimalToMicroInt(cacheInUSDPer1MStr);
    const cacheOutUSDPer1MMicro = parseDecimalToMicroInt(cacheOutUSDPer1MStr);

    const inCostMicro = costMicroUSD(nonCachedInTok, inUSDPer1MMicro);
    const outCostMicro = costMicroUSD(nonCachedOutTok, outUSDPer1MMicro);
    const cacheInCostMicro = costMicroUSD(cachedInTok, cacheInUSDPer1MMicro);
    const cacheOutCostMicro = costMicroUSD(cachedOutTok, cacheOutUSDPer1MMicro);
    const sumCostMicro = inCostMicro + outCostMicro + cacheInCostMicro + cacheOutCostMicro;

    const actualUSD = ev.state === 'committed' ? ev.committed_usd : ev.state === 'reserved' ? ev.reserved_usd : '0';
    const actualMicro = parseDecimalToMicroInt(actualUSD);
    const pricingBreakdown = detailState.status === 'loaded' ? detailState.data.pricing_breakdown : undefined;

	  return (
	    <>
	      <tr className="rlm-usage-row" role="button" aria-expanded={isOpen} onClick={() => void onToggle()}>
	        <td className="ps-4 text-nowrap font-monospace">
	          <span className="material-symbols-rounded text-muted rlm-usage-chevron me-1 align-middle">chevron_right</span>
          <span className="align-middle">{formatUTCDateTime(ev.time)}</span>
        </td>
        <td className="text-nowrap">
          <div className="badge bg-light text-dark border fw-normal">{model}</div>
          <div className="text-muted smaller mt-1 font-monospace">{endpoint}</div>
        </td>
        <td className="text-center">
          {code === '200' ? (
            <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill">200</span>
          ) : (
            <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill">{code}</span>
          )}
        </td>
        <td className="text-end font-monospace text-muted">{ev.latency_ms ? `${ev.latency_ms} ms` : '- ms'}</td>
        <td className="text-end font-monospace">
          <div>
            <span className="text-muted">In:</span> {tokenText(ev.input_tokens)}
          </div>
          <div>
            <span className="text-muted">Out:</span> {tokenText(ev.output_tokens)}
          </div>
          {cached !== '-' ? (
            <div className="text-success smaller">
              <span className="material-symbols-rounded">bolt</span> {cached}
            </div>
          ) : null}
        </td>
        <td className="text-end font-monospace fw-bold text-dark">{cost}</td>
        <td className="text-center text-nowrap">
          <span className={`badge rounded-pill px-2 ${state.badgeClass} mb-1`}>{state.label}</span>
          {ev.is_stream ? (
            <div className="badge bg-info-subtle text-info border border-info-subtle rounded-pill px-2 scale-90">STREAM</div>
          ) : null}
          {errText ? (
            <div className="text-danger smaller mt-1" title={errText}>
              <span className="material-symbols-rounded">error</span> 错误
            </div>
          ) : null}
        </td>
        <td
          className="pe-4 font-monospace text-muted small user-select-all"
          style={{ maxWidth: 120, overflow: 'hidden', textOverflow: 'ellipsis' }}
          title={ev.request_id}
          onClick={(e) => e.stopPropagation()}
        >
          {ev.request_id}
        </td>
      </tr>

      {isOpen ? (
        <tr className="rlm-usage-detail-row">
          <td colSpan={8} className="p-0 border-0">
            <div className="bg-light border-top px-4 py-3">
	              <div className="row g-3 small">
	                <div className="col-12 col-lg-6">
	                  <div className="text-muted smaller">Request ID</div>
	                  <div className="font-monospace user-select-all">{ev.request_id}</div>
	                </div>
	                <div className="col-6 col-lg-3">
	                  <div className="text-muted smaller">Event ID</div>
	                  <div className="font-monospace">{ev.id}</div>
	                </div>
                  <div className="col-6 col-lg-3">
                    <div className="text-muted smaller">Key 名称</div>
                    <div className="font-monospace">{tokenName || '-'}</div>
                  </div>
	
	                <div className="col-12 col-lg-6">
	                  <div className="text-muted smaller">Endpoint</div>
	                  <div className="font-monospace">
	                    {method} {endpoint}
                  </div>
                </div>
                <div className="col-12 col-lg-6">
                  <div className="text-muted smaller">模型</div>
                  <div className="font-monospace">{model}</div>
                </div>

                <div className="col-6 col-lg-3">
                  <div className="text-muted smaller">状态码</div>
                  <div className="font-monospace">{code}</div>
                </div>
                <div className="col-6 col-lg-3">
                  <div className="text-muted smaller">耗时</div>
                  <div className="font-monospace">{ev.latency_ms} ms</div>
                </div>
                <div className="col-6 col-lg-3">
                  <div className="text-muted smaller">请求/响应大小</div>
                  <div className="font-monospace">
                    {ev.request_bytes} / {ev.response_bytes} bytes
                  </div>
                </div>
	                <div className="col-6 col-lg-3">
	                  <div className="text-muted smaller">费用</div>
	                  <div className="font-monospace">{cost}</div>
	                </div>
	
                  <div className="col-12">
                    <div className="text-muted smaller">费用计算</div>
                    {pricingBreakdown ? (
                      <div className="font-monospace">
                        <div>
                          输入(总/缓存/计费): {pricingBreakdown.input_tokens_total} / {pricingBreakdown.input_tokens_cached} / {pricingBreakdown.input_tokens_billable}
                        </div>
                        <div>
                          输出(总/缓存/计费): {pricingBreakdown.output_tokens_total} / {pricingBreakdown.output_tokens_cached} / {pricingBreakdown.output_tokens_billable}
                        </div>
                        <div>
                          输入(非缓存): {pricingBreakdown.input_tokens_billable} × {formatUSDPer1M(pricingBreakdown.input_usd_per_1m)} = {formatUSD(pricingBreakdown.input_cost_usd)}
                        </div>
                        <div>
                          输出(非缓存): {pricingBreakdown.output_tokens_billable} × {formatUSDPer1M(pricingBreakdown.output_usd_per_1m)} = {formatUSD(pricingBreakdown.output_cost_usd)}
                        </div>
                        <div>
                          缓存输入: {pricingBreakdown.input_tokens_cached} × {formatUSDPer1M(pricingBreakdown.cache_input_usd_per_1m)} = {formatUSD(pricingBreakdown.cache_input_cost_usd)}
                        </div>
                        <div>
                          缓存输出: {pricingBreakdown.output_tokens_cached} × {formatUSDPer1M(pricingBreakdown.cache_output_usd_per_1m)} = {formatUSD(pricingBreakdown.cache_output_cost_usd)}
                        </div>
                        <div className="mt-1">
                          基础费用: {formatUSD(pricingBreakdown.base_cost_usd)}
                        </div>
                        <div>
                          用户分组倍率: {pricingBreakdown.user_group_factors.length > 0
                            ? pricingBreakdown.user_group_factors.map((item) => `${item.group_name}×${formatUSDPlain(item.multiplier)}`).join(' × ')
                            : 'default×1'}
                        </div>
                        <div>
                          用户倍率合计: ×{formatUSDPlain(pricingBreakdown.user_multiplier)}
                        </div>
                        {pricingBreakdown.subscription_group ? (
                          <div>
                            订阅分组: {pricingBreakdown.subscription_group}（仅用于套餐购买权限校验，不参与计费倍率）
                          </div>
                        ) : null}
                        <div>
                          生效倍率: ×{formatUSDPlain(pricingBreakdown.effective_multiplier)}
                        </div>
                        <div className="mt-1">
                          最终费用: {formatUSD(pricingBreakdown.base_cost_usd)} × {formatUSDPlain(pricingBreakdown.effective_multiplier)} = {formatUSD(pricingBreakdown.final_cost_usd)}{' '}
                          <span className="text-muted smaller">
                            （{costSourceLabel(pricingBreakdown.cost_source)}费用: {formatUSD(pricingBreakdown.cost_source_usd)}）
                          </span>
                        </div>
                        {parseDecimalToMicroInt(pricingBreakdown.diff_from_source_usd) !== 0n ? (
                          <div className="text-muted smaller">
                            差值(事件费用-公式): {formatUSD(pricingBreakdown.diff_from_source_usd)}
                          </div>
                        ) : null}
                      </div>
                    ) : pricingAvailable ? (
                      <div className="font-monospace">
                        <div>
                          输入(非缓存): {nonCachedInTok} × {formatUSDPer1M(inUSDPer1MStr)} = {microUSDToUSDWithDollar(inCostMicro)}
                        </div>
                        <div>
                          输出(非缓存): {nonCachedOutTok} × {formatUSDPer1M(outUSDPer1MStr)} = {microUSDToUSDWithDollar(outCostMicro)}
                        </div>
                        <div>
                          缓存输入: {cachedInTok} × {formatUSDPer1M(cacheInUSDPer1MStr)} = {microUSDToUSDWithDollar(cacheInCostMicro)}
                        </div>
                        <div>
                          缓存输出: {cachedOutTok} × {formatUSDPer1M(cacheOutUSDPer1MStr)} = {microUSDToUSDWithDollar(cacheOutCostMicro)}
                        </div>
                        <div className="mt-1">
                          合计: {microUSDToUSDWithDollar(sumCostMicro)}{' '}
                          <span className="text-muted smaller">
                            （事件费用: {microUSDToUSDWithDollar(actualMicro)}）
                          </span>
                        </div>
                      </div>
                    ) : (
                      <div className="text-muted smaller">（未找到可用定价，无法计算明细）</div>
                    )}
                  </div>

	                <div className="col-12 col-lg-6">
	                  <div className="text-muted smaller">Tokens</div>
	                  <div className="font-monospace">
	                    In: {tokenText(ev.input_tokens)} / Out: {tokenText(ev.output_tokens)} / Cache: {cached}
                  </div>
                </div>
                <div className="col-12 col-lg-6">
                  <div className="text-muted smaller">状态</div>
                  <div className="font-monospace">
                    {state.label} ({ev.state})
                    {ev.is_stream ? ' · STREAM' : ''}
                  </div>
                </div>

                <div className="col-12 col-lg-3">
                  <div className="text-muted smaller">错误类型</div>
                  <div className="font-monospace">{ev.error_class || '-'}</div>
                </div>
                <div className="col-12 col-lg-9">
                  <div className="text-muted smaller">错误信息</div>
                  {ev.error_message ? (
                    <pre className="mb-0 font-monospace rlm-prewrap">{ev.error_message}</pre>
                  ) : ev.error_class ? (
                    <pre className="mb-0 font-monospace rlm-prewrap">{ev.error_class}</pre>
                  ) : (
                    <div className="text-muted">-</div>
                  )}
                </div>

                <div className="col-12">
                  <hr className="my-2" />
                  <div className="text-muted smaller">
                    原始请求体（客户端）<span className="ms-2">· 仅失败请求会保存</span>
                  </div>
                  <pre className="mb-0 font-monospace rlm-prewrap rlm-usage-detail-pre">{detailTextDown}</pre>
                </div>
                <div className="col-12">
                  <div className="text-muted smaller">
                    转发请求体（最终）<span className="ms-2">· 仅失败请求会保存</span>
                  </div>
                  <pre className="mb-0 font-monospace rlm-prewrap rlm-usage-detail-pre">{detailTextUpReq}</pre>
                </div>
                <div className="col-12">
                  <div className="text-muted smaller">上游响应体</div>
                  <pre className="mb-0 font-monospace rlm-prewrap rlm-usage-detail-pre">{detailTextUpResp}</pre>
                </div>
              </div>
            </div>
          </td>
        </tr>
      ) : null}
    </>
  );
}

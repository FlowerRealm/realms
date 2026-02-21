import { useEffect, useMemo, useState } from 'react';

import { listUserTokens, type UserToken } from '../api/tokens';
import {
  getUsageEventDetail,
  getUsageEvents,
  getUsageTimeSeries,
  getUsageWindows,
  type UsageEvent,
  type UsageEventDetail,
  type UsageTimeSeriesPoint,
  type UsageWindow,
} from '../api/usage';
import { useAuth } from '../auth/AuthContext';
import { SegmentedFrame } from '../components/SegmentedFrame';
import { formatUSDPlain } from '../format/money';
import { UsageEventsCard } from './usage/UsageEventsCard';
import { UsageSummaryCard } from './usage/UsageSummaryCard';
import { UsageTimeSeriesCard } from './usage/UsageTimeSeriesCard';
import { UsageTopUsersCard } from './usage/UsageTopUsersCard';
import { formatLocalDate, formatLocalDateTimeMinute, type TopUserView } from './usage/usageUtils';

type DetailField = 'committed_usd' | 'requests' | 'tokens' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second';
type DetailGranularity = 'hour' | 'day';

export function UsagePage() {
  const { user } = useAuth();

  const [data, setData] = useState<UsageWindow | null>(null);
  const [events, setEvents] = useState<UsageEvent[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState('');

  const [tokenByID, setTokenByID] = useState<Record<number, UserToken>>({});

  const [start, setStart] = useState('');
  const [end, setEnd] = useState('');
  const [limit, setLimit] = useState(50);

  const [seriesStart, setSeriesStart] = useState('');
  const [seriesEnd, setSeriesEnd] = useState('');
  const [nextBeforeID, setNextBeforeID] = useState<number | null>(null);
  const [beforeStack, setBeforeStack] = useState<number[]>([]);

  const [expandedID, setExpandedID] = useState<number | null>(null);
  const [detailByEventID, setDetailByEventID] = useState<Record<number, UsageEventDetail>>({});
  const [detailLoadingID, setDetailLoadingID] = useState<number | null>(null);

  const [detailSeries, setDetailSeries] = useState<UsageTimeSeriesPoint[]>([]);
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState('');
  const [detailField, setDetailField] = useState<DetailField>('committed_usd');
  const [detailGranularity, setDetailGranularity] = useState<DetailGranularity>('hour');

  const fieldOptions: Array<{ value: DetailField; label: string }> = [
    { value: 'committed_usd', label: '消耗 (USD)' },
    { value: 'requests', label: '请求数' },
    { value: 'tokens', label: 'Token' },
    { value: 'cache_ratio', label: '缓存率 (%)' },
    { value: 'avg_first_token_latency', label: '首字延迟 (ms)' },
    { value: 'tokens_per_second', label: 'Tokens/s' },
  ];
  const granularityOptions: Array<{ value: DetailGranularity; label: string }> = [
    { value: 'hour', label: '按小时' },
    { value: 'day', label: '按天' },
  ];

  const canPrev = beforeStack.length > 0;
  const canNext = useMemo(() => !!nextBeforeID && events.length === limit, [events.length, limit, nextBeforeID]);

  async function refresh(currentBeforeID?: number, override?: { start?: string; end?: string }) {
    setErr('');
    setLoading(true);
    try {
      const startValue = (override?.start ?? start).trim();
      const endValue = (override?.end ?? end).trim();
      const [w, e] = await Promise.all([
        getUsageWindows(startValue || undefined, endValue || undefined),
        getUsageEvents(limit, currentBeforeID, startValue || undefined, endValue || undefined),
      ]);
      if (!w.success) throw new Error(w.message || '加载失败');
      if (!e.success) throw new Error(e.message || '加载失败');

      const window0 = w.data?.windows?.[0] ?? null;
      setData(window0);
      setEvents(e.data?.events || []);
      setNextBeforeID(e.data?.next_before_id ?? null);

      const day0 = window0 ? formatLocalDate(String(window0.since)) : '';
      const effectiveStart = startValue || day0 || '';
      const effectiveEnd = endValue || (startValue ? startValue : day0) || '';
      setSeriesStart(effectiveStart);
      setSeriesEnd(effectiveEnd);

      if (window0) {
        if (!startValue && day0) setStart(day0);
        if (!endValue && (startValue || day0)) setEnd(startValue || day0);
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setData(null);
      setEvents([]);
      setNextBeforeID(null);
      setSeriesStart('');
      setSeriesEnd('');
    } finally {
      setLoading(false);
    }
  }

  async function loadDetail(eventID: number) {
    if (detailByEventID[eventID]) return;
    setDetailLoadingID(eventID);
    try {
      const res = await getUsageEventDetail(eventID);
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

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

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

  const hasSeriesSource = data !== null;

  useEffect(() => {
    if (!hasSeriesSource) {
      setDetailSeries([]);
      setDetailSeriesErr('');
      setDetailSeriesLoading(false);
      return;
    }
    let active = true;
    void (async () => {
      setDetailSeriesErr('');
      setDetailSeriesLoading(true);
      try {
        const res = await getUsageTimeSeries(seriesStart || undefined, seriesEnd || undefined, detailGranularity);
        if (!res.success) throw new Error(res.message || '加载时间序列失败');
        if (!active) return;
        setDetailSeries(res.data?.points || []);
      } catch (e) {
        if (!active) return;
        setDetailSeries([]);
        setDetailSeriesErr(e instanceof Error ? e.message : '加载时间序列失败');
      } finally {
        if (active) setDetailSeriesLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [detailGranularity, hasSeriesSource, seriesEnd, seriesStart]);

  const rangeSinceText = data ? formatLocalDateTimeMinute(String(data.since)) : '';
  const rangeUntilText = data ? formatLocalDateTimeMinute(String(data.until)) : '';

  const avgFirstTokenLatencyText = useMemo(() => {
    const values = (detailSeries || []).map((p) => p.avg_first_token_latency).filter((v) => Number.isFinite(v) && v > 0);
    if (values.length === 0) return '-';
    const avg = values.reduce((acc, v) => acc + v, 0) / values.length;
    if (!Number.isFinite(avg) || avg <= 0) return '-';
    return `${avg.toFixed(1)} ms`;
  }, [detailSeries]);
  const avgTokensPerSecondText = useMemo(() => {
    const values = (detailSeries || []).map((p) => p.tokens_per_second).filter((v) => Number.isFinite(v) && v > 0);
    if (values.length === 0) return '-';
    const avg = values.reduce((acc, v) => acc + v, 0) / values.length;
    if (!Number.isFinite(avg) || avg <= 0) return '-';
    return avg.toFixed(2);
  }, [detailSeries]);

  const topUsers: TopUserView[] = useMemo(() => {
    if (!user || !data) return [];
    const email = (user.email || user.username || '').toString().trim() || '-';
    return [
      {
        user_id: user.id,
        email,
        role: (user.role || '').toString().trim() || '-',
        status: typeof user.status === 'number' ? user.status : 1,
        committed_usd: formatUSDPlain(data.committed_usd),
        reserved_usd: formatUSDPlain(data.reserved_usd),
      },
    ];
  }, [data, user]);

  const selfEmail = (user?.email || user?.username || '').toString().trim() || '-';
  const selfID = typeof user?.id === 'number' ? user.id : '-';

  const onPrevPage = () => {
    const nextStack = beforeStack.slice(0, -1);
    setBeforeStack(nextStack);
    setExpandedID(null);
    const nextBefore = nextStack.length > 0 ? nextStack[nextStack.length - 1] : undefined;
    void refresh(nextBefore);
  };

  const onNextPage = () => {
    if (!nextBeforeID) return;
    setBeforeStack((s) => [...s, nextBeforeID]);
    setExpandedID(null);
    void refresh(nextBeforeID);
  };

  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <div>
          <div className="d-flex justify-content-between align-items-center mb-4">
            <div>
              <h3 className="mb-1 fw-bold">全站用量统计</h3>
              <div className="text-muted small">系统级数据汇总，涵盖所有用户及上游通道。</div>
            </div>
          </div>

          {err ? (
            <div className="alert alert-danger mb-3">
              <span className="me-2 material-symbols-rounded">warning</span>
              {err}
            </div>
          ) : null}

          <form
            className="row g-2 align-items-end mb-0"
            onSubmit={(e) => {
              e.preventDefault();
              setBeforeStack([]);
              setExpandedID(null);
              void refresh(undefined);
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
                  setBeforeStack([]);
                  setExpandedID(null);
                  void refresh(undefined, { start: '', end: '' });
                }}
              >
                重置
              </button>
            </div>
          </form>
        </div>

      {loading ? (
        <div className="text-muted">加载中…</div>
      ) : data ? (
        <div className="row g-4">
          <div className="col-12">
            <UsageSummaryCard
              data={data}
              rangeSinceText={rangeSinceText}
              rangeUntilText={rangeUntilText}
              avgFirstTokenLatencyText={avgFirstTokenLatencyText}
              avgTokensPerSecondText={avgTokensPerSecondText}
            />
          </div>

          <div className="col-12">
            <UsageTimeSeriesCard
              rangeSinceText={rangeSinceText}
              rangeUntilText={rangeUntilText}
              detailSeries={detailSeries}
              detailSeriesErr={detailSeriesErr}
              detailSeriesLoading={detailSeriesLoading}
              detailField={detailField}
              setDetailField={setDetailField}
              detailGranularity={detailGranularity}
              setDetailGranularity={setDetailGranularity}
              fieldOptions={fieldOptions}
              granularityOptions={granularityOptions}
            />
          </div>

          <div className="col-12">
            <UsageTopUsersCard topUsers={topUsers} />
          </div>

          <div className="col-12">
            <UsageEventsCard
              events={events}
              tokenByID={tokenByID}
              expandedID={expandedID}
              setExpandedID={setExpandedID}
              loadDetail={loadDetail}
              detailLoadingID={detailLoadingID}
              detailByEventID={detailByEventID}
              canPrev={canPrev}
              canNext={canNext}
              loading={loading}
              onPrevPage={onPrevPage}
              onNextPage={onNextPage}
              selfEmail={selfEmail}
              selfID={selfID}
            />
          </div>
        </div>
      ) : null}
      </SegmentedFrame>
    </div>
  );
}

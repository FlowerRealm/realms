import { useEffect, useMemo, useRef, useState } from 'react';

import { listUserTokens, type UserToken } from '../api/tokens';
import {
  getUsageEventDetail,
  getUsageEventsV2,
  getUsageTimeSeries,
  getUsageWindows,
  type UsageEvent,
  type UsageEventDetail,
  type UsageTimeSeriesPoint,
  type UsageWindow,
} from '../api/usage';
import { useAuth } from '../auth/AuthContext';
import { DateRangePicker, SelectPicker } from '../components/DateRangePicker';
import { SegmentedFrame } from '../components/SegmentedFrame';
import { UsageAdvancedFiltersDropdown, type UsageAdvancedFiltersDropdownHandle } from '../components/UsageAdvancedFiltersDropdown';
import { formatSecondsFromMilliseconds } from '../format/duration';
import { UsageEventsCard } from './usage/UsageEventsCard';
import { UsageSummaryCard } from './usage/UsageSummaryCard';
import { UsageTimeSeriesCard } from './usage/UsageTimeSeriesCard';
import { formatLocalDate, formatLocalDateTimeMinute } from './usage/usageUtils';
import { fillDailyBuckets } from '../utils/timeSeries';

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
  const [allTime, setAllTime] = useState(false);
  const [dateRangeCustomized, setDateRangeCustomized] = useState(false);
  const [limit, setLimit] = useState(50);
  const [filterKey, setFilterKey] = useState('');
  const [filterModel, setFilterModel] = useState('');
  const advRef = useRef<UsageAdvancedFiltersDropdownHandle | null>(null);

  const [seriesStart, setSeriesStart] = useState('');
  const [seriesEnd, setSeriesEnd] = useState('');
  const [nextBeforeID, setNextBeforeID] = useState<number | null>(null);
  const [beforeStack, setBeforeStack] = useState<number[]>([]);

  const [expandedID, setExpandedID] = useState<number | null>(null);
  const [detailByEventID, setDetailByEventID] = useState<Record<number, UsageEventDetail>>({});
  const [detailLoadingID, setDetailLoadingID] = useState<number | null>(null);

  const [detailSeries, setDetailSeries] = useState<UsageTimeSeriesPoint[]>([]);
  const [detailSeriesStart, setDetailSeriesStart] = useState('');
  const [detailSeriesEnd, setDetailSeriesEnd] = useState('');
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState('');
  const [detailField, setDetailField] = useState<DetailField>('committed_usd');
  const [detailGranularity, setDetailGranularity] = useState<DetailGranularity>('hour');

  const fieldOptions: Array<{ value: DetailField; label: string }> = [
    { value: 'committed_usd', label: '消耗 (USD)' },
    { value: 'requests', label: '请求数' },
    { value: 'tokens', label: 'Token' },
    { value: 'cache_ratio', label: '缓存率 (%)' },
    { value: 'avg_first_token_latency', label: '首字延迟 (s)' },
    { value: 'tokens_per_second', label: 'Tokens/s' },
  ];
  const granularityOptions: Array<{ value: DetailGranularity; label: string }> = [
    { value: 'hour', label: '按小时' },
    { value: 'day', label: '按天' },
  ];

  const canPrev = beforeStack.length > 0;
  const canNext = useMemo(() => !!nextBeforeID && events.length === limit, [events.length, limit, nextBeforeID]);

  async function refresh(
    currentBeforeID?: number,
    override?: { start?: string; end?: string; allTime?: boolean; filterKey?: string; filterModel?: string },
  ) {
    setErr('');
    setLoading(true);
    try {
      const startValue = (override?.start ?? start).trim();
      const endValue = (override?.end ?? end).trim();
      const allTimeValue = !!(override?.allTime ?? allTime);
      const allTimeActive = allTimeValue && !startValue && !endValue;
      const indexParts: string[] = [];
      const q_key = (override?.filterKey ?? filterKey).trim();
      const q_model = (override?.filterModel ?? filterModel).trim();
      if (q_key) indexParts.push('key');
      if (q_model) indexParts.push('model');
      const index = indexParts.length ? indexParts.join(',') : undefined;
      const [w, e] = await Promise.all([
        getUsageWindows(startValue || undefined, endValue || undefined, undefined, allTimeActive),
        getUsageEventsV2({
          limit,
          before_id: currentBeforeID,
          start: allTimeActive ? undefined : startValue || undefined,
          end: allTimeActive ? undefined : endValue || undefined,
          index,
          q_key: q_key || undefined,
          q_model: q_model || undefined,
        }),
      ]);
      if (!w.success) throw new Error(w.message || '加载失败');
      if (!e.success) throw new Error(e.message || '加载失败');

      const window0 = w.data?.windows?.[0] ?? null;
      setData(window0);
      setEvents(e.data?.events || []);
      setNextBeforeID(e.data?.next_before_id ?? null);

      const day0 = window0 ? formatLocalDate(String(window0.since)) : '';
      const day1 = window0 ? formatLocalDate(String(window0.until)) : '';
      if (allTimeActive) {
        setSeriesStart(day0 || '');
        setSeriesEnd(day1 || '');
      } else {
        const effectiveStart = startValue || day0 || '';
        const effectiveEnd = endValue || (startValue ? startValue : day0) || '';
        setSeriesStart(effectiveStart);
        setSeriesEnd(effectiveEnd);
      }

      if (window0 && !allTimeActive) {
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
      setDetailSeriesStart('');
      setDetailSeriesEnd('');
      setDetailSeriesErr('');
      setDetailSeriesLoading(false);
      return;
    }
    let active = true;
    void (async () => {
      setDetailSeriesErr('');
      setDetailSeriesLoading(true);
      try {
        const allTimeActive = allTime && !start.trim() && !end.trim();
        const implicitDayRange = detailGranularity === 'day' && !dateRangeCustomized && !allTimeActive;
        const res = await getUsageTimeSeries(
          allTimeActive || implicitDayRange ? undefined : seriesStart || undefined,
          allTimeActive || implicitDayRange ? undefined : seriesEnd || undefined,
          detailGranularity,
          undefined, // tokenID
          allTimeActive,
        );
        if (!res.success) throw new Error(res.message || '加载时间序列失败');
        if (!active) return;
        const startValue = res.data?.start || '';
        const endValue = res.data?.end || '';
        const points = res.data?.points || [];
        setDetailSeriesStart(startValue);
        setDetailSeriesEnd(endValue);
        setDetailSeries(
          detailGranularity === 'day'
            ? fillDailyBuckets(points, startValue, endValue, (bucket) => ({
                bucket,
                requests: 0,
                tokens: 0,
                committed_usd: 0,
                cache_ratio: 0,
                avg_first_token_latency: 0,
                tokens_per_second: 0,
              }))
            : points,
        );
      } catch (e) {
        if (!active) return;
        setDetailSeries([]);
        setDetailSeriesStart('');
        setDetailSeriesEnd('');
        setDetailSeriesErr(e instanceof Error ? e.message : '加载时间序列失败');
      } finally {
        if (active) setDetailSeriesLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [allTime, dateRangeCustomized, detailGranularity, end, hasSeriesSource, seriesEnd, seriesStart, start]);

  const rangeSinceText = data ? formatLocalDateTimeMinute(String(data.since)) : '';
  const rangeUntilText = data ? formatLocalDateTimeMinute(String(data.until)) : '';

  const avgFirstTokenLatencyText = useMemo(() => {
    const values = (detailSeries || []).map((p) => p.avg_first_token_latency).filter((v) => Number.isFinite(v) && v > 0);
    if (values.length === 0) return '-';
    const avg = values.reduce((acc, v) => acc + v, 0) / values.length;
    if (!Number.isFinite(avg) || avg <= 0) return '-';
    return formatSecondsFromMilliseconds(avg);
  }, [detailSeries]);
  const avgTokensPerSecondText = useMemo(() => {
    const values = (detailSeries || []).map((p) => p.tokens_per_second).filter((v) => Number.isFinite(v) && v > 0);
    if (values.length === 0) return '-';
    const avg = values.reduce((acc, v) => acc + v, 0) / values.length;
    if (!Number.isFinite(avg) || avg <= 0) return '-';
    return avg.toFixed(2);
  }, [detailSeries]);

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
          <div className="d-flex justify-content-between align-items-center mb-3">
            <div>
              <h3 className="mb-1 fw-bold">用量统计</h3>
              <div className="text-muted small">按日期范围汇总用量，并支持事件明细查看。</div>
            </div>
          </div>

          {err ? (
            <div className="alert alert-danger mb-3">
              <span className="me-2 material-symbols-rounded">warning</span>
              {err}
            </div>
          ) : null}

          <div className="card border-0 shadow-sm mb-0">
            <div className="card-body py-3 px-4">
              <div className="d-flex flex-wrap align-items-end gap-3">
                <div className="d-flex flex-wrap align-items-center gap-2">
                  <div className="text-muted smaller fw-medium text-nowrap">时间区间</div>
                  <DateRangePicker
                    start={start}
                    end={end}
                    onChange={(r) => {
                      const isAll = !r.start.trim() && !r.end.trim();
                      setDateRangeCustomized(true);
                      setAllTime(isAll);
                      if (isAll) setDetailGranularity('day');
                      setStart(r.start);
                      setEnd(r.end);
                      setBeforeStack([]);
                      setExpandedID(null);
                    }}
                    loading={loading}
                  />
                </div>

                <div className="d-flex flex-wrap align-items-center gap-2">
                  <div className="text-muted smaller fw-medium text-nowrap">显示条数</div>
                  <SelectPicker
                    value={limit}
                    options={[
                      { label: '20', value: 20 },
                      { label: '50', value: 50 },
                      { label: '100', value: 100 },
                    ]}
                    label="条"
                    onChange={(val) => {
                      setLimit(val);
                      setBeforeStack([]);
                      setExpandedID(null);
                    }}
                  />
                </div>

                <div className="d-flex align-items-center gap-2">
                  <UsageAdvancedFiltersDropdown
                    ref={advRef}
                    disabled={loading}
                    toggleTestId="usage-adv-toggle"
                    fields={[
                      {
                        inputId: 'usageFilterKeyValue',
                        label: 'Key',
                        title: 'Key 名称',
                        placeholder: '输入 Key 名称',
                        value: filterKey,
                        onChange: (v) => {
                          setFilterKey(v);
                          setBeforeStack([]);
                          setExpandedID(null);
                        },
                      },
                      {
                        inputId: 'usageFilterModelValue',
                        label: '模型',
                        title: '模型',
                        placeholder: '输入模型名',
                        value: filterModel,
                        onChange: (v) => {
                          setFilterModel(v);
                          setBeforeStack([]);
                          setExpandedID(null);
                        },
                      },
                    ]}
                  />
                </div>

                <div className="ms-auto d-flex gap-2">
                  <button
                    className="btn btn-primary btn-sm"
                    type="button"
                    disabled={loading}
                    onClick={() => {
                      setBeforeStack([]);
                      setExpandedID(null);
                      void refresh(undefined);
                    }}
                  >
                    <span className="material-symbols-rounded me-1">refresh</span>
                    更新
                  </button>
                  <button
                    className="btn btn-light border btn-sm"
                    type="button"
                    disabled={loading}
                    onClick={() => {
                      setDateRangeCustomized(false);
                      setAllTime(false);
                      setStart('');
                      setEnd('');
                      advRef.current?.close();
                      setFilterKey('');
                      setFilterModel('');
                      setBeforeStack([]);
                      setExpandedID(null);
                      void refresh(undefined, { start: '', end: '', allTime: false, filterKey: '', filterModel: '' });
                    }}
                  >
                    重置
                  </button>
                </div>
              </div>
            </div>
          </div>
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
                rangeSinceText={detailSeriesStart || '-'}
                rangeUntilText={detailSeriesEnd || '-'}
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

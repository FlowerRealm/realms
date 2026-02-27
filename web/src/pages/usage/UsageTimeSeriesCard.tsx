import { useEffect, useRef, type MutableRefObject } from 'react';

import type { UsageTimeSeriesPoint } from '../../api/usage';
import { formatIntComma } from '../../format/int';

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (ctx: CanvasRenderingContext2D, config: unknown) => ChartInstance;

type DetailField = 'committed_usd' | 'requests' | 'tokens' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second';
type DetailGranularity = 'hour' | 'day';
type FieldOption = { value: DetailField; label: string };
type GranularityOption = { value: DetailGranularity; label: string };

export function UsageTimeSeriesCard({
  rangeSinceText,
  rangeUntilText,
  detailSeries,
  detailSeriesErr,
  detailSeriesLoading,
  detailField,
  setDetailField,
  detailGranularity,
  setDetailGranularity,
  fieldOptions,
  granularityOptions,
}: {
  rangeSinceText: string;
  rangeUntilText: string;
  detailSeries: UsageTimeSeriesPoint[];
  detailSeriesErr: string;
  detailSeriesLoading: boolean;
  detailField: DetailField;
  setDetailField: (value: DetailField) => void;
  detailGranularity: DetailGranularity;
  setDetailGranularity: (value: DetailGranularity) => void;
  fieldOptions: FieldOption[];
  granularityOptions: GranularityOption[];
}) {
  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);

  useEffect(() => {
    const ChartCtor = (globalThis.window as unknown as { Chart?: ChartConstructor })?.Chart;

    const destroy = (ref: MutableRefObject<ChartInstance | null>) => {
      try {
        ref.current?.destroy?.();
      } catch {
        // ignore
      }
      ref.current = null;
    };

    destroy(detailTimeLineChartRef);

    if (!ChartCtor) return;
    const canvas = detailTimeLineRef.current;
    if (!canvas) return;
    const ctx = canvas.getContext('2d');
    if (!ctx) return;

    const css = getComputedStyle(canvas);
    const rgb = (varName: string, fallback: string) => (css.getPropertyValue(varName).trim() || fallback).trim();
    const color = (rgbValue: string, alpha: number) => `rgba(${rgbValue}, ${alpha})`;
    const palette = {
      info: rgb('--bs-info-rgb', '53, 90, 96'),
      success: rgb('--bs-success-rgb', '47, 107, 75'),
      warning: rgb('--bs-warning-rgb', '122, 98, 50'),
      danger: rgb('--bs-danger-rgb', '122, 52, 52'),
      primary: rgb('--bs-primary-rgb', '60, 138, 97'),
      secondary: rgb('--bs-secondary-rgb', '99, 116, 107'),
    };

    const fieldMeta: Record<string, { label: string; color: string; read: (point: UsageTimeSeriesPoint) => number }> = {
      committed_usd: {
        label: '消耗 (USD)',
        color: color(palette.primary, 0.95),
        read: (point) => point.committed_usd,
      },
      requests: {
        label: '请求数',
        color: color(palette.info, 0.95),
        read: (point) => point.requests,
      },
      tokens: {
        label: 'Token',
        color: color(palette.success, 0.95),
        read: (point) => point.tokens,
      },
      cache_ratio: {
        label: '缓存率 (%)',
        color: color(palette.warning, 0.95),
        read: (point) => point.cache_ratio,
      },
      avg_first_token_latency: {
        label: '首字延迟 (s)',
        color: color(palette.danger, 0.95),
        read: (point) => point.avg_first_token_latency / 1000,
      },
      tokens_per_second: {
        label: 'Tokens/s',
        color: color(palette.secondary, 0.95),
        read: (point) => point.tokens_per_second,
      },
    };
    const meta = fieldMeta[detailField];

    detailTimeLineChartRef.current = new ChartCtor(ctx, {
      type: 'line',
      data: {
        labels: detailSeries.map((point) => point.bucket),
        datasets: [
          {
            label: meta.label,
            data: detailSeries.map((point) => meta.read(point)),
            borderColor: meta.color,
            backgroundColor: meta.color.replace('0.95', '0.18'),
            pointRadius: 2,
            tension: 0.2,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { position: 'bottom' },
          title: { display: true, text: '全站用量 · 时间序列' },
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: {
              autoSkip: true,
              maxTicksLimit: detailGranularity === 'hour' ? 10 : 14,
              maxRotation: 0,
              minRotation: 0,
            },
          },
          y: {
            beginAtZero: true,
            suggestedMax: detailField === 'cache_ratio' ? 100 : undefined,
            grid: { color: color(palette.secondary, 0.18) },
            ...(detailField === 'requests' || detailField === 'tokens'
              ? { ticks: { callback: (value: string | number) => formatIntComma(value) } }
              : {}),
          },
        },
      },
    });

    return () => {
      destroy(detailTimeLineChartRef);
    };
  }, [detailSeries, detailField, detailGranularity]);

  return (
    <div className="card border-0 p-0 overflow-hidden">
      <div className="card-header bg-white py-3 border-bottom px-4">
        <h5 className="mb-0 fw-bold">
          <i className="ri-line-chart-line me-2"></i>全站时间序列
        </h5>
      </div>
      <div className="card-body p-4">
        <div className="d-flex flex-wrap align-items-center gap-3 mb-2">
          <div className="d-flex align-items-center gap-2 flex-grow-1">
            <div className="d-flex flex-wrap gap-1">
              {fieldOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={`btn btn-sm ${detailField === option.value ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setDetailField(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
          <div className="d-flex align-items-center gap-2 ms-auto">
            <div className="d-flex gap-1">
              {granularityOptions.map((option) => (
                <button
                  key={option.value}
                  type="button"
                  className={`btn btn-sm ${detailGranularity === option.value ? 'btn-primary' : 'btn-outline-secondary'}`}
                  onClick={() => setDetailGranularity(option.value)}
                >
                  {option.label}
                </button>
              ))}
            </div>
          </div>
        </div>
        <div className="small text-muted mb-2">
          时间区间：{rangeSinceText} ~ {rangeUntilText}
        </div>
        {detailSeriesErr ? <div className="alert alert-danger py-2 mb-2">{detailSeriesErr}</div> : null}
        {detailSeriesLoading ? (
          <div className="text-muted small py-4">时间序列加载中…</div>
        ) : (
          <>
            <div style={{ height: 280 }}>
              <canvas ref={detailTimeLineRef}></canvas>
            </div>
          </>
        )}
      </div>
    </div>
  );
}

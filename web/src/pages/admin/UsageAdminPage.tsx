import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
} from "react";

import { useAuth } from "../../auth/AuthContext";
import {
  getAdminUsageEventDetail,
  getAdminUsagePage,
  getAdminUsageTimeSeries,
  type AdminUsagePage,
  type AdminUsageTimeSeriesPoint,
  type UsageEventDetail,
} from "../../api/admin/usage";
import {
  DateRangePicker,
  SelectPicker,
} from "../../components/DateRangePicker";
import { SegmentedFrame } from "../../components/SegmentedFrame";
import { ChannelSuggestInput } from "../../components/ChannelSuggestInput";
import { ModelSuggestInput } from "../../components/ModelSuggestInput";
import { UserSuggestInput } from "../../components/UserSuggestInput";
import {
  UsageAdvancedFiltersDropdown,
  type UsageAdvancedFiltersDropdownHandle,
} from "../../components/UsageAdvancedFiltersDropdown";
import {
  formatLatencyPairSeconds,
  formatSecondsFromMilliseconds,
} from "../../format/duration";
import { formatIntComma } from "../../format/int";

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (
  ctx: CanvasRenderingContext2D,
  config: unknown,
) => ChartInstance;

function badgeForState(cls: string): string {
  const s = (cls || "").trim();
  if (s) return `badge rounded-pill ${s}`;
  return "badge rounded-pill bg-light text-secondary border";
}

function costSourceLabel(source: string): string {
  switch ((source || "").trim()) {
    case "committed":
      return "已结算";
    case "reserved":
      return "预留";
    default:
      return "事件";
  }
}

function formatDecimalPlain(raw: string): string {
  let s = (raw || "").toString().trim();
  if (!s) return "0";
  if (s.startsWith("+")) s = s.slice(1).trim();
  if (s.startsWith("$")) s = s.slice(1).trim();
  if (!s) return "0";
  if (s.includes(".")) {
    s = s.replace(/0+$/, "").replace(/\.$/, "");
  }
  if (s === "-0" || s === "") return "0";
  return s;
}

function formatUSD(raw: string): string {
  const s = formatDecimalPlain(raw);
  if (s.startsWith("-")) return `-$${s.slice(1)}`;
  return `$${s}`;
}

function normalizeServiceTier(raw?: string | null): string {
  const tier = (raw || "").trim().toLowerCase();
  if (tier === "fast" || tier === "priority") return "priority";
  return tier;
}

function serviceTierBadgeLabel(raw?: string | null): string {
  const tier = normalizeServiceTier(raw);
  return tier ? tier.toUpperCase() : "";
}

function serviceTierText(raw?: string | null): string {
  const tier = normalizeServiceTier(raw);
  return tier || "-";
}

export function UsageAdminPage() {
  useAuth();

  const [data, setData] = useState<AdminUsagePage | null>(null);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");

  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [allTime, setAllTime] = useState(false);
  const [limit, setLimit] = useState(50);
  const [beforeID, setBeforeID] = useState<number | undefined>(undefined);
  const [afterID, setAfterID] = useState<number | undefined>(undefined);
  const [filterUser, setFilterUser] = useState("");
  const [filterUserID, setFilterUserID] = useState<number | undefined>(
    undefined,
  );
  const [filterChannel, setFilterChannel] = useState("");
  const [filterModel, setFilterModel] = useState("");
  const [filterChannelID, setFilterChannelID] = useState<number | undefined>(
    undefined,
  );
  const [filterModelExact, setFilterModelExact] = useState<string | undefined>(
    undefined,
  );
  const advRef = useRef<UsageAdvancedFiltersDropdownHandle | null>(null);

  const [expandedID, setExpandedID] = useState<number | null>(null);
  const [detailByEventID, setDetailByEventID] = useState<
    Record<number, UsageEventDetail>
  >({});
  const [detailLoadingID, setDetailLoadingID] = useState<number | null>(null);
  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);
  const [detailSeries, setDetailSeries] = useState<AdminUsageTimeSeriesPoint[]>(
    [],
  );
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState("");
  const [detailField, setDetailField] = useState<
    | "committed_usd"
    | "requests"
    | "tokens"
    | "cache_ratio"
    | "avg_first_token_latency"
    | "tokens_per_second"
  >("committed_usd");
  const [detailGranularity, setDetailGranularity] = useState<"hour" | "day">(
    "hour",
  );
  const fieldOptions: Array<{
    value:
      | "committed_usd"
      | "requests"
      | "tokens"
      | "cache_ratio"
      | "avg_first_token_latency"
      | "tokens_per_second";
    label: string;
  }> = [
    { value: "committed_usd", label: "消耗 (USD)" },
    { value: "requests", label: "请求数" },
    { value: "tokens", label: "Token" },
    { value: "cache_ratio", label: "缓存率 (%)" },
    { value: "avg_first_token_latency", label: "首字延迟 (s)" },
    { value: "tokens_per_second", label: "Tokens/s" },
  ];
  const granularityOptions: Array<{ value: "hour" | "day"; label: string }> = [
    { value: "hour", label: "按小时" },
    { value: "day", label: "按天" },
  ];

  async function refresh(opts?: {
    keepCursor?: boolean;
    override?: Partial<{
      start: string;
      end: string;
      allTime: boolean;
      filterUser: string;
      filterUserID: number | undefined;
      filterChannel: string;
      filterModel: string;
      filterChannelID: number | undefined;
      filterModelExact: string | undefined;
    }>;
  }) {
    setErr("");
    setLoading(true);
    try {
      const startValue = (opts?.override?.start ?? start).trim();
      const endValue = (opts?.override?.end ?? end).trim();
      const allTimeValue = !!(opts?.override?.allTime ?? allTime);
      const allTimeActive = allTimeValue && !startValue && !endValue;
      const indexParts: string[] = [];
      const q_user = (opts?.override?.filterUser ?? filterUser).trim();
      const q_user_id = opts?.override?.filterUserID ?? filterUserID;
      const q_channel = (opts?.override?.filterChannel ?? filterChannel).trim();
      const q_model = (opts?.override?.filterModel ?? filterModel).trim();
      if (!q_user_id && q_user) indexParts.push("user");
      const q_channel_id = opts?.override?.filterChannelID ?? filterChannelID;
      const q_model_exact =
        (opts?.override?.filterModelExact ?? filterModelExact) || undefined;
      if (!q_channel_id && q_channel) indexParts.push("channel");
      if (!q_model_exact && q_model) indexParts.push("model");
      const index = indexParts.length ? indexParts.join(",") : undefined;
      const params: {
        start?: string;
        end?: string;
        all_time?: boolean;
        limit?: number;
        before_id?: number;
        after_id?: number;
        user_id?: number;
        upstream_channel_id?: number;
        model?: string;
        index?: string;
        q_user?: string;
        q_channel?: string;
        q_model?: string;
      } = {
        limit,
        index,
        user_id:
          typeof q_user_id === "number" && q_user_id > 0
            ? q_user_id
            : undefined,
        q_user: !q_user_id ? q_user || undefined : undefined,
        upstream_channel_id:
          typeof q_channel_id === "number" && q_channel_id > 0
            ? q_channel_id
            : undefined,
        model: q_model_exact ? q_model_exact : undefined,
        q_channel: !q_channel_id ? q_channel || undefined : undefined,
        q_model: !q_model_exact ? q_model || undefined : undefined,
      };
      if (allTimeActive) params.all_time = true;
      else {
        params.start = startValue || undefined;
        params.end = endValue || undefined;
      }
      if (opts?.keepCursor) {
        if (beforeID) params.before_id = beforeID;
        if (afterID) params.after_id = afterID;
      }
      const res = await getAdminUsagePage(params);
      if (!res.success) throw new Error(res.message || "加载失败");
      const d = res.data || null;
      setData(d);
      if (d && !allTimeActive) {
        if (!startValue) setStart(d.start || "");
        if (!endValue) setEnd(d.end || "");
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载失败");
      setData(null);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const windowStats = data?.window;
  const topUsers = data?.top_users || [];
  const events = data?.events || [];
  const seriesStart = data?.start || "";
  const seriesEnd = data?.end || "";
  const hasSeriesSource = data !== null;

  const canPrev = useMemo(
    () =>
      typeof data?.prev_after_id === "number" && (data?.prev_after_id || 0) > 0,
    [data?.prev_after_id],
  );
  const canNext = useMemo(
    () =>
      typeof data?.next_before_id === "number" &&
      (data?.next_before_id || 0) > 0,
    [data?.next_before_id],
  );

  async function loadDetail(eventID: number) {
    if (detailByEventID[eventID]) return;
    setDetailLoadingID(eventID);
    try {
      const res = await getAdminUsageEventDetail(eventID);
      if (!res.success) throw new Error(res.message || "加载详情失败");
      const d = res.data;
      if (d) {
        setDetailByEventID((prev) => ({ ...prev, [eventID]: d }));
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : "加载详情失败");
    } finally {
      setDetailLoadingID(null);
    }
  }

  useEffect(() => {
    if (!hasSeriesSource) {
      setDetailSeries([]);
      setDetailSeriesErr("");
      setDetailSeriesLoading(false);
      return;
    }
    let active = true;
    void (async () => {
      setDetailSeriesErr("");
      setDetailSeriesLoading(true);
      try {
        const res = await getAdminUsageTimeSeries({
          start: seriesStart || undefined,
          end: seriesEnd || undefined,
          granularity: detailGranularity,
        });
        if (!res.success) throw new Error(res.message || "加载时间序列失败");
        if (!active) return;
        setDetailSeries(res.data?.points || []);
      } catch (e) {
        if (!active) return;
        setDetailSeries([]);
        setDetailSeriesErr(e instanceof Error ? e.message : "加载时间序列失败");
      } finally {
        if (active) setDetailSeriesLoading(false);
      }
    })();
    return () => {
      active = false;
    };
  }, [hasSeriesSource, seriesStart, seriesEnd, detailGranularity]);

  useEffect(() => {
    const ChartCtor = (
      globalThis.window as unknown as { Chart?: ChartConstructor }
    )?.Chart;

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
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    const css = getComputedStyle(canvas);
    const rgb = (varName: string, fallback: string) =>
      (css.getPropertyValue(varName).trim() || fallback).trim();
    const color = (rgbValue: string, alpha: number) =>
      `rgba(${rgbValue}, ${alpha})`;
    const palette = {
      info: rgb("--bs-info-rgb", "53, 90, 96"),
      success: rgb("--bs-success-rgb", "47, 107, 75"),
      warning: rgb("--bs-warning-rgb", "122, 98, 50"),
      danger: rgb("--bs-danger-rgb", "122, 52, 52"),
      primary: rgb("--bs-primary-rgb", "60, 138, 97"),
      secondary: rgb("--bs-secondary-rgb", "99, 116, 107"),
    };

    const fieldMeta: Record<
      string,
      {
        label: string;
        color: string;
        read: (point: AdminUsageTimeSeriesPoint) => number;
      }
    > = {
      committed_usd: {
        label: "消耗 (USD)",
        color: color(palette.primary, 0.95),
        read: (point) => point.committed_usd,
      },
      requests: {
        label: "请求数",
        color: color(palette.info, 0.95),
        read: (point) => point.requests,
      },
      tokens: {
        label: "Token",
        color: color(palette.success, 0.95),
        read: (point) => point.tokens,
      },
      cache_ratio: {
        label: "缓存率 (%)",
        color: color(palette.warning, 0.95),
        read: (point) => point.cache_ratio,
      },
      avg_first_token_latency: {
        label: "首字延迟 (s)",
        color: color(palette.danger, 0.95),
        read: (point) => point.avg_first_token_latency / 1000,
      },
      tokens_per_second: {
        label: "Tokens/s",
        color: color(palette.secondary, 0.95),
        read: (point) => point.tokens_per_second,
      },
    };
    const meta = fieldMeta[detailField];

    detailTimeLineChartRef.current = new ChartCtor(ctx, {
      type: "line",
      data: {
        labels: detailSeries.map((point) => point.bucket),
        datasets: [
          {
            label: meta.label,
            data: detailSeries.map((point) => meta.read(point)),
            borderColor: meta.color,
            backgroundColor: meta.color.replace("0.95", "0.18"),
            pointRadius: 2,
            tension: 0.2,
          },
        ],
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: { position: "bottom" },
          title: { display: true, text: "全站用量 · 时间序列" },
        },
        scales: {
          x: {
            grid: { display: false },
            ticks: {
              autoSkip: true,
              maxTicksLimit: detailGranularity === "hour" ? 10 : 14,
              maxRotation: 0,
              minRotation: 0,
            },
          },
          y: {
            beginAtZero: true,
            suggestedMax: detailField === "cache_ratio" ? 100 : undefined,
            grid: { color: color(palette.secondary, 0.18) },
            ...(detailField === "requests" || detailField === "tokens"
              ? {
                  ticks: {
                    callback: (value: string | number) => formatIntComma(value),
                  },
                }
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
    <div className="fade-in-up">
      <SegmentedFrame>
        <div>
          <div className="d-flex justify-content-between align-items-center mb-4">
            <div>
              <h3 className="mb-1 fw-bold">全站用量统计</h3>
              <div className="text-muted small">
                系统级数据汇总，涵盖所有用户及上游通道。
              </div>
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
                  <div className="text-muted smaller fw-medium text-nowrap">
                    时间区间
                  </div>
                  <DateRangePicker
                    start={start}
                    end={end}
                    onChange={(r) => {
                      const isAll = !r.start.trim() && !r.end.trim();
                      setAllTime(isAll);
                      if (isAll) setDetailGranularity("day");
                      setStart(r.start);
                      setEnd(r.end);
                      setBeforeID(undefined);
                      setAfterID(undefined);
                    }}
                    loading={loading}
                  />
                </div>

                <div className="d-flex flex-wrap align-items-center gap-2">
                  <div className="text-muted smaller fw-medium text-nowrap">
                    显示条数
                  </div>
                  <SelectPicker
                    value={limit}
                    options={[
                      { label: "20", value: 20 },
                      { label: "50", value: 50 },
                      { label: "100", value: 100 },
                      { label: "200", value: 200 },
                    ]}
                    label="条"
                    onChange={(val) => setLimit(val)}
                  />
                </div>

                <div className="d-flex align-items-center gap-2">
                  <UsageAdvancedFiltersDropdown
                    ref={advRef}
                    disabled={loading}
                    toggleTestId="admin-usage-adv-toggle"
                    fields={[
                      {
                        inputId: "adminUsageFilterUserValue",
                        label: "用户",
                        title: "用户名/邮箱",
                        placeholder: "输入用户名或邮箱",
                        value: filterUser,
                        onChange: (v) => {
                          setFilterUser(v);
                          setFilterUserID(undefined);
                          setBeforeID(undefined);
                          setAfterID(undefined);
                        },
                        render: ({
                          id,
                          value,
                          onChange,
                          placeholder,
                          disabled,
                        }) => (
                          <UserSuggestInput
                            id={id}
                            value={value}
                            placeholder={placeholder}
                            disabled={disabled}
                            onChange={onChange}
                            onSelect={(u) => {
                              setFilterUserID(u.id);
                              setFilterUser(u.email || `@${u.username}`);
                              setBeforeID(undefined);
                              setAfterID(undefined);
                            }}
                          />
                        ),
                      },
                      {
                        inputId: "adminUsageFilterChannelValue",
                        label: "渠道",
                        title: "渠道(ID/名称)",
                        placeholder: "输入渠道 ID 或名称",
                        value: filterChannel,
                        onChange: (v) => {
                          setFilterChannel(v);
                          setFilterChannelID(undefined);
                          setBeforeID(undefined);
                          setAfterID(undefined);
                        },
                        render: ({
                          id,
                          value,
                          onChange,
                          placeholder,
                          disabled,
                        }) => {
                          const startValue = start.trim();
                          const endValue = end.trim();
                          const allTimeActive =
                            !!allTime && !startValue && !endValue;
                          return (
                            <ChannelSuggestInput
                              id={id}
                              value={value}
                              placeholder={placeholder}
                              disabled={disabled}
                              start={
                                allTimeActive
                                  ? undefined
                                  : startValue || undefined
                              }
                              end={
                                allTimeActive
                                  ? undefined
                                  : endValue || undefined
                              }
                              allTime={allTimeActive}
                              onChange={onChange}
                              onSelect={(ch) => {
                                setFilterChannelID(ch.id);
                                setFilterChannel(ch.name);
                                setBeforeID(undefined);
                                setAfterID(undefined);
                              }}
                            />
                          );
                        },
                      },
                      {
                        inputId: "adminUsageFilterModelValue",
                        label: "模型",
                        title: "模型",
                        placeholder: "输入模型名",
                        value: filterModel,
                        onChange: (v) => {
                          setFilterModel(v);
                          setFilterModelExact(undefined);
                          setBeforeID(undefined);
                          setAfterID(undefined);
                        },
                        render: ({
                          id,
                          value,
                          onChange,
                          placeholder,
                          disabled,
                        }) => {
                          const startValue = start.trim();
                          const endValue = end.trim();
                          const allTimeActive =
                            !!allTime && !startValue && !endValue;
                          return (
                            <ModelSuggestInput
                              id={id}
                              value={value}
                              placeholder={placeholder}
                              disabled={disabled}
                              start={
                                allTimeActive
                                  ? undefined
                                  : startValue || undefined
                              }
                              end={
                                allTimeActive
                                  ? undefined
                                  : endValue || undefined
                              }
                              allTime={allTimeActive}
                              onChange={onChange}
                              onSelect={(m) => {
                                setFilterModelExact(m);
                                setFilterModel(m);
                                setBeforeID(undefined);
                                setAfterID(undefined);
                              }}
                            />
                          );
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
                      setBeforeID(undefined);
                      setAfterID(undefined);
                      void refresh();
                    }}
                  >
                    <span className="material-symbols-rounded me-1">
                      refresh
                    </span>
                    更新
                  </button>
                  <button
                    className="btn btn-light border btn-sm"
                    type="button"
                    disabled={loading}
                    onClick={() => {
                      setStart("");
                      setEnd("");
                      setAllTime(false);
                      advRef.current?.close();
                      setFilterUser("");
                      setFilterUserID(undefined);
                      setFilterChannel("");
                      setFilterModel("");
                      setFilterChannelID(undefined);
                      setFilterModelExact(undefined);
                      setBeforeID(undefined);
                      setAfterID(undefined);
                      void refresh({
                        override: {
                          start: "",
                          end: "",
                          allTime: false,
                          filterUser: "",
                          filterUserID: undefined,
                          filterChannel: "",
                          filterModel: "",
                          filterChannelID: undefined,
                          filterModelExact: undefined,
                        },
                      });
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
        ) : data && windowStats ? (
          <div className="row g-4">
            <div className="col-12">
              <div className="card border-0 overflow-hidden">
                <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
                  <div>
                    <span className="text-primary fw-bold text-uppercase small">
                      {windowStats.window}
                    </span>
                    <span className="text-primary text-opacity-75 smaller ms-2">
                      统计区间: {windowStats.since} ~ {windowStats.until}
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
                        <div className="text-muted smaller mb-1">
                          总营收流水（USD）
                        </div>
                        <h1 className="display-6 fw-bold mb-0 text-dark">
                          {windowStats.total_usd}
                        </h1>
                      </div>
                      <div className="row g-0 py-3 bg-light rounded-3 px-3">
                        <div className="col-6 border-end">
                          <div className="text-muted smaller">已结算</div>
                          <div className="fw-bold h5 mb-0 text-success">
                            {windowStats.committed_usd}
                          </div>
                        </div>
                        <div className="col-6 ps-3">
                          <div className="text-muted smaller">预留中</div>
                          <div className="fw-bold h5 mb-0 text-warning">
                            {windowStats.reserved_usd}
                          </div>
                        </div>
                      </div>
                      <div className="mt-3 smaller text-muted">
                        <i className="ri-information-line me-1"></i>{" "}
                        预留中费用指尚未结束或结算中的请求估算。
                      </div>
                    </div>
                    <div className="col-lg-8 ps-lg-4">
                      <div className="row g-3">
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              全局请求数
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {formatIntComma(windowStats.requests)}
                            </div>
                            <div className="text-primary smaller fw-medium">
                              {formatIntComma(windowStats.rpm)} RPM
                            </div>
                          </div>
                        </div>
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              Token 吞吐
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {formatIntComma(windowStats.tokens)}
                            </div>
                            <div className="text-primary smaller fw-medium">
                              {formatIntComma(windowStats.tpm)} TPM
                            </div>
                          </div>
                        </div>
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              缓存率
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {windowStats.cache_ratio}
                            </div>
                            <div className="text-muted smaller fw-medium">
                              输入 + 输出
                            </div>
                          </div>
                        </div>
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              缓存 Token
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {formatIntComma(windowStats.cached_tokens)}
                            </div>
                            <div className="text-muted smaller fw-medium">
                              输入 + 输出
                            </div>
                          </div>
                        </div>
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              平均首字延迟
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {formatSecondsFromMilliseconds(
                                windowStats.avg_first_token_latency,
                              )}
                            </div>
                            <div className="text-muted smaller fw-medium">
                              基于有效首字样本
                            </div>
                          </div>
                        </div>
                        <div className="col-sm-6 col-md-3">
                          <div className="metric-card p-3 rounded-3 border">
                            <div className="text-muted smaller mb-1">
                              平均 Tokens/s
                            </div>
                            <div className="h4 fw-bold mb-1">
                              {windowStats.tokens_per_second || "-"}
                            </div>
                            <div className="text-muted smaller fw-medium">
                              输出 Token 解码速率
                            </div>
                          </div>
                        </div>
                        <div className="col-12 mt-3">
                          <div className="bg-light p-3 rounded-3">
                            <div className="row text-center small">
                              <div className="col-6 border-end">
                                <div className="text-muted smaller">
                                  输入总计
                                </div>
                                <div className="fw-medium">
                                  {formatIntComma(windowStats.input_tokens)}
                                </div>
                              </div>
                              <div className="col-6">
                                <div className="text-muted smaller">
                                  输出总计
                                </div>
                                <div className="fw-medium">
                                  {formatIntComma(windowStats.output_tokens)}
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
            </div>

            <div className="col-12">
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
                            className={`btn btn-sm ${detailField === option.value ? "btn-primary" : "btn-outline-secondary"}`}
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
                            className={`btn btn-sm ${detailGranularity === option.value ? "btn-primary" : "btn-outline-secondary"}`}
                            onClick={() => setDetailGranularity(option.value)}
                          >
                            {option.label}
                          </button>
                        ))}
                      </div>
                    </div>
                  </div>
                  <div className="small text-muted mb-2">
                    时间区间：{windowStats.since} ~ {windowStats.until}
                  </div>
                  {detailSeriesErr ? (
                    <div className="alert alert-danger py-2 mb-2">
                      {detailSeriesErr}
                    </div>
                  ) : null}
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
            </div>

            <div className="col-12">
              <div className="card border-0 p-0 overflow-hidden">
                <div className="card-header bg-white py-3 border-bottom-0 px-4">
                  <h5 className="mb-0 fw-bold">
                    <i className="ri-group-line me-2"></i>
                    消费排行用户（统计区间）
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
                                  {(u.email || "?").slice(0, 1)}
                                </div>
                                <div>
                                  <div className="fw-bold small">{u.email}</div>
                                  <div className="text-muted smaller">
                                    {u.role}
                                  </div>
                                </div>
                              </div>
                            </td>
                            <td>
                              {u.status === 1 ? (
                                <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill px-2">
                                  正常
                                </span>
                              ) : (
                                <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill px-2">
                                  禁用
                                </span>
                              )}
                            </td>
                            <td className="text-end font-monospace small fw-bold text-dark">
                              {u.committed_usd}
                            </td>
                            <td className="text-end font-monospace small text-muted pe-4">
                              {u.reserved_usd}
                            </td>
                          </tr>
                        ))}
                        {topUsers.length === 0 ? (
                          <tr>
                            <td
                              colSpan={4}
                              className="text-center py-5 text-muted small"
                            >
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
                  <div className="table-responsive rlm-table-responsive-no-x">
                    <table className="table table-hover align-middle mb-0 border-0 rlm-table-fit">
                      <colgroup>
                        <col />
                        <col />
                        <col />
                        <col className="rlm-usage-col-status" />
                        <col className="rlm-usage-col-latency" />
                        <col className="rlm-usage-col-tokens" />
                        <col className="rlm-usage-col-tps" />
                        <col className="rlm-usage-col-cost" />
                        <col />
                        <col className="rlm-usage-col-channel" />
                        <col className="rlm-usage-col-request" />
                      </colgroup>
                      <thead className="table-light text-muted smaller uppercase">
                        <tr>
                          <th className="ps-4 border-0">时间</th>
                          <th className="border-0">用户</th>
                          <th className="border-0">接口 / 模型</th>
                          <th className="text-center border-0 rlm-usage-cell-compact">
                            状态码
                          </th>
                          <th className="text-end border-0 rlm-usage-cell-compact">
                            耗时/首字
                          </th>
                          <th className="text-end border-0 rlm-usage-cell-compact">
                            Tokens
                          </th>
                          <th className="text-end border-0 rlm-usage-cell-compact">
                            Tokens/s
                          </th>
                          <th className="text-end border-0 rlm-usage-cell-compact">
                            费用
                          </th>
                          <th className="text-center border-0">状态</th>
                          <th className="text-center border-0 rlm-usage-cell-compact">
                            渠道
                          </th>
                          <th className="pe-4 border-0">Request ID</th>
                        </tr>
                      </thead>
                      <tbody className="small">
                        {events.map((e) => {
                          const modelCheck = detailByEventID[e.id]?.model_check;
                          return (
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
                                <i
                                  className={`ri-arrow-right-s-line text-muted me-1 align-middle ${expandedID === e.id ? "rotate-90" : ""}`}
                                ></i>
                                <span className="align-middle">{e.time}</span>
                              </td>
                              <td className="text-nowrap">
                                <div className="fw-bold small">
                                  {e.user_email}
                                </div>
                                <div className="text-muted smaller">
                                  ID: {e.user_id}
                                </div>
                              </td>
                              <td className="text-nowrap">
                                <div className="badge bg-light text-dark border fw-normal">
                                  {e.model}
                                </div>
                                <div className="text-muted smaller mt-1 font-monospace">
                                  {e.endpoint}
                                </div>
                                {e.account && e.account !== "-" ? (
                                  <div className="text-muted smaller font-monospace">
                                    acct: {e.account}
                                  </div>
                                ) : null}
                              </td>
                              <td className="text-center rlm-usage-cell-compact">
                                {e.status_code === "200" ? (
                                  <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill">
                                    200
                                  </span>
                                ) : (
                                  <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill">
                                    {e.status_code}
                                  </span>
                                )}
                              </td>
                              <td className="text-end font-monospace text-muted rlm-usage-cell-compact">
                                {formatLatencyPairSeconds(
                                  e.latency_ms,
                                  e.first_token_latency_ms,
                                )}
                              </td>
                              <td className="text-end font-monospace rlm-usage-cell-compact">
                                <div>
                                  <span className="text-muted">In:</span>{" "}
                                  {formatIntComma(e.input_tokens)}
                                </div>
                                <div>
                                  <span className="text-muted">Out:</span>{" "}
                                  {formatIntComma(e.output_tokens)}
                                </div>
                                {e.cached_tokens !== "-" ? (
                                  <div className="text-muted smaller">
                                    <span className="material-symbols-rounded">
                                      bolt
                                    </span>{" "}
                                    {formatIntComma(e.cached_tokens)}
                                  </div>
                                ) : null}
                              </td>
                              <td className="text-end font-monospace text-muted rlm-usage-cell-compact">
                                {formatIntComma(e.tokens_per_second)}
                              </td>
                              <td className="text-end font-monospace fw-bold text-dark rlm-usage-cell-compact">
                                {e.cost_usd}
                              </td>
                              <td className="text-center text-nowrap">
                                <span
                                  className={badgeForState(e.state_badge_class)}
                                >
                                  {e.state_label}
                                </span>
                                {e.is_stream ? (
                                  <div className="badge bg-info-subtle text-info border border-info-subtle rounded-pill px-2 scale-90 mt-1">
                                    STREAM
                                  </div>
                                ) : null}
                                {serviceTierBadgeLabel(e.service_tier) ? (
                                  <div className="badge bg-warning-subtle text-warning border border-warning-subtle rounded-pill px-2 scale-90 mt-1">
                                    {serviceTierBadgeLabel(e.service_tier)}
                                  </div>
                                ) : null}
                                {e.model_mismatch ? (
                                  <div className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill px-2 scale-90 mt-1">
                                    MODEL
                                  </div>
                                ) : null}
                                {e.error ? (
                                  <div
                                    className="text-danger smaller mt-1"
                                    title={e.error}
                                  >
                                    <span className="material-symbols-rounded">
                                      error
                                    </span>{" "}
                                    错误
                                  </div>
                                ) : null}
                              </td>
                              <td className="text-center text-nowrap rlm-usage-cell-compact">
                                {e.upstream_channel_name ? (
                                  <span className="badge bg-light text-dark border fw-normal">
                                    {e.upstream_channel_name}
                                  </span>
                                ) : e.upstream_channel_id &&
                                  e.upstream_channel_id !== "-" ? (
                                  <span className="badge bg-light text-dark border fw-normal">
                                    #{e.upstream_channel_id}
                                  </span>
                                ) : (
                                  <span className="text-muted">-</span>
                                )}
                              </td>
                              <td
                                className="pe-4 font-monospace text-muted small user-select-all"
                                style={{
                                  maxWidth: 160,
                                  overflow: "hidden",
                                  textOverflow: "ellipsis",
                                }}
                                title={e.request_id}
                              >
                                {e.request_id}
                              </td>
                            </tr>
                            {expandedID === e.id ? (
                              <tr
                                key={`${e.id}-detail`}
                                className="rlm-usage-detail-row"
                              >
                                <td colSpan={11} className="p-0 border-0">
                                  <div className="bg-light px-4 py-3 mt-1">
                                    {detailLoadingID === e.id ? (
                                      <div className="text-muted small">
                                        加载详情中…
                                      </div>
                                    ) : null}
                                    {detailByEventID[e.id] ? (
                                      <div className="row g-3 small">
                                        <div className="col-12 col-lg-4">
                                          <div className="text-muted smaller">
                                            Event ID
                                          </div>
                                          <div className="font-monospace">
                                            {e.id}
                                          </div>
                                        </div>
                                        <div className="col-12 col-lg-4">
                                          <div className="text-muted smaller">
                                            Error Class
                                          </div>
                                          <div className="font-monospace">
                                            {e.error_class || "-"}
                                          </div>
                                        </div>
                                        <div className="col-12">
                                          <div className="text-muted smaller">
                                            Error Message
                                          </div>
                                          <div className="font-monospace text-break">
                                            {e.error_message || "-"}
                                          </div>
                                        </div>
                                        <div className="col-12 col-lg-4">
                                          <div className="text-muted smaller">
                                            Service Tier
                                          </div>
                                          <div className="font-monospace">
                                            {serviceTierText(
                                              detailByEventID[e.id]
                                                ?.pricing_breakdown
                                                ?.service_tier ||
                                                e.service_tier,
                                            )}
                                          </div>
                                        </div>
                                        {modelCheck ? (
                                          <>
                                            <div className="col-12 col-lg-4">
                                              <div className="text-muted smaller">
                                                转发模型
                                              </div>
                                              <div className="font-monospace text-break">
                                                {modelCheck.forwarded_model ||
                                                  "-"}
                                              </div>
                                            </div>
                                            <div className="col-12 col-lg-4">
                                              <div className="text-muted smaller">
                                                上游返回模型
                                              </div>
                                              <div className="font-monospace text-break">
                                                {modelCheck.upstream_response_model ||
                                                  "-"}
                                              </div>
                                            </div>
                                            <div className="col-12 col-lg-4">
                                              <div className="text-muted smaller">
                                                模型一致性
                                              </div>
                                              <div
                                                className={
                                                  modelCheck.mismatch
                                                    ? "text-danger fw-bold"
                                                    : "text-success fw-bold"
                                                }
                                              >
                                                {modelCheck.mismatch
                                                  ? "不一致"
                                                  : "一致"}
                                              </div>
                                            </div>
                                          </>
                                        ) : null}

                                        {detailByEventID[e.id]
                                          ?.pricing_breakdown ? (
                                          <div className="col-12">
                                            <div className="text-muted smaller">
                                              金额计算流程
                                            </div>
                                            <div className="font-monospace">
                                              <div>
                                                公式:
                                                ((输入总-缓存输入)×输入单价 +
                                                (输出总-缓存输出)×输出单价 +
                                                缓存输入×缓存输入单价 +
                                                缓存输出×缓存输出单价) ×
                                                生效倍率
                                              </div>
                                              <div className="mt-1">
                                                实际: ((
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.input_tokens_total || 0,
                                                )}
                                                -
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.input_tokens_cached || 0,
                                                )}
                                                )×
                                                {formatUSD(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.input_usd_per_1m || "0",
                                                )}
                                                /1M + (
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.output_tokens_total || 0,
                                                )}
                                                -
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.output_tokens_cached || 0,
                                                )}
                                                )×
                                                {formatUSD(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.output_usd_per_1m || "0",
                                                )}
                                                /1M +{" "}
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.input_tokens_cached || 0,
                                                )}
                                                ×
                                                {formatUSD(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.cache_input_usd_per_1m ||
                                                    "0",
                                                )}
                                                /1M +{" "}
                                                {formatIntComma(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.output_tokens_cached || 0,
                                                )}
                                                ×
                                                {formatUSD(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.cache_output_usd_per_1m ||
                                                    "0",
                                                )}
                                                /1M) ×{" "}
                                                {formatDecimalPlain(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.effective_multiplier ||
                                                    "1",
                                                )}{" "}
                                                ={" "}
                                                {formatUSD(
                                                  detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.final_cost_usd || "0",
                                                )}{" "}
                                                <span className="text-muted smaller">
                                                  （
                                                  {costSourceLabel(
                                                    detailByEventID[e.id]
                                                      ?.pricing_breakdown
                                                      ?.cost_source || "",
                                                  )}
                                                  费用:{" "}
                                                  {formatUSD(
                                                    detailByEventID[e.id]
                                                      ?.pricing_breakdown
                                                      ?.cost_source_usd || "0",
                                                  )}
                                                  ；倍率: 支付×
                                                  {formatDecimalPlain(
                                                    detailByEventID[e.id]
                                                      ?.pricing_breakdown
                                                      ?.payment_multiplier ||
                                                      "1",
                                                  )}{" "}
                                                  × 渠道组(
                                                  {detailByEventID[e.id]
                                                    ?.pricing_breakdown
                                                    ?.group_name || "default"}
                                                  )×
                                                  {formatDecimalPlain(
                                                    detailByEventID[e.id]
                                                      ?.pricing_breakdown
                                                      ?.group_multiplier || "1",
                                                  )}
                                                  ）
                                                </span>
                                              </div>
                                            </div>
                                          </div>
                                        ) : null}
                                      </div>
                                    ) : (
                                      <div className="text-muted small">
                                        （展开后自动加载费用计算明细）
                                      </div>
                                    )}
                                  </div>
                                </td>
                              </tr>
                            ) : null}
                          </>
                        )})}
                        {events.length === 0 ? (
                          <tr>
                            <td
                              colSpan={11}
                              className="text-center py-5 text-muted small"
                            >
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
      </SegmentedFrame>
    </div>
  );
}

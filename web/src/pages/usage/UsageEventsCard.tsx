import type { UserToken } from '../../api/tokens';
import type { UsageEvent, UsageEventDetail } from '../../api/usage';
import { formatLatencyPairSeconds } from '../../format/duration';
import { formatIntComma } from '../../format/int';
import {
  badgeForState,
  costLabel,
  costSourceLabel,
  errorText,
  formatDecimalPlain,
  formatLocalDateTime,
  formatUSD,
  stateLabel,
  tokenNameFromMap,
  tokensPerSecond,
} from './usageUtils';

function normalizeServiceTier(raw?: string | null): string {
  const tier = (raw || '').trim().toLowerCase();
  if (tier === 'fast' || tier === 'priority') return 'priority';
  return tier;
}

function serviceTierBadgeLabel(raw?: string | null): string {
  const tier = normalizeServiceTier(raw);
  return tier ? tier.toUpperCase() : '';
}

function serviceTierText(raw?: string | null): string {
  const tier = normalizeServiceTier(raw);
  return tier || '-';
}

export function UsageEventsCard({
  events,
  tokenByID,
  expandedID,
  setExpandedID,
  loadDetail,
  detailLoadingID,
  detailByEventID,
  canPrev,
  canNext,
  loading,
  onPrevPage,
  onNextPage,
  selfEmail,
  selfID,
}: {
  events: UsageEvent[];
  tokenByID: Record<number, UserToken>;
  expandedID: number | null;
  setExpandedID: (value: number | null) => void;
  loadDetail: (eventID: number) => void | Promise<void>;
  detailLoadingID: number | null;
  detailByEventID: Record<number, UsageEventDetail>;
  canPrev: boolean;
  canNext: boolean;
  loading: boolean;
  onPrevPage: () => void;
  onNextPage: () => void;
  selfEmail: string;
  selfID: string | number;
}) {
  return (
    <div className="card border-0 p-0 overflow-hidden">
      <div className="card-header bg-white py-3 border-bottom-0 px-4 d-flex justify-content-between align-items-center">
        <h5 className="mb-0 fw-bold">
          <i className="ri-list-check-2 me-2"></i>请求明细
        </h5>
        <div className="d-flex gap-2">
          <button type="button" className="btn btn-sm btn-outline-secondary" disabled={!canPrev || loading} onClick={onPrevPage}>
            上一页
          </button>
          <button type="button" className="btn btn-sm btn-outline-secondary" disabled={!canNext || loading} onClick={onNextPage}>
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
              <col className="rlm-usage-col-key" />
              <col className="rlm-usage-col-request" />
            </colgroup>
            <thead className="table-light text-muted smaller uppercase">
              <tr>
                <th className="ps-4 border-0">时间</th>
                <th className="border-0">用户</th>
                <th className="border-0">接口 / 模型</th>
                <th className="text-center border-0 rlm-usage-cell-compact">状态码</th>
                <th className="text-end border-0 rlm-usage-cell-compact">耗时/首字</th>
                <th className="text-end border-0 rlm-usage-cell-compact">Tokens</th>
                <th className="text-end border-0 rlm-usage-cell-compact">Tokens/s</th>
                <th className="text-end border-0 rlm-usage-cell-compact">费用</th>
                <th className="text-center border-0">状态</th>
                <th className="text-center border-0 rlm-usage-cell-compact">Key</th>
                <th className="pe-4 border-0">Request ID</th>
              </tr>
            </thead>
            <tbody className="small">
              {events.map((e) => {
                const endpoint = (e.endpoint || '').trim() || '-';
                const model = (e.model || '').trim() || '-';
                const keyName = tokenNameFromMap(tokenByID, e.token_id);
                const code = e.status_code ? String(e.status_code) : '-';
                const cached = (() => {
                  let v = 0;
                  if (typeof e.cached_input_tokens === 'number' && e.cached_input_tokens > 0) v += e.cached_input_tokens;
                  if (typeof e.cached_output_tokens === 'number' && e.cached_output_tokens > 0) v += e.cached_output_tokens;
                  if (v <= 0) return '-';
                  return String(v);
                })();
                const tps = tokensPerSecond(e);
                const cost = costLabel(e);
                const state = stateLabel(e.state);
                const errText = errorText(e.error_class, e.error_message);
                const serviceTier = serviceTierBadgeLabel(e.service_tier);

                return (
                  <>
                    <tr
                      key={e.id}
                      className="rlm-usage-row"
                      role="button"
                      onClick={() => {
                        const next = expandedID === e.id ? null : e.id;
                        setExpandedID(next);
                        if (next) void loadDetail(e.id);
                      }}
                    >
                      <td className="ps-4 text-nowrap font-monospace">
                        <i className={`ri-arrow-right-s-line text-muted me-1 align-middle ${expandedID === e.id ? 'rotate-90' : ''}`}></i>
                        <span className="align-middle">{formatLocalDateTime(String(e.time))}</span>
                      </td>
                      <td className="text-nowrap">
                        <div className="fw-bold small">{selfEmail}</div>
                        <div className="text-muted smaller">ID: {selfID}</div>
                      </td>
                      <td className="text-nowrap">
                        <div className="badge bg-light text-dark border fw-normal">{model}</div>
                        <div className="text-muted smaller mt-1 font-monospace">{endpoint}</div>
                      </td>
                      <td className="text-center rlm-usage-cell-compact">
                        {code === '200' ? (
                          <span className="badge bg-success-subtle text-success border border-success-subtle rounded-pill">200</span>
                        ) : (
                          <span className="badge bg-danger-subtle text-danger border border-danger-subtle rounded-pill">{code}</span>
                        )}
                      </td>
                      <td className="text-end font-monospace text-muted rlm-usage-cell-compact">{formatLatencyPairSeconds(e.latency_ms, undefined)}</td>
                      <td className="text-end font-monospace rlm-usage-cell-compact">
                        <div>
                          <span className="text-muted">In:</span> {formatIntComma(e.input_tokens)}
                        </div>
                        <div>
                          <span className="text-muted">Out:</span> {formatIntComma(e.output_tokens)}
                        </div>
                        {cached !== '-' ? (
                          <div className="text-muted smaller">
                            <span className="material-symbols-rounded">bolt</span> {formatIntComma(cached)}
                          </div>
                        ) : null}
                      </td>
                      <td className="text-end font-monospace text-muted rlm-usage-cell-compact">{formatIntComma(tps)}</td>
                      <td className="text-end font-monospace fw-bold text-dark rlm-usage-cell-compact">{cost}</td>
                      <td className="text-center text-nowrap">
                        <span className={badgeForState(state.badgeClass)}>{state.label}</span>
                        {e.is_stream ? (
                          <div className="badge bg-info-subtle text-info border border-info-subtle rounded-pill px-2 scale-90 mt-1">STREAM</div>
                        ) : null}
                        {serviceTier ? (
                          <div className="badge bg-warning-subtle text-warning border border-warning-subtle rounded-pill px-2 scale-90 mt-1">{serviceTier}</div>
                        ) : null}
                        {errText ? (
                          <div className="text-danger smaller mt-1" title={errText}>
                            <span className="material-symbols-rounded">error</span> 错误
                          </div>
                        ) : null}
                      </td>
                      <td className="text-center text-nowrap rlm-usage-cell-compact">
                        {keyName && keyName !== '-' ? <span className="badge bg-light text-dark border fw-normal">{keyName}</span> : <span className="text-muted">-</span>}
                      </td>
                      <td
                        className="pe-4 font-monospace text-muted small user-select-all"
                        style={{ maxWidth: 160, overflow: 'hidden', textOverflow: 'ellipsis' }}
                        title={e.request_id}
                      >
                        {e.request_id}
                      </td>
                    </tr>
                    {expandedID === e.id ? (
                      <tr key={`${e.id}-detail`} className="rlm-usage-detail-row">
                        <td colSpan={11} className="p-0 border-0">
                          <div className="bg-light px-4 py-3 mt-1">
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
                                <div className="col-12 col-lg-4">
                                  <div className="text-muted smaller">Service Tier</div>
                                  <div className="font-monospace">{serviceTierText(detailByEventID[e.id]?.pricing_breakdown?.service_tier || e.service_tier)}</div>
                                </div>

                                {detailByEventID[e.id]?.pricing_breakdown ? (
                                  <div className="col-12">
                                    <div className="text-muted smaller">金额计算流程</div>
                                    <div className="font-monospace">
                                      <div>
                                        公式: ((输入总-缓存输入)×输入单价 + (输出总-缓存输出)×输出单价 + 缓存输入×缓存输入单价 + 缓存输出×缓存输出单价) × 生效倍率
                                      </div>
                                      <div className="mt-1">
                                        实际: (({formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.input_tokens_total || 0)}-{formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.input_tokens_cached || 0)})×{formatUSD(detailByEventID[e.id]?.pricing_breakdown?.input_usd_per_1m || '0')}/1M + ({formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.output_tokens_total || 0)}-{formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.output_tokens_cached || 0)})×{formatUSD(detailByEventID[e.id]?.pricing_breakdown?.output_usd_per_1m || '0')}/1M + {formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.input_tokens_cached || 0)}×{formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_input_usd_per_1m || '0')}/1M + {formatIntComma(detailByEventID[e.id]?.pricing_breakdown?.output_tokens_cached || 0)}×{formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cache_output_usd_per_1m || '0')}/1M) × {formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.effective_multiplier || '1')} = {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.final_cost_usd || '0')}{' '}
                                        <span className="text-muted smaller">
                                          （{costSourceLabel(detailByEventID[e.id]?.pricing_breakdown?.cost_source || '')}费用: {formatUSD(detailByEventID[e.id]?.pricing_breakdown?.cost_source_usd || '0')}；倍率: 支付×{formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.payment_multiplier || '1')} × 渠道组({detailByEventID[e.id]?.pricing_breakdown?.group_name || 'default'})×{formatDecimalPlain(detailByEventID[e.id]?.pricing_breakdown?.group_multiplier || '1')}）
                                        </span>
                                      </div>
                                    </div>
                                  </div>
                                ) : null}
                              </div>
                            ) : (
                              <div className="text-muted small">（展开后自动加载费用计算明细）</div>
                            )}
                          </div>
                        </td>
                      </tr>
                    ) : null}
                  </>
                );
              })}
              {events.length === 0 ? (
                <tr>
                  <td colSpan={11} className="text-center py-5 text-muted small">
                    暂无请求记录
                  </td>
                </tr>
              ) : null}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}

import {
  Fragment,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MutableRefObject,
} from "react";

import { useAuth } from "../../auth/AuthContext";
import { AutoSaveIndicator } from "../../components/AutoSaveIndicator";
import { BootstrapModal } from "../../components/BootstrapModal";
import { SegmentedFrame } from "../../components/SegmentedFrame";
import { closeModalById } from "../../components/modal";
import { useAutoSave } from "../../hooks/useAutoSave";
import { formatSecondsFromMilliseconds } from "../../format/duration";
import { formatIntComma } from "../../format/int";
import {
  createChannel,
  createChannelCredential,
  createChannelCodexAccount,
  deleteChannelCodexAccount,
  deleteChannel,
  deleteChannelCredential,
  completeChannelCodexOAuth,
  getChannel,
  getChannelKey,
  getChannelsPage,
  getChannelTimeSeries,
  listChannelCodexAccounts,
  listChannelCredentials,
  refreshChannelCodexAccount,
  startChannelCodexOAuth,
  testChannel,
  updateChannel,
  updateChannelHeaderOverride,
  updateChannelMeta,
  updateChannelModelSuffixPreserve,
  updateChannelParamOverride,
  updateChannelRequestBodyBlacklist,
  updateChannelRequestBodyWhitelist,
  updateChannelSetting,
  updateChannelStatusCodeMapping,
  type Channel,
  type ChannelAdminItem,
  type ChannelCredential,
  type ChannelModelProbeResult,
  type ChannelTimeSeriesPoint,
  type CodexOAuthAccount,
} from "../../api/channels";
import {
  listAdminChannelGroups,
  upsertAdminChannelGroupPointer,
  type AdminChannelGroup,
} from "../../api/admin/channelGroups";
import {
  createChannelModel,
  listChannelModels,
  updateChannelModel,
  type ChannelModelBinding,
} from "../../api/channelModels";
import { listSelectableManagedModelIDsAdmin } from "../../api/models";
import { DateRangePicker } from "../../components/DateRangePicker";

function channelTypeLabel(t: string): string {
  if (t === "openai_compatible") return "OpenAI 兼容";
  if (t === "anthropic") return "Anthropic";
  if (t === "codex_oauth") return "Codex OAuth";
  return t;
}

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1)
    return {
      cls: "badge bg-success bg-opacity-10 text-success border border-success-subtle",
      label: "启用",
    };
  return {
    cls: "badge bg-secondary bg-opacity-10 text-secondary border",
    label: "禁用",
  };
}

type ChannelPatch = Partial<{
  name: string;
  status: number;
  base_url: string;
  groups: string;
  priority: number;
  promotion: boolean;
  allow_service_tier: boolean;
  fast_mode: boolean;
  disable_store: boolean;
  allow_safety_identifier: boolean;
}>;

function parseGroupsCSV(raw: string): string[] {
  const s = raw.trim();
  if (!s) return [];
  const uniq = new Set<string>();
  for (const part of s.split(",")) {
    const v = part.trim();
    if (v) uniq.add(v);
  }
  return Array.from(uniq);
}

function toggleGroupsCSV(raw: string, name: string, checked: boolean): string {
  const set = new Set(parseGroupsCSV(raw));
  if (checked) set.add(name);
  else set.delete(name);
  return Array.from(set).join(",");
}

function validateJSON(raw: string, kind: "object" | "array"): string {
  const s = (raw || "").trim();
  if (!s) return "";
  try {
    const v = JSON.parse(s) as unknown;
    if (kind === "array") {
      if (!Array.isArray(v)) return "JSON 必须为数组";
      return "";
    }
    if (!v || typeof v !== "object" || Array.isArray(v))
      return "JSON 必须为对象";
    return "";
  } catch {
    return "JSON 不合法";
  }
}

function compactProbeMessage(raw: string): string {
  let msg = raw.trim();
  if (!msg) return "";
  msg = msg.replace(
    /Post "[^"]+": context deadline exceeded \(Client\.Timeout exceeded while awaiting headers\)/g,
    "请求超时",
  );
  msg = msg.replace(
    /context deadline exceeded \(Client\.Timeout exceeded while awaiting headers\)/g,
    "请求超时",
  );
  return msg;
}

function modelCheckLabel(status?: string): string {
  if (status === "ok") return "模型一致";
  if (status === "mismatch") return "模型不一致";
  return "模型未知";
}

function modelCheckBadgeClass(status?: string): string {
  if (status === "ok") {
    return "badge bg-success bg-opacity-10 text-success border border-success-subtle";
  }
  if (status === "mismatch") {
    return "badge bg-warning bg-opacity-10 text-warning border border-warning-subtle";
  }
  return "badge bg-secondary bg-opacity-10 text-secondary border";
}

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (
  ctx: CanvasRenderingContext2D,
  config: unknown,
) => ChartInstance;

type ChannelModelLiveState = {
  model: string;
  status: "success" | "failed";
  message: string;
  output: string;
  result?: ChannelModelProbeResult;
};

type ChannelTestPanelState = {
  running: boolean;
  source: string;
  total: number;
  done: number;
  models: ChannelModelLiveState[];
  error: string;
};

type ChannelPointerTarget = {
  id: number;
  name: string;
  groups: string;
};

function ChannelCommonTab({
  enabled,
  resetKey,
  channelID,
  channelGroups,
  editName,
  setEditName,
  editStatus,
  setEditStatus,
  editBaseURL,
  setEditBaseURL,
  editGroups,
  setEditGroups,
  editPriority,
  setEditPriority,
  editPromotion,
  setEditPromotion,
  editAllowServiceTier,
  setEditAllowServiceTier,
  editFastMode,
  setEditFastMode,
  editDisableStore,
  setEditDisableStore,
  editAllowSafetyIdentifier,
  setEditAllowSafetyIdentifier,
  applyChannelPatch,
}: {
  enabled: boolean;
  resetKey: number;
  channelID: number;
  channelGroups: AdminChannelGroup[];
  editName: string;
  setEditName: (v: string) => void;
  editStatus: number;
  setEditStatus: (v: number) => void;
  editBaseURL: string;
  setEditBaseURL: (v: string) => void;
  editGroups: string;
  setEditGroups: (v: string) => void;
  editPriority: string;
  setEditPriority: (v: string) => void;
  editPromotion: boolean;
  setEditPromotion: (v: boolean) => void;
  editAllowServiceTier: boolean;
  setEditAllowServiceTier: (v: boolean) => void;
  editFastMode: boolean;
  setEditFastMode: (v: boolean) => void;
  editDisableStore: boolean;
  setEditDisableStore: (v: boolean) => void;
  editAllowSafetyIdentifier: boolean;
  setEditAllowSafetyIdentifier: (v: boolean) => void;
  applyChannelPatch: (id: number, patch: ChannelPatch) => void;
}) {
  const commonAutosave = useAutoSave({
    enabled,
    resetKey,
    value: {
      name: editName,
      status: editStatus,
      base_url: editBaseURL,
      groups: editGroups,
      priority: editPriority,
      promotion: editPromotion,
    },
    validate: (v) => {
      if (!v.name.trim()) return "名称不能为空";
      if (!v.base_url.trim()) return "接口基础地址不能为空";
      return "";
    },
    save: async (v) => {
      const res = await updateChannel({
        id: channelID,
        name: v.name.trim(),
        status: v.status,
        base_url: v.base_url.trim(),
        groups: v.groups.trim(),
        priority: Number.parseInt(v.priority, 10) || 0,
        promotion: !!v.promotion,
      });
      if (!res.success) throw new Error(res.message || "保存失败");
      applyChannelPatch(channelID, {
        name: v.name.trim(),
        status: v.status,
        base_url: v.base_url.trim(),
        groups: v.groups.trim(),
        priority: Number.parseInt(v.priority, 10) || 0,
        promotion: !!v.promotion,
      });
    },
  });

  const requestPolicyAutosave = useAutoSave({
    enabled,
    resetKey,
    value: {
      allow_service_tier: editAllowServiceTier,
      fast_mode: editFastMode,
      disable_store: editDisableStore,
      allow_safety_identifier: editAllowSafetyIdentifier,
    },
    validate: (v) => {
      if (v.fast_mode && !v.allow_service_tier)
        return "启用 Fast mode 时必须同时允许透传 service_tier";
      return "";
    },
    save: async (v) => {
      const res = await updateChannel({
        id: channelID,
        allow_service_tier: v.allow_service_tier,
        fast_mode: v.fast_mode,
        disable_store: v.disable_store,
        allow_safety_identifier: v.allow_safety_identifier,
      });
      if (!res.success) throw new Error(res.message || "保存失败");
      applyChannelPatch(channelID, {
        allow_service_tier: v.allow_service_tier,
        fast_mode: v.fast_mode,
        disable_store: v.disable_store,
        allow_safety_identifier: v.allow_safety_identifier,
      });
    },
  });

  return (
    <div className="d-flex flex-column gap-3">
      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">常用设置</div>
        <div className="card-body">
          <form
            className="row g-3"
            onSubmit={(e) => {
              e.preventDefault();
              commonAutosave.flush();
            }}
          >
            <div className="col-md-8">
              <label className="form-label fw-medium">名称</label>
              <input
                className="form-control"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                required
              />
            </div>
            <div className="col-md-4">
              <label className="form-label fw-medium">状态</label>
              <select
                className="form-select"
                value={editStatus}
                onChange={(e) =>
                  setEditStatus(Number.parseInt(e.target.value, 10) || 0)
                }
              >
                <option value={1}>启用</option>
                <option value={0}>禁用</option>
              </select>
            </div>
            <div className="col-12">
              <label className="form-label fw-medium">接口基础地址</label>
              <input
                className="form-control font-monospace"
                value={editBaseURL}
                onChange={(e) => setEditBaseURL(e.target.value)}
                required
              />
              <div className="form-text small text-muted">
                保存后立即生效；密钥与模型绑定可在本弹窗继续配置。
              </div>
            </div>

            <div className="col-12">
              <label className="form-label fw-medium">渠道组设置</label>
              <div
                className="card p-2"
                style={{ maxHeight: 260, overflowY: "auto" }}
              >
                {channelGroups.length === 0 ? (
                  <div className="text-muted small px-2 py-1">
                    暂无渠道组（请先到“渠道组”创建）。
                  </div>
                ) : (
                  channelGroups.map((g) => {
                    const selected = parseGroupsCSV(editGroups).includes(
                      g.name,
                    );
                    const disabled = g.status !== 1 && !selected;
                    return (
                      <div className="form-check" key={g.id}>
                        <input
                          className="form-check-input"
                          type="checkbox"
                          id={`group_edit_${channelID}_${g.name}`}
                          checked={selected}
                          disabled={disabled}
                          onChange={(e) =>
                            setEditGroups(
                              toggleGroupsCSV(
                                editGroups,
                                g.name,
                                e.target.checked,
                              ),
                            )
                          }
                        />
                        <label
                          className="form-check-label w-100"
                          htmlFor={`group_edit_${channelID}_${g.name}`}
                        >
                          {g.name}{" "}
                          {g.status !== 1 ? (
                            <span className="badge bg-secondary ms-1 smaller">
                              禁用
                            </span>
                          ) : null}
                        </label>
                      </div>
                    );
                  })
                )}
              </div>
              <div className="form-text small text-muted mt-2">
                用于上游调度选择渠道。
              </div>
            </div>

            <div className="col-md-6">
              <label className="form-label fw-medium">优先级</label>
              <input
                className="form-control"
                value={editPriority}
                onChange={(e) => setEditPriority(e.target.value)}
                inputMode="numeric"
              />
            </div>
            <div className="col-md-6 d-flex align-items-end">
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="editPromotion"
                  checked={editPromotion}
                  onChange={(e) => setEditPromotion(e.target.checked)}
                />
                <label className="form-check-label" htmlFor="editPromotion">
                  优先（promotion）
                </label>
              </div>
            </div>

            <div className="col-12">
              <AutoSaveIndicator
                status={commonAutosave.status}
                blockedReason={commonAutosave.blockedReason}
                error={commonAutosave.error}
                onRetry={commonAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">请求字段策略</div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              requestPolicyAutosave.flush();
            }}
          >
            <div className="d-flex flex-column gap-2">
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="editAllowServiceTier"
                  checked={editAllowServiceTier}
                  onChange={(e) => setEditAllowServiceTier(e.target.checked)}
                />
                <label
                  className="form-check-label"
                  htmlFor="editAllowServiceTier"
                >
                  允许透传 <code>service_tier</code>
                </label>
                <div className="form-text small text-muted">
                  可能触发上游额外计费；启用 Fast mode 时必须同时开启。
                </div>
              </div>
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="editFastMode"
                  checked={editFastMode}
                  onChange={(e) => {
                    const checked = e.target.checked;
                    setEditFastMode(checked);
                    if (checked) setEditAllowServiceTier(true);
                  }}
                />
                <label className="form-check-label" htmlFor="editFastMode">
                  允许用户使用 Fast mode（<code>service_tier="priority"</code>）
                </label>
                <div className="form-text small text-muted">
                  仅控制是否允许透传 Fast；不会自动把请求改写成 Fast。启用时会强制保留 <code>service_tier</code> 透传。
                </div>
              </div>
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="editDisableStore"
                  checked={editDisableStore}
                  onChange={(e) => setEditDisableStore(e.target.checked)}
                />
                <label className="form-check-label" htmlFor="editDisableStore">
                  禁用透传 <code>store</code>
                </label>
                <div className="form-text small text-muted">
                  涉及数据存储授权；默认允许透传。
                </div>
              </div>
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="editAllowSafetyIdentifier"
                  checked={editAllowSafetyIdentifier}
                  onChange={(e) =>
                    setEditAllowSafetyIdentifier(e.target.checked)
                  }
                />
                <label
                  className="form-check-label"
                  htmlFor="editAllowSafetyIdentifier"
                >
                  允许透传 <code>safety_identifier</code>
                </label>
                <div className="form-text small text-muted">
                  可能暴露用户信息；默认会过滤。
                </div>
              </div>
            </div>
            <div className="mt-3">
              <AutoSaveIndicator
                status={requestPolicyAutosave.status}
                blockedReason={requestPolicyAutosave.blockedReason}
                error={requestPolicyAutosave.error}
                onRetry={requestPolicyAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}

function ChannelAdvancedTab({
  enabled,
  resetKey,
  channelID,
  channelType,
  metaOpenAIOrganization,
  setMetaOpenAIOrganization,
  metaTestModel,
  setMetaTestModel,
  metaTag,
  setMetaTag,
  metaWeight,
  setMetaWeight,
  metaAutoBan,
  setMetaAutoBan,
  metaRemark,
  setMetaRemark,
  settingThinkingToContent,
  setSettingThinkingToContent,
  settingPassThroughBodyEnabled,
  setSettingPassThroughBodyEnabled,
  settingProxy,
  setSettingProxy,
  settingSystemPrompt,
  setSettingSystemPrompt,
  settingSystemPromptOverride,
  setSettingSystemPromptOverride,
  paramOverride,
  setParamOverride,
  headerOverride,
  setHeaderOverride,
  modelSuffixPreserve,
  setModelSuffixPreserve,
  requestBodyWhitelist,
  setRequestBodyWhitelist,
  requestBodyBlacklist,
  setRequestBodyBlacklist,
  statusCodeMapping,
  setStatusCodeMapping,
}: {
  enabled: boolean;
  resetKey: number;
  channelID: number;
  channelType: string;
  metaOpenAIOrganization: string;
  setMetaOpenAIOrganization: (v: string) => void;
  metaTestModel: string;
  setMetaTestModel: (v: string) => void;
  metaTag: string;
  setMetaTag: (v: string) => void;
  metaWeight: string;
  setMetaWeight: (v: string) => void;
  metaAutoBan: boolean;
  setMetaAutoBan: (v: boolean) => void;
  metaRemark: string;
  setMetaRemark: (v: string) => void;
  settingThinkingToContent: boolean;
  setSettingThinkingToContent: (v: boolean) => void;
  settingPassThroughBodyEnabled: boolean;
  setSettingPassThroughBodyEnabled: (v: boolean) => void;
  settingProxy: string;
  setSettingProxy: (v: string) => void;
  settingSystemPrompt: string;
  setSettingSystemPrompt: (v: string) => void;
  settingSystemPromptOverride: boolean;
  setSettingSystemPromptOverride: (v: boolean) => void;
  paramOverride: string;
  setParamOverride: (v: string) => void;
  headerOverride: string;
  setHeaderOverride: (v: string) => void;
  modelSuffixPreserve: string;
  setModelSuffixPreserve: (v: string) => void;
  requestBodyWhitelist: string;
  setRequestBodyWhitelist: (v: string) => void;
  requestBodyBlacklist: string;
  setRequestBodyBlacklist: (v: string) => void;
  statusCodeMapping: string;
  setStatusCodeMapping: (v: string) => void;
}) {
  const metaAutosave = useAutoSave({
    enabled,
    resetKey,
    value: {
      openai_organization: metaOpenAIOrganization,
      test_model: metaTestModel,
      tag: metaTag,
      remark: metaRemark,
      weight: metaWeight,
      auto_ban: metaAutoBan,
    },
    save: async (v) => {
      const res = await updateChannelMeta(channelID, {
        openai_organization: v.openai_organization.trim() || null,
        test_model: v.test_model.trim() || null,
        tag: v.tag.trim() || null,
        remark: v.remark.trim() || null,
        weight: Number.parseInt(v.weight, 10) || 0,
        auto_ban: v.auto_ban,
      });
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const settingAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 800,
    value: {
      thinking_to_content: settingThinkingToContent,
      pass_through_body_enabled: settingPassThroughBodyEnabled,
      proxy: settingProxy,
      system_prompt: settingSystemPrompt,
      system_prompt_override: settingSystemPromptOverride,
    },
    save: async (v) => {
      const res = await updateChannelSetting(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const paramAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: paramOverride,
    validate: (v) => validateJSON(v, "object"),
    save: async (v) => {
      const res = await updateChannelParamOverride(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const headerAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: headerOverride,
    validate: (v) => validateJSON(v, "object"),
    save: async (v) => {
      const res = await updateChannelHeaderOverride(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const suffixAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: modelSuffixPreserve,
    validate: (v) => validateJSON(v, "array"),
    save: async (v) => {
      const res = await updateChannelModelSuffixPreserve(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const whitelistAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: requestBodyWhitelist,
    validate: (v) => validateJSON(v, "array"),
    save: async (v) => {
      const res = await updateChannelRequestBodyWhitelist(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const blacklistAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: requestBodyBlacklist,
    validate: (v) => validateJSON(v, "array"),
    save: async (v) => {
      const res = await updateChannelRequestBodyBlacklist(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  const statusCodeAutosave = useAutoSave({
    enabled,
    resetKey,
    debounceMs: 1000,
    value: statusCodeMapping,
    validate: (v) => validateJSON(v, "object"),
    save: async (v) => {
      const res = await updateChannelStatusCodeMapping(channelID, v);
      if (!res.success) throw new Error(res.message || "保存失败");
    },
  });

  return (
    <div className="d-flex flex-column gap-3">
      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">渠道属性</div>
        <div className="card-body">
          <form
            className="row g-3"
            onSubmit={(e) => {
              e.preventDefault();
              metaAutosave.flush();
            }}
          >
            {channelType === "openai_compatible" ? (
              <div className="col-md-6">
                <label className="form-label fw-medium">
                  OpenAI Organization（组织 ID）
                </label>
                <input
                  className="form-control font-monospace"
                  value={metaOpenAIOrganization}
                  onChange={(e) => setMetaOpenAIOrganization(e.target.value)}
                  placeholder="org_xxx"
                />
                <div className="form-text small text-muted">
                  会注入到上游请求头 <code>OpenAI-Organization</code>
                  ；可被“请求头覆盖”覆盖。
                </div>
              </div>
            ) : null}
            <div className="col-md-6">
              <label className="form-label fw-medium">默认测试模型</label>
              <input
                className="form-control font-monospace"
                value={metaTestModel}
                onChange={(e) => setMetaTestModel(e.target.value)}
                placeholder="留空=自动选择"
              />
              <div className="form-text small text-muted">
                用于“测试”按钮：优先级高于模型绑定与默认值。
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label fw-medium">标记（Tag）</label>
              <input
                className="form-control"
                value={metaTag}
                onChange={(e) => setMetaTag(e.target.value)}
                placeholder="例如：prod-1"
              />
              <div className="form-text small text-muted">
                用于标记/检索（仅保存，不参与调度）。
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label fw-medium">权重（可选）</label>
              <input
                className="form-control"
                type="number"
                min={0}
                value={metaWeight}
                onChange={(e) => setMetaWeight(e.target.value)}
              />
              <div className="form-text small text-muted">
                当前不参与调度（Realms 调度以渠道组/优先级/推荐为准）。
              </div>
            </div>
            <div className="col-md-6">
              <label className="form-label fw-medium">自动封禁</label>
              <select
                className="form-select"
                value={metaAutoBan ? "1" : "0"}
                onChange={(e) => setMetaAutoBan(e.target.value === "1")}
              >
                <option value="1">启用</option>
                <option value="0">禁用</option>
              </select>
              <div className="form-text small text-muted">
                禁用后：失败不会封禁该渠道（credential 冷却仍生效）。
              </div>
            </div>
            <div className="col-12">
              <label className="form-label fw-medium">备注</label>
              <input
                className="form-control"
                value={metaRemark}
                onChange={(e) => setMetaRemark(e.target.value)}
                placeholder="可选"
              />
              <div className="form-text small text-muted">
                仅用于管理端备注（不参与调度）。
              </div>
            </div>
            <div className="col-12">
              <AutoSaveIndicator
                status={metaAutosave.status}
                blockedReason={metaAutosave.blockedReason}
                error={metaAutosave.error}
                onRetry={metaAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">请求处理设置</div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              settingAutosave.flush();
            }}
          >
            <div className="d-flex flex-column gap-2">
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="setting_thinking_to_content"
                  checked={settingThinkingToContent}
                  onChange={(e) =>
                    setSettingThinkingToContent(e.target.checked)
                  }
                />
                <label
                  className="form-check-label"
                  htmlFor="setting_thinking_to_content"
                >
                  推理内容合并到正文
                </label>
                <div className="form-text small text-muted">
                  将流式 <code>reasoning_content</code> 转为{" "}
                  <code>&lt;think&gt;...&lt;/think&gt;</code> 并拼接到{" "}
                  <code>content</code> 中返回。
                </div>
              </div>
              <div className="form-check">
                <input
                  className="form-check-input"
                  type="checkbox"
                  id="setting_pass_through_body_enabled"
                  checked={settingPassThroughBodyEnabled}
                  onChange={(e) =>
                    setSettingPassThroughBodyEnabled(e.target.checked)
                  }
                />
                <label
                  className="form-check-label"
                  htmlFor="setting_pass_through_body_enabled"
                >
                  透传请求体（不改写）
                </label>
                <div className="form-text small text-muted">
                  启用后：该渠道将直接透传原始请求体（不再应用模型改写/策略/黑白名单/参数改写/系统提示）。
                </div>
              </div>
            </div>

            <div className="mt-3">
              <label className="form-label fw-medium">代理（可选）</label>
              <input
                className="form-control font-monospace"
                value={settingProxy}
                onChange={(e) => setSettingProxy(e.target.value)}
                placeholder="http(s)://host:port 或 socks5://host:port；留空=继承环境代理；direct=禁用"
              />
              <div className="form-text small text-muted">
                按渠道指定上游网络代理。
              </div>
            </div>

            <div className="mt-3">
              <label className="form-label fw-medium">系统提示词（可选）</label>
              <textarea
                className="form-control font-monospace"
                rows={4}
                value={settingSystemPrompt}
                onChange={(e) => setSettingSystemPrompt(e.target.value)}
                placeholder="可选：统一注入系统提示"
              />
              <div className="form-text small text-muted">
                对 <code>/v1/chat/completions</code> 注入 system 消息；对{" "}
                <code>/v1/responses</code> 注入 instructions。
              </div>
            </div>

            <div className="form-check mt-2">
              <input
                className="form-check-input"
                type="checkbox"
                id="setting_system_prompt_override"
                checked={settingSystemPromptOverride}
                onChange={(e) =>
                  setSettingSystemPromptOverride(e.target.checked)
                }
              />
              <label
                className="form-check-label"
                htmlFor="setting_system_prompt_override"
              >
                始终拼接系统提示词
              </label>
              <div className="form-text small text-muted">
                当请求已包含 system/instructions
                时：是否将“系统提示词”拼接到最前。
              </div>
            </div>

            <div className="mt-3">
              <AutoSaveIndicator
                status={settingAutosave.status}
                blockedReason={settingAutosave.blockedReason}
                error={settingAutosave.error}
                onRetry={settingAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">参数改写</div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              paramAutosave.flush();
            }}
          >
            <textarea
              className="form-control font-monospace"
              rows={10}
              value={paramOverride}
              onChange={(e) => setParamOverride(e.target.value)}
              placeholder='{"operations":[{"path":"metadata.channel","mode":"set","value":"example"}]}'
            />
            <div className="form-text small text-muted mt-2">
              留空表示禁用。JSON 必须为对象，会在转发前按渠道应用。
            </div>
            <div className="mt-3">
              <AutoSaveIndicator
                status={paramAutosave.status}
                blockedReason={paramAutosave.blockedReason}
                error={paramAutosave.error}
                onRetry={paramAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">请求头覆盖</div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              headerAutosave.flush();
            }}
          >
            <textarea
              className="form-control font-monospace"
              rows={10}
              value={headerOverride}
              onChange={(e) => setHeaderOverride(e.target.value)}
              placeholder='{"OpenAI-Organization":"org_xxx","X-Proxy-Key":"{api_key}"}'
            />
            <div className="form-text small text-muted mt-2">
              留空表示禁用。JSON 必须为对象，value 必须为字符串；支持变量{" "}
              <code>{"{api_key}"}</code>（会替换为该渠道实际使用的上游
              key/token）。
            </div>
            <div className="mt-3">
              <AutoSaveIndicator
                status={headerAutosave.status}
                blockedReason={headerAutosave.blockedReason}
                error={headerAutosave.error}
                onRetry={headerAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">
          模型后缀保护名单
        </div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              suffixAutosave.flush();
            }}
          >
            <textarea
              className="form-control font-monospace"
              rows={6}
              value={modelSuffixPreserve}
              onChange={(e) => setModelSuffixPreserve(e.target.value)}
              placeholder='["o1-mini-high","gpt-5-mini-high"]'
            />
            <div className="form-text small text-muted mt-2">
              留空表示禁用。JSON 必须为数组；命中时跳过模型后缀解析（
              <code>-low/-medium/-high/-minimal/-none/-xhigh</code>）。
            </div>
            <div className="mt-3">
              <AutoSaveIndicator
                status={suffixAutosave.status}
                blockedReason={suffixAutosave.blockedReason}
                error={suffixAutosave.error}
                onRetry={suffixAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">请求体黑白名单</div>
        <div className="card-body">
          <div className="row g-3">
            <div className="col-12 col-lg-6">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  whitelistAutosave.flush();
                }}
              >
                <label className="form-label fw-medium mb-1">
                  白名单（仅保留）
                </label>
                <textarea
                  className="form-control font-monospace"
                  rows={8}
                  value={requestBodyWhitelist}
                  onChange={(e) => setRequestBodyWhitelist(e.target.value)}
                  placeholder='["model","input","max_output_tokens","metadata.channel"]'
                />
                <div className="form-text small text-muted mt-2">
                  留空表示禁用。JSON 必须为数组，每项为 JSON path（gjson/sjson
                  语法）；启用后会先“仅保留白名单字段”，再应用黑名单与参数改写。
                </div>
                <div className="mt-3">
                  <AutoSaveIndicator
                    status={whitelistAutosave.status}
                    blockedReason={whitelistAutosave.blockedReason}
                    error={whitelistAutosave.error}
                    onRetry={whitelistAutosave.retry}
                  />
                </div>
              </form>
            </div>
            <div className="col-12 col-lg-6">
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  blacklistAutosave.flush();
                }}
              >
                <label className="form-label fw-medium mb-1">
                  黑名单（删除字段）
                </label>
                <textarea
                  className="form-control font-monospace"
                  rows={8}
                  value={requestBodyBlacklist}
                  onChange={(e) => setRequestBodyBlacklist(e.target.value)}
                  placeholder='["metadata.sensitive","user","store"]'
                />
                <div className="form-text small text-muted mt-2">
                  留空表示禁用。JSON 必须为数组，每项为 JSON path（gjson/sjson
                  语法）；会在每次 selection 转发前按渠道应用。
                </div>
                <div className="mt-3">
                  <AutoSaveIndicator
                    status={blacklistAutosave.status}
                    blockedReason={blacklistAutosave.blockedReason}
                    error={blacklistAutosave.error}
                    onRetry={blacklistAutosave.retry}
                  />
                </div>
              </form>
            </div>
          </div>
        </div>
      </div>

      <div className="card border-0 shadow-sm">
        <div className="card-header bg-white fw-bold py-3">状态码映射</div>
        <div className="card-body">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              statusCodeAutosave.flush();
            }}
          >
            <textarea
              className="form-control font-monospace"
              rows={6}
              value={statusCodeMapping}
              onChange={(e) => setStatusCodeMapping(e.target.value)}
              placeholder='{"401":"200","429":"200"}'
            />
            <div className="form-text small text-muted mt-2">
              留空表示禁用。仅影响对下游返回的 HTTP 状态码，不影响内部 failover
              判定与日志/用量记录。
            </div>
            <div className="mt-3">
              <AutoSaveIndicator
                status={statusCodeAutosave.status}
                blockedReason={statusCodeAutosave.blockedReason}
                error={statusCodeAutosave.error}
                onRetry={statusCodeAutosave.retry}
              />
            </div>
          </form>
        </div>
      </div>
    </div>
  );
}

export function ChannelsPage() {
  useAuth();
  const allowCodexOAuth = true;
  const channelTableCols = 3;

  const [channels, setChannels] = useState<ChannelAdminItem[]>([]);
  const channelsRef = useRef<ChannelAdminItem[]>([]);

  const [managedModelIDs, setManagedModelIDs] = useState<string[]>([]);
  const [channelGroups, setChannelGroups] = useState<AdminChannelGroup[]>([]);
  const [loading, setLoading] = useState(true);
  const [, setErr] = useState("");
  const [, setNotice] = useState("");
  const [testingChannelID, setTestingChannelID] = useState<number | null>(null);
  const [expandedChannelID, setExpandedChannelID] = useState<number | null>(
    null,
  );
  const [testPanels, setTestPanels] = useState<
    Record<number, ChannelTestPanelState>
  >({});
  const [pointerTarget, setPointerTarget] =
    useState<ChannelPointerTarget | null>(null);
  const [pointerGroupID, setPointerGroupID] = useState("");

  const [usageStart, setUsageStart] = useState("");
  const [usageEnd, setUsageEnd] = useState("");
  const [usageAllTime, setUsageAllTime] = useState(false);
  const [usageResolvedStart, setUsageResolvedStart] = useState("");
  const [usageResolvedEnd, setUsageResolvedEnd] = useState("");
  const [usageRangeDirty, setUsageRangeDirty] = useState(false);
  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);
  const [detailSeries, setDetailSeries] = useState<ChannelTimeSeriesPoint[]>(
    [],
  );
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState("");
  const [detailPanelByChannel, setDetailPanelByChannel] = useState<
    Record<number, "stats" | "test" | "accounts">
  >({});
  const [detailField, setDetailField] = useState<
    | "committed_usd"
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
      | "tokens"
      | "cache_ratio"
      | "avg_first_token_latency"
      | "tokens_per_second";
    label: string;
  }> = [
    { value: "committed_usd", label: "消耗 (USD)" },
    { value: "tokens", label: "Token" },
    { value: "cache_ratio", label: "缓存率 (%)" },
    { value: "avg_first_token_latency", label: "首字延迟 (s)" },
    { value: "tokens_per_second", label: "Tokens/s" },
  ];
  const granularityOptions: Array<{ value: "hour" | "day"; label: string }> = [
    { value: "hour", label: "按小时" },
    { value: "day", label: "按天" },
  ];

  const [createType, setCreateType] = useState<
    "openai_compatible" | "anthropic" | "codex_oauth"
  >("openai_compatible");
  const [createName, setCreateName] = useState("");
  const [createBaseURL, setCreateBaseURL] = useState("https://api.openai.com");
  const [createKey, setCreateKey] = useState("");
  const [createGroups, setCreateGroups] = useState("");
  const [createPriority, setCreatePriority] = useState("0");
  const [createPromotion, setCreatePromotion] = useState(false);
  const [createAllowServiceTier, setCreateAllowServiceTier] = useState(false);
  const [createFastMode, setCreateFastMode] = useState(true);
  const [createDisableStore, setCreateDisableStore] = useState(false);
  const [createAllowSafetyIdentifier, setCreateAllowSafetyIdentifier] =
    useState(false);

  const [settingsChannelID, setSettingsChannelID] = useState<number | null>(
    null,
  );
  const [settingsChannelName, setSettingsChannelName] = useState("");
  const [settingsChannel, setSettingsChannel] = useState<Channel | null>(null);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsTab, setSettingsTab] = useState<
    "common" | "keys" | "models" | "advanced"
  >("common");
  const [settingsAutosaveResetKey, setSettingsAutosaveResetKey] = useState(0);

  const [editName, setEditName] = useState("");
  const [editGroups, setEditGroups] = useState("");
  const [editBaseURL, setEditBaseURL] = useState("");
  const [editStatus, setEditStatus] = useState(1);
  const [editPriority, setEditPriority] = useState("0");
  const [editPromotion, setEditPromotion] = useState(false);
  const [editAllowServiceTier, setEditAllowServiceTier] = useState(false);
  const [editFastMode, setEditFastMode] = useState(true);
  const [editDisableStore, setEditDisableStore] = useState(false);
  const [editAllowSafetyIdentifier, setEditAllowSafetyIdentifier] =
    useState(false);

  const [credentials, setCredentials] = useState<ChannelCredential[]>([]);
  const [newCredentialName, setNewCredentialName] = useState("");
  const [newCredentialKey, setNewCredentialKey] = useState("");
  const [keyValue, setKeyValue] = useState("");
  const [codexAccountsByChannel, setCodexAccountsByChannel] = useState<
    Record<number, CodexOAuthAccount[]>
  >({});
  const [codexAccountsLoadingByChannel, setCodexAccountsLoadingByChannel] =
    useState<Record<number, boolean>>({});
  const [codexAccountsErrByChannel, setCodexAccountsErrByChannel] = useState<
    Record<number, string>
  >({});
  const [codexCallbackURL, setCodexCallbackURL] = useState("");
  const [codexManualAccountID, setCodexManualAccountID] = useState("");
  const [codexManualEmail, setCodexManualEmail] = useState("");
  const [codexManualAccessToken, setCodexManualAccessToken] = useState("");
  const [codexManualRefreshToken, setCodexManualRefreshToken] = useState("");
  const [codexManualIDToken, setCodexManualIDToken] = useState("");
  const [codexManualExpiresAt, setCodexManualExpiresAt] = useState("");

  const [bindings, setBindings] = useState<ChannelModelBinding[]>([]);
  const [selectedModelIDs, setSelectedModelIDs] = useState<string[]>([]);
  const [modelRedirects, setModelRedirects] = useState<Record<string, string>>(
    {},
  );
  const [modelSearch, setModelSearch] = useState("");

  const [metaOpenAIOrganization, setMetaOpenAIOrganization] = useState("");
  const [metaTestModel, setMetaTestModel] = useState("");
  const [metaTag, setMetaTag] = useState("");
  const [metaWeight, setMetaWeight] = useState("0");
  const [metaAutoBan, setMetaAutoBan] = useState(true);
  const [metaRemark, setMetaRemark] = useState("");

  const [settingThinkingToContent, setSettingThinkingToContent] =
    useState(false);
  const [settingPassThroughBodyEnabled, setSettingPassThroughBodyEnabled] =
    useState(false);
  const [settingProxy, setSettingProxy] = useState("");
  const [settingSystemPrompt, setSettingSystemPrompt] = useState("");
  const [settingSystemPromptOverride, setSettingSystemPromptOverride] =
    useState(false);

  const [paramOverride, setParamOverride] = useState("");
  const [headerOverride, setHeaderOverride] = useState("");
  const [modelSuffixPreserve, setModelSuffixPreserve] = useState("");
  const [requestBodyWhitelist, setRequestBodyWhitelist] = useState("");
  const [requestBodyBlacklist, setRequestBodyBlacklist] = useState("");
  const [statusCodeMapping, setStatusCodeMapping] = useState("");
  const oauthQueryHandled = useRef(false);

  const applyChannelPatch = useCallback(
    (id: number, patch: ChannelPatch) => {
      if (patch.name !== undefined) setSettingsChannelName(patch.name);
      setSettingsChannel((prev) =>
        prev && prev.id === id ? ({ ...prev, ...patch } as Channel) : prev,
      );
      setChannels((prev) =>
        prev.map((c) =>
          c.id === id ? ({ ...c, ...patch } as ChannelAdminItem) : c,
        ),
      );
    },
    [setChannels, setSettingsChannel, setSettingsChannelName],
  );

  const enabledCount = useMemo(
    () => channels.filter((c) => c.status === 1).length,
    [channels],
  );
  const disabledCount = useMemo(
    () => channels.length - enabledCount,
    [channels.length, enabledCount],
  );
  const firstDisabledIndex = useMemo(
    () => channels.findIndex((c) => c.status !== 1),
    [channels],
  );
  const selectableModelIDs = useMemo(() => {
    const uniq = new Set<string>();
    for (const id of managedModelIDs) {
      const v = id.trim();
      if (v) uniq.add(v);
    }
    const out = Array.from(uniq);
    out.sort((a, b) => a.localeCompare(b, "zh-CN"));
    return out;
  }, [managedModelIDs]);

  const filteredModelIDs = useMemo(() => {
    const q = modelSearch.trim().toLowerCase();
    if (!q) return selectableModelIDs;
    return selectableModelIDs.filter((id) => id.toLowerCase().includes(q));
  }, [selectableModelIDs, modelSearch]);

  const selectedModelSet = useMemo(
    () => new Set(selectedModelIDs),
    [selectedModelIDs],
  );
  const managedModelIDSet = useMemo(
    () => new Set(managedModelIDs.map((id) => id.trim()).filter((id) => id !== "")),
    [managedModelIDs],
  );
  const bindingByPublicID = useMemo(() => {
    const m = new Map<string, ChannelModelBinding>();
    for (const b of bindings) m.set(b.public_id, b);
    return m;
  }, [bindings]);
  const staleEnabledBindings = useMemo(
    () =>
      bindings
        .filter((b) => b.status === 1)
        .filter((b) => !managedModelIDSet.has(b.public_id.trim())),
    [bindings, managedModelIDSet],
  );

  function normalizeChannelSections(
    list: ChannelAdminItem[],
  ): ChannelAdminItem[] {
    const enabled = list.filter((ch) => ch.status === 1);
    const disabled = list.filter((ch) => ch.status !== 1);
    return [...enabled, ...disabled];
  }

  function fmtDateTime(iso?: string | null): string {
    if (!iso) return "-";
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return "-";
    return d.toLocaleString("zh-CN", { hour12: false });
  }

  function clearTestPanel(channelID: number) {
    setTestPanels((prev) => {
      const current = prev[channelID];
      if (!current) return prev;
      const next = { ...prev };
      delete next[channelID];
      return next;
    });
  }

  function openChannelPanel(channelID: number) {
    if (expandedChannelID !== null && expandedChannelID !== channelID) {
      clearTestPanel(expandedChannelID);
    }
    setExpandedChannelID(channelID);
  }

  function toggleChannelPanel(channelID: number) {
    if (expandedChannelID === channelID) {
      clearTestPanel(channelID);
      setExpandedChannelID(null);
      return;
    }
    openChannelPanel(channelID);
  }

  const refresh = useCallback(
    async (params?: { start?: string; end?: string; all_time?: boolean }) => {
      setErr("");
      setNotice("");
      setLoading(true);
      try {
        const startValue = (params?.start ?? "").trim();
        const endValue = (params?.end ?? "").trim();
        const allTimeActive = !!params?.all_time;
        const pageParams = allTimeActive
          ? { all_time: true }
          : { start: startValue || undefined, end: endValue || undefined };

        const [pageRes, modelsRes] = await Promise.all([
          getChannelsPage(pageParams),
          listSelectableManagedModelIDsAdmin(),
        ]);
        if (!modelsRes.success)
          throw new Error(modelsRes.message || "加载模型失败");
        setManagedModelIDs(
          (modelsRes.data || [])
            .filter((id) => typeof id === "string" && id.trim() !== ""),
        );

        if (!pageRes.success)
          throw new Error(pageRes.message || "加载渠道失败");
        const pageChannels = pageRes.data?.channels || [];
        const normalizedChannels = normalizeChannelSections(
          pageChannels,
        ).filter((ch) => (allowCodexOAuth ? true : ch.type !== "codex_oauth"));
        channelsRef.current = normalizedChannels;
        setUsageResolvedStart(pageRes.data?.start || "");
        setUsageResolvedEnd(pageRes.data?.end || "");
        if (!allTimeActive) {
          setUsageStart(pageRes.data?.start || "");
          setUsageEnd(pageRes.data?.end || "");
        }
        setChannels(normalizedChannels);
      } catch (e) {
        setErr(e instanceof Error ? e.message : "加载失败");
      } finally {
        setLoading(false);
      }
    },
    [allowCodexOAuth],
  );

  const refreshWithCurrentRange = useCallback(async () => {
    const startValue = usageStart.trim();
    const endValue = usageEnd.trim();
    const allTimeValue = usageAllTime;
    await refresh({ start: startValue, end: endValue, all_time: allTimeValue });
  }, [refresh, usageAllTime, usageEnd, usageStart]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!usageRangeDirty) return;
    const t = window.setTimeout(() => {
      setUsageRangeDirty(false);
      void refreshWithCurrentRange();
    }, 400);
    return () => window.clearTimeout(t);
  }, [usageRangeDirty, refreshWithCurrentRange]);

  useEffect(() => {
    if (!expandedChannelID) {
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
        const allTimeActive =
          usageAllTime && !usageStart.trim() && !usageEnd.trim();
        const res = await getChannelTimeSeries(expandedChannelID, {
          start: allTimeActive ? undefined : usageStart.trim() || undefined,
          end: allTimeActive ? undefined : usageEnd.trim() || undefined,
          all_time: allTimeActive ? true : undefined,
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
  }, [
    expandedChannelID,
    usageAllTime,
    usageStart,
    usageEnd,
    detailGranularity,
  ]);

  useEffect(() => {
    void (async () => {
      try {
        const res = await listAdminChannelGroups();
        if (res.success) setChannelGroups(res.data || []);
      } catch {
        // ignore
      }
    })();
  }, []);

  const defaultGroupName = useMemo(() => {
    const byDefault = (
      channelGroups.find((g) => g.is_default)?.name || ""
    ).trim();
    if (byDefault) return byDefault;
    const byFirst = (channelGroups[0]?.name || "").trim();
    if (byFirst) return byFirst;
    return "default";
  }, [channelGroups]);

  const channelGroupByName = useMemo(() => {
    const m = new Map<string, AdminChannelGroup>();
    for (const g of channelGroups) {
      const name = (g.name || "").trim();
      if (!name) continue;
      if (m.has(name)) continue;
      m.set(name, g);
    }
    return m;
  }, [channelGroups]);

  const pointerGroupOptions = useMemo(() => {
    if (!pointerTarget) return [];
    const names = parseGroupsCSV(pointerTarget.groups || "");
    const out: AdminChannelGroup[] = [];
    for (const name of names) {
      const g = channelGroupByName.get(name);
      if (!g || g.status !== 1) continue;
      out.push(g);
    }
    return out;
  }, [pointerTarget, channelGroupByName]);

  async function reloadCredentials(channelID: number) {
    const res = await listChannelCredentials(channelID);
    if (!res.success) throw new Error(res.message || "加载密钥失败");
    setCredentials(res.data || []);
  }

  async function reloadCodexAccounts(channelID: number) {
    const res = await listChannelCodexAccounts(channelID);
    if (!res.success) throw new Error(res.message || "加载账号失败");
    const accounts = res.data || [];
    setCodexAccountsByChannel((prev) => ({ ...prev, [channelID]: accounts }));
    setCodexAccountsErrByChannel((prev) => ({ ...prev, [channelID]: "" }));
  }

  const loadCodexAccountsForChannel = useCallback(
    async (channelID: number, force = false) => {
      if (
        !force &&
        Object.prototype.hasOwnProperty.call(codexAccountsByChannel, channelID)
      )
        return;
      setCodexAccountsLoadingByChannel((prev) => ({
        ...prev,
        [channelID]: true,
      }));
      setCodexAccountsErrByChannel((prev) => ({ ...prev, [channelID]: "" }));
      try {
        const res = await listChannelCodexAccounts(channelID);
        if (!res.success) throw new Error(res.message || "加载账号失败");
        const accounts = res.data || [];
        setCodexAccountsByChannel((prev) => ({
          ...prev,
          [channelID]: accounts,
        }));
      } catch (e) {
        setCodexAccountsErrByChannel((prev) => ({
          ...prev,
          [channelID]: e instanceof Error ? e.message : "加载账号失败",
        }));
      } finally {
        setCodexAccountsLoadingByChannel((prev) => ({
          ...prev,
          [channelID]: false,
        }));
      }
    },
    [codexAccountsByChannel],
  );

  useEffect(() => {
    if (!expandedChannelID) return;
    if (!allowCodexOAuth) return;
    const target = channels.find((item) => item.id === expandedChannelID);
    if (!target || target.type !== "codex_oauth") return;
    const panel = detailPanelByChannel[expandedChannelID] || "stats";
    if (panel !== "accounts") return;
    void loadCodexAccountsForChannel(expandedChannelID);
  }, [
    allowCodexOAuth,
    channels,
    detailPanelByChannel,
    expandedChannelID,
    loadCodexAccountsForChannel,
  ]);

  const openChannelSettingsModal = useCallback(
    (
      ch: { id: number; name?: string },
      tab: "common" | "keys" | "models" | "advanced" = "common",
    ) => {
      setSettingsTab(tab);
      setSettingsChannelID(ch.id);
      setSettingsChannelName(ch.name || `#${ch.id}`);
      setSettingsChannel(null);

      if (typeof window === "undefined") return;
      const modalRoot = document.getElementById("editChannelModal");
      const modalCtor = (
        window as Window & {
          bootstrap?: {
            Modal?: {
              getOrCreateInstance: (el: Element) => { show: () => void };
            };
          };
        }
      ).bootstrap?.Modal;
      if (!modalRoot || !modalCtor?.getOrCreateInstance) return;
      modalCtor.getOrCreateInstance(modalRoot).show();
    },
    [],
  );

  useEffect(() => {
    if (!allowCodexOAuth) return;
    if (oauthQueryHandled.current || loading) return;
    if (typeof window === "undefined") return;
    oauthQueryHandled.current = true;

    const params = new URLSearchParams(window.location.search);
    const openChannelSettings = Number.parseInt(
      params.get("open_channel_settings") || "",
      10,
    );
    const oauthState = (params.get("oauth") || "").trim();
    const oauthErr = (params.get("err") || "").trim();

    if (oauthState === "ok") setNotice("Codex OAuth 授权成功");
    if (oauthState === "error") setErr(oauthErr || "Codex OAuth 授权失败");

    if (openChannelSettings > 0) {
      const target = channels.find((ch) => ch.id === openChannelSettings);
      if (target) openChannelSettingsModal(target, "keys");
    }

    if (openChannelSettings > 0 || oauthState !== "" || oauthErr !== "") {
      params.delete("open_channel_settings");
      params.delete("oauth");
      params.delete("err");
      const nextQuery = params.toString();
      const nextURL = `${window.location.pathname}${nextQuery ? `?${nextQuery}` : ""}${window.location.hash || ""}`;
      window.history.replaceState({}, "", nextURL);
    }
  }, [allowCodexOAuth, channels, loading, openChannelSettingsModal]);

  useEffect(() => {
    if (!allowCodexOAuth) return;
    if (typeof window === "undefined") return;
    const onMessage = (event: MessageEvent) => {
      const payload = event.data as {
        type?: string;
        redirectURL?: string;
      } | null;
      if (!payload || payload.type !== "realms_codex_oauth_callback") return;
      let openChannelSettings = 0;
      let oauthState = "";
      if (payload.redirectURL) {
        try {
          const parsed = new URL(payload.redirectURL, window.location.origin);
          openChannelSettings = Number.parseInt(
            parsed.searchParams.get("open_channel_settings") || "",
            10,
          );
          oauthState = (parsed.searchParams.get("oauth") || "").trim();
        } catch {
          // ignore
        }
      }
      if (oauthState === "ok") setNotice("Codex OAuth 授权成功");
      if (oauthState === "error") setErr("Codex OAuth 授权失败");

      if (openChannelSettings > 0) {
        const target = channels.find((ch) => ch.id === openChannelSettings);
        if (target) openChannelSettingsModal(target, "keys");
      } else if (settingsChannelID && settingsChannel?.type === "codex_oauth") {
        void reloadCodexAccounts(settingsChannelID).catch(() => {});
      }
    };
    window.addEventListener("message", onMessage);
    return () => {
      window.removeEventListener("message", onMessage);
    };
  }, [
    allowCodexOAuth,
    channels,
    openChannelSettingsModal,
    settingsChannel,
    settingsChannelID,
  ]);

  const applyChannelModelBindings = useCallback(
    (items: ChannelModelBinding[]) => {
      setBindings(items);

      const selected = items
        .filter((b) => b.status === 1)
        .map((b) => b.public_id)
        .filter((id) => managedModelIDSet.has(id.trim()))
        .filter((id) => id.trim() !== "");
      selected.sort((a, b) => a.localeCompare(b, "zh-CN"));
      setSelectedModelIDs(selected);

      const redirects: Record<string, string> = {};
      for (const b of items) {
        if (b.status !== 1) continue;
        if (b.public_id.trim() === "") continue;
        if (b.upstream_model.trim() === "") continue;
        if (b.upstream_model === b.public_id) continue;
        redirects[b.public_id] = b.upstream_model;
      }
      setModelRedirects(redirects);
    },
    [managedModelIDSet],
  );

  async function reloadBindings(channelID: number) {
    const res = await listChannelModels(channelID);
    if (!res.success) throw new Error(res.message || "加载绑定失败");
    applyChannelModelBindings(res.data || []);
  }

  const loadChannelSettings = useCallback(
    async (channelID: number) => {
      setErr("");
      setNotice("");
      setSettingsLoading(true);
      try {
        const [chRes, credsRes, bindingsRes] = await Promise.all([
          getChannel(channelID),
          listChannelCredentials(channelID),
          listChannelModels(channelID),
        ]);
        if (!chRes.success) throw new Error(chRes.message || "加载渠道失败");
        const ch = chRes.data;
        if (!ch) throw new Error("渠道不存在");
        setSettingsChannel(ch);

        setEditName(ch.name || "");
        setEditGroups(ch.groups || "");
        setEditBaseURL(ch.base_url || "");
        setEditStatus(ch.status || 0);
        setEditPriority(String(ch.priority || 0));
        setEditPromotion(!!ch.promotion);
        setEditAllowServiceTier(
          ch.fast_mode !== false ? true : ch.allow_service_tier !== false,
        );
        setEditFastMode(ch.fast_mode !== false);
        setEditDisableStore(!!ch.disable_store);
        setEditAllowSafetyIdentifier(!!ch.allow_safety_identifier);

        if (!credsRes.success)
          throw new Error(credsRes.message || "加载密钥失败");
        setCredentials(credsRes.data || []);
        setNewCredentialName("");
        setNewCredentialKey("");
        setKeyValue("");
        setCodexCallbackURL("");
        setCodexManualAccountID("");
        setCodexManualEmail("");
        setCodexManualAccessToken("");
        setCodexManualRefreshToken("");
        setCodexManualIDToken("");
        setCodexManualExpiresAt("");
        if (ch.type === "codex_oauth") {
          await reloadCodexAccounts(channelID);
        } else {
          setCodexAccountsByChannel((prev) => {
            if (!Object.prototype.hasOwnProperty.call(prev, channelID))
              return prev;
            const next = { ...prev };
            delete next[channelID];
            return next;
          });
        }

        if (!bindingsRes.success)
          throw new Error(bindingsRes.message || "加载绑定失败");
        applyChannelModelBindings(bindingsRes.data || []);

        setMetaOpenAIOrganization(ch.openai_organization || "");
        setMetaTestModel(ch.test_model || "");
        setMetaTag(ch.tag || "");
        setMetaWeight(String(ch.weight || 0));
        setMetaAutoBan(ch.auto_ban ?? true);
        setMetaRemark(ch.remark || "");

        const setting = ch.setting || {};
        setSettingThinkingToContent(!!setting.thinking_to_content);
        setSettingPassThroughBodyEnabled(!!setting.pass_through_body_enabled);
        setSettingProxy(setting.proxy || "");
        setSettingSystemPrompt(setting.system_prompt || "");
        setSettingSystemPromptOverride(!!setting.system_prompt_override);

        setParamOverride(ch.param_override || "");
        setHeaderOverride(ch.header_override || "");
        setModelSuffixPreserve(ch.model_suffix_preserve || "");
        setRequestBodyWhitelist(ch.request_body_whitelist || "");
        setRequestBodyBlacklist(ch.request_body_blacklist || "");
        setStatusCodeMapping(ch.status_code_mapping || "");

        setModelSearch("");
        setSettingsAutosaveResetKey((x) => x + 1);
      } catch (e) {
        setErr(e instanceof Error ? e.message : "加载失败");
        setSettingsChannel(null);
        setCredentials([]);
        setBindings([]);
        setSelectedModelIDs([]);
        setModelRedirects({});
      } finally {
        setSettingsLoading(false);
      }
    },
    [applyChannelModelBindings],
  );

  useEffect(() => {
    if (!settingsChannelID) return;
    void loadChannelSettings(settingsChannelID);
  }, [settingsChannelID, loadChannelSettings]);

  async function saveModelsConfigOrThrow() {
    if (!settingsChannelID) throw new Error("未选择渠道");
    const selected = selectedModelIDs
      .map((m) => m.trim())
      .filter((m) => m !== "");
    const selectedSet = new Set<string>(selected);

    const bindingByPublicID = new Map<string, ChannelModelBinding>();
    for (const b of bindings) bindingByPublicID.set(b.public_id, b);

    for (const b of bindings) {
      const enabled = selectedSet.has(b.public_id);
      const desiredStatus = enabled ? 1 : 0;
      const redirected = (modelRedirects[b.public_id] || "").trim();
      const desiredUpstreamModel = enabled
        ? redirected || b.public_id
        : b.upstream_model;

      if (
        b.status === desiredStatus &&
        (!enabled || b.upstream_model === desiredUpstreamModel)
      )
        continue;
      const res = await updateChannelModel(settingsChannelID, {
        id: b.id,
        public_id: b.public_id,
        upstream_model: desiredUpstreamModel.trim() || b.public_id,
        status: desiredStatus,
      });
      if (!res.success) throw new Error(res.message || "保存失败");
    }

    for (const publicID of selected) {
      if (bindingByPublicID.has(publicID)) continue;
      const redirected = (modelRedirects[publicID] || "").trim();
      const upstreamModel = redirected || publicID;
      const res = await createChannelModel(
        settingsChannelID,
        publicID,
        upstreamModel,
        1,
      );
      if (!res.success) throw new Error(res.message || "创建失败");
    }

    await reloadBindings(settingsChannelID);
  }

  const modelsAutosave = useAutoSave({
    enabled:
      !!settingsChannelID &&
      !!settingsChannel &&
      !settingsLoading &&
      settingsTab === "models",
    resetKey: settingsAutosaveResetKey,
    debounceMs: 1200,
    value: { selectedModelIDs, modelRedirects },
    validate: () => {
      if (!selectedModelIDs.length && staleEnabledBindings.length === 0) {
        return "请先在上方选择模型";
      }
      return "";
    },
    save: async () => {
      await saveModelsConfigOrThrow();
    },
  });

  useEffect(() => {
    const ChartCtor = (window as unknown as { Chart?: ChartConstructor }).Chart;

    const destroy = (ref: MutableRefObject<ChartInstance | null>) => {
      try {
        ref.current?.destroy?.();
      } catch {
        // ignore
      }
      ref.current = null;
    };

    destroy(detailTimeLineChartRef);

    if (!ChartCtor || !expandedChannelID) return;
    const channel = channels.find((c) => c.id === expandedChannelID);
    if (!channel) return;
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
        read: (p: ChannelTimeSeriesPoint) => number;
      }
    > = {
      committed_usd: {
        label: "消耗 (USD)",
        color: color(palette.primary, 0.95),
        read: (p) => p.committed_usd,
      },
      tokens: {
        label: "Token",
        color: color(palette.success, 0.95),
        read: (p) => p.tokens,
      },
      cache_ratio: {
        label: "缓存率 (%)",
        color: color(palette.warning, 0.95),
        read: (p) => p.cache_ratio,
      },
      avg_first_token_latency: {
        label: "首字延迟 (s)",
        color: color(palette.danger, 0.95),
        read: (p) => p.avg_first_token_latency / 1000,
      },
      tokens_per_second: {
        label: "Tokens/s",
        color: color(palette.secondary, 0.95),
        read: (p) => p.tokens_per_second,
      },
    };
    const meta = fieldMeta[detailField];
    const datasets = [
      {
        label: meta.label,
        data: detailSeries.map((p) => meta.read(p)),
        borderColor: meta.color,
        backgroundColor: meta.color.replace("0.95", "0.18"),
        pointRadius: 2,
        tension: 0.2,
      },
    ];

    detailTimeLineChartRef.current = new ChartCtor(ctx, {
      type: "line",
      data: {
        labels: detailSeries.map((p) => p.bucket),
        datasets,
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: "index", intersect: false },
        plugins: {
          legend: { position: "bottom" },
          title: {
            display: true,
            text: `${channel.name || `渠道 #${channel.id}`} · 时间序列`,
          },
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
            ...(detailField === "tokens"
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
  }, [
    channels,
    expandedChannelID,
    detailSeries,
    detailField,
    detailGranularity,
  ]);

  return (
    <div className="fade-in-up">
      <SegmentedFrame>
        <div className="d-flex justify-content-between align-items-start flex-wrap gap-3">
          <div>
            <h2 className="h4 fw-bold mb-1">上游渠道管理</h2>
            <p className="text-muted small mb-0">
              管理模型转发渠道。当前 {formatIntComma(enabledCount)} 启用 /{" "}
              {formatIntComma(disabledCount)} 禁用 /{" "}
              {formatIntComma(channels.length)} 总计。
            </p>
          </div>
          <button
            type="button"
            className="btn btn-primary"
            data-bs-toggle="modal"
            data-bs-target="#createChannelModal"
          >
            <i className="ri-add-line me-1"></i> 新建渠道
          </button>
        </div>

        <div
          className="d-flex flex-wrap align-items-center gap-2 mb-0 bg-white p-2 rounded-3 border-light shadow-sm mt-3"
          style={{ border: "1px solid #f1f3f5" }}
        >
          <div className="d-flex align-items-center px-2">
            <span
              className="small text-muted me-2"
              style={{ whiteSpace: "nowrap", fontSize: "12px" }}
            >
              统计区间
            </span>
            <DateRangePicker
              start={usageStart}
              end={usageEnd}
              onChange={(r) => {
                const isAll = !r.start.trim() && !r.end.trim();
                setUsageAllTime(isAll);
                if (isAll) setDetailGranularity("day");
                setUsageStart(r.start);
                setUsageEnd(r.end);
                setUsageRangeDirty(true);
              }}
              loading={loading}
            />
          </div>

          <div className="ms-auto d-flex gap-2 pe-1">
            <button
              className="btn btn-sm"
              style={{
                backgroundColor: "#326c52",
                color: "#ffffff",
                fontWeight: 500,
                height: "28px",
                fontSize: "12px",
                display: "flex",
                alignItems: "center",
                borderRadius: "4px",
                padding: "0 12px",
                transition: "all 0.2s",
                border: "none",
              }}
              type="button"
              disabled={loading}
              onClick={() => {
                void refreshWithCurrentRange();
              }}
            >
              <span
                className="material-symbols-rounded me-1"
                style={{ fontSize: "16px" }}
              >
                refresh
              </span>
              刷新数据
            </button>
            <button
              className="btn btn-sm"
              style={{
                height: "28px",
                fontSize: "12px",
                border: "1px solid #e9ecef",
                borderRadius: "4px",
                backgroundColor: "#ffffff",
                color: "#6c757d",
                padding: "0 12px",
                display: "flex",
                alignItems: "center",
                transition: "all 0.2s",
              }}
              type="button"
              disabled={loading}
              onClick={() => {
                setUsageAllTime(false);
                setUsageStart("");
                setUsageEnd("");
                setUsageRangeDirty(true);
              }}
            >
              重置
            </button>
          </div>
        </div>

        <div>
          <div className="card border-0 shadow-sm overflow-hidden mb-0">
            <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
              <div>
                <span className="text-primary fw-bold text-uppercase small">
                  渠道列表
                </span>
              </div>
            </div>
            <div className="table-responsive">
              <table className="table table-hover align-middle mb-0">
                <thead className="table-light">
                  <tr>
                    <th className="ps-4">渠道详情</th>
                    <th>状态</th>
                    <th className="text-end pe-4">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {loading ? (
                    <tr>
                      <td
                        colSpan={channelTableCols}
                        className="text-center py-5 text-muted"
                      >
                        加载中…
                      </td>
                    </tr>
                  ) : channels.length === 0 ? (
                    <tr>
                      <td
                        colSpan={channelTableCols}
                        className="text-center py-5 text-muted"
                      >
                        <span className="fs-1 d-block mb-3 material-symbols-rounded">
                          inbox
                        </span>
                        暂无渠道。
                      </td>
                    </tr>
                  ) : (
                    <>
                      {channels.map((ch, idx) => {
                        const st = statusBadge(ch.status);
                        const channelDisabled = ch.status !== 1;
                        const runtime = ch.runtime;
                        const usage = ch.usage;
                        const testPanel = testPanels[ch.id];
                        const panelOpen = expandedChannelID === ch.id;
                        const testRunning = testingChannelID === ch.id;
                        const anyTesting = testingChannelID !== null;
                        const activeTestPanel =
                          testPanel &&
                          (testPanel.running ||
                            testPanel.error.trim() !== "" ||
                            testPanel.models.length > 0)
                            ? testPanel
                            : null;
                        const detailPanel =
                          detailPanelByChannel[ch.id] ||
                          (activeTestPanel ? "test" : "stats");
                        const codexPanelAccounts =
                          codexAccountsByChannel[ch.id] || [];
                        const codexPanelLoading =
                          !!codexAccountsLoadingByChannel[ch.id];
                        const codexPanelErr =
                          codexAccountsErrByChannel[ch.id] || "";
                        const rowBaseClassName = [
                          "rlm-channel-row-main",
                          channelDisabled ? "table-secondary opacity-75" : "",
                        ]
                          .filter((v) => v)
                          .join(" ");
                        const groupNames = parseGroupsCSV(ch.groups || "");
                        const pointerGroups = groupNames
                          .map((name) => channelGroupByName.get(name))
                          .filter(
                            (g): g is AdminChannelGroup =>
                              !!g && g.status === 1,
                          );
                        const canSetPointer =
                          !channelDisabled && pointerGroups.length > 0;
                        const setPointerTitle = channelDisabled
                          ? "禁用渠道不可设为指针"
                          : pointerGroups.length === 0
                            ? "该渠道未加入任何启用的渠道组"
                            : "设为指针";

                        const renderMainRowCells = () => (
                          <>
                            <td className="ps-4" style={{ minWidth: 0 }}>
                              <div className="d-flex flex-column">
                                <div className="d-flex flex-wrap align-items-center gap-2">
                                  <span className="fw-bold text-dark">
                                    {ch.name || `渠道 #${ch.id}`}
                                  </span>
                                  <span className="text-muted small">
                                    ({channelTypeLabel(ch.type)})
                                  </span>
                                  {ch.in_use ? (
                                    <span className="badge bg-info bg-opacity-10 text-info border border-info-subtle">
                                      使用中
                                    </span>
                                  ) : null}
                                  {ch.promotion ? (
                                    <span className="small text-warning fw-medium">
                                      <i className="ri-fire-line me-1"></i>优先
                                    </span>
                                  ) : null}
                                </div>
                                <div className="d-flex flex-wrap align-items-center gap-2 small text-muted mt-1">
                                  {ch.base_url ? (
                                    <span
                                      className="font-monospace d-inline-block user-select-all"
                                      style={{
                                        maxWidth: 360,
                                        whiteSpace: "nowrap",
                                        overflow: "hidden",
                                        textOverflow: "ellipsis",
                                      }}
                                      title={ch.base_url}
                                    >
                                      {ch.base_url}
                                    </span>
                                  ) : null}
                                  <div className="d-flex align-items-center">
                                    {ch.base_url ? (
                                      <span className="text-secondary">·</span>
                                    ) : null}
                                    <span
                                      className={`${ch.base_url ? "ms-2 " : ""}me-1`}
                                    >
                                      渠道组:
                                    </span>
                                    <span className="text-secondary font-monospace user-select-all">
                                      {(ch.groups || "").trim() || "-"}
                                    </span>
                                  </div>
                                </div>
                              </div>
                            </td>
                            <td>
                              <span className={st.cls}>{st.label}</span>
                              {runtime?.available && runtime.banned_active ? (
                                <div className="mt-1">
                                  <span
                                    className="badge bg-warning-subtle text-warning-emphasis border px-2"
                                    title={
                                      runtime.banned_until
                                        ? `封禁至 ${runtime.banned_until}`
                                        : undefined
                                    }
                                  >
                                    <i className="ri-forbid-2-line me-1"></i>
                                    封禁中 · 剩余{" "}
                                    {runtime.banned_remaining || "-"}
                                  </span>
                                </div>
                              ) : null}
                              {runtime?.available && runtime.fail_score > 0 ? (
                                <div className="mt-1">
                                  <span
                                    className="badge bg-light text-secondary border"
                                    title="失败计分（运行态 fail score，越高越容易触发封禁/探测）"
                                  >
                                    失败计分：{runtime.fail_score}
                                  </span>
                                </div>
                              ) : null}
                            </td>
                            <td className="text-end pe-4 text-nowrap">
                              <div className="d-flex gap-1 justify-content-end">
                                <button
                                  className="btn btn-sm btn-light border text-primary"
                                  type="button"
                                  title="测试连接"
                                  disabled={loading || anyTesting}
                                  onClick={async () => {
                                    openChannelPanel(ch.id);
                                    setDetailPanelByChannel((prev) => ({
                                      ...prev,
                                      [ch.id]: "test",
                                    }));
                                    setErr("");
                                    setNotice("");
                                    setTestingChannelID(ch.id);
                                    setTestPanels((prev) => ({
                                      ...prev,
                                      [ch.id]: {
                                        running: true,
                                        source: "",
                                        total: 0,
                                        done: 0,
                                        models: [],
                                        error: "",
                                      },
                                    }));
                                    try {
                                      const res = await testChannel(ch.id);
                                      const probe = res.data?.probe;
                                      const finalModels =
                                        probe?.results?.map((item) => ({
                                          model: item.model,
                                          status: item.ok
                                            ? ("success" as const)
                                            : ("failed" as const),
                                          message: (
                                            compactProbeMessage(
                                              item.message || "",
                                            ) ||
                                            item.message ||
                                            ""
                                          ).toString(),
                                          output: item.sample || "",
                                          result: item,
                                        })) || [];
                                      setTestPanels((prev) => ({
                                        ...prev,
                                        [ch.id]: {
                                          running: false,
                                          source: probe?.source || "",
                                          total: probe?.total ?? finalModels.length,
                                          done: probe?.total ?? finalModels.length,
                                          models: finalModels,
                                          error: "",
                                        },
                                      }));
                                      if (!res.success)
                                        throw new Error(
                                          res.message || "测试失败",
                                        );
                                      setNotice(res.message || "测试成功");
                                      await refreshWithCurrentRange();
                                    } catch (e) {
                                      const msg =
                                        e instanceof Error
                                          ? e.message
                                          : "测试失败";
                                      setErr(msg.toString().trim());
                                      setTestPanels((prev) => {
                                        const current = prev[ch.id];
                                        if (!current) return prev;
                                        return {
                                          ...prev,
                                          [ch.id]: {
                                            ...current,
                                            running: false,
                                            error: msg.toString().trim(),
                                          },
                                        };
                                      });
                                    } finally {
                                      setTestingChannelID((prev) =>
                                        prev === ch.id ? null : prev,
                                      );
                                      setTestPanels((prev) => {
                                        const current = prev[ch.id];
                                        if (!current) return prev;
                                        return {
                                          ...prev,
                                          [ch.id]: {
                                            ...current,
                                            running: false,
                                          },
                                        };
                                      });
                                    }
                                  }}
                                >
                                  {testRunning ? (
                                    <span
                                      className="spinner-border spinner-border-sm me-1"
                                      role="status"
                                      aria-hidden="true"
                                    ></span>
                                  ) : (
                                    <i className="ri-flashlight-line me-1"></i>
                                  )}
                                  {testRunning ? "测试中" : "测试"}
                                </button>

                                <button
                                  className={`btn btn-sm ${ch.status === 1 ? "btn-light border text-warning" : "btn-light border text-success"}`}
                                  type="button"
                                  title={
                                    ch.status === 1 ? "禁用渠道" : "启用渠道"
                                  }
                                  disabled={loading}
                                  onClick={async () => {
                                    const targetStatus =
                                      ch.status === 1 ? 0 : 1;
                                    setErr("");
                                    setNotice("");
                                    try {
                                      const res = await updateChannel({
                                        id: ch.id,
                                        status: targetStatus,
                                      });
                                      if (!res.success)
                                        throw new Error(
                                          res.message || "更新状态失败",
                                        );
                                      if (settingsChannelID === ch.id) {
                                        setEditStatus(targetStatus);
                                      }
                                      setNotice(
                                        targetStatus === 1
                                          ? "渠道已启用"
                                          : "渠道已禁用",
                                      );
                                      await refreshWithCurrentRange();
                                    } catch (e) {
                                      setErr(
                                        e instanceof Error
                                          ? e.message
                                          : "更新状态失败",
                                      );
                                    }
                                  }}
                                >
                                  <i
                                    className={`me-1 ${ch.status === 1 ? "ri-pause-circle-line" : "ri-play-circle-line"}`}
                                  ></i>
                                  {ch.status === 1 ? "禁用" : "启用"}
                                </button>

                                <button
                                  className="btn btn-sm btn-light border text-warning"
                                  type="button"
                                  title={setPointerTitle}
                                  disabled={loading || !canSetPointer}
                                  data-bs-toggle="modal"
                                  data-bs-target="#setChannelPointerModal"
                                  onClick={() => {
                                    if (!canSetPointer) return;
                                    setPointerTarget({
                                      id: ch.id,
                                      name: ch.name || `渠道 #${ch.id}`,
                                      groups: ch.groups || "",
                                    });
                                    setPointerGroupID(
                                      String(pointerGroups[0].id),
                                    );
                                  }}
                                >
                                  <i className="ri-pushpin-2-line me-1"></i>指针
                                </button>

                                <button
                                  className="btn btn-sm btn-primary"
                                  type="button"
                                  title="设置"
                                  disabled={loading}
                                  onClick={() =>
                                    openChannelSettingsModal(
                                      { id: ch.id, name: ch.name },
                                      "common",
                                    )
                                  }
                                >
                                  <i className="ri-settings-3-line me-1"></i>
                                  设置
                                </button>

                                <button
                                  className="btn btn-sm btn-light border text-danger"
                                  type="button"
                                  title="删除"
                                  disabled={loading}
                                  onClick={async () => {
                                    if (
                                      !window.confirm(
                                        `确认删除渠道 ${ch.name || ch.id} ? 此操作不可恢复。`,
                                      )
                                    )
                                      return;
                                    setErr("");
                                    setNotice("");
                                    try {
                                      const res = await deleteChannel(ch.id);
                                      if (!res.success)
                                        throw new Error(
                                          res.message || "删除失败",
                                        );
                                      setNotice("已删除");
                                      await refreshWithCurrentRange();
                                    } catch (e) {
                                      setErr(
                                        e instanceof Error
                                          ? e.message
                                          : "删除失败",
                                      );
                                    }
                                  }}
                                >
                                  <i className="ri-delete-bin-line me-1"></i>
                                  删除
                                </button>
                              </div>
                            </td>
                          </>
                        );

                        return (
                          <Fragment key={ch.id}>
                            {idx === firstDisabledIndex ? (
                              <tr className="table-light">
                                <td
                                  colSpan={channelTableCols}
                                  className="px-4 py-2"
                                >
                                  <span className="text-muted small">
                                    <i className="ri-forbid-2-line me-1"></i>
                                    已禁用渠道（{disabledCount}
                                    ）已固定在底部分区
                                  </span>
                                </td>
                              </tr>
                            ) : null}
                            <tr
                              className={rowBaseClassName || undefined}
                              data-rlm-channel-row="main"
                              data-rlm-channel-id={ch.id}
                              data-rlm-channel-disabled={
                                channelDisabled ? "1" : "0"
                              }
                              onClick={(e) => {
                                const target = e.target as HTMLElement;
                                if (
                                  target.closest(
                                    "button, a, input, textarea, select, label",
                                  )
                                )
                                  return;
                                toggleChannelPanel(ch.id);
                              }}
                            >
                              {renderMainRowCells()}
                            </tr>
                            {panelOpen ? (
                              <tr
                                className={`${channelDisabled ? "table-secondary opacity-75" : "bg-light-subtle"} rlm-channel-detail-row`}
                              >
                                <td
                                  colSpan={channelTableCols}
                                  className="px-4 py-3"
                                >
                                  <div className="d-flex flex-wrap align-items-center gap-2 mb-3">
                                    <button
                                      type="button"
                                      className={`btn btn-sm ${detailPanel === "stats" ? "btn-primary" : "btn-light border"}`}
                                      onClick={() =>
                                        setDetailPanelByChannel((prev) => ({
                                          ...prev,
                                          [ch.id]: "stats",
                                        }))
                                      }
                                    >
                                      详细统计
                                    </button>
                                    <button
                                      type="button"
                                      className={`btn btn-sm ${detailPanel === "test" ? "btn-primary" : "btn-light border"}`}
                                      onClick={() =>
                                        setDetailPanelByChannel((prev) => ({
                                          ...prev,
                                          [ch.id]: "test",
                                        }))
                                      }
                                    >
                                      测试
                                    </button>
                                    {allowCodexOAuth &&
                                    ch.type === "codex_oauth" ? (
                                      <button
                                        type="button"
                                        className={`btn btn-sm ${detailPanel === "accounts" ? "btn-primary" : "btn-light border"}`}
                                        onClick={() => {
                                          setDetailPanelByChannel((prev) => ({
                                            ...prev,
                                            [ch.id]: "accounts",
                                          }));
                                          void loadCodexAccountsForChannel(
                                            ch.id,
                                          );
                                        }}
                                      >
                                        账号统计
                                      </button>
                                    ) : null}
                                  </div>

                                  {detailPanel === "test" ? (
                                    <div className="border rounded-3 p-3 bg-white">
                                      <div className="d-flex flex-wrap align-items-center gap-2">
                                        <span className="fw-semibold text-dark">
                                          测试详情
                                        </span>
                                        {activeTestPanel?.running ? (
                                          <span className="badge bg-primary bg-opacity-10 text-primary border border-primary-subtle">
                                            测试中{" "}
                                            {formatIntComma(
                                              activeTestPanel?.done,
                                            )}{" "}
                                            /{" "}
                                            {formatIntComma(
                                              activeTestPanel?.total,
                                            )}
                                          </span>
                                        ) : null}
                                      </div>
                                      {activeTestPanel?.error?.trim() ? (
                                        <pre
                                          className="mb-0 small text-danger mt-2"
                                          style={{
                                            whiteSpace: "pre-wrap",
                                            wordBreak: "break-word",
                                          }}
                                        >
                                          {activeTestPanel.error}
                                        </pre>
                                      ) : null}
                                      {activeTestPanel?.models?.length ? (
                                        <div className="d-flex flex-column gap-1 mt-2">
                                          {(activeTestPanel?.models || []).map(
                                            (item) => {
                                              const forwardedModel =
                                                item.result?.forwarded_model ||
                                                item.model;
                                              const upstreamResponseModel =
                                                item.result
                                                  ?.upstream_response_model ||
                                                "-";
                                              const summaryMessage =
                                                item.message ||
                                                item.result?.message ||
                                                "";
                                              const outputText =
                                                item.output ||
                                                item.result?.sample ||
                                                "";
                                              return (
                                                <div
                                                  key={item.model}
                                                  className="d-flex flex-column gap-1 mt-1 border rounded-3 p-2"
                                                >
                                                  <div className="d-flex flex-wrap align-items-center gap-2 small">
                                                    <span className="font-monospace text-dark">
                                                      {item.model}
                                                    </span>
                                                    <span
                                                      className={
                                                        item.status ===
                                                        "success"
                                                          ? "badge bg-success bg-opacity-10 text-success border border-success-subtle"
                                                          : item.status ===
                                                              "failed"
                                                            ? "badge bg-danger bg-opacity-10 text-danger border border-danger-subtle"
                                                            : item.status ===
                                                                "running"
                                                              ? "badge bg-primary bg-opacity-10 text-primary border border-primary-subtle"
                                                              : "badge bg-secondary bg-opacity-10 text-secondary border"
                                                      }
                                                    >
                                                      {item.status ===
                                                      "success"
                                                        ? "成功"
                                                        : item.status ===
                                                            "failed"
                                                          ? "失败"
                                                          : item.status ===
                                                              "running"
                                                            ? "进行中"
                                                            : "等待中"}
                                                    </span>
                                                    <span
                                                      className={modelCheckBadgeClass(
                                                        item.result
                                                          ?.model_check_status,
                                                      )}
                                                    >
                                                      {modelCheckLabel(
                                                        item.result
                                                          ?.model_check_status,
                                                      )}
                                                    </span>
                                                  </div>
                                                  <div className="small text-muted">
                                                    请求模型：
                                                    <span className="font-monospace text-dark ms-1">
                                                      {forwardedModel || "-"}
                                                    </span>
                                                  </div>
                                                  <div className="small text-muted">
                                                    返回模型：
                                                    <span className="font-monospace text-dark ms-1">
                                                      {upstreamResponseModel}
                                                    </span>
                                                  </div>
                                                  {summaryMessage ? (
                                                    <div className="small text-muted">
                                                      {summaryMessage}
                                                    </div>
                                                  ) : null}
                                                  {outputText ? (
                                                    <pre
                                                      className="mb-0 small text-muted"
                                                      style={{
                                                        whiteSpace: "pre-wrap",
                                                        wordBreak: "break-word",
                                                      }}
                                                    >
                                                      {outputText}
                                                    </pre>
                                                  ) : null}
                                                </div>
                                              );
                                            },
                                          )}
                                        </div>
                                      ) : (
                                        <div className="text-muted small mt-2">
                                          暂无测试记录，点击上方“测试”按钮可触发探测。
                                        </div>
                                      )}
                                    </div>
                                  ) : detailPanel === "accounts" &&
                                    allowCodexOAuth &&
                                    ch.type === "codex_oauth" ? (
                                    <div className="border rounded-3 p-3 bg-white">
                                      <div className="d-flex flex-wrap align-items-center gap-2 mb-2">
                                        <span className="fw-semibold text-dark">
                                          账号统计
                                        </span>
                                        <button
                                          type="button"
                                          className="btn btn-sm btn-light border"
                                          onClick={async () => {
                                            setErr("");
                                            setNotice("");
                                            try {
                                              await loadCodexAccountsForChannel(
                                                ch.id,
                                                true,
                                              );
                                              await refreshWithCurrentRange();
                                              setNotice("账号统计已刷新");
                                            } catch (e) {
                                              setErr(
                                                e instanceof Error
                                                  ? e.message
                                                  : "刷新失败",
                                              );
                                            }
                                          }}
                                        >
                                          <i className="ri-refresh-line me-1"></i>
                                          刷新账号统计
                                        </button>
                                      </div>
                                      {codexPanelErr ? (
                                        <div className="alert alert-danger py-2 mb-2">
                                          {codexPanelErr}
                                        </div>
                                      ) : null}
                                      {codexPanelLoading ? (
                                        <div className="text-muted small py-3">
                                          账号统计加载中…
                                        </div>
                                      ) : codexPanelAccounts.length === 0 ? (
                                        <div className="text-muted small">
                                          暂无账号。
                                        </div>
                                      ) : (
                                        <div className="table-responsive">
                                          <table className="table table-hover align-middle mb-0">
                                            <thead className="table-light">
                                              <tr>
                                                <th className="ps-3">账号</th>
                                                <th>订阅与额度</th>
                                                <th>状态</th>
                                                <th className="text-end pe-3">
                                                  操作
                                                </th>
                                              </tr>
                                            </thead>
                                            <tbody>
                                              {codexPanelAccounts.map((acc) => {
                                                const cooldownActive =
                                                  !!acc.cooldown_until &&
                                                  new Date(
                                                    acc.cooldown_until,
                                                  ).getTime() > Date.now();

                                                const getQuotaStyles = (
                                                  p: number,
                                                ) => {
                                                  if (p >= 90)
                                                    return {
                                                      bar: "#dcc8c8",
                                                      text: "#7d5a5a",
                                                    };
                                                  if (p >= 70)
                                                    return {
                                                      bar: "#dcd7c8",
                                                      text: "#7d705a",
                                                    };
                                                  return {
                                                    bar: "#c8dcd0",
                                                    text: "#5a7d5e",
                                                  };
                                                };

                                                const renderQuotaRow = (
                                                  title: string,
                                                  percent:
                                                    | number
                                                    | null
                                                    | undefined,
                                                  resetAt?: string | null,
                                                ) => {
                                                  if (
                                                    typeof percent !== "number"
                                                  )
                                                    return null;
                                                  const styles =
                                                    getQuotaStyles(percent);
                                                  const resetHint = (() => {
                                                    if (
                                                      !resetAt ||
                                                      percent < 70
                                                    )
                                                      return null;
                                                    const d = new Date(resetAt);
                                                    if (
                                                      d.getTime() <= Date.now()
                                                    )
                                                      return null;
                                                    return d.toLocaleTimeString(
                                                      [],
                                                      {
                                                        hour: "2-digit",
                                                        minute: "2-digit",
                                                        hour12: false,
                                                      },
                                                    );
                                                  })();

                                                  return (
                                                    <div
                                                      className="d-flex align-items-center gap-2 mb-1"
                                                      style={{
                                                        maxWidth: "240px",
                                                      }}
                                                    >
                                                      <span
                                                        className="text-muted"
                                                        style={{
                                                          fontSize: "11px",
                                                          width: "20px",
                                                          flexShrink: 0,
                                                        }}
                                                      >
                                                        {title}
                                                      </span>
                                                      <div
                                                        className="progress flex-grow-1"
                                                        style={{
                                                          height: "4px",
                                                          backgroundColor:
                                                            "#f0f0f0",
                                                          borderRadius: "2px",
                                                        }}
                                                      >
                                                        <div
                                                          className="progress-bar"
                                                          style={{
                                                            width: `${percent}%`,
                                                            backgroundColor:
                                                              styles.bar,
                                                            borderRadius: "2px",
                                                            transition:
                                                              "width 0.3s",
                                                          }}
                                                        />
                                                      </div>
                                                      <span
                                                        className="font-monospace fw-bold"
                                                        style={{
                                                          fontSize: "11px",
                                                          color: styles.text,
                                                          width: "32px",
                                                          textAlign: "right",
                                                        }}
                                                      >
                                                        {percent}%
                                                      </span>
                                                      {resetHint && (
                                                        <span
                                                          className="text-muted smaller"
                                                          style={{
                                                            marginLeft: "2px",
                                                          }}
                                                        >
                                                          ({resetHint})
                                                        </span>
                                                      )}
                                                    </div>
                                                  );
                                                };

                                                return (
                                                  <tr key={acc.id}>
                                                    <td className="ps-3">
                                                      <div className="d-flex flex-column">
                                                        <span className="fw-semibold text-dark">
                                                          {acc.email ||
                                                            "未绑定邮箱"}
                                                        </span>
                                                        <span className="text-muted small font-monospace">
                                                          {acc.account_id ||
                                                            "-"}
                                                        </span>
                                                        <span className="text-muted smaller">
                                                          更新于：
                                                          {fmtDateTime(
                                                            acc.updated_at,
                                                          )}
                                                        </span>
                                                      </div>
                                                    </td>
                                                    <td>
                                                      {acc.quota_error ||
                                                      acc.balance_error ? (
                                                        <div className="d-flex flex-column small">
                                                          <span className="text-danger fw-medium">
                                                            ⚠️{" "}
                                                            {acc.quota_error ||
                                                              acc.balance_error}
                                                          </span>
                                                          <span className="text-muted smaller">
                                                            同步:{" "}
                                                            {fmtDateTime(
                                                              acc.quota_updated_at ||
                                                                acc.updated_at,
                                                            )}
                                                          </span>
                                                        </div>
                                                      ) : (
                                                        <div className="d-flex flex-column">
                                                          {renderQuotaRow(
                                                            "5h",
                                                            acc.quota_primary_used_percent,
                                                            acc.quota_primary_reset_at,
                                                          )}
                                                          {renderQuotaRow(
                                                            "周",
                                                            acc.quota_secondary_used_percent,
                                                            acc.quota_secondary_reset_at,
                                                          )}
                                                          <div
                                                            className="d-flex justify-content-between text-muted smaller"
                                                            style={{
                                                              maxWidth: "240px",
                                                              marginTop: "2px",
                                                            }}
                                                          >
                                                            <span>
                                                              {acc.quota_credits_unlimited
                                                                ? "✨ 无限"
                                                                : ""}
                                                            </span>
                                                            <span>
                                                              同步:{" "}
                                                              {fmtDateTime(
                                                                acc.quota_updated_at ||
                                                                  acc.updated_at,
                                                              )}
                                                            </span>
                                                          </div>
                                                        </div>
                                                      )}
                                                    </td>
                                                    <td>
                                                      {cooldownActive ? (
                                                        <span className="badge rounded-pill bg-warning bg-opacity-10 text-warning px-2">
                                                          冷却中
                                                        </span>
                                                      ) : acc.status === 1 ? (
                                                        <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">
                                                          运行中
                                                        </span>
                                                      ) : (
                                                        <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">
                                                          已禁用
                                                        </span>
                                                      )}
                                                    </td>
                                                    <td className="text-end pe-3">
                                                      <div className="d-inline-flex gap-2">
                                                        <button
                                                          type="button"
                                                          className="btn btn-sm btn-light border"
                                                          onClick={async () => {
                                                            setErr("");
                                                            setNotice("");
                                                            try {
                                                              const res =
                                                                await refreshChannelCodexAccount(
                                                                  ch.id,
                                                                  acc.id,
                                                                );
                                                              if (!res.success)
                                                                throw new Error(
                                                                  res.message ||
                                                                    "刷新失败",
                                                                );
                                                              await loadCodexAccountsForChannel(
                                                                ch.id,
                                                                true,
                                                              );
                                                              await refreshWithCurrentRange();
                                                              setNotice(
                                                                res.message ||
                                                                  "已刷新",
                                                              );
                                                            } catch (e) {
                                                              setErr(
                                                                e instanceof
                                                                  Error
                                                                  ? e.message
                                                                  : "刷新失败",
                                                              );
                                                            }
                                                          }}
                                                        >
                                                          刷新
                                                        </button>
                                                        <button
                                                          type="button"
                                                          className="btn btn-sm btn-light border text-danger"
                                                          onClick={async () => {
                                                            if (
                                                              !window.confirm(
                                                                "确认彻底删除该账号？且不可恢复。",
                                                              )
                                                            )
                                                              return;
                                                            setErr("");
                                                            setNotice("");
                                                            try {
                                                              const res =
                                                                await deleteChannelCodexAccount(
                                                                  ch.id,
                                                                  acc.id,
                                                                );
                                                              if (!res.success)
                                                                throw new Error(
                                                                  res.message ||
                                                                    "删除失败",
                                                                );
                                                              await loadCodexAccountsForChannel(
                                                                ch.id,
                                                                true,
                                                              );
                                                              await refreshWithCurrentRange();
                                                              setNotice(
                                                                res.message ||
                                                                  "已删除账号",
                                                              );
                                                            } catch (e) {
                                                              setErr(
                                                                e instanceof
                                                                  Error
                                                                  ? e.message
                                                                  : "删除失败",
                                                              );
                                                            }
                                                          }}
                                                        >
                                                          删除
                                                        </button>
                                                      </div>
                                                    </td>
                                                  </tr>
                                                );
                                              })}
                                            </tbody>
                                          </table>
                                        </div>
                                      )}
                                    </div>
                                  ) : (
                                    <>
                                      <div className="d-flex flex-wrap align-items-center gap-3 small text-muted">
                                        <div className="d-flex align-items-center">
                                          <span className="me-1">消耗:</span>
                                          <span className="font-monospace fw-bold text-dark">
                                            {usage?.committed_usd ?? "0"}
                                          </span>
                                        </div>
                                        <div className="d-flex align-items-center">
                                          <span className="me-1">Token:</span>
                                          <span className="fw-medium text-dark">
                                            {formatIntComma(usage?.tokens ?? 0)}
                                          </span>
                                        </div>
                                        <div className="d-flex align-items-center">
                                          <span className="me-1">缓存:</span>
                                          <span className="fw-medium text-muted">
                                            {usage?.cache_ratio ?? "0.0%"}
                                          </span>
                                        </div>
                                        <div className="d-flex align-items-center">
                                          <span className="me-1">首字:</span>
                                          <span className="fw-medium text-dark">
                                            {formatSecondsFromMilliseconds(
                                              usage?.avg_first_token_latency,
                                            )}
                                          </span>
                                        </div>
                                        <div className="d-flex align-items-center">
                                          <span className="me-1">
                                            Tokens/s:
                                          </span>
                                          <span className="fw-medium text-dark">
                                            {usage?.tokens_per_second ?? "-"}
                                          </span>
                                        </div>
                                      </div>
                                      <div className="border rounded-3 p-3 bg-white mt-3">
                                        <div className="d-flex flex-wrap align-items-center gap-3 mb-2">
                                          <div className="d-flex align-items-center gap-2 flex-grow-1">
                                            <div className="d-flex flex-wrap gap-1">
                                              {fieldOptions.map((option) => (
                                                <button
                                                  key={option.value}
                                                  type="button"
                                                  className={`btn btn-sm ${detailField === option.value ? "btn-primary" : "btn-outline-secondary"}`}
                                                  onClick={() =>
                                                    setDetailField(option.value)
                                                  }
                                                >
                                                  {option.label}
                                                </button>
                                              ))}
                                            </div>
                                          </div>
                                          <div className="d-flex align-items-center gap-2 ms-auto">
                                            <div className="d-flex gap-1">
                                              {granularityOptions.map(
                                                (option) => (
                                                  <button
                                                    key={option.value}
                                                    type="button"
                                                    className={`btn btn-sm ${detailGranularity === option.value ? "btn-primary" : "btn-outline-secondary"}`}
                                                    onClick={() =>
                                                      setDetailGranularity(
                                                        option.value,
                                                      )
                                                    }
                                                  >
                                                    {option.label}
                                                  </button>
                                                ),
                                              )}
                                            </div>
                                          </div>
                                        </div>
                                        <div className="small text-muted mb-2">
                                          时间区间：
                                          {(usageAllTime
                                            ? usageResolvedStart
                                            : usageStart) || "-"}{" "}
                                          ~{" "}
                                          {(usageAllTime
                                            ? usageResolvedEnd
                                            : usageEnd) || "-"}
                                        </div>
                                        {detailSeriesErr ? (
                                          <div className="alert alert-danger py-2 mb-2">
                                            {detailSeriesErr}
                                          </div>
                                        ) : null}
                                        {detailSeriesLoading ? (
                                          <div className="text-muted small py-4">
                                            时间序列加载中…
                                          </div>
                                        ) : (
                                          <>
                                            <div style={{ height: 280 }}>
                                              <canvas
                                                ref={
                                                  panelOpen
                                                    ? detailTimeLineRef
                                                    : undefined
                                                }
                                              ></canvas>
                                            </div>
                                          </>
                                        )}
                                      </div>
                                    </>
                                  )}
                                </td>
                              </tr>
                            ) : null}
                          </Fragment>
                        );
                      })}
                    </>
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </SegmentedFrame>

      <BootstrapModal
        id="setChannelPointerModal"
        title={
          pointerTarget
            ? `设为指针：${pointerTarget.name || `#${pointerTarget.id}`}`
            : "设为指针"
        }
        dialogClassName="modal-dialog-centered"
        onHidden={() => {
          setPointerTarget(null);
          setPointerGroupID("");
        }}
      >
        {!pointerTarget ? (
          <div className="text-muted">未选择渠道。</div>
        ) : pointerGroupOptions.length === 0 ? (
          <div className="text-muted">
            该渠道未加入任何启用的渠道组，无法设为指针。
          </div>
        ) : (
          <form
            className="row g-3"
            onSubmit={async (e) => {
              e.preventDefault();
              if (!pointerTarget) return;
              const groupID = Number.parseInt(pointerGroupID, 10) || 0;
              if (groupID <= 0) {
                setErr("请选择渠道组");
                return;
              }
              const g =
                pointerGroupOptions.find((x) => x.id === groupID) || null;
              if (
                !window.confirm(
                  `确认将渠道 ${pointerTarget.name || pointerTarget.id} 设为渠道组 ${g?.name || groupID} 的指针？`,
                )
              )
                return;
              setErr("");
              setNotice("");
              try {
                const res = await upsertAdminChannelGroupPointer(groupID, {
                  channel_id: pointerTarget.id,
                  pinned: true,
                });
                if (!res.success) throw new Error(res.message || "设置失败");
                setNotice(
                  `已设置指针：${g?.name || groupID} → ${pointerTarget.name || pointerTarget.id}`,
                );
                closeModalById("setChannelPointerModal");
              } catch (e) {
                setErr(e instanceof Error ? e.message : "设置失败");
              }
            }}
          >
            <div className="col-12">
              <label className="form-label">选择渠道组</label>
              <select
                className="form-select"
                value={pointerGroupID}
                onChange={(e) => setPointerGroupID(e.target.value)}
              >
                {pointerGroupOptions.map((g) => (
                  <option key={g.id} value={String(g.id)}>
                    {g.name} #{g.id}
                  </option>
                ))}
              </select>
              <div className="form-text small text-muted">
                指针会固定到该渠道，直到被重新设置。
              </div>
            </div>

            <div className="modal-footer border-top-0 px-0 pb-0">
              <button
                type="button"
                className="btn btn-light"
                data-bs-dismiss="modal"
              >
                取消
              </button>
              <button type="submit" className="btn btn-primary px-4">
                确认设置
              </button>
            </div>
          </form>
        )}
      </BootstrapModal>

      <BootstrapModal
        id="createChannelModal"
        title="新建渠道"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateType("openai_compatible");
          setCreateName("");
          setCreateBaseURL("https://api.openai.com");
          setCreateKey("");
          setCreateGroups("");
          setCreatePriority("0");
          setCreatePromotion(false);
          setCreateAllowServiceTier(false);
          setCreateFastMode(true);
          setCreateDisableStore(false);
          setCreateAllowSafetyIdentifier(false);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr("");
            setNotice("");
            try {
              const res = await createChannel({
                type: createType,
                name: createName.trim(),
                base_url: createBaseURL.trim(),
                key:
                  createType === "codex_oauth"
                    ? undefined
                    : createKey.trim() || undefined,
                groups: createGroups.trim() || undefined,
                priority: Number.parseInt(createPriority, 10) || 0,
                promotion: createPromotion,
                allow_service_tier: createAllowServiceTier,
                fast_mode: createFastMode,
                disable_store: createDisableStore,
                allow_safety_identifier: createAllowSafetyIdentifier,
              });
              if (!res.success) throw new Error(res.message || "创建失败");
              setNotice("已创建");
              closeModalById("createChannelModal");
              await refreshWithCurrentRange();
            } catch (e) {
              setErr(e instanceof Error ? e.message : "创建失败");
            }
          }}
        >
          <div className="col-md-4">
            <label className="form-label">类型</label>
            <select
              className="form-select"
              value={createType}
              onChange={(e) => {
                const t = e.target.value as
                  | "openai_compatible"
                  | "anthropic"
                  | "codex_oauth";
                if (!allowCodexOAuth && t === "codex_oauth") return;
                setCreateType(t);
                if (t === "openai_compatible")
                  setCreateBaseURL("https://api.openai.com");
                if (t === "anthropic")
                  setCreateBaseURL("https://api.anthropic.com");
                if (t === "codex_oauth") {
                  setCreateBaseURL("https://chatgpt.com/backend-api/codex");
                  setCreateKey("");
                }
              }}
            >
              <option value="openai_compatible">
                openai_compatible（OpenAI 兼容）
              </option>
              <option value="anthropic">anthropic（Anthropic）</option>
              {allowCodexOAuth ? (
                <option value="codex_oauth">codex_oauth（Codex OAuth）</option>
              ) : null}
            </select>
          </div>
          <div className="col-md-8">
            <label className="form-label">名称</label>
            <input
              className="form-control"
              value={createName}
              onChange={(e) => setCreateName(e.target.value)}
              placeholder="例如：OpenAI 主渠道"
              required
            />
          </div>
          <div className="col-md-8">
            <label className="form-label">接口基础地址</label>
            <input
              className="form-control font-monospace"
              value={createBaseURL}
              onChange={(e) => setCreateBaseURL(e.target.value)}
              placeholder="https://api.openai.com"
              required
            />
          </div>
          <div className="col-md-4">
            <label className="form-label">优先级</label>
            <input
              className="form-control"
              value={createPriority}
              onChange={(e) => setCreatePriority(e.target.value)}
              inputMode="numeric"
              placeholder="0"
            />
          </div>
          <div className="col-md-8">
            <label className="form-label">渠道组（groups，逗号分隔）</label>
            <input
              className="form-control font-monospace"
              value={createGroups}
              onChange={(e) => setCreateGroups(e.target.value)}
              placeholder={defaultGroupName || "default"}
            />
          </div>
          <div className="col-md-4 d-flex align-items-end">
            <div className="form-check">
              <input
                className="form-check-input"
                type="checkbox"
                id="createPromotion"
                checked={createPromotion}
                onChange={(e) => setCreatePromotion(e.target.checked)}
              />
              <label className="form-check-label" htmlFor="createPromotion">
                promotion（优先）
              </label>
            </div>
          </div>

          {allowCodexOAuth && createType === "codex_oauth" ? (
            <div className="col-12">
              <div className="alert alert-light border mb-0">
                <div className="fw-semibold">codex_oauth 不需要 API Key</div>
              </div>
            </div>
          ) : (
            <div className="col-12">
              <label className="form-label">初始 Key（可选）</label>
              <input
                className="form-control font-monospace"
                value={createKey}
                onChange={(e) => setCreateKey(e.target.value)}
                placeholder={
                  createType === "anthropic" ? "sk-ant-..." : "sk-..."
                }
                autoComplete="new-password"
              />
              <div className="form-text small text-muted">
                留空表示先创建渠道，再在“设置”中追加 Key。
              </div>
            </div>
          )}

          <div className="col-12">
            <div className="form-check">
              <input
                className="form-check-input"
                type="checkbox"
                id="createAllowServiceTier"
                checked={createAllowServiceTier}
                disabled={createFastMode}
                onChange={(e) => setCreateAllowServiceTier(e.target.checked)}
              />
              <label className="form-check-label" htmlFor="createAllowServiceTier">
                允许透传 <code>service_tier</code>
              </label>
            </div>
            <div className="form-check">
              <input
                className="form-check-input"
                type="checkbox"
                id="createFastMode"
                checked={createFastMode}
                onChange={(e) => {
                  const checked = e.target.checked;
                  setCreateFastMode(checked);
                  if (checked) setCreateAllowServiceTier(true);
                }}
              />
              <label className="form-check-label" htmlFor="createFastMode">
                允许用户使用 Fast mode（<code>service_tier="priority"</code>）
              </label>
              <div className="form-text small text-muted">
                默认允许。该开关只控制是否接受 Fast，不会自动开启 Fast；启用时会强制保留 <code>service_tier</code> 透传。
              </div>
            </div>
            <div className="form-check">
              <input
                className="form-check-input"
                type="checkbox"
                id="createDisableStore"
                checked={createDisableStore}
                onChange={(e) => setCreateDisableStore(e.target.checked)}
              />
              <label className="form-check-label" htmlFor="createDisableStore">
                禁用透传 <code>store</code>
              </label>
            </div>
            <div className="form-check">
              <input
                className="form-check-input"
                type="checkbox"
                id="createAllowSafetyIdentifier"
                checked={createAllowSafetyIdentifier}
                onChange={(e) =>
                  setCreateAllowSafetyIdentifier(e.target.checked)
                }
              />
              <label
                className="form-check-label"
                htmlFor="createAllowSafetyIdentifier"
              >
                允许透传 <code>safety_identifier</code>
              </label>
            </div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button
              type="button"
              className="btn btn-light"
              data-bs-dismiss="modal"
            >
              取消
            </button>
            <button
              type="submit"
              className="btn btn-primary px-4"
              disabled={loading}
            >
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editChannelModal"
        title={
          settingsChannelID
            ? `渠道设置：${settingsChannelName || `#${settingsChannelID}`}`
            : "渠道设置"
        }
        dialogClassName="modal-lg modal-dialog-scrollable"
        bodyClassName="bg-light"
        footer={
          <button
            type="button"
            className="btn btn-light"
            data-bs-dismiss="modal"
          >
            关闭
          </button>
        }
        onHidden={() => {
          setSettingsChannelID(null);
          setSettingsChannelName("");
          setSettingsChannel(null);
          setSettingsLoading(false);
          setSettingsTab("common");

          setCredentials([]);
          setNewCredentialName("");
          setNewCredentialKey("");
          setKeyValue("");
          setCodexCallbackURL("");
          setCodexManualAccountID("");
          setCodexManualEmail("");
          setCodexManualAccessToken("");
          setCodexManualRefreshToken("");
          setCodexManualIDToken("");
          setCodexManualExpiresAt("");

          setBindings([]);
          setSelectedModelIDs([]);
          setModelRedirects({});
          setModelSearch("");

          setMetaOpenAIOrganization("");
          setMetaTestModel("");
          setMetaTag("");
          setMetaWeight("0");
          setMetaAutoBan(true);
          setMetaRemark("");

          setSettingThinkingToContent(false);
          setSettingPassThroughBodyEnabled(false);
          setSettingProxy("");
          setSettingSystemPrompt("");
          setSettingSystemPromptOverride(false);

          setParamOverride("");
          setHeaderOverride("");
          setModelSuffixPreserve("");
          setRequestBodyWhitelist("");
          setRequestBodyBlacklist("");
          setStatusCodeMapping("");
        }}
      >
        {!settingsChannelID ? (
          <div className="text-muted">未选择渠道。</div>
        ) : settingsLoading ? (
          <div className="text-muted">加载中…</div>
        ) : !settingsChannel ? (
          <div className="text-muted">加载失败。</div>
        ) : (
          <>
            <div className="d-flex flex-wrap align-items-center gap-2 mb-3">
              <span className="fw-semibold text-dark">
                {settingsChannel.name || `渠道 #${settingsChannel.id}`}
              </span>
              <span className="text-muted small">#{settingsChannel.id}</span>
              <span className="text-muted small">
                ({channelTypeLabel(settingsChannel.type)})
              </span>
            </div>

            <ul className="nav nav-tabs mb-3 rlm-tabs-borderless">
              <li className="nav-item">
                <button
                  type="button"
                  className={`nav-link ${settingsTab === "common" ? "active" : ""}`}
                  onClick={() => setSettingsTab("common")}
                >
                  常用
                </button>
              </li>
              <li className="nav-item">
                <button
                  type="button"
                  className={`nav-link ${settingsTab === "keys" ? "active" : ""}`}
                  onClick={() => setSettingsTab("keys")}
                >
                  账号
                </button>
              </li>
              <li className="nav-item">
                <button
                  type="button"
                  className={`nav-link ${settingsTab === "models" ? "active" : ""}`}
                  onClick={() => setSettingsTab("models")}
                >
                  模型绑定
                </button>
              </li>
              <li className="nav-item">
                <button
                  type="button"
                  className={`nav-link ${settingsTab === "advanced" ? "active" : ""}`}
                  onClick={() => setSettingsTab("advanced")}
                >
                  高级
                </button>
              </li>
            </ul>

            {settingsTab === "common" ? (
              <ChannelCommonTab
                enabled={
                  !!settingsChannelID &&
                  !!settingsChannel &&
                  !settingsLoading &&
                  settingsTab === "common"
                }
                resetKey={settingsAutosaveResetKey}
                channelID={settingsChannelID}
                channelGroups={channelGroups}
                editName={editName}
                setEditName={setEditName}
                editStatus={editStatus}
                setEditStatus={setEditStatus}
                editBaseURL={editBaseURL}
                setEditBaseURL={setEditBaseURL}
                editGroups={editGroups}
                setEditGroups={setEditGroups}
                editPriority={editPriority}
                setEditPriority={setEditPriority}
                editPromotion={editPromotion}
                setEditPromotion={setEditPromotion}
                editAllowServiceTier={editAllowServiceTier}
                setEditAllowServiceTier={setEditAllowServiceTier}
                editFastMode={editFastMode}
                setEditFastMode={setEditFastMode}
                editDisableStore={editDisableStore}
                setEditDisableStore={setEditDisableStore}
                editAllowSafetyIdentifier={editAllowSafetyIdentifier}
                setEditAllowSafetyIdentifier={setEditAllowSafetyIdentifier}
                applyChannelPatch={applyChannelPatch}
              />
            ) : null}

            {settingsTab === "keys" ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-body">
                    {allowCodexOAuth &&
                    settingsChannel.type === "codex_oauth" ? (
                      <div className="d-flex flex-column gap-3">
                        <div className="p-3 border rounded bg-white">
                          <div className="fw-semibold mb-2">OAuth 授权</div>
                          <form
                            className="d-flex flex-column gap-2"
                            onSubmit={async (e) => {
                              e.preventDefault();
                              if (!settingsChannelID) return;
                              const callbackURL = codexCallbackURL.trim();
                              if (!callbackURL) return;
                              setErr("");
                              setNotice("");
                              try {
                                const res = await completeChannelCodexOAuth(
                                  settingsChannelID,
                                  callbackURL,
                                );
                                if (!res.success)
                                  throw new Error(
                                    res.message || "完成授权失败",
                                  );
                                setCodexCallbackURL("");
                                await reloadCodexAccounts(settingsChannelID);
                                await refreshWithCurrentRange();
                                setNotice(res.message || "已完成授权");
                              } catch (e) {
                                setErr(
                                  e instanceof Error
                                    ? e.message
                                    : "完成授权失败",
                                );
                              }
                            }}
                          >
                            <button
                              type="button"
                              className="btn btn-sm btn-primary align-self-start"
                              onClick={async () => {
                                if (!settingsChannelID) return;
                                setErr("");
                                setNotice("");
                                const popup = window.open("", "_blank");
                                if (!popup) {
                                  setErr(
                                    "浏览器拦截了弹窗，请允许本站弹窗后重试",
                                  );
                                  return;
                                }
                                try {
                                  const res =
                                    await startChannelCodexOAuth(
                                      settingsChannelID,
                                    );
                                  if (!res.success)
                                    throw new Error(
                                      res.message || "发起授权失败",
                                    );
                                  const authURL = (
                                    res.data?.auth_url || ""
                                  ).trim();
                                  if (!authURL)
                                    throw new Error("未返回授权链接");
                                  popup.location.href = authURL;
                                  setNotice(
                                    "已发起 OAuth 授权，请在新窗口完成后返回此页面。",
                                  );
                                } catch (e) {
                                  try {
                                    popup.close();
                                  } catch {
                                    // ignore
                                  }
                                  setErr(
                                    e instanceof Error
                                      ? e.message
                                      : "发起授权失败",
                                  );
                                }
                              }}
                            >
                              <i className="ri-external-link-line me-1"></i>
                              发起授权（新窗口）
                            </button>
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="http://localhost:1455/auth/callback?code=...&state=..."
                              value={codexCallbackURL}
                              onChange={(e) =>
                                setCodexCallbackURL(e.target.value)
                              }
                            />
                            <div className="form-text small text-muted">
                              如回跳失败，可粘贴浏览器地址栏完整 URL（含
                              code/state）。
                            </div>
                            <button
                              type="submit"
                              className="btn btn-sm btn-light border align-self-start"
                              disabled={!codexCallbackURL.trim()}
                            >
                              完成授权
                            </button>
                          </form>
                        </div>
                        <div className="p-3 border rounded bg-white">
                          <div className="fw-semibold mb-2">手工录入</div>
                          <form
                            className="d-flex flex-column gap-2"
                            onSubmit={async (e) => {
                              e.preventDefault();
                              if (!settingsChannelID) return;
                              const accessToken = codexManualAccessToken.trim();
                              const refreshToken =
                                codexManualRefreshToken.trim();
                              if (!accessToken || !refreshToken) return;
                              setErr("");
                              setNotice("");
                              try {
                                const res = await createChannelCodexAccount(
                                  settingsChannelID,
                                  {
                                    account_id:
                                      codexManualAccountID.trim() || undefined,
                                    email: codexManualEmail.trim() || undefined,
                                    access_token: accessToken,
                                    refresh_token: refreshToken,
                                    id_token:
                                      codexManualIDToken.trim() || undefined,
                                    expires_at:
                                      codexManualExpiresAt.trim() || undefined,
                                  },
                                );
                                if (!res.success)
                                  throw new Error(res.message || "保存失败");
                                setCodexManualAccountID("");
                                setCodexManualEmail("");
                                setCodexManualAccessToken("");
                                setCodexManualRefreshToken("");
                                setCodexManualIDToken("");
                                setCodexManualExpiresAt("");
                                await reloadCodexAccounts(settingsChannelID);
                                await refreshWithCurrentRange();
                                setNotice(res.message || "已保存");
                              } catch (e) {
                                setErr(
                                  e instanceof Error ? e.message : "保存失败",
                                );
                              }
                            }}
                          >
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="account_id（可选，留空则尝试从 id_token 解析）"
                              value={codexManualAccountID}
                              onChange={(e) =>
                                setCodexManualAccountID(e.target.value)
                              }
                            />
                            <input
                              className="form-control form-control-sm"
                              type="email"
                              placeholder="邮箱（可选）"
                              value={codexManualEmail}
                              onChange={(e) =>
                                setCodexManualEmail(e.target.value)
                              }
                            />
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="access_token"
                              value={codexManualAccessToken}
                              onChange={(e) =>
                                setCodexManualAccessToken(e.target.value)
                              }
                              required
                            />
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="refresh_token"
                              value={codexManualRefreshToken}
                              onChange={(e) =>
                                setCodexManualRefreshToken(e.target.value)
                              }
                              required
                            />
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="id_token（可选）"
                              value={codexManualIDToken}
                              onChange={(e) =>
                                setCodexManualIDToken(e.target.value)
                              }
                            />
                            <input
                              className="form-control form-control-sm font-monospace"
                              placeholder="expires_at（可选，RFC3339）"
                              value={codexManualExpiresAt}
                              onChange={(e) =>
                                setCodexManualExpiresAt(e.target.value)
                              }
                            />
                            <button
                              type="submit"
                              className="btn btn-sm btn-primary align-self-start"
                            >
                              保存账号
                            </button>
                          </form>
                        </div>
                        <div className="form-text small text-muted mb-0">
                          账号详情已迁移到渠道行展开后的“账号统计”面板。
                        </div>
                      </div>
                    ) : (
                      <>
                        <div className="form-text small text-muted mb-3">
                          密钥将以明文存储，仅展示提示；删除不可恢复。
                        </div>

                        {credentials.length === 0 ? (
                          <div className="text-muted small">暂无密钥。</div>
                        ) : (
                          <div className="table-responsive">
                            <table className="table table-hover align-middle mb-0">
                              <thead className="table-light">
                                <tr>
                                  <th className="ps-3">名称</th>
                                  <th>密钥提示</th>
                                  <th>状态</th>
                                  <th className="text-end pe-3">操作</th>
                                </tr>
                              </thead>
                              <tbody>
                                {credentials.map((c) => (
                                  <tr key={c.id}>
                                    <td className="ps-3">
                                      {c.name ? (
                                        <span className="fw-semibold text-dark">
                                          {c.name}
                                        </span>
                                      ) : (
                                        <span className="text-muted small">
                                          -
                                        </span>
                                      )}
                                    </td>
                                    <td>
                                      <code className="text-secondary bg-light border p-2 rounded d-inline-block">
                                        {c.masked_key || "-"}
                                      </code>
                                    </td>
                                    <td>
                                      {c.status === 1 ? (
                                        <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">
                                          <i className="ri-checkbox-circle-line me-1"></i>
                                          启用
                                        </span>
                                      ) : (
                                        <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">
                                          <i className="ri-close-circle-line me-1"></i>
                                          禁用
                                        </span>
                                      )}
                                    </td>
                                    <td className="text-end pe-3">
                                      <button
                                        type="button"
                                        className="btn btn-sm btn-light border text-danger"
                                        onClick={async () => {
                                          if (!settingsChannelID) return;
                                          if (
                                            !window.confirm(
                                              "确认彻底删除该凭证？且不可恢复。",
                                            )
                                          )
                                            return;
                                          setErr("");
                                          setNotice("");
                                          try {
                                            const res =
                                              await deleteChannelCredential(
                                                settingsChannelID,
                                                c.id,
                                              );
                                            if (!res.success)
                                              throw new Error(
                                                res.message || "删除失败",
                                              );
                                            setNotice(res.message || "已删除");
                                            await reloadCredentials(
                                              settingsChannelID,
                                            );
                                            await refreshWithCurrentRange();
                                          } catch (e) {
                                            setErr(
                                              e instanceof Error
                                                ? e.message
                                                : "删除失败",
                                            );
                                          }
                                        }}
                                      >
                                        <i className="ri-delete-bin-line me-1"></i>
                                        删除
                                      </button>
                                    </td>
                                  </tr>
                                ))}
                              </tbody>
                            </table>
                          </div>
                        )}

                        <hr className="my-4 text-muted opacity-25" />

                        <form
                          className="row g-3"
                          onSubmit={async (e) => {
                            e.preventDefault();
                            if (!settingsChannelID) return;
                            setErr("");
                            setNotice("");
                            try {
                              const res = await createChannelCredential(
                                settingsChannelID,
                                newCredentialKey.trim(),
                                newCredentialName.trim() || undefined,
                              );
                              if (!res.success)
                                throw new Error(res.message || "添加失败");
                              setNotice(res.message || "已添加");
                              setNewCredentialKey("");
                              setNewCredentialName("");
                              await reloadCredentials(settingsChannelID);
                              await refreshWithCurrentRange();
                            } catch (e) {
                              setErr(
                                e instanceof Error ? e.message : "添加失败",
                              );
                            }
                          }}
                        >
                          <div className="col-md-4">
                            <label className="form-label fw-medium">
                              备注名称（可选）
                            </label>
                            <input
                              className="form-control"
                              value={newCredentialName}
                              onChange={(e) =>
                                setNewCredentialName(e.target.value)
                              }
                              placeholder="例如：team-a-gpt4"
                            />
                          </div>
                          <div className="col-md-8">
                            <label className="form-label fw-medium">
                              API 密钥
                            </label>
                            <input
                              className="form-control font-monospace"
                              value={newCredentialKey}
                              onChange={(e) =>
                                setNewCredentialKey(e.target.value)
                              }
                              required
                              placeholder="sk-..."
                              autoComplete="new-password"
                            />
                            <div className="form-text small text-muted">
                              密钥将以明文存储。
                            </div>
                          </div>
                          <div className="col-12">
                            <button
                              type="submit"
                              className="btn btn-primary btn-sm"
                              disabled={!newCredentialKey.trim()}
                            >
                              <i className="ri-add-line me-1"></i>添加密钥
                            </button>
                          </div>
                        </form>
                      </>
                    )}
                  </div>
                </div>

                {allowCodexOAuth &&
                settingsChannel.type === "codex_oauth" ? null : (
                  <div className="card border-0 shadow-sm">
                    <div className="card-header bg-white fw-bold py-3">
                      查看明文 Key（可选）
                    </div>
                    <div className="card-body">
                      <div className="form-text small text-muted mb-3">
                        仅 root 可见；读取第一个 credential 的明文
                        key，请妥善保管。
                      </div>
                      <button
                        type="button"
                        className="btn btn-sm btn-light border"
                        onClick={async () => {
                          if (!settingsChannelID) return;
                          setErr("");
                          setNotice("");
                          setKeyValue("");
                          try {
                            const res = await getChannelKey(settingsChannelID);
                            if (!res.success)
                              throw new Error(res.message || "获取失败");
                            setKeyValue(res.data?.key || "");
                          } catch (e) {
                            setErr(e instanceof Error ? e.message : "获取失败");
                          }
                        }}
                      >
                        <i className="ri-key-2-line me-1"></i>读取明文 Key
                      </button>

                      {keyValue ? (
                        <div className="mt-3">
                          <textarea
                            className="form-control font-monospace"
                            rows={4}
                            value={keyValue}
                            readOnly
                          />
                          <div className="d-grid mt-2">
                            <button
                              type="button"
                              className="btn btn-primary btn-sm"
                              onClick={async () => {
                                try {
                                  await navigator.clipboard.writeText(keyValue);
                                  setNotice("已复制到剪贴板");
                                } catch {
                                  setErr("复制失败（浏览器不支持或无权限）");
                                }
                              }}
                            >
                              复制
                            </button>
                          </div>
                        </div>
                      ) : null}
                    </div>
                  </div>
                )}
              </div>
            ) : null}

            {settingsTab === "models" ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">
                    模型选择
                  </div>
                  <div className="card-body">
                    <div className="d-flex flex-wrap align-items-end gap-2">
                      <div className="flex-grow-1">
                        <label className="form-label fw-medium mb-1">
                          搜索模型
                        </label>
                        <input
                          className="form-control form-control-sm"
                          value={modelSearch}
                          onChange={(e) => setModelSearch(e.target.value)}
                          placeholder="输入模型名称过滤"
                        />
                      </div>
                      <button
                        type="button"
                        className="btn btn-sm btn-light border"
                        onClick={() => {
                          const ids = filteredModelIDs;
                          if (ids.length === 0) return;
                          setSelectedModelIDs((prev) => {
                            const uniq = new Set(prev);
                            for (const id of ids) uniq.add(id);
                            const next = Array.from(uniq);
                            next.sort((a, b) => a.localeCompare(b, "zh-CN"));
                            return next;
                          });
                          setModelRedirects((prev) => {
                            const next: Record<string, string> = { ...prev };
                            for (const id of ids) {
                              if (next[id] !== undefined) continue;
                              const b = bindingByPublicID.get(id);
                              if (!b) continue;
                              if (b.upstream_model.trim() === "") continue;
                              if (b.upstream_model === id) continue;
                              next[id] = b.upstream_model;
                            }
                            return next;
                          });
                        }}
                      >
                        全选筛选结果
                      </button>
                      <button
                        type="button"
                        className="btn btn-sm btn-white border text-dark"
                        onClick={() => {
                          setSelectedModelIDs([]);
                        }}
                        disabled={selectedModelIDs.length === 0}
                      >
                        清空选择
                      </button>
                    </div>

                    <div className="text-muted small mt-2">
                      已选择{" "}
                      <span className="fw-semibold text-dark">
                        {selectedModelIDs.length}
                      </span>{" "}
                      / {selectableModelIDs.length} 个（当前筛选：
                      {filteredModelIDs.length} 个）
                    </div>

                    <div
                      className="card p-2 mt-2"
                      style={{ maxHeight: 320, overflowY: "auto" }}
                    >
                      {filteredModelIDs.length === 0 ? (
                        <div className="text-muted small p-2">
                          没有匹配的模型。
                        </div>
                      ) : (
                        filteredModelIDs.map((id) => (
                          <div className="form-check" key={id}>
                            <label
                              className="form-check-label w-100 d-flex align-items-center"
                              style={{ cursor: "pointer" }}
                            >
                              <input
                                className="form-check-input me-2"
                                type="checkbox"
                                checked={selectedModelSet.has(id)}
                                onChange={(e) => {
                                  const checked = e.target.checked;
                                  setSelectedModelIDs((prev) => {
                                    const has = prev.includes(id);
                                    if (checked && !has) {
                                      const next = [...prev, id];
                                      next.sort((a, b) =>
                                        a.localeCompare(b, "zh-CN"),
                                      );
                                      return next;
                                    }
                                    if (!checked && has)
                                      return prev.filter((m) => m !== id);
                                    return prev;
                                  });
                                  if (!checked) return;
                                  const b = bindingByPublicID.get(id);
                                  if (!b) return;
                                  if (b.upstream_model.trim() === "") return;
                                  if (b.upstream_model === id) return;
                                  setModelRedirects((prev) => {
                                    if (prev[id] !== undefined) return prev;
                                    return { ...prev, [id]: b.upstream_model };
                                  });
                                }}
                              />
                              <span className="font-monospace small user-select-all">
                                {id}
                              </span>
                            </label>
                          </div>
                        ))
                      )}
                    </div>

                    {staleEnabledBindings.length > 0 ? (
                      <div className="alert alert-warning py-2 px-3 mt-3 mb-0 small">
                        当前渠道还有 {staleEnabledBindings.length} 个已启用绑定不在模型目录中：{" "}
                        {staleEnabledBindings
                          .map((b) => b.public_id.trim())
                          .filter((id) => id !== "")
                          .join("、")}
                        。它们不会再出现在可选列表；保存后会被自动禁用。
                      </div>
                    ) : null}

                    <div className="form-text small text-muted mt-2">
                      选择该渠道允许使用的模型；下方可选配置“模型重定向”。
                    </div>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">
                    模型重定向
                  </div>
                  <div className="card-body">
                    <div className="form-text small text-muted mb-3">
                      对已选择的模型生效；留空表示不重定向（使用同名模型）。
                    </div>
                    {selectedModelIDs.length === 0 ? (
                      <div className="text-muted small">
                        请先在上方选择模型。
                      </div>
                    ) : (
                      <div className="table-responsive">
                        <table className="table table-hover align-middle mb-0">
                          <thead className="table-light">
                            <tr>
                              <th className="ps-3">对外模型</th>
                              <th>重定向到（上游模型）</th>
                            </tr>
                          </thead>
                          <tbody>
                            {selectedModelIDs.map((id) => (
                              <tr key={id}>
                                <td className="ps-3">
                                  <span className="font-monospace small user-select-all">
                                    {id}
                                  </span>
                                </td>
                                <td>
                                  <input
                                    className="form-control form-control-sm font-monospace"
                                    value={modelRedirects[id] ?? ""}
                                    onChange={(e) => {
                                      const v = e.target.value;
                                      setModelRedirects((prev) => {
                                        const next = { ...prev };
                                        const trimmed = v.trim();
                                        if (trimmed === "" || trimmed === id)
                                          delete next[id];
                                        else next[id] = v;
                                        return next;
                                      });
                                    }}
                                    placeholder="留空=不重定向（使用同名）"
                                  />
                                </td>
                              </tr>
                            ))}
                          </tbody>
                        </table>
                      </div>
                    )}

                    <div className="d-flex justify-content-end mt-3">
                      <AutoSaveIndicator
                        status={modelsAutosave.status}
                        blockedReason={modelsAutosave.blockedReason}
                        error={modelsAutosave.error}
                        onRetry={modelsAutosave.retry}
                      />
                    </div>
                  </div>
                </div>
              </div>
            ) : null}

            {settingsTab === "advanced" ? (
              <ChannelAdvancedTab
                enabled={
                  !!settingsChannelID &&
                  !!settingsChannel &&
                  !settingsLoading &&
                  settingsTab === "advanced"
                }
                resetKey={settingsAutosaveResetKey}
                channelID={settingsChannelID}
                channelType={settingsChannel.type}
                metaOpenAIOrganization={metaOpenAIOrganization}
                setMetaOpenAIOrganization={setMetaOpenAIOrganization}
                metaTestModel={metaTestModel}
                setMetaTestModel={setMetaTestModel}
                metaTag={metaTag}
                setMetaTag={setMetaTag}
                metaWeight={metaWeight}
                setMetaWeight={setMetaWeight}
                metaAutoBan={metaAutoBan}
                setMetaAutoBan={setMetaAutoBan}
                metaRemark={metaRemark}
                setMetaRemark={setMetaRemark}
                settingThinkingToContent={settingThinkingToContent}
                setSettingThinkingToContent={setSettingThinkingToContent}
                settingPassThroughBodyEnabled={settingPassThroughBodyEnabled}
                setSettingPassThroughBodyEnabled={
                  setSettingPassThroughBodyEnabled
                }
                settingProxy={settingProxy}
                setSettingProxy={setSettingProxy}
                settingSystemPrompt={settingSystemPrompt}
                setSettingSystemPrompt={setSettingSystemPrompt}
                settingSystemPromptOverride={settingSystemPromptOverride}
                setSettingSystemPromptOverride={setSettingSystemPromptOverride}
                paramOverride={paramOverride}
                setParamOverride={setParamOverride}
                headerOverride={headerOverride}
                setHeaderOverride={setHeaderOverride}
                modelSuffixPreserve={modelSuffixPreserve}
                setModelSuffixPreserve={setModelSuffixPreserve}
                requestBodyWhitelist={requestBodyWhitelist}
                setRequestBodyWhitelist={setRequestBodyWhitelist}
                requestBodyBlacklist={requestBodyBlacklist}
                setRequestBodyBlacklist={setRequestBodyBlacklist}
                statusCodeMapping={statusCodeMapping}
                setStatusCodeMapping={setStatusCodeMapping}
              />
            ) : null}
          </>
        )}
      </BootstrapModal>
    </div>
  );
}

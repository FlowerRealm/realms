import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type MutableRefObject } from 'react';

import { BootstrapModal } from '../../components/BootstrapModal';
import { closeModalById } from '../../components/modal';
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
  getPinnedChannelInfo,
  listChannelCodexAccounts,
  listChannelCredentials,
  pinChannel,
  refreshChannelCodexAccount,
  refreshChannelCodexAccounts,
  reorderChannels,
  startChannelCodexOAuth,
  testChannelStream,
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
  type ChannelProbeSummary,
  type ChannelTestProgressEvent,
  type ChannelTimeSeriesPoint,
  type CodexOAuthAccount,
  type PinnedChannelInfo,
} from '../../api/channels';
import { listAdminChannelGroups, type AdminChannelGroup } from '../../api/admin/channelGroups';
import { createChannelModel, listChannelModels, updateChannelModel, type ChannelModelBinding } from '../../api/channelModels';
import { listManagedModelsAdmin } from '../../api/models';

function channelTypeLabel(t: string): string {
  if (t === 'openai_compatible') return 'OpenAI 兼容';
  if (t === 'anthropic') return 'Anthropic';
  if (t === 'codex_oauth') return 'Codex OAuth';
  return t;
}

function statusBadge(status: number): { cls: string; label: string } {
  if (status === 1) return { cls: 'badge bg-success bg-opacity-10 text-success border border-success-subtle', label: '启用' };
  return { cls: 'badge bg-secondary bg-opacity-10 text-secondary border', label: '禁用' };
}

function healthBadge(ch: Channel): { cls: string; label: string; hint?: string } {
  if (ch.type === 'codex_oauth') {
    return { cls: 'badge bg-light text-secondary border', label: '不支持测试' };
  }
  if (!ch.last_test_at) {
    return { cls: 'badge bg-light text-secondary border', label: '未测试' };
  }
  const latency = Number.isFinite(ch.last_test_latency_ms) ? `${ch.last_test_latency_ms}ms` : '-';
  if (ch.last_test_ok) {
    return { cls: 'badge bg-success bg-opacity-10 text-success border border-success-subtle', label: `正常 · ${latency}` };
  }
  return { cls: 'badge bg-danger bg-opacity-10 text-danger border border-danger-subtle', label: `异常 · ${latency}` };
}

function formatPinnedChannelLabel(channelID: number, fallback: string, channels: ChannelAdminItem[]): string {
  const matched = channels.find((ch) => ch.id === channelID);
  const matchedName = (matched?.name || '').trim();
  if (matchedName) return `${matchedName} (#${channelID})`;
  const raw = (fallback || '').trim();
  if (raw) return raw;
  if (channelID > 0) return `渠道 #${channelID}`;
  return '';
}

function normalizePinnedInfo(info: PinnedChannelInfo | null, channels: ChannelAdminItem[]): PinnedChannelInfo | null {
  if (!info || !info.available) return info;

  const runtimePinned = channels.find((ch) => !!ch.runtime?.pinned_active);
  let out = { ...info };

  if ((!out.pinned_active || out.pinned_channel_id <= 0) && runtimePinned) {
    out.pinned_active = true;
    out.pinned_channel_id = runtimePinned.id;
    out.pinned_channel = runtimePinned.name?.trim() ? `${runtimePinned.name.trim()} (#${runtimePinned.id})` : `渠道 #${runtimePinned.id}`;
  }

  if (out.pinned_active && out.pinned_channel_id > 0) {
    out.pinned_channel = formatPinnedChannelLabel(out.pinned_channel_id, out.pinned_channel, channels);
  }

  return out;
}

function compactProbeMessage(raw: string): string {
  let msg = raw.trim();
  if (!msg) return '';
  msg = msg.replace(/\s+/g, ' ');
  msg = msg.replace(/Post "[^"]+": context deadline exceeded \(Client\.Timeout exceeded while awaiting headers\)/g, '请求超时');
  msg = msg.replace(/context deadline exceeded \(Client\.Timeout exceeded while awaiting headers\)/g, '请求超时');
  if (msg.includes('；失败示例：')) {
    const idx = msg.indexOf('；失败示例：');
    msg = `${msg.slice(0, idx)}；失败详情见下方模型结果`;
  }
  if (msg.length > 140) {
    msg = `${msg.slice(0, 140)}…`;
  }
  return msg;
}

type ChartInstance = {
  destroy?: () => void;
};

type ChartConstructor = new (ctx: CanvasRenderingContext2D, config: unknown) => ChartInstance;

type ChannelModelLiveState = {
  model: string;
  status: 'pending' | 'running' | 'success' | 'failed';
  message: string;
  result?: ChannelModelProbeResult;
};

type ChannelTestPanelState = {
  running: boolean;
  source: string;
  total: number;
  done: number;
  currentModel: string;
  models: ChannelModelLiveState[];
  summary?: ChannelProbeSummary;
  summaryMessage: string;
};

export function ChannelsPage() {
  const [channels, setChannels] = useState<ChannelAdminItem[]>([]);
  const [managedModelIDs, setManagedModelIDs] = useState<string[]>([]);
  const [channelGroups, setChannelGroups] = useState<AdminChannelGroup[]>([]);
  const [pinned, setPinned] = useState<PinnedChannelInfo | null>(null);
  const [loading, setLoading] = useState(true);
  const [reordering, setReordering] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');
  const [testingChannelID, setTestingChannelID] = useState<number | null>(null);
  const [expandedChannelID, setExpandedChannelID] = useState<number | null>(null);
  const [testPanels, setTestPanels] = useState<Record<number, ChannelTestPanelState>>({});

  const [usageStart, setUsageStart] = useState('');
  const [usageEnd, setUsageEnd] = useState('');
  const [usageRangeDirty, setUsageRangeDirty] = useState(false);
  const detailTimeLineRef = useRef<HTMLCanvasElement | null>(null);
  const detailTimeLineChartRef = useRef<ChartInstance | null>(null);
  const [detailSeries, setDetailSeries] = useState<ChannelTimeSeriesPoint[]>([]);
  const [detailSeriesLoading, setDetailSeriesLoading] = useState(false);
  const [detailSeriesErr, setDetailSeriesErr] = useState('');
  const [detailField, setDetailField] = useState<'committed_usd' | 'tokens' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second'>('committed_usd');
  const [detailGranularity, setDetailGranularity] = useState<'hour' | 'day'>('hour');
  const fieldOptions: Array<{ value: 'committed_usd' | 'tokens' | 'cache_ratio' | 'avg_first_token_latency' | 'tokens_per_second'; label: string }> = [
    { value: 'committed_usd', label: '消耗 (USD)' },
    { value: 'tokens', label: 'Token' },
    { value: 'cache_ratio', label: '缓存率 (%)' },
    { value: 'avg_first_token_latency', label: '首字延迟 (ms)' },
    { value: 'tokens_per_second', label: 'Tokens/s' },
  ];
  const granularityOptions: Array<{ value: 'hour' | 'day'; label: string }> = [
    { value: 'hour', label: '按小时' },
    { value: 'day', label: '按天' },
  ];

  const [draggingID, setDraggingID] = useState<number | null>(null);
  const [dropOverID, setDropOverID] = useState<number | null>(null);

  const [createType, setCreateType] = useState<'openai_compatible' | 'anthropic' | 'codex_oauth'>('openai_compatible');
  const [createName, setCreateName] = useState('');
  const [createBaseURL, setCreateBaseURL] = useState('https://api.openai.com');
  const [createKey, setCreateKey] = useState('');
  const [createGroups, setCreateGroups] = useState('default');
  const [createPriority, setCreatePriority] = useState('0');
  const [createPromotion, setCreatePromotion] = useState(false);
  const [createAllowServiceTier, setCreateAllowServiceTier] = useState(false);
  const [createDisableStore, setCreateDisableStore] = useState(false);
  const [createAllowSafetyIdentifier, setCreateAllowSafetyIdentifier] = useState(false);

  const [settingsChannelID, setSettingsChannelID] = useState<number | null>(null);
  const [settingsChannelName, setSettingsChannelName] = useState('');
  const [settingsChannel, setSettingsChannel] = useState<Channel | null>(null);
  const [settingsLoading, setSettingsLoading] = useState(false);
  const [settingsTab, setSettingsTab] = useState<'common' | 'keys' | 'models' | 'advanced'>('common');

  const [editName, setEditName] = useState('');
  const [editGroups, setEditGroups] = useState('');
  const [editBaseURL, setEditBaseURL] = useState('');
  const [editStatus, setEditStatus] = useState(1);
  const [editPriority, setEditPriority] = useState('0');
  const [editPromotion, setEditPromotion] = useState(false);
  const [editAllowServiceTier, setEditAllowServiceTier] = useState(false);
  const [editDisableStore, setEditDisableStore] = useState(false);
  const [editAllowSafetyIdentifier, setEditAllowSafetyIdentifier] = useState(false);

  const [credentials, setCredentials] = useState<ChannelCredential[]>([]);
  const [newCredentialName, setNewCredentialName] = useState('');
  const [newCredentialKey, setNewCredentialKey] = useState('');
  const [keyValue, setKeyValue] = useState('');
  const [codexAccounts, setCodexAccounts] = useState<CodexOAuthAccount[]>([]);
  const [codexCallbackURL, setCodexCallbackURL] = useState('');
  const [codexManualAccountID, setCodexManualAccountID] = useState('');
  const [codexManualEmail, setCodexManualEmail] = useState('');
  const [codexManualAccessToken, setCodexManualAccessToken] = useState('');
  const [codexManualRefreshToken, setCodexManualRefreshToken] = useState('');
  const [codexManualIDToken, setCodexManualIDToken] = useState('');
  const [codexManualExpiresAt, setCodexManualExpiresAt] = useState('');

  const [bindings, setBindings] = useState<ChannelModelBinding[]>([]);
  const [selectedModelIDs, setSelectedModelIDs] = useState<string[]>([]);
  const [modelRedirects, setModelRedirects] = useState<Record<string, string>>({});
  const [modelsSaving, setModelsSaving] = useState(false);
  const [modelSearch, setModelSearch] = useState('');

  const [metaOpenAIOrganization, setMetaOpenAIOrganization] = useState('');
  const [metaTestModel, setMetaTestModel] = useState('');
  const [metaTag, setMetaTag] = useState('');
  const [metaWeight, setMetaWeight] = useState('0');
  const [metaAutoBan, setMetaAutoBan] = useState(true);
  const [metaRemark, setMetaRemark] = useState('');

  const [settingThinkingToContent, setSettingThinkingToContent] = useState(false);
  const [settingPassThroughBodyEnabled, setSettingPassThroughBodyEnabled] = useState(false);
  const [settingProxy, setSettingProxy] = useState('');
  const [settingSystemPrompt, setSettingSystemPrompt] = useState('');
  const [settingSystemPromptOverride, setSettingSystemPromptOverride] = useState(false);

  const [paramOverride, setParamOverride] = useState('');
  const [headerOverride, setHeaderOverride] = useState('');
  const [modelSuffixPreserve, setModelSuffixPreserve] = useState('');
  const [requestBodyWhitelist, setRequestBodyWhitelist] = useState('');
  const [requestBodyBlacklist, setRequestBodyBlacklist] = useState('');
  const [statusCodeMapping, setStatusCodeMapping] = useState('');
  const oauthQueryHandled = useRef(false);

  const enabledCount = useMemo(() => channels.filter((c) => c.status === 1).length, [channels]);
  const disabledCount = useMemo(() => channels.length - enabledCount, [channels.length, enabledCount]);
  const firstDisabledIndex = useMemo(() => channels.findIndex((c) => c.status !== 1), [channels]);
  const selectableModelIDs = useMemo(() => {
    const uniq = new Set<string>();
    for (const id of managedModelIDs) {
      const v = id.trim();
      if (v) uniq.add(v);
    }
    for (const b of bindings) {
      const v = b.public_id.trim();
      if (v) uniq.add(v);
    }
    const out = Array.from(uniq);
    out.sort((a, b) => a.localeCompare(b, 'zh-CN'));
    return out;
  }, [managedModelIDs, bindings]);

  const filteredModelIDs = useMemo(() => {
    const q = modelSearch.trim().toLowerCase();
    if (!q) return selectableModelIDs;
    return selectableModelIDs.filter((id) => id.toLowerCase().includes(q));
  }, [selectableModelIDs, modelSearch]);

  const selectedModelSet = useMemo(() => new Set(selectedModelIDs), [selectedModelIDs]);
  const bindingByPublicID = useMemo(() => {
    const m = new Map<string, ChannelModelBinding>();
    for (const b of bindings) m.set(b.public_id, b);
    return m;
  }, [bindings]);

  function moveChannelBefore(list: ChannelAdminItem[], movingID: number, targetID: number): ChannelAdminItem[] {
    const from = list.findIndex((c) => c.id === movingID);
    const to = list.findIndex((c) => c.id === targetID);
    if (from < 0 || to < 0 || from === to) return list;
    const next = [...list];
    const [picked] = next.splice(from, 1);
    const insertAt = from < to ? to - 1 : to;
    next.splice(insertAt, 0, picked);
    return next;
  }

  function normalizeChannelSections(list: ChannelAdminItem[]): ChannelAdminItem[] {
    const enabled = list.filter((ch) => ch.status === 1);
    const disabled = list.filter((ch) => ch.status !== 1);
    return [...enabled, ...disabled];
  }

  function fmtNumber(n: number): string {
    if (!Number.isFinite(n)) return '-';
    return new Intl.NumberFormat('zh-CN').format(n);
  }

  function fmtHHMM(iso?: string | null): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    return d.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false });
  }

  function fmtDateTime(iso?: string | null): string {
    if (!iso) return '-';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '-';
    return d.toLocaleString('zh-CN', { hour12: false });
  }

  function upsertModelState(models: ChannelModelLiveState[], model: string, patch: Partial<ChannelModelLiveState>): ChannelModelLiveState[] {
    const idx = models.findIndex((item) => item.model === model);
    if (idx < 0) {
      return [...models, { model, status: 'pending', message: '', ...patch }];
    }
    const next = [...models];
    next[idx] = { ...next[idx], ...patch };
    return next;
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

  function applyTestProgress(channelID: number, evt: ChannelTestProgressEvent) {
    setTestPanels((prev) => {
      const current = prev[channelID] || {
        running: true,
        source: '',
        total: 0,
        done: 0,
        currentModel: '',
        models: [],
        summaryMessage: '',
      };
      if (evt.type === 'start') {
        const modelList = Array.isArray(evt.models) ? evt.models : [];
        return {
          ...prev,
          [channelID]: {
            ...current,
            running: true,
            source: evt.source || current.source,
            total: evt.total || modelList.length,
            done: 0,
            currentModel: '',
            models: modelList.map((model) => ({ model, status: 'pending', message: '' })),
            summary: undefined,
            summaryMessage: '',
          },
        };
      }
      if (evt.type === 'model_start') {
        const model = evt.model || '';
        return {
          ...prev,
          [channelID]: {
            ...current,
            running: true,
            source: evt.source || current.source,
            total: evt.total || current.total,
            currentModel: model,
            models: model ? upsertModelState(current.models, model, { status: 'running', message: '测试中...' }) : current.models,
          },
        };
      }
      if (evt.type === 'model_done') {
        const model = evt.model || evt.result?.model || '';
        const result = evt.result;
        const done = Math.max(current.done, evt.index || current.done);
        return {
          ...prev,
          [channelID]: {
            ...current,
            running: true,
            source: evt.source || current.source,
            total: evt.total || current.total,
            done,
            currentModel: done >= (evt.total || current.total || 0) ? '' : current.currentModel,
            models: model
              ? upsertModelState(current.models, model, {
                  status: result?.ok ? 'success' : 'failed',
                  message: compactProbeMessage(result?.message || ''),
                  result,
                })
              : current.models,
          },
        };
      }
      return prev;
    });
  }

  async function refresh(params?: { start?: string; end?: string }) {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const [pageRes, modelsRes, pinnedRes] = await Promise.all([getChannelsPage(params), listManagedModelsAdmin(1, 1000), getPinnedChannelInfo()]);
      if (!modelsRes.success) throw new Error(modelsRes.message || '加载模型失败');
      setManagedModelIDs(
        (modelsRes.data?.items || [])
          .filter((m) => m.status === 1)
          .map((m) => m.public_id)
          .filter((id) => typeof id === 'string' && id.trim() !== ''),
      );

      if (!pageRes.success) throw new Error(pageRes.message || '加载渠道失败');
      const pageChannels = pageRes.data?.channels || [];
      if (pinnedRes.success) {
        setPinned(normalizePinnedInfo(pinnedRes.data || null, pageChannels));
      } else {
        setPinned(null);
      }
      setUsageStart(pageRes.data?.start || '');
      setUsageEnd(pageRes.data?.end || '');
      setChannels(normalizeChannelSections(pageChannels));
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!usageRangeDirty) return;
    const t = window.setTimeout(() => {
      setUsageRangeDirty(false);
      void refresh({ start: usageStart.trim(), end: usageEnd.trim() });
    }, 400);
    return () => window.clearTimeout(t);
  }, [usageRangeDirty, usageStart, usageEnd]);

  useEffect(() => {
    if (!expandedChannelID) {
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
        const res = await getChannelTimeSeries(expandedChannelID, {
          start: usageStart.trim() || undefined,
          end: usageEnd.trim() || undefined,
          granularity: detailGranularity,
        });
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
  }, [expandedChannelID, usageStart, usageEnd, detailGranularity]);

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

  function parseGroupsCSV(raw: string): string[] {
    const s = raw.trim();
    if (!s) return ['default'];
    const uniq = new Set<string>();
    for (const part of s.split(',')) {
      const v = part.trim();
      if (v) uniq.add(v);
    }
    if (uniq.size === 0) return ['default'];
    return Array.from(uniq);
  }

  function toggleGroupsCSV(raw: string, name: string, checked: boolean): string {
    const set = new Set(parseGroupsCSV(raw));
    if (checked) set.add(name);
    else set.delete(name);
    const out = Array.from(set);
    return out.join(',');
  }

  async function reloadCredentials(channelID: number) {
    const res = await listChannelCredentials(channelID);
    if (!res.success) throw new Error(res.message || '加载密钥失败');
    setCredentials(res.data || []);
  }

  async function reloadCodexAccounts(channelID: number) {
    const res = await listChannelCodexAccounts(channelID);
    if (!res.success) throw new Error(res.message || '加载账号失败');
    setCodexAccounts(res.data || []);
  }

  const openChannelSettingsModal = useCallback((ch: { id: number; name?: string }, tab: 'common' | 'keys' | 'models' | 'advanced' = 'common') => {
    setSettingsTab(tab);
    setSettingsChannelID(ch.id);
    setSettingsChannelName(ch.name || `#${ch.id}`);
    setSettingsChannel(null);

    if (typeof window === 'undefined') return;
    const modalRoot = document.getElementById('editChannelModal');
    const modalCtor = (window as Window & { bootstrap?: { Modal?: { getOrCreateInstance: (el: Element) => { show: () => void } } } }).bootstrap?.Modal;
    if (!modalRoot || !modalCtor?.getOrCreateInstance) return;
    modalCtor.getOrCreateInstance(modalRoot).show();
  }, []);

  useEffect(() => {
    if (oauthQueryHandled.current || loading) return;
    if (typeof window === 'undefined') return;
    oauthQueryHandled.current = true;

    const params = new URLSearchParams(window.location.search);
    const openChannelSettings = Number.parseInt(params.get('open_channel_settings') || '', 10);
    const oauthState = (params.get('oauth') || '').trim();
    const oauthErr = (params.get('err') || '').trim();

    if (oauthState === 'ok') setNotice('Codex OAuth 授权成功');
    if (oauthState === 'error') setErr(oauthErr || 'Codex OAuth 授权失败');

    if (openChannelSettings > 0) {
      const target = channels.find((ch) => ch.id === openChannelSettings);
      if (target) openChannelSettingsModal(target, 'keys');
    }

    if (openChannelSettings > 0 || oauthState !== '' || oauthErr !== '') {
      params.delete('open_channel_settings');
      params.delete('oauth');
      params.delete('err');
      const nextQuery = params.toString();
      const nextURL = `${window.location.pathname}${nextQuery ? `?${nextQuery}` : ''}${window.location.hash || ''}`;
      window.history.replaceState({}, '', nextURL);
    }
  }, [channels, loading, openChannelSettingsModal]);

  useEffect(() => {
    if (typeof window === 'undefined') return;
    const onMessage = (event: MessageEvent) => {
      const payload = event.data as { type?: string; redirectURL?: string } | null;
      if (!payload || payload.type !== 'realms_codex_oauth_callback') return;
      let openChannelSettings = 0;
      let oauthState = '';
      if (payload.redirectURL) {
        try {
          const parsed = new URL(payload.redirectURL, window.location.origin);
          openChannelSettings = Number.parseInt(parsed.searchParams.get('open_channel_settings') || '', 10);
          oauthState = (parsed.searchParams.get('oauth') || '').trim();
        } catch {
          // ignore
        }
      }
      if (oauthState === 'ok') setNotice('Codex OAuth 授权成功');
      if (oauthState === 'error') setErr('Codex OAuth 授权失败');

      if (openChannelSettings > 0) {
        const target = channels.find((ch) => ch.id === openChannelSettings);
        if (target) openChannelSettingsModal(target, 'keys');
      } else if (settingsChannelID && settingsChannel?.type === 'codex_oauth') {
        void reloadCodexAccounts(settingsChannelID).catch(() => {});
      }
    };
    window.addEventListener('message', onMessage);
    return () => {
      window.removeEventListener('message', onMessage);
    };
  }, [channels, openChannelSettingsModal, settingsChannel, settingsChannelID]);

  const applyChannelModelBindings = useCallback((items: ChannelModelBinding[]) => {
    setBindings(items);

    const selected = items
      .filter((b) => b.status === 1)
      .map((b) => b.public_id)
      .filter((id) => id.trim() !== '');
    selected.sort((a, b) => a.localeCompare(b, 'zh-CN'));
    setSelectedModelIDs(selected);

    const redirects: Record<string, string> = {};
    for (const b of items) {
      if (b.status !== 1) continue;
      if (b.public_id.trim() === '') continue;
      if (b.upstream_model.trim() === '') continue;
      if (b.upstream_model === b.public_id) continue;
      redirects[b.public_id] = b.upstream_model;
    }
    setModelRedirects(redirects);
  }, []);

  async function reloadBindings(channelID: number) {
    const res = await listChannelModels(channelID);
    if (!res.success) throw new Error(res.message || '加载绑定失败');
    applyChannelModelBindings(res.data || []);
  }

  const loadChannelSettings = useCallback(async (channelID: number) => {
    setErr('');
    setNotice('');
    setSettingsLoading(true);
    try {
      const [chRes, credsRes, bindingsRes] = await Promise.all([getChannel(channelID), listChannelCredentials(channelID), listChannelModels(channelID)]);
      if (!chRes.success) throw new Error(chRes.message || '加载渠道失败');
      const ch = chRes.data;
      if (!ch) throw new Error('渠道不存在');
      setSettingsChannel(ch);

      setEditName(ch.name || '');
      setEditGroups(ch.groups || 'default');
      setEditBaseURL(ch.base_url || '');
      setEditStatus(ch.status || 0);
      setEditPriority(String(ch.priority || 0));
      setEditPromotion(!!ch.promotion);
      setEditAllowServiceTier(!!ch.allow_service_tier);
      setEditDisableStore(!!ch.disable_store);
      setEditAllowSafetyIdentifier(!!ch.allow_safety_identifier);

      if (!credsRes.success) throw new Error(credsRes.message || '加载密钥失败');
      setCredentials(credsRes.data || []);
      setNewCredentialName('');
      setNewCredentialKey('');
      setKeyValue('');
      setCodexCallbackURL('');
      setCodexManualAccountID('');
      setCodexManualEmail('');
      setCodexManualAccessToken('');
      setCodexManualRefreshToken('');
      setCodexManualIDToken('');
      setCodexManualExpiresAt('');
      if (ch.type === 'codex_oauth') {
        await reloadCodexAccounts(channelID);
      } else {
        setCodexAccounts([]);
      }

      if (!bindingsRes.success) throw new Error(bindingsRes.message || '加载绑定失败');
      applyChannelModelBindings(bindingsRes.data || []);

      setMetaOpenAIOrganization(ch.openai_organization || '');
      setMetaTestModel(ch.test_model || '');
      setMetaTag(ch.tag || '');
      setMetaWeight(String(ch.weight || 0));
      setMetaAutoBan(ch.auto_ban ?? true);
      setMetaRemark(ch.remark || '');

      const setting = ch.setting || {};
      setSettingThinkingToContent(!!setting.thinking_to_content);
      setSettingPassThroughBodyEnabled(!!setting.pass_through_body_enabled);
      setSettingProxy(setting.proxy || '');
      setSettingSystemPrompt(setting.system_prompt || '');
      setSettingSystemPromptOverride(!!setting.system_prompt_override);

      setParamOverride(ch.param_override || '');
      setHeaderOverride(ch.header_override || '');
      setModelSuffixPreserve(ch.model_suffix_preserve || '');
      setRequestBodyWhitelist(ch.request_body_whitelist || '');
      setRequestBodyBlacklist(ch.request_body_blacklist || '');
      setStatusCodeMapping(ch.status_code_mapping || '');

      setModelSearch('');
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setSettingsChannel(null);
      setCredentials([]);
      setCodexAccounts([]);
      setBindings([]);
      setSelectedModelIDs([]);
      setModelRedirects({});
    } finally {
      setSettingsLoading(false);
    }
  }, [applyChannelModelBindings]);

  async function saveModelsConfig() {
    if (!settingsChannelID) return;
    setErr('');
    setNotice('');
    setModelsSaving(true);
    try {
      const selected = selectedModelIDs
        .map((m) => m.trim())
        .filter((m) => m !== '');
      const selectedSet = new Set<string>(selected);

      const bindingByPublicID = new Map<string, ChannelModelBinding>();
      for (const b of bindings) {
        bindingByPublicID.set(b.public_id, b);
      }

      for (const b of bindings) {
        const enabled = selectedSet.has(b.public_id);
        const desiredStatus = enabled ? 1 : 0;
        const redirected = (modelRedirects[b.public_id] || '').trim();
        const desiredUpstreamModel = enabled ? redirected || b.public_id : b.upstream_model;

        if (b.status === desiredStatus && (!enabled || b.upstream_model === desiredUpstreamModel)) continue;
        const res = await updateChannelModel(settingsChannelID, {
          id: b.id,
          public_id: b.public_id,
          upstream_model: desiredUpstreamModel.trim() || b.public_id,
          status: desiredStatus,
        });
        if (!res.success) throw new Error(res.message || '保存失败');
      }

      for (const publicID of selected) {
        if (bindingByPublicID.has(publicID)) continue;
        const redirected = (modelRedirects[publicID] || '').trim();
        const upstreamModel = redirected || publicID;
        const res = await createChannelModel(settingsChannelID, publicID, upstreamModel, 1);
        if (!res.success) throw new Error(res.message || '创建失败');
      }

      setNotice('已保存模型配置');
      await reloadBindings(settingsChannelID);
      await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败');
    } finally {
      setModelsSaving(false);
    }
  }

  useEffect(() => {
    if (!settingsChannelID) return;
    void loadChannelSettings(settingsChannelID);
  }, [settingsChannelID, loadChannelSettings]);

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
    const ctx = detailTimeLineRef.current?.getContext('2d');
    if (!ctx) return;

    const fieldMeta: Record<string, { label: string; color: string; read: (p: ChannelTimeSeriesPoint) => number }> = {
      committed_usd: {
        label: '消耗 (USD)',
        color: 'rgba(99, 102, 241, 0.95)',
        read: (p) => p.committed_usd,
      },
      tokens: {
        label: 'Token',
        color: 'rgba(16, 185, 129, 0.95)',
        read: (p) => p.tokens,
      },
      cache_ratio: {
        label: '缓存率 (%)',
        color: 'rgba(245, 158, 11, 0.95)',
        read: (p) => p.cache_ratio,
      },
      avg_first_token_latency: {
        label: '首字延迟 (ms)',
        color: 'rgba(239, 68, 68, 0.95)',
        read: (p) => p.avg_first_token_latency,
      },
      tokens_per_second: {
        label: 'Tokens/s',
        color: 'rgba(14, 165, 233, 0.95)',
        read: (p) => p.tokens_per_second,
      },
    };
    const meta = fieldMeta[detailField];
    const datasets = [
      {
        label: meta.label,
        data: detailSeries.map((p) => meta.read(p)),
        borderColor: meta.color,
        backgroundColor: meta.color.replace('0.95', '0.18'),
        pointRadius: 2,
        tension: 0.2,
      },
    ];

    detailTimeLineChartRef.current = new ChartCtor(ctx, {
      type: 'line',
      data: {
        labels: detailSeries.map((p) => p.bucket),
        datasets,
      },
      options: {
        responsive: true,
        maintainAspectRatio: false,
        interaction: { mode: 'index', intersect: false },
        plugins: {
          legend: { position: 'bottom' },
          title: { display: true, text: `${channel.name || `渠道 #${channel.id}`} · 时间序列` },
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
            grid: { color: 'rgba(148, 163, 184, 0.18)' },
          },
        },
      },
    });

    return () => {
      destroy(detailTimeLineChartRef);
    };
  }, [channels, expandedChannelID, detailSeries, detailField, detailGranularity]);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-start mb-4 flex-wrap gap-3">
        <div>
          <h2 className="h4 fw-bold mb-1">上游渠道管理</h2>
          <p className="text-muted small mb-0">
            管理模型转发渠道。支持拖拽排序调整优先级（越靠前优先级越高）。当前 {enabledCount} 启用 / {disabledCount} 禁用 / {channels.length} 总计。
          </p>
        </div>
        <button type="button" className="btn btn-primary" data-bs-toggle="modal" data-bs-target="#createChannelModal">
          <i className="ri-add-line me-1"></i> 新建渠道
        </button>
      </div>

      <div className="row g-2 align-items-end mb-4">
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">开始日期</label>
          <input
            className="form-control form-control-sm"
            type="date"
            value={usageStart}
            onChange={(e) => {
              setUsageStart(e.target.value);
              setUsageRangeDirty(true);
            }}
          />
        </div>
        <div className="col-auto">
          <label className="form-label small text-muted mb-1">结束日期</label>
          <input
            className="form-control form-control-sm"
            type="date"
            value={usageEnd}
            onChange={(e) => {
              setUsageEnd(e.target.value);
              setUsageRangeDirty(true);
            }}
          />
        </div>
        <div className="col-auto">
          <div className="form-text small text-muted mb-0">统计区间（可选）：修改后自动更新。</div>
        </div>
      </div>

      {notice ? (
        <div className="alert alert-success d-flex align-items-center mb-4" role="alert">
          <span className="me-2 material-symbols-rounded">check_circle</span>
          <div>{notice}</div>
        </div>
      ) : null}

      {err ? (
        <div className="alert alert-danger d-flex align-items-center mb-4" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      {pinned?.available ? (
        <div className="card border-0 shadow-sm mb-4">
          <div className="card-body py-3 d-flex flex-wrap gap-2 align-items-center">
            <span className="text-muted small">智能调度</span>
            {pinned.pinned_active ? (
              <span className="badge bg-warning-subtle text-warning-emphasis border" title={pinned.pinned_note || undefined}>
                渠道指针：{pinned.pinned_channel}
              </span>
            ) : (
              <span className="badge bg-light text-muted border">渠道指针：-</span>
            )}
          </div>
        </div>
      ) : null}

      <div className="card border-0 shadow-sm overflow-hidden mb-0">
        <div className="bg-primary bg-opacity-10 py-3 px-4 d-flex justify-content-between align-items-center">
          <div>
            <span className="text-primary fw-bold text-uppercase small">渠道列表</span>
          </div>
          <div className="text-primary text-opacity-75 small">
            <i className="ri-drag-move-2-line me-1"></i> 支持拖拽排序
          </div>
        </div>
        <div className="table-responsive">
          <table className="table table-hover align-middle mb-0">
            <thead className="table-light">
              <tr>
                <th style={{ width: 60 }}></th>
                <th className="ps-4">渠道详情</th>
                <th>状态</th>
                <th>健康状况</th>
                <th className="text-end pe-4">操作</th>
              </tr>
            </thead>
            <tbody>
                  {loading ? (
                    <tr>
                      <td colSpan={5} className="text-center py-5 text-muted">
                        加载中…
                      </td>
                    </tr>
                  ) : channels.length === 0 ? (
                    <tr>
                      <td colSpan={5} className="text-center py-5 text-muted">
                        <span className="fs-1 d-block mb-3 material-symbols-rounded">inbox</span>
                        暂无渠道。
                      </td>
                    </tr>
                  ) : (
                    channels.map((ch, idx) => {
                      const st = statusBadge(ch.status);
                      const hb = healthBadge(ch);
                      const channelDisabled = ch.status !== 1;
                      const isPinned = !!pinned?.pinned_active && pinned.pinned_channel_id === ch.id;
                      const runtime = ch.runtime;
                      const usage = ch.usage;
                      const testPanel = testPanels[ch.id];
                      const checkedAt = fmtHHMM(ch.last_test_at);
                      const panelOpen = expandedChannelID === ch.id;
                      const testRunning = testingChannelID === ch.id;
                      const anyTesting = testingChannelID !== null;
                      const activeTestPanel = testPanel && (testPanel.running || testPanel.summary != null || testPanel.summaryMessage.trim() !== '' || testPanel.models.length > 0) ? testPanel : null;
                      const rowClassName = [dropOverID === ch.id ? 'table-primary' : '', channelDisabled ? 'table-secondary opacity-75' : '']
                        .filter((v) => v)
                        .join(' ');
                      return (
                        <Fragment key={ch.id}>
                        {idx === firstDisabledIndex ? (
                          <tr className="table-light">
                            <td colSpan={5} className="px-4 py-2">
                              <span className="text-muted small">
                                <i className="ri-forbid-2-line me-1"></i>已禁用渠道（{disabledCount}）已固定在底部分区
                              </span>
                            </td>
                          </tr>
                        ) : null}
                        <tr
                          className={rowClassName || undefined}
                          onClick={(e) => {
                            const target = e.target as HTMLElement;
                            if (target.closest('button, a, input, textarea, select, label')) return;
                            toggleChannelPanel(ch.id);
                          }}
                          onDragOver={(e) => {
                            if (loading || reordering || channelDisabled) return;
                            e.preventDefault();
                            setDropOverID(ch.id);
                          }}
                          onDragLeave={() => {
                            if (dropOverID === ch.id) setDropOverID(null);
                          }}
                          onDrop={async (e) => {
                            e.preventDefault();
                            if (loading || reordering || channelDisabled) return;
                            const moving = draggingID;
                            if (!moving || moving === ch.id) return;
                            const prev = channels;
                            const movingChannel = prev.find((item) => item.id === moving);
                            if (!movingChannel || movingChannel.status !== 1) return;
                            const next = moveChannelBefore(prev, moving, ch.id);
                            if (next === prev) return;
                            setChannels(next);
                            setDraggingID(null);
                            setDropOverID(null);
                            setReordering(true);
                            setErr('');
                            setNotice('');
                            try {
                              const res = await reorderChannels(next.map((c) => c.id));
                              if (!res.success) throw new Error(res.message || '保存排序失败');
                              setNotice(res.message || '已保存排序');
                              await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                            } catch (e) {
                              setChannels(prev);
                              setErr(e instanceof Error ? e.message : '保存排序失败');
                            } finally {
                              setReordering(false);
                            }
                          }}
                        >
                          <td
                            className="text-center text-muted"
                            style={{ cursor: reordering || channelDisabled ? 'not-allowed' : 'grab' }}
                            title={channelDisabled ? '禁用渠道固定在底部' : '拖动排序'}
                          >
                            <span
                              className="d-inline-flex align-items-center justify-content-center"
                              style={{ width: 48 }}
                              draggable={!loading && !reordering && !channelDisabled}
                              onDragStart={(e) => {
                                if (loading || reordering || channelDisabled) return;
                                setDraggingID(ch.id);
                                setDropOverID(ch.id);
                                e.dataTransfer.effectAllowed = 'move';
                                try {
                                  e.dataTransfer.setData('text/plain', String(ch.id));
                                } catch {
                                  // ignore
                                }
                              }}
                              onDragEnd={() => {
                                setDraggingID(null);
                                setDropOverID(null);
                              }}
                            >
                              <i className="ri-drag-move-2-line fs-5"></i>
                            </span>
                          </td>
	                          <td className="ps-4" style={{ minWidth: 0 }}>
	                            <div className="d-flex flex-column">
	                              <div className="d-flex flex-wrap align-items-center gap-2">
	                                <span className="fw-bold text-dark">{ch.name || `渠道 #${ch.id}`}</span>
	                                <span className="text-muted small">({channelTypeLabel(ch.type)})</span>
	                                {isPinned ? (
	                                  <span className="small text-primary fw-medium">
	                                    <i className="ri-pushpin-2-fill me-1"></i>指针
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
                                    style={{ maxWidth: 360, whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}
                                    title={ch.base_url}
                                  >
                                    {ch.base_url}
                                  </span>
                                ) : null}
                                <div className="d-flex align-items-center">
                                  {ch.base_url ? <span className="text-secondary">·</span> : null}
                                  <span className={`${ch.base_url ? 'ms-2 ' : ''}me-1`}>组:</span>
                                  <span className="text-secondary font-monospace user-select-all">
                                    {ch.groups || 'default'}
                                  </span>
                                </div>
                              </div>
                            </div>
                          </td>
                          <td>
                            <span className={st.cls}>{st.label}</span>
                            {runtime?.available && runtime.banned_active ? (
                              <div className="mt-1">
                                <span className="badge bg-warning-subtle text-warning-emphasis border px-2" title={runtime.banned_until ? `封禁至 ${runtime.banned_until}` : undefined}>
                                  <i className="ri-forbid-2-line me-1"></i>封禁中 · 剩余 {runtime.banned_remaining || '-'}
                                </span>
                              </div>
                            ) : null}
                            {runtime?.available && runtime.fail_score > 0 ? (
                              <div className="mt-1">
                                <span className="badge bg-light text-secondary border" title="失败计分（运行态 fail score，越高越容易触发封禁/探测）">
                                  失败计分：{runtime.fail_score}
                                </span>
                              </div>
                            ) : null}
                          </td>
                          <td>
                            <div className="d-flex flex-column">
		                              <span className={hb.cls} title={hb.hint}>
		                                {hb.label}
		                              </span>
		                              {checkedAt ? (
		                                <small className="text-muted mt-1 smaller" title={ch.last_test_at || undefined}>
		                                  {checkedAt} 已检查
		                                </small>
		                              ) : null}
		                            </div>
		                          </td>
                          <td className="text-end pe-4 text-nowrap">
                            <div className="d-flex gap-1 justify-content-end">
                              <button
                                className="btn btn-sm btn-light border text-primary"
                                type="button"
                                title="测试连接"
                                disabled={loading || reordering || ch.type === 'codex_oauth' || anyTesting}
                                onClick={async () => {
                                  if (ch.type === 'codex_oauth') return;
                                  openChannelPanel(ch.id);
                                  setErr('');
                                  setNotice('');
                                  setTestingChannelID(ch.id);
                                  setTestPanels((prev) => ({
                                    ...prev,
                                    [ch.id]: {
                                      running: true,
                                      source: '',
                                      total: 0,
                                      done: 0,
                                      currentModel: '',
                                      models: [],
                                      summary: undefined,
                                      summaryMessage: '',
                                    },
                                  }));
                                  try {
                                    const res = await testChannelStream(ch.id, (evt) => applyTestProgress(ch.id, evt));
                                    const probe = res.data?.probe;
                                    setTestPanels((prev) => {
                                      const current = prev[ch.id];
                                      if (!current) return prev;
                                      const finalModels =
                                        probe?.results?.length && probe.results.length > 0
                                          ? probe.results.map((item) => ({
                                              model: item.model,
                                              status: item.ok ? ('success' as const) : ('failed' as const),
                                              message: compactProbeMessage(item.message || ''),
                                              result: item,
                                            }))
                                          : current.models;
                                      return {
                                        ...prev,
                                        [ch.id]: {
                                          ...current,
                                          running: false,
                                          source: probe?.source || current.source,
                                          total: probe?.total ?? current.total,
                                          done: probe?.total ?? current.done,
                                          currentModel: '',
                                          models: finalModels,
                                          summary: probe,
                                          summaryMessage: compactProbeMessage(res.message || probe?.message || ''),
                                        },
                                      };
                                    });
                                    if (!res.success) throw new Error(res.message || '测试失败');
                                    setNotice(res.message || '测试成功');
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                  } catch (e) {
                                    const msg = e instanceof Error ? e.message : '测试失败';
                                    const compactMsg = compactProbeMessage(msg);
                                    setErr('');
                                    setTestPanels((prev) => {
                                      const current = prev[ch.id];
                                      if (!current) return prev;
                                      return {
                                        ...prev,
                                        [ch.id]: {
                                          ...current,
                                          running: false,
                                          currentModel: '',
                                          summaryMessage: compactMsg || msg,
                                        },
                                      };
                                    });
                                  } finally {
                                    setTestingChannelID((prev) => (prev === ch.id ? null : prev));
                                    setTestPanels((prev) => {
                                      const current = prev[ch.id];
                                      if (!current) return prev;
                                      return {
                                        ...prev,
                                        [ch.id]: {
                                          ...current,
                                          running: false,
                                          currentModel: '',
                                        },
                                      };
                                    });
                                  }
                                }}
                              >
                                {testRunning ? (
                                  <span className="spinner-border spinner-border-sm me-1" role="status" aria-hidden="true"></span>
                                ) : (
                                  <i className="ri-flashlight-line me-1"></i>
                                )}
                                {testRunning ? '测试中' : '测试'}
                              </button>

                              <button
                                className="btn btn-sm btn-light border text-primary"
                                type="button"
                                title="设为指针"
                                disabled={loading || reordering || !(pinned?.available ?? false)}
                                onClick={async () => {
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await pinChannel(ch.id);
                                    if (!res.success) throw new Error(res.message || '设置失败');
                                    setNotice(res.message || '已设置');
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '设置失败');
                                  }
                                }}
                              >
                                <i className="ri-rocket-2-line me-1"></i>设为指针
                              </button>

                              <button
                                className={`btn btn-sm ${ch.status === 1 ? 'btn-light border text-warning' : 'btn-light border text-success'}`}
                                type="button"
                                title={ch.status === 1 ? '禁用渠道' : '启用渠道'}
                                disabled={loading || reordering}
                                onClick={async () => {
                                  const targetStatus = ch.status === 1 ? 0 : 1;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await updateChannel({ id: ch.id, status: targetStatus });
                                    if (!res.success) throw new Error(res.message || '更新状态失败');
                                    if (settingsChannelID === ch.id) {
                                      setEditStatus(targetStatus);
                                    }
                                    setNotice(targetStatus === 1 ? '渠道已启用' : '渠道已禁用');
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '更新状态失败');
                                  }
                                }}
                              >
                                <i className={`me-1 ${ch.status === 1 ? 'ri-pause-circle-line' : 'ri-play-circle-line'}`}></i>
                                {ch.status === 1 ? '禁用' : '启用'}
                              </button>

                              <button
                                className="btn btn-sm btn-primary"
                                type="button"
                                title="设置"
                                disabled={loading || reordering}
                                onClick={() => openChannelSettingsModal({ id: ch.id, name: ch.name }, 'common')}
                              >
                                <i className="ri-settings-3-line me-1"></i>设置
                              </button>

                              <button
                                className="btn btn-sm btn-light border text-danger"
                                type="button"
                                title="删除"
                                disabled={loading || reordering}
                                onClick={async () => {
                                  if (!window.confirm(`确认删除渠道 ${ch.name || ch.id} ? 此操作不可恢复。`)) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await deleteChannel(ch.id);
                                    if (!res.success) throw new Error(res.message || '删除失败');
                                    setNotice('已删除');
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '删除失败');
                                  }
                                }}
                              >
                                <i className="ri-delete-bin-line me-1"></i>删除
                              </button>
                            </div>
                          </td>
                        </tr>
                        {panelOpen ? (
                          <tr className={`${channelDisabled ? 'table-secondary opacity-75' : 'bg-light-subtle'} rlm-channel-detail-row`}>
                            <td colSpan={5} className="px-4 py-3">
                              {activeTestPanel ? (
                                <div className="border rounded-3 p-3 bg-white">
                                  <div className="d-flex flex-wrap align-items-center gap-2">
                                    <span className="fw-semibold text-dark">测试详情</span>
                                    {activeTestPanel.running ? (
                                      <span className="badge bg-primary bg-opacity-10 text-primary border border-primary-subtle">
                                        测试中 {activeTestPanel.done}/{activeTestPanel.total || '-'}
                                      </span>
                                    ) : (
                                      <span
                                        className={`badge ${
                                          activeTestPanel.summary?.ok
                                            ? 'bg-success bg-opacity-10 text-success border border-success-subtle'
                                            : 'bg-secondary bg-opacity-10 text-secondary border'
                                        }`}
                                      >
                                        {activeTestPanel.summary?.ok ? '测试通过' : '测试完成'}
                                      </span>
                                    )}
                                    {activeTestPanel.currentModel ? <span className="badge bg-light text-dark border">当前模型：{activeTestPanel.currentModel}</span> : null}
                                  </div>
                                  {activeTestPanel.models.length > 0 ? (
                                    <div className="d-flex flex-column gap-1 mt-2">
                                      {activeTestPanel.models.map((item) => (
                                        <div key={item.model} className="d-flex flex-wrap align-items-center gap-2 small">
                                          <span
                                            className={`badge ${
                                              item.status === 'success'
                                                ? 'bg-success bg-opacity-10 text-success border border-success-subtle'
                                                : item.status === 'failed'
                                                  ? 'bg-danger bg-opacity-10 text-danger border border-danger-subtle'
                                                  : item.status === 'running'
                                                    ? 'bg-primary bg-opacity-10 text-primary border border-primary-subtle'
                                                    : 'bg-light text-secondary border'
                                            }`}
                                          >
                                            {item.status === 'success' ? '通过' : item.status === 'failed' ? '失败' : item.status === 'running' ? '测试中' : '待测试'}
                                          </span>
                                          <span className="font-monospace text-dark">{item.model}</span>
                                          {item.message ? <span className="text-muted">{item.message}</span> : null}
                                        </div>
                                      ))}
                                    </div>
                                  ) : null}
                                </div>
                              ) : (
                                <>
                                  <div className="d-flex flex-wrap align-items-center gap-3 small text-muted">
                                    <div className="d-flex align-items-center">
                                      <span className="me-1">消耗:</span>
                                      <span className="font-monospace fw-bold text-dark">{usage?.committed_usd ?? '0'}</span>
                                    </div>
                                    <div className="d-flex align-items-center">
                                      <span className="me-1">Token:</span>
                                      <span className="fw-medium text-dark">{fmtNumber(usage?.tokens ?? 0)}</span>
                                    </div>
                                    <div className="d-flex align-items-center">
                                      <span className="me-1">缓存:</span>
                                      <span className="fw-medium text-success">{usage?.cache_ratio ?? '0.0%'}</span>
                                    </div>
                                    <div className="d-flex align-items-center">
                                      <span className="me-1">首字:</span>
                                      <span className="fw-medium text-dark">{usage?.avg_first_token_latency ?? '-'}</span>
                                    </div>
                                    <div className="d-flex align-items-center">
                                      <span className="me-1">Tokens/s:</span>
                                      <span className="fw-medium text-dark">{usage?.tokens_per_second ?? '-'}</span>
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
                                    <div className="small text-muted mb-2">时间区间：{usageStart || '-'} ~ {usageEnd || '-'}</div>
                                    {detailSeriesErr ? <div className="alert alert-danger py-2 mb-2">{detailSeriesErr}</div> : null}
                                    {detailSeriesLoading ? (
                                      <div className="text-muted small py-4">时间序列加载中…</div>
                                    ) : (
                                      <>
                                        <div style={{ height: 280 }}>
                                          <canvas ref={panelOpen ? detailTimeLineRef : undefined}></canvas>
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
                    })
                  )}
            </tbody>
          </table>
        </div>
      </div>

      <BootstrapModal
        id="createChannelModal"
        title="新建渠道"
        dialogClassName="modal-dialog-centered modal-lg"
        onHidden={() => {
          setCreateType('openai_compatible');
          setCreateName('');
          setCreateBaseURL('https://api.openai.com');
          setCreateKey('');
          setCreateGroups('default');
          setCreatePriority('0');
          setCreatePromotion(false);
          setCreateAllowServiceTier(false);
          setCreateDisableStore(false);
          setCreateAllowSafetyIdentifier(false);
        }}
      >
        <form
          className="row g-3"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr('');
            setNotice('');
            try {
              const res = await createChannel({
                type: createType,
                name: createName.trim(),
                base_url: createBaseURL.trim(),
                key: createType === 'codex_oauth' ? undefined : createKey.trim() || undefined,
                groups: createGroups.trim() || undefined,
                priority: Number.parseInt(createPriority, 10) || 0,
                promotion: createPromotion,
                allow_service_tier: createAllowServiceTier,
                disable_store: createDisableStore,
                allow_safety_identifier: createAllowSafetyIdentifier,
              });
              if (!res.success) throw new Error(res.message || '创建失败');
              setNotice('已创建');
              closeModalById('createChannelModal');
              await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
            } catch (e) {
              setErr(e instanceof Error ? e.message : '创建失败');
            }
          }}
        >
          <div className="col-md-4">
            <label className="form-label">类型</label>
            <select
              className="form-select"
              value={createType}
              onChange={(e) => {
                const t = e.target.value as 'openai_compatible' | 'anthropic' | 'codex_oauth';
                setCreateType(t);
                if (t === 'openai_compatible') setCreateBaseURL('https://api.openai.com');
                if (t === 'anthropic') setCreateBaseURL('https://api.anthropic.com');
                if (t === 'codex_oauth') {
                  setCreateBaseURL('https://chatgpt.com/backend-api/codex');
                  setCreateKey('');
                }
              }}
            >
              <option value="openai_compatible">openai_compatible（OpenAI 兼容）</option>
              <option value="anthropic">anthropic（Anthropic）</option>
              <option value="codex_oauth">codex_oauth（Codex OAuth）</option>
            </select>
          </div>
          <div className="col-md-8">
            <label className="form-label">名称</label>
            <input className="form-control" value={createName} onChange={(e) => setCreateName(e.target.value)} placeholder="例如：OpenAI 主渠道" required />
          </div>
          <div className="col-md-8">
            <label className="form-label">接口基础地址</label>
            <input className="form-control font-monospace" value={createBaseURL} onChange={(e) => setCreateBaseURL(e.target.value)} placeholder="https://api.openai.com" required />
          </div>
          <div className="col-md-4">
            <label className="form-label">优先级</label>
            <input className="form-control" value={createPriority} onChange={(e) => setCreatePriority(e.target.value)} inputMode="numeric" placeholder="0" />
          </div>
          <div className="col-md-8">
            <label className="form-label">分组（groups，逗号分隔）</label>
            <input className="form-control font-monospace" value={createGroups} onChange={(e) => setCreateGroups(e.target.value)} placeholder="default" />
          </div>
          <div className="col-md-4 d-flex align-items-end">
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="createPromotion" checked={createPromotion} onChange={(e) => setCreatePromotion(e.target.checked)} />
              <label className="form-check-label" htmlFor="createPromotion">
                promotion（优先）
              </label>
            </div>
          </div>

          {createType === 'codex_oauth' ? (
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
                placeholder={createType === 'anthropic' ? 'sk-ant-...' : 'sk-...'}
                autoComplete="new-password"
              />
              <div className="form-text small text-muted">留空表示先创建渠道，再在“设置”中追加 Key。</div>
            </div>
          )}

          <div className="col-12">
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="createAllowServiceTier" checked={createAllowServiceTier} onChange={(e) => setCreateAllowServiceTier(e.target.checked)} />
              <label className="form-check-label" htmlFor="createAllowServiceTier">
                允许透传 <code>service_tier</code>
              </label>
            </div>
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="createDisableStore" checked={createDisableStore} onChange={(e) => setCreateDisableStore(e.target.checked)} />
              <label className="form-check-label" htmlFor="createDisableStore">
                禁用透传 <code>store</code>
              </label>
            </div>
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="createAllowSafetyIdentifier" checked={createAllowSafetyIdentifier} onChange={(e) => setCreateAllowSafetyIdentifier(e.target.checked)} />
              <label className="form-check-label" htmlFor="createAllowSafetyIdentifier">
                允许透传 <code>safety_identifier</code>
              </label>
            </div>
          </div>

          <div className="modal-footer border-top-0 px-0 pb-0">
            <button type="button" className="btn btn-light" data-bs-dismiss="modal">
              取消
            </button>
            <button type="submit" className="btn btn-primary px-4" disabled={loading}>
              创建
            </button>
          </div>
        </form>
      </BootstrapModal>

      <BootstrapModal
        id="editChannelModal"
        title={settingsChannelID ? `渠道设置：${settingsChannelName || `#${settingsChannelID}`}` : '渠道设置'}
        dialogClassName="modal-lg modal-dialog-scrollable"
        bodyClassName="bg-light"
        footer={
          <button type="button" className="btn btn-light" data-bs-dismiss="modal">
            关闭
          </button>
        }
        onHidden={() => {
          setSettingsChannelID(null);
          setSettingsChannelName('');
          setSettingsChannel(null);
          setSettingsLoading(false);
          setSettingsTab('common');

          setCredentials([]);
          setNewCredentialName('');
          setNewCredentialKey('');
          setKeyValue('');
          setCodexAccounts([]);
          setCodexCallbackURL('');
          setCodexManualAccountID('');
          setCodexManualEmail('');
          setCodexManualAccessToken('');
          setCodexManualRefreshToken('');
          setCodexManualIDToken('');
          setCodexManualExpiresAt('');

          setBindings([]);
          setSelectedModelIDs([]);
          setModelRedirects({});
          setModelsSaving(false);
          setModelSearch('');

          setMetaOpenAIOrganization('');
          setMetaTestModel('');
          setMetaTag('');
          setMetaWeight('0');
          setMetaAutoBan(true);
          setMetaRemark('');

          setSettingThinkingToContent(false);
          setSettingPassThroughBodyEnabled(false);
          setSettingProxy('');
          setSettingSystemPrompt('');
          setSettingSystemPromptOverride(false);

          setParamOverride('');
          setHeaderOverride('');
          setModelSuffixPreserve('');
          setRequestBodyWhitelist('');
          setRequestBodyBlacklist('');
          setStatusCodeMapping('');
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
              <span className="fw-semibold text-dark">{settingsChannel.name || `渠道 #${settingsChannel.id}`}</span>
              <span className="text-muted small">#{settingsChannel.id}</span>
              <span className="text-muted small">({channelTypeLabel(settingsChannel.type)})</span>
            </div>

            <ul className="nav nav-tabs mb-3">
              <li className="nav-item">
                <button type="button" className={`nav-link ${settingsTab === 'common' ? 'active' : ''}`} onClick={() => setSettingsTab('common')}>
                  常用
                </button>
              </li>
              <li className="nav-item">
                <button type="button" className={`nav-link ${settingsTab === 'keys' ? 'active' : ''}`} onClick={() => setSettingsTab('keys')}>
                  密钥
                </button>
              </li>
              <li className="nav-item">
                <button type="button" className={`nav-link ${settingsTab === 'models' ? 'active' : ''}`} onClick={() => setSettingsTab('models')}>
                  模型绑定
                </button>
              </li>
              <li className="nav-item">
                <button type="button" className={`nav-link ${settingsTab === 'advanced' ? 'active' : ''}`} onClick={() => setSettingsTab('advanced')}>
                  高级
                </button>
              </li>
            </ul>

            {settingsTab === 'common' ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">常用设置</div>
                  <div className="card-body">
                    <form
                      className="row g-3"
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannel({
                            id: settingsChannelID,
                            name: editName.trim(),
                            status: editStatus,
                            base_url: editBaseURL.trim(),
                            groups: editGroups.trim(),
                            priority: Number.parseInt(editPriority, 10) || 0,
                            promotion: editPromotion,
                          });
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice('已保存');
                          setSettingsChannelName(editName.trim());
                          await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
                      }}
                    >
                      <div className="col-md-8">
                        <label className="form-label fw-medium">名称</label>
                        <input className="form-control" value={editName} onChange={(e) => setEditName(e.target.value)} required />
                      </div>
                      <div className="col-md-4">
                        <label className="form-label fw-medium">状态</label>
                        <select className="form-select" value={editStatus} onChange={(e) => setEditStatus(Number.parseInt(e.target.value, 10) || 0)}>
                          <option value={1}>启用</option>
                          <option value={0}>禁用</option>
                        </select>
                      </div>
                      <div className="col-12">
                        <label className="form-label fw-medium">接口基础地址</label>
                        <input className="form-control font-monospace" value={editBaseURL} onChange={(e) => setEditBaseURL(e.target.value)} required />
                        <div className="form-text small text-muted">保存后立即生效；密钥与模型绑定可在本弹窗继续配置。</div>
                      </div>

                      <div className="col-12">
                        <label className="form-label fw-medium">分组设置</label>
                        <div className="card p-2" style={{ maxHeight: 260, overflowY: 'auto' }}>
                          {channelGroups.length === 0 ? (
                            <div className="text-muted small px-2 py-1">暂无分组（请先到“渠道分组”创建）。</div>
                          ) : (
                            channelGroups.map((g) => {
                              const selected = parseGroupsCSV(editGroups).includes(g.name);
                              const disabled = g.status !== 1 && !selected;
                              return (
                                <div className="form-check" key={g.id}>
                                  <input
                                    className="form-check-input"
                                    type="checkbox"
                                    id={`group_edit_${settingsChannelID}_${g.name}`}
                                    checked={selected}
                                    disabled={disabled}
                                    onChange={(e) => setEditGroups(toggleGroupsCSV(editGroups, g.name, e.target.checked))}
                                  />
                                  <label className="form-check-label w-100" htmlFor={`group_edit_${settingsChannelID}_${g.name}`}>
                                    {g.name} {g.status !== 1 ? <span className="badge bg-secondary ms-1 smaller">禁用</span> : null}
                                  </label>
                                </div>
                              );
                            })
                          )}
                        </div>
                        <div className="form-text small text-muted mt-2">用于上游调度选择渠道。</div>
                      </div>

                      <div className="col-md-6">
                        <label className="form-label fw-medium">优先级</label>
                        <input className="form-control" value={editPriority} onChange={(e) => setEditPriority(e.target.value)} inputMode="numeric" />
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
                        <button type="submit" className="btn btn-primary btn-sm" disabled={loading}>
                          <i className="ri-save-line me-1"></i>保存
                        </button>
                      </div>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">请求字段策略</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannel({
                            id: settingsChannelID,
                            allow_service_tier: editAllowServiceTier,
                            disable_store: editDisableStore,
                            allow_safety_identifier: editAllowSafetyIdentifier,
                          });
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice('已保存');
                          await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
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
                          <label className="form-check-label" htmlFor="editAllowServiceTier">
                            允许透传 <code>service_tier</code>
                          </label>
                          <div className="form-text small text-muted">可能触发上游额外计费；默认会过滤。</div>
                        </div>
                        <div className="form-check">
                          <input className="form-check-input" type="checkbox" id="editDisableStore" checked={editDisableStore} onChange={(e) => setEditDisableStore(e.target.checked)} />
                          <label className="form-check-label" htmlFor="editDisableStore">
                            禁用透传 <code>store</code>
                          </label>
                          <div className="form-text small text-muted">涉及数据存储授权；默认允许透传。</div>
                        </div>
                        <div className="form-check">
                          <input
                            className="form-check-input"
                            type="checkbox"
                            id="editAllowSafetyIdentifier"
                            checked={editAllowSafetyIdentifier}
                            onChange={(e) => setEditAllowSafetyIdentifier(e.target.checked)}
                          />
                          <label className="form-check-label" htmlFor="editAllowSafetyIdentifier">
                            允许透传 <code>safety_identifier</code>
                          </label>
                          <div className="form-text small text-muted">可能暴露用户信息；默认会过滤。</div>
                        </div>
                      </div>
                      <button type="submit" className="btn btn-primary btn-sm mt-3" disabled={loading}>
                        <i className="ri-save-line me-1"></i>保存策略
                      </button>
                    </form>
                  </div>
                </div>
              </div>
            ) : null}

            {settingsTab === 'keys' ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">密钥管理</div>
                  <div className="card-body">
                    {settingsChannel.type === 'codex_oauth' ? (
                      <div className="d-flex flex-column gap-3">
                        <div className="d-flex flex-wrap gap-2">
                          <button
                            type="button"
                            className="btn btn-sm btn-primary"
                            onClick={async () => {
                              if (!settingsChannelID) return;
                              setErr('');
                              setNotice('');
                              try {
                                const res = await startChannelCodexOAuth(settingsChannelID);
                                if (!res.success) throw new Error(res.message || '发起授权失败');
                                const authURL = (res.data?.auth_url || '').trim();
                                if (!authURL) throw new Error('未返回授权链接');
                                const popup = window.open(authURL, '_blank', 'noopener,noreferrer');
                                if (!popup) window.location.href = authURL;
                                setNotice('已发起 OAuth 授权，请在新窗口完成后返回此页面。');
                              } catch (e) {
                                setErr(e instanceof Error ? e.message : '发起授权失败');
                              }
                            }}
                          >
                            <i className="ri-external-link-line me-1"></i>发起授权（新窗口）
                          </button>
                          <button
                            type="button"
                            className="btn btn-sm btn-light border"
                            onClick={async () => {
                              if (!settingsChannelID) return;
                              setErr('');
                              setNotice('');
                              try {
                                const res = await refreshChannelCodexAccounts(settingsChannelID);
                                if (!res.success) throw new Error(res.message || '刷新失败');
                                await reloadCodexAccounts(settingsChannelID);
                                await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                setNotice(res.message || '已刷新');
                              } catch (e) {
                                setErr(e instanceof Error ? e.message : '刷新失败');
                              }
                            }}
                          >
                            <i className="ri-refresh-line me-1"></i>全部刷新
                          </button>
                        </div>

                        <div className="row g-3">
                          <div className="col-12 col-lg-6">
                            <div className="p-3 border rounded bg-light bg-opacity-10 h-100">
                              <div className="fw-semibold mb-2">完成授权（粘贴回调 URL）</div>
                              <form
                                className="d-flex flex-column gap-2"
                                onSubmit={async (e) => {
                                  e.preventDefault();
                                  if (!settingsChannelID) return;
                                  const callbackURL = codexCallbackURL.trim();
                                  if (!callbackURL) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await completeChannelCodexOAuth(settingsChannelID, callbackURL);
                                    if (!res.success) throw new Error(res.message || '完成授权失败');
                                    setCodexCallbackURL('');
                                    await reloadCodexAccounts(settingsChannelID);
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                    setNotice(res.message || '已完成授权');
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '完成授权失败');
                                  }
                                }}
                              >
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="http://localhost:1455/auth/callback?code=...&state=..."
                                  value={codexCallbackURL}
                                  onChange={(e) => setCodexCallbackURL(e.target.value)}
                                />
                                <div className="form-text small text-muted">如回跳失败，可粘贴浏览器地址栏完整 URL（含 code/state）。</div>
                                <button type="submit" className="btn btn-sm btn-light border" disabled={!codexCallbackURL.trim()}>
                                  完成授权
                                </button>
                              </form>
                            </div>
                          </div>

                          <div className="col-12 col-lg-6">
                            <div className="p-3 border rounded bg-light bg-opacity-10 h-100">
                              <div className="fw-semibold mb-2">手工录入（可选）</div>
                              <form
                                className="d-flex flex-column gap-2"
                                onSubmit={async (e) => {
                                  e.preventDefault();
                                  if (!settingsChannelID) return;
                                  const accessToken = codexManualAccessToken.trim();
                                  const refreshToken = codexManualRefreshToken.trim();
                                  if (!accessToken || !refreshToken) return;
                                  setErr('');
                                  setNotice('');
                                  try {
                                    const res = await createChannelCodexAccount(settingsChannelID, {
                                      account_id: codexManualAccountID.trim() || undefined,
                                      email: codexManualEmail.trim() || undefined,
                                      access_token: accessToken,
                                      refresh_token: refreshToken,
                                      id_token: codexManualIDToken.trim() || undefined,
                                      expires_at: codexManualExpiresAt.trim() || undefined,
                                    });
                                    if (!res.success) throw new Error(res.message || '保存失败');
                                    setCodexManualAccountID('');
                                    setCodexManualEmail('');
                                    setCodexManualAccessToken('');
                                    setCodexManualRefreshToken('');
                                    setCodexManualIDToken('');
                                    setCodexManualExpiresAt('');
                                    await reloadCodexAccounts(settingsChannelID);
                                    await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                    setNotice(res.message || '已添加账号');
                                  } catch (e) {
                                    setErr(e instanceof Error ? e.message : '保存失败');
                                  }
                                }}
                              >
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="account_id（可选，留空则尝试从 id_token 解析）"
                                  value={codexManualAccountID}
                                  onChange={(e) => setCodexManualAccountID(e.target.value)}
                                />
                                <input
                                  className="form-control form-control-sm"
                                  type="email"
                                  placeholder="邮箱（可选）"
                                  value={codexManualEmail}
                                  onChange={(e) => setCodexManualEmail(e.target.value)}
                                />
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="access_token"
                                  value={codexManualAccessToken}
                                  onChange={(e) => setCodexManualAccessToken(e.target.value)}
                                  required
                                />
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="refresh_token"
                                  value={codexManualRefreshToken}
                                  onChange={(e) => setCodexManualRefreshToken(e.target.value)}
                                  required
                                />
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="id_token（可选）"
                                  value={codexManualIDToken}
                                  onChange={(e) => setCodexManualIDToken(e.target.value)}
                                />
                                <input
                                  className="form-control form-control-sm font-monospace"
                                  placeholder="expires_at（可选，RFC3339）"
                                  value={codexManualExpiresAt}
                                  onChange={(e) => setCodexManualExpiresAt(e.target.value)}
                                />
                                <button type="submit" className="btn btn-sm btn-primary">
                                  保存账号
                                </button>
                              </form>
                            </div>
                          </div>
                        </div>

                        {codexAccounts.length === 0 ? (
                          <div className="text-muted small">暂无账号。</div>
                        ) : (
                          <div className="table-responsive">
                            <table className="table table-hover align-middle mb-0">
                              <thead className="table-light">
                                <tr>
                                  <th className="ps-3">账号</th>
                                  <th>订阅与额度</th>
                                  <th>状态</th>
                                  <th className="text-end pe-3">操作</th>
                                </tr>
                              </thead>
                              <tbody>
                                {codexAccounts.map((acc) => {
                                  const cooldownActive = !!acc.cooldown_until && new Date(acc.cooldown_until).getTime() > Date.now();
                                  return (
                                    <tr key={acc.id}>
                                      <td className="ps-3">
                                        <div className="d-flex flex-column">
                                          <span className="fw-semibold text-dark">{acc.email || '未绑定邮箱'}</span>
                                          <span className="text-muted small font-monospace">{acc.account_id || '-'}</span>
                                          <span className="text-muted smaller">更新：{fmtDateTime(acc.updated_at)}</span>
                                        </div>
                                      </td>
                                      <td>
                                        {acc.quota_error || acc.balance_error ? (
                                          <span className="text-danger small">{acc.quota_error || acc.balance_error}</span>
                                        ) : (
                                          <div className="d-flex flex-column small">
                                            <span>可用额度：{acc.balance_total_available_usd ? `$${acc.balance_total_available_usd}` : '-'}</span>
                                            <span className="text-muted">
                                              5h：{typeof acc.quota_primary_used_percent === 'number' ? `${acc.quota_primary_used_percent}%` : '-'} · 周：
                                              {typeof acc.quota_secondary_used_percent === 'number' ? `${acc.quota_secondary_used_percent}%` : '-'}
                                            </span>
                                          </div>
                                        )}
                                      </td>
                                      <td>
                                        {cooldownActive ? (
                                          <span className="badge rounded-pill bg-warning bg-opacity-10 text-warning px-2">冷却中</span>
                                        ) : acc.status === 1 ? (
                                          <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">运行中</span>
                                        ) : (
                                          <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">已禁用</span>
                                        )}
                                      </td>
                                      <td className="text-end pe-3">
                                        <div className="d-inline-flex gap-2">
                                          <button
                                            type="button"
                                            className="btn btn-sm btn-light border"
                                            onClick={async () => {
                                              if (!settingsChannelID) return;
                                              setErr('');
                                              setNotice('');
                                              try {
                                                const res = await refreshChannelCodexAccount(settingsChannelID, acc.id);
                                                if (!res.success) throw new Error(res.message || '刷新失败');
                                                await reloadCodexAccounts(settingsChannelID);
                                                setNotice(res.message || '已刷新');
                                              } catch (e) {
                                                setErr(e instanceof Error ? e.message : '刷新失败');
                                              }
                                            }}
                                          >
                                            刷新
                                          </button>
                                          <button
                                            type="button"
                                            className="btn btn-sm btn-light border text-danger"
                                            onClick={async () => {
                                              if (!settingsChannelID) return;
                                              if (!window.confirm('确认彻底删除该账号？且不可恢复。')) return;
                                              setErr('');
                                              setNotice('');
                                              try {
                                                const res = await deleteChannelCodexAccount(settingsChannelID, acc.id);
                                                if (!res.success) throw new Error(res.message || '删除失败');
                                                await reloadCodexAccounts(settingsChannelID);
                                                await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                                setNotice(res.message || '已删除');
                                              } catch (e) {
                                                setErr(e instanceof Error ? e.message : '删除失败');
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
                        <div className="form-text small text-muted mb-3">密钥将以明文存储，仅展示提示；删除不可恢复。</div>

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
                                      {c.name ? <span className="fw-semibold text-dark">{c.name}</span> : <span className="text-muted small">-</span>}
                                    </td>
                                    <td>
                                      <code className="text-secondary bg-light border p-2 rounded d-inline-block">{c.masked_key || '-'}</code>
                                    </td>
                                    <td>
                                      {c.status === 1 ? (
                                        <span className="badge rounded-pill bg-success bg-opacity-10 text-success px-2">
                                          <i className="ri-checkbox-circle-line me-1"></i>启用
                                        </span>
                                      ) : (
                                        <span className="badge rounded-pill bg-secondary bg-opacity-10 text-secondary px-2">
                                          <i className="ri-close-circle-line me-1"></i>禁用
                                        </span>
                                      )}
                                    </td>
                                    <td className="text-end pe-3">
                                      <button
                                        type="button"
                                        className="btn btn-sm btn-light border text-danger"
                                        onClick={async () => {
                                          if (!settingsChannelID) return;
                                          if (!window.confirm('确认彻底删除该凭证？且不可恢复。')) return;
                                          setErr('');
                                          setNotice('');
                                          try {
                                            const res = await deleteChannelCredential(settingsChannelID, c.id);
                                            if (!res.success) throw new Error(res.message || '删除失败');
                                            setNotice(res.message || '已删除');
                                            await reloadCredentials(settingsChannelID);
                                            await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                                          } catch (e) {
                                            setErr(e instanceof Error ? e.message : '删除失败');
                                          }
                                        }}
                                      >
                                        <i className="ri-delete-bin-line me-1"></i>删除
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
                            setErr('');
                            setNotice('');
                            try {
                              const res = await createChannelCredential(settingsChannelID, newCredentialKey.trim(), newCredentialName.trim() || undefined);
                              if (!res.success) throw new Error(res.message || '添加失败');
                              setNotice(res.message || '已添加');
                              setNewCredentialKey('');
                              setNewCredentialName('');
                              await reloadCredentials(settingsChannelID);
                              await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                            } catch (e) {
                              setErr(e instanceof Error ? e.message : '添加失败');
                            }
                          }}
                        >
                          <div className="col-md-4">
                            <label className="form-label fw-medium">备注名称（可选）</label>
                            <input className="form-control" value={newCredentialName} onChange={(e) => setNewCredentialName(e.target.value)} placeholder="例如：team-a-gpt4" />
                          </div>
                          <div className="col-md-8">
                            <label className="form-label fw-medium">API 密钥</label>
                            <input
                              className="form-control font-monospace"
                              value={newCredentialKey}
                              onChange={(e) => setNewCredentialKey(e.target.value)}
                              required
                              placeholder="sk-..."
                              autoComplete="new-password"
                            />
                            <div className="form-text small text-muted">密钥将以明文存储。</div>
                          </div>
                          <div className="col-12">
                            <button type="submit" className="btn btn-primary btn-sm" disabled={!newCredentialKey.trim()}>
                              <i className="ri-add-line me-1"></i>添加密钥
                            </button>
                          </div>
                        </form>
                      </>
                    )}
                  </div>
                </div>

                {settingsChannel.type === 'codex_oauth' ? null : (
                  <div className="card border-0 shadow-sm">
                    <div className="card-header bg-white fw-bold py-3">查看明文 Key（可选）</div>
                    <div className="card-body">
                      <div className="form-text small text-muted mb-3">仅 root 可见；读取第一个 credential 的明文 key，请妥善保管。</div>
                      <button
                        type="button"
                        className="btn btn-sm btn-light border"
                        onClick={async () => {
                          if (!settingsChannelID) return;
                          setErr('');
                          setNotice('');
                          setKeyValue('');
                          try {
                            const res = await getChannelKey(settingsChannelID);
                            if (!res.success) throw new Error(res.message || '获取失败');
                            setKeyValue(res.data?.key || '');
                          } catch (e) {
                            setErr(e instanceof Error ? e.message : '获取失败');
                          }
                        }}
                      >
                        <i className="ri-key-2-line me-1"></i>读取明文 Key
                      </button>

                      {keyValue ? (
                        <div className="mt-3">
                          <textarea className="form-control font-monospace" rows={4} value={keyValue} readOnly />
                          <div className="d-grid mt-2">
                            <button
                              type="button"
                              className="btn btn-primary btn-sm"
                              onClick={async () => {
                                try {
                                  await navigator.clipboard.writeText(keyValue);
                                  setNotice('已复制到剪贴板');
                                } catch {
                                  setErr('复制失败（浏览器不支持或无权限）');
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

            {settingsTab === 'models' ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">模型选择</div>
                  <div className="card-body">
                    <div className="d-flex flex-wrap align-items-end gap-2">
                      <div className="flex-grow-1">
                        <label className="form-label fw-medium mb-1">搜索模型</label>
                        <input className="form-control form-control-sm" value={modelSearch} onChange={(e) => setModelSearch(e.target.value)} placeholder="输入模型名称过滤" />
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
                            next.sort((a, b) => a.localeCompare(b, 'zh-CN'));
                            return next;
                          });
                          setModelRedirects((prev) => {
                            const next: Record<string, string> = { ...prev };
                            for (const id of ids) {
                              if (next[id] !== undefined) continue;
                              const b = bindingByPublicID.get(id);
                              if (!b) continue;
                              if (b.upstream_model.trim() === '') continue;
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
                      已选择 <span className="fw-semibold text-dark">{selectedModelIDs.length}</span> / {selectableModelIDs.length} 个（当前筛选：{filteredModelIDs.length} 个）
                    </div>

                    <div className="card p-2 mt-2" style={{ maxHeight: 320, overflowY: 'auto' }}>
                      {filteredModelIDs.length === 0 ? (
                        <div className="text-muted small p-2">没有匹配的模型。</div>
                      ) : (
                        filteredModelIDs.map((id) => (
                          <div className="form-check" key={id}>
                            <label className="form-check-label w-100 d-flex align-items-center" style={{ cursor: 'pointer' }}>
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
                                      next.sort((a, b) => a.localeCompare(b, 'zh-CN'));
                                      return next;
                                    }
                                    if (!checked && has) return prev.filter((m) => m !== id);
                                    return prev;
                                  });
                                  if (!checked) return;
                                  const b = bindingByPublicID.get(id);
                                  if (!b) return;
                                  if (b.upstream_model.trim() === '') return;
                                  if (b.upstream_model === id) return;
                                  setModelRedirects((prev) => {
                                    if (prev[id] !== undefined) return prev;
                                    return { ...prev, [id]: b.upstream_model };
                                  });
                                }}
                              />
                              <span className="font-monospace small user-select-all">{id}</span>
                            </label>
                          </div>
                        ))
                      )}
                    </div>

                    <div className="form-text small text-muted mt-2">选择该渠道允许使用的模型；下方可选配置“模型重定向”。</div>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">模型重定向</div>
                  <div className="card-body">
                    <div className="form-text small text-muted mb-3">对已选择的模型生效；留空表示不重定向（使用同名模型）。</div>
                    {selectedModelIDs.length === 0 ? (
                      <div className="text-muted small">请先在上方选择模型。</div>
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
                                  <span className="font-monospace small user-select-all">{id}</span>
                                </td>
                                <td>
                                  <input
                                    className="form-control form-control-sm font-monospace"
                                    value={modelRedirects[id] ?? ''}
                                    onChange={(e) => {
                                      const v = e.target.value;
                                      setModelRedirects((prev) => {
                                        const next = { ...prev };
                                        const trimmed = v.trim();
                                        if (trimmed === '' || trimmed === id) delete next[id];
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
                      <button type="button" className="btn btn-primary btn-sm" onClick={saveModelsConfig} disabled={modelsSaving}>
                        <i className="ri-save-line me-1"></i>保存模型配置
                      </button>
                    </div>
                  </div>
                </div>
              </div>
            ) : null}

            {settingsTab === 'advanced' ? (
              <div className="d-flex flex-column gap-3">
                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">渠道属性</div>
                  <div className="card-body">
                    <form
                      className="row g-3"
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelMeta(settingsChannelID, {
                            openai_organization: metaOpenAIOrganization.trim() || null,
                            test_model: metaTestModel.trim() || null,
                            tag: metaTag.trim() || null,
                            remark: metaRemark.trim() || null,
                            weight: Number.parseInt(metaWeight, 10) || 0,
                            auto_ban: metaAutoBan,
                          });
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                          await refresh({ start: usageStart.trim(), end: usageEnd.trim() });
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
                      }}
                    >
                      {settingsChannel.type === 'openai_compatible' ? (
                        <div className="col-md-6">
                          <label className="form-label fw-medium">OpenAI Organization（组织 ID）</label>
                          <input className="form-control font-monospace" value={metaOpenAIOrganization} onChange={(e) => setMetaOpenAIOrganization(e.target.value)} placeholder="org_xxx" />
                          <div className="form-text small text-muted">
                            会注入到上游请求头 <code>OpenAI-Organization</code>；可被“请求头覆盖”覆盖。
                          </div>
                        </div>
                      ) : null}
                      <div className="col-md-6">
                        <label className="form-label fw-medium">默认测试模型</label>
                        <input className="form-control font-monospace" value={metaTestModel} onChange={(e) => setMetaTestModel(e.target.value)} placeholder="留空=自动选择" />
                        <div className="form-text small text-muted">用于“测试”按钮：优先级高于模型绑定与默认值。</div>
                      </div>
                      <div className="col-md-6">
                        <label className="form-label fw-medium">标记（Tag）</label>
                        <input className="form-control" value={metaTag} onChange={(e) => setMetaTag(e.target.value)} placeholder="例如：prod-1" />
                        <div className="form-text small text-muted">用于标记/检索（仅保存，不参与调度）。</div>
                      </div>
                      <div className="col-md-6">
                        <label className="form-label fw-medium">权重（可选）</label>
                        <input className="form-control" type="number" min={0} value={metaWeight} onChange={(e) => setMetaWeight(e.target.value)} />
                        <div className="form-text small text-muted">当前不参与调度（Realms 调度以分组/优先级/推荐为准）。</div>
                      </div>
                      <div className="col-md-6">
                        <label className="form-label fw-medium">自动封禁</label>
                        <select className="form-select" value={metaAutoBan ? '1' : '0'} onChange={(e) => setMetaAutoBan(e.target.value === '1')}>
                          <option value="1">启用</option>
                          <option value="0">禁用</option>
                        </select>
                        <div className="form-text small text-muted">禁用后：失败不会封禁该渠道（credential 冷却仍生效）。</div>
                      </div>
                      <div className="col-12">
                        <label className="form-label fw-medium">备注</label>
                        <input className="form-control" value={metaRemark} onChange={(e) => setMetaRemark(e.target.value)} placeholder="可选" />
                        <div className="form-text small text-muted">仅用于管理端备注（不参与调度）。</div>
                      </div>
                      <div className="col-12">
                        <button type="submit" className="btn btn-primary btn-sm">
                          <i className="ri-save-line me-1"></i>保存
                        </button>
                      </div>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">请求处理设置</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelSetting(settingsChannelID, {
                            thinking_to_content: settingThinkingToContent,
                            pass_through_body_enabled: settingPassThroughBodyEnabled,
                            proxy: settingProxy,
                            system_prompt: settingSystemPrompt,
                            system_prompt_override: settingSystemPromptOverride,
                          });
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
                      }}
                    >
                      <div className="d-flex flex-column gap-2">
                        <div className="form-check">
                          <input
                            className="form-check-input"
                            type="checkbox"
                            id="setting_thinking_to_content"
                            checked={settingThinkingToContent}
                            onChange={(e) => setSettingThinkingToContent(e.target.checked)}
                          />
                          <label className="form-check-label" htmlFor="setting_thinking_to_content">
                            推理内容合并到正文
                          </label>
                          <div className="form-text small text-muted">
                            将流式 <code>reasoning_content</code> 转为 <code>&lt;think&gt;...&lt;/think&gt;</code> 并拼接到 <code>content</code> 中返回。
                          </div>
                        </div>
                        <div className="form-check">
                          <input
                            className="form-check-input"
                            type="checkbox"
                            id="setting_pass_through_body_enabled"
                            checked={settingPassThroughBodyEnabled}
                            onChange={(e) => setSettingPassThroughBodyEnabled(e.target.checked)}
                          />
                          <label className="form-check-label" htmlFor="setting_pass_through_body_enabled">
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
                        <div className="form-text small text-muted">按渠道指定上游网络代理。</div>
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
                          对 <code>/v1/chat/completions</code> 注入 system 消息；对 <code>/v1/responses</code> 注入 instructions。
                        </div>
                      </div>

                      <div className="form-check mt-2">
                        <input
                          className="form-check-input"
                          type="checkbox"
                          id="setting_system_prompt_override"
                          checked={settingSystemPromptOverride}
                          onChange={(e) => setSettingSystemPromptOverride(e.target.checked)}
                        />
                        <label className="form-check-label" htmlFor="setting_system_prompt_override">
                          始终拼接系统提示词
                        </label>
                        <div className="form-text small text-muted">当请求已包含 system/instructions 时：是否将“系统提示词”拼接到最前。</div>
                      </div>

                      <button type="submit" className="btn btn-primary btn-sm mt-3">
                        <i className="ri-save-line me-1"></i>保存
                      </button>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">参数改写</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelParamOverride(settingsChannelID, paramOverride);
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
                      }}
                    >
                      <textarea
                        className="form-control font-monospace"
                        rows={10}
                        value={paramOverride}
                        onChange={(e) => setParamOverride(e.target.value)}
                        placeholder='{"operations":[{"path":"metadata.channel","mode":"set","value":"example"}]}'
                      />
                      <div className="form-text small text-muted mt-2">留空表示禁用。JSON 必须为对象，会在转发前按渠道应用。</div>
                      <button type="submit" className="btn btn-primary btn-sm mt-3">
                        <i className="ri-save-line me-1"></i>保存改写
                      </button>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">请求头覆盖</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelHeaderOverride(settingsChannelID, headerOverride);
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
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
                        留空表示禁用。JSON 必须为对象，value 必须为字符串；支持变量 <code>{'{api_key}'}</code>（会替换为该渠道实际使用的上游 key/token）。
                      </div>
                      <button type="submit" className="btn btn-primary btn-sm mt-3">
                        <i className="ri-save-line me-1"></i>保存
                      </button>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">模型后缀保护名单</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelModelSuffixPreserve(settingsChannelID, modelSuffixPreserve);
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
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
                        留空表示禁用。JSON 必须为数组；命中时跳过模型后缀解析（<code>-low/-medium/-high/-minimal/-none/-xhigh</code>）。
                      </div>
                      <button type="submit" className="btn btn-primary btn-sm mt-3">
                        <i className="ri-save-line me-1"></i>保存
                      </button>
                    </form>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">请求体黑白名单</div>
                  <div className="card-body">
                    <div className="row g-3">
                      <div className="col-12 col-lg-6">
                        <form
                          onSubmit={async (e) => {
                            e.preventDefault();
                            if (!settingsChannelID) return;
                            setErr('');
                            setNotice('');
                            try {
                              const res = await updateChannelRequestBodyWhitelist(settingsChannelID, requestBodyWhitelist);
                              if (!res.success) throw new Error(res.message || '保存失败');
                              setNotice(res.message || '已保存白名单');
                            } catch (e) {
                              setErr(e instanceof Error ? e.message : '保存失败');
                            }
                          }}
                        >
                          <label className="form-label fw-medium mb-1">白名单（仅保留）</label>
                          <textarea
                            className="form-control font-monospace"
                            rows={8}
                            value={requestBodyWhitelist}
                            onChange={(e) => setRequestBodyWhitelist(e.target.value)}
                            placeholder='["model","input","max_output_tokens","metadata.channel"]'
                          />
                          <div className="form-text small text-muted mt-2">
                            留空表示禁用。JSON 必须为数组，每项为 JSON path（gjson/sjson 语法）；启用后会先“仅保留白名单字段”，再应用黑名单与参数改写。
                          </div>
                          <button type="submit" className="btn btn-primary btn-sm mt-3">
                            <i className="ri-save-line me-1"></i>保存白名单
                          </button>
                        </form>
                      </div>
                      <div className="col-12 col-lg-6">
                        <form
                          onSubmit={async (e) => {
                            e.preventDefault();
                            if (!settingsChannelID) return;
                            setErr('');
                            setNotice('');
                            try {
                              const res = await updateChannelRequestBodyBlacklist(settingsChannelID, requestBodyBlacklist);
                              if (!res.success) throw new Error(res.message || '保存失败');
                              setNotice(res.message || '已保存黑名单');
                            } catch (e) {
                              setErr(e instanceof Error ? e.message : '保存失败');
                            }
                          }}
                        >
                          <label className="form-label fw-medium mb-1">黑名单（删除字段）</label>
                          <textarea
                            className="form-control font-monospace"
                            rows={8}
                            value={requestBodyBlacklist}
                            onChange={(e) => setRequestBodyBlacklist(e.target.value)}
                            placeholder='["metadata.sensitive","user","store"]'
                          />
                          <div className="form-text small text-muted mt-2">
                            留空表示禁用。JSON 必须为数组，每项为 JSON path（gjson/sjson 语法）；会在每次 selection 转发前按渠道应用。
                          </div>
                          <button type="submit" className="btn btn-primary btn-sm mt-3">
                            <i className="ri-save-line me-1"></i>保存黑名单
                          </button>
                        </form>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="card border-0 shadow-sm">
                  <div className="card-header bg-white fw-bold py-3">状态码映射</div>
                  <div className="card-body">
                    <form
                      onSubmit={async (e) => {
                        e.preventDefault();
                        if (!settingsChannelID) return;
                        setErr('');
                        setNotice('');
                        try {
                          const res = await updateChannelStatusCodeMapping(settingsChannelID, statusCodeMapping);
                          if (!res.success) throw new Error(res.message || '保存失败');
                          setNotice(res.message || '已保存');
                        } catch (e) {
                          setErr(e instanceof Error ? e.message : '保存失败');
                        }
                      }}
                    >
                      <textarea
                        className="form-control font-monospace"
                        rows={6}
                        value={statusCodeMapping}
                        onChange={(e) => setStatusCodeMapping(e.target.value)}
                        placeholder='{"401":"200","429":"200"}'
                      />
                      <div className="form-text small text-muted mt-2">留空表示禁用。仅影响对下游返回的 HTTP 状态码，不影响内部 failover 判定与日志/用量记录。</div>
                      <button type="submit" className="btn btn-primary btn-sm mt-3">
                        <i className="ri-save-line me-1"></i>保存
                      </button>
                    </form>
                  </div>
                </div>
              </div>
            ) : null}
          </>
        )}
      </BootstrapModal>
    </div>
  );
}

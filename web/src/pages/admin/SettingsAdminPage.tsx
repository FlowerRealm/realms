import { useCallback, useEffect, useMemo, useState } from 'react';

import { getAdminSettings, resetAdminSettings, updateAdminSettings, type AdminSettings, type FeatureBanItem, type UpdateAdminSettingsRequest } from '../../api/admin/settings';
import { AutoSaveIndicator } from '../../components/AutoSaveIndicator';
import { SegmentedFrame } from '../../components/SegmentedFrame';
import { TimeZoneInput } from '../../components/TimeZoneInput';
import { useAutoSave } from '../../hooks/useAutoSave';

type TabKey = 'features' | 'base' | 'email' | 'billing';

function boolBadge(on: boolean): string {
  return on ? 'badge bg-light text-secondary border' : 'badge bg-light text-secondary border';
}

function initForm(s: AdminSettings): UpdateAdminSettingsRequest {
  const featureEnabled: Record<string, boolean> = {};
  for (const g of s.feature_ban_groups || []) {
    for (const item of g.items || []) {
      featureEnabled[item.key] = !item.disabled;
    }
  }

  return {
    site_base_url: s.site_base_url || '',
    allow_open_registration: !!s.allow_open_registration,
    admin_time_zone: s.admin_time_zone || '',

    email_verification_enable: !!s.email_verification_enabled,

    smtp_server: s.smtp_server || '',
    smtp_port: s.smtp_port || 587,
    smtp_ssl_enabled: !!s.smtp_ssl_enabled,
    smtp_account: s.smtp_account || '',
    smtp_from: s.smtp_from || '',
    smtp_token: '',

    billing_enable_pay_as_you_go: !!s.billing_enable_pay_as_you_go,
    billing_min_topup_cny: s.billing_min_topup_cny || '',
    billing_credit_usd_per_cny: s.billing_credit_usd_per_cny || '',
    billing_paygo_price_multiplier: s.billing_paygo_price_multiplier || '1',

    feature_enabled: featureEnabled,
  };
}

function featureToggleDisabled(item: FeatureBanItem): boolean {
  if (!item.editable) return true;
  if (item.forced_by_build) return true;
  return false;
}

export function SettingsAdminPage() {
  const [tab, setTab] = useState<TabKey>('features');

  const [settings, setSettings] = useState<AdminSettings | null>(null);
  const [form, setForm] = useState<UpdateAdminSettingsRequest | null>(null);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState('');
  const [notice, setNotice] = useState('');
  const [autosaveResetKey, setAutosaveResetKey] = useState(0);

  const showBillingTab = useMemo(() => {
    const v = settings?.features?.billing_disabled;
    return !(typeof v === 'boolean' && v);
  }, [settings]);

  const visibleFeatureBanGroups = useMemo(() => {
    return settings?.feature_ban_groups || [];
  }, [settings?.feature_ban_groups]);

  async function refresh() {
    setErr('');
    setNotice('');
    setLoading(true);
    try {
      const res = await getAdminSettings();
      if (!res.success) throw new Error(res.message || '加载失败');
      const s = res.data || null;
      setSettings(s);
      if (s) setForm(initForm(s));
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败');
      setSettings(null);
      setForm(null);
    } finally {
      setLoading(false);
    }
  }

  const refreshSilent = useCallback(async () => {
    try {
      const res = await getAdminSettings();
      if (!res.success) return;
      const s = res.data || null;
      setSettings(s);
      if (s) {
        setForm(initForm(s));
        setAutosaveResetKey((x) => x + 1);
      }
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, []);

  const autosaveTrackForm = useMemo(() => {
    if (!form) return null;
    return { ...form, smtp_token: '' };
  }, [form]);

  const autosave = useAutoSave({
    enabled: !!form && !loading && !saving,
    value: form as UpdateAdminSettingsRequest,
    trackValue: autosaveTrackForm,
    resetKey: autosaveResetKey,
    validate: (v) => {
      if (!v) return '未加载';
      if (typeof v.smtp_port !== 'number' || !Number.isFinite(v.smtp_port) || v.smtp_port <= 0) return 'SMTP port 不合法';
      return '';
    },
    save: async (v) => {
      setErr('');
      const res = await updateAdminSettings(v);
      if (!res.success) throw new Error(res.message || '保存失败');
      setNotice(res.message || '已自动保存');
      if ((v.smtp_token || '').trim()) {
        setForm((prev) => (prev ? { ...prev, smtp_token: '' } : prev));
      }
    },
    afterSave: async () => {
      await refreshSilent();
    },
  });

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">系统设置</h3>
          <p className="text-muted small mb-0">少量运行期配置持久化到数据库；启动期只保留最少的必要环境变量。</p>
        </div>
        <AutoSaveIndicator status={autosave.status} blockedReason={autosave.blockedReason} error={autosave.error} onRetry={autosave.retry} className="small" />
      </div>

      {notice ? (
        <div className="alert alert-success d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">check_circle</span>
          <div>{notice}</div>
        </div>
      ) : null}

      {err ? (
        <div className="alert alert-danger d-flex align-items-center" role="alert">
          <span className="me-2 material-symbols-rounded">warning</span>
          <div>{err}</div>
        </div>
      ) : null}

      {loading ? (
        <div className="text-muted">加载中…</div>
      ) : !settings || !form ? (
        <div className="alert alert-warning">未找到设置。</div>
      ) : (
        <SegmentedFrame>
          <div className="d-flex flex-column flex-md-row align-items-center justify-content-between gap-2">
            <ul className="nav nav-pills" role="tablist">
              <li className="nav-item" role="presentation">
                <button className={`nav-link d-flex align-items-center gap-2 ${tab === 'features' ? 'active' : ''}`} type="button" onClick={() => setTab('features')}>
                  <i className="ri-function-line"></i> 功能开关
                </button>
              </li>
              <li className="nav-item" role="presentation">
                <button className={`nav-link d-flex align-items-center gap-2 ${tab === 'base' ? 'active' : ''}`} type="button" onClick={() => setTab('base')}>
                  <i className="ri-settings-4-line"></i> 基础设置
                </button>
              </li>
              <li className="nav-item" role="presentation">
                <button className={`nav-link d-flex align-items-center gap-2 ${tab === 'email' ? 'active' : ''}`} type="button" onClick={() => setTab('email')}>
                  <i className="ri-mail-settings-line"></i> 邮件服务
                </button>
              </li>
              {showBillingTab ? (
                <li className="nav-item" role="presentation">
                  <button className={`nav-link d-flex align-items-center gap-2 ${tab === 'billing' ? 'active' : ''}`} type="button" onClick={() => setTab('billing')}>
                    <i className="ri-bank-card-line"></i> 计费支付
                  </button>
                </li>
              ) : null}
            </ul>

            <div className="d-flex gap-2">
              <button
                type="button"
                className="btn btn-outline-secondary btn-sm"
                onClick={async () => {
                  if (!window.confirm('确认恢复默认？此操作会清理数据库中的设置覆盖。')) return;
                  setErr('');
                  setNotice('');
                  setSaving(true);
                  try {
                    const res = await resetAdminSettings();
                    if (!res.success) throw new Error(res.message || '恢复默认失败');
                    setNotice(res.message || '已恢复默认');
                    await refresh();
                  } catch (e) {
                    setErr(e instanceof Error ? e.message : '恢复默认失败');
                  } finally {
                    setSaving(false);
                  }
                }}
                disabled={saving}
              >
                恢复默认
              </button>
            </div>
          </div>

          <div>
            {tab === 'features' ? (
              <div>
                <div className="mb-3">
                  <h5 className="fw-semibold mb-1">功能禁用</h5>
                  <div className="text-muted small">开关默认开启（功能启用）；关闭则表示禁用该功能（隐藏入口并在后端返回 404）。</div>
                </div>

                {visibleFeatureBanGroups.map((g) => (
                  <div key={g.title} className="mb-4">
                    <h6 className="fw-medium text-dark border-bottom pb-2 mb-3">{g.title}</h6>
                    <div className="row g-3">
                      {g.items.map((item) => {
                        const enabled = !!form.feature_enabled[item.key];
                        const disabled = featureToggleDisabled(item);
                        return (
                          <div key={item.key} className="col-md-6 col-lg-4">
                            <div className="d-flex align-items-center justify-content-between p-3 border bg-white h-100">
                              <div className="d-flex flex-column" style={{ minWidth: 0 }}>
                                <div className="fw-medium mb-1 text-truncate" title={item.label}>
                                  {item.label}
                                </div>
                                <div className="text-muted small text-truncate" title={item.hint}>
                                  {item.hint}
                                </div>
                                <div className="mt-1">
                                  {item.forced_by_build ? (
                                    <span className="badge bg-warning-subtle text-warning-emphasis border smaller">
                                      编译期剔除
                                    </span>
                                  ) : item.override ? (
                                    <span className="badge bg-light text-dark border smaller">
                                      已覆盖
                                    </span>
                                  ) : null}
                                </div>
                              </div>
                              <div className="form-check form-switch ms-2">
                                <input
                                  className="form-check-input"
                                  type="checkbox"
                                  role="switch"
                                  checked={enabled}
                                  disabled={disabled}
                                  onChange={(e) => {
                                    const next = { ...form, feature_enabled: { ...form.feature_enabled, [item.key]: e.target.checked } };
                                    setForm(next);
                                  }}
                                />
                              </div>
                            </div>
                          </div>
                        );
                      })}
                    </div>
                  </div>
                ))}
              </div>
            ) : null}

            {tab === 'base' ? (
              <div className="row g-4">
                <div className="col-12 col-lg-6">
                  <div className="card">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">基础设置</h5>

                      <div className="mb-3">
                        <label className="form-label fw-medium d-flex justify-content-between">
                          <span>Site Base URL</span>
                          {settings.site_base_url_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : <span className="badge bg-light text-dark border">请求推导</span>}
                        </label>
                        <input
                          className="form-control"
                          value={form.site_base_url}
                          onChange={(e) => setForm({ ...form, site_base_url: e.target.value })}
                          placeholder="https://example.com"
                        />
                        <div className="form-text small text-muted">
                          生效值：<code className="user-select-all">{settings.site_base_url_effective}</code>
                          {settings.site_base_url_invalid ? <span className="badge bg-warning ms-2">不合法</span> : null}
                        </div>
                      </div>

                      <div className="mb-3">
                        <div className="d-flex align-items-center justify-content-between">
                          <div>
                            <label className="form-label fw-medium d-flex align-items-center gap-2 mb-1">
                              <span>开放注册</span>
                              {settings.allow_open_registration_override ? (
                                <span className="badge bg-light text-dark border">数据库覆盖</span>
                              ) : (
                                <span className="badge bg-light text-dark border">系统默认</span>
                              )}
                            </label>
                            <div className="form-text small text-muted">
                              无用户时默认开放；创建首个 root 后默认关闭，可在这里重新开启。
                            </div>
                          </div>
                          <div className="form-check form-switch ms-3">
                            <input
                              className="form-check-input"
                              type="checkbox"
                              role="switch"
                              checked={form.allow_open_registration}
                              onChange={(e) => setForm({ ...form, allow_open_registration: e.target.checked })}
                              style={{ width: '3em', height: '1.5em' }}
                            />
                          </div>
                        </div>
                      </div>

                      <div className="mb-3">
                        <label className="form-label fw-medium d-flex justify-content-between">
                          <span>系统时区（Timezone）</span>
                          {settings.admin_time_zone_override ? (
                            <span className="badge bg-light text-dark border">数据库覆盖</span>
                          ) : (
                            <span className="badge bg-light text-dark border">系统默认</span>
                          )}
                        </label>
                        <TimeZoneInput value={form.admin_time_zone} onChange={(v) => setForm({ ...form, admin_time_zone: v })} className="form-control mb-2" />
                        <div className="form-text small text-muted">
                          生效值：<code className="user-select-all">{settings.admin_time_zone_effective}</code>
                          {settings.admin_time_zone_invalid ? <span className="badge bg-warning ms-2">不合法</span> : null}
                        </div>
                      </div>
                    </div>
                  </div>
                </div>

                <div className="col-12 col-lg-6">
                  <div className="card">
                    <div className="card-body">
                        <h5 className="fw-semibold mb-3">启动期配置清单（只读）</h5>
                        <div className="form-text small text-muted mb-3">
                          以下配置项只属于启动期；需要通过环境变量配置，并在重启后生效。
                        </div>
                      <div className="row g-2">
                        {settings.startup_config_keys.map((k) => (
                          <div key={k} className="col-md-6">
                            <code className="user-select-all">{k}</code>
                          </div>
                        ))}
                        {settings.startup_config_keys.length === 0 ? <div className="text-muted small">（空）</div> : null}
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : null}

            {tab === 'email' ? (
              <div className="row g-4">
                <div className="col-12">
                  <div className="card">
                    <div className="card-body d-flex align-items-center justify-content-between">
                      <div>
                        <h5 className="fw-semibold mb-1">邮箱验证码</h5>
                        <p className="text-muted small mb-0">
                          开启后注册页将要求验证码，并开放 <code>/api/email/verification/send</code>。
                          {settings.email_verification_override ? <span className="badge bg-light text-dark border ms-2">数据库覆盖</span> : <span className="badge bg-light text-dark border ms-2">系统默认</span>}
                        </p>
                      </div>
                      <div className="form-check form-switch">
                        <input
                          className="form-check-input"
                          type="checkbox"
                          role="switch"
                          checked={form.email_verification_enable}
                          onChange={(e) => setForm({ ...form, email_verification_enable: e.target.checked })}
                          style={{ width: '3em', height: '1.5em' }}
                        />
                      </div>
                    </div>
                  </div>
                </div>

                <div className="col-12">
                  <div className="card">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">SMTP 配置</h5>

                      <div className="row g-3">
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Host</span>
                            {settings.smtp_server_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_server} onChange={(e) => setForm({ ...form, smtp_server: e.target.value })} placeholder="smtp.example.com" />
                        </div>
                        <div className="col-md-3">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Port</span>
                            {settings.smtp_port_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input
                            className="form-control"
                            value={String(form.smtp_port || 0)}
                            onChange={(e) => setForm({ ...form, smtp_port: Number.parseInt(e.target.value, 10) || 0 })}
                            inputMode="numeric"
                            placeholder="587"
                          />
                        </div>
                        <div className="col-md-3">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>SSL/TLS</span>
                            {settings.smtp_ssl_enabled_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <div className="form-check form-switch mt-2">
                            <input
                              className="form-check-input"
                              type="checkbox"
                              role="switch"
                              checked={form.smtp_ssl_enabled}
                              onChange={(e) => setForm({ ...form, smtp_ssl_enabled: e.target.checked })}
                            />
                            <label className="form-check-label">启用 SSL</label>
                          </div>
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Account</span>
                            {settings.smtp_account_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_account} onChange={(e) => setForm({ ...form, smtp_account: e.target.value })} placeholder="noreply@example.com" />
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>From Address</span>
                            {settings.smtp_from_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_from} onChange={(e) => setForm({ ...form, smtp_from: e.target.value })} placeholder="Name <addr@example.com>" />
                          <div className="form-text small text-muted">格式："Name &lt;addr@example.com&gt;"</div>
                        </div>
                        <div className="col-12">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Password / Token</span>
                            {settings.smtp_token_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input
                            className="form-control"
                            value={form.smtp_token}
                            onChange={(e) => setForm({ ...form, smtp_token: e.target.value })}
                            placeholder="留空表示保持不变"
                            type="password"
                            autoComplete="new-password"
                          />
                          <div className="form-text small text-muted">
                            当前状态：{settings.smtp_token_set ? '已设置' : '未设置'}（密钥不回显）。
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : null}

            {tab === 'billing' ? (
              <div className="row g-4">
                <div className="col-12">
                  <div className="card">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">计费</h5>

                      <div className="mb-3 d-flex align-items-center justify-content-between">
                        <div>
                          <div className="fw-medium">按量计费</div>
                          <div className="text-muted small">
                            {settings.billing_enable_pay_as_you_go_override ? <span className="badge bg-light text-dark border me-2">数据库覆盖</span> : null}
                            当前：<span className={boolBadge(form.billing_enable_pay_as_you_go)}>{form.billing_enable_pay_as_you_go ? '启用' : '禁用'}</span>
                          </div>
                        </div>
                        <div className="form-check form-switch">
                          <input
                            className="form-check-input"
                            type="checkbox"
                            role="switch"
                            checked={form.billing_enable_pay_as_you_go}
                            onChange={(e) => setForm({ ...form, billing_enable_pay_as_you_go: e.target.checked })}
                            style={{ width: '3em', height: '1.5em' }}
                          />
                        </div>
                      </div>

                      <div className="row g-3">
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>最低充值（CNY）</span>
                            {settings.billing_min_topup_cny_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.billing_min_topup_cny} onChange={(e) => setForm({ ...form, billing_min_topup_cny: e.target.value })} placeholder="10.00" />
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>充值汇率（USD / CNY）</span>
                            {settings.billing_credit_usd_per_cny_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <input
                            className="form-control"
                            value={form.billing_credit_usd_per_cny}
                            onChange={(e) => setForm({ ...form, billing_credit_usd_per_cny: e.target.value })}
                            placeholder="0.15"
                          />
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>按量计费倍率</span>
                            {settings.billing_paygo_price_multiplier_override ? <span className="badge bg-light text-dark border">数据库覆盖</span> : null}
                          </label>
                          <div className="input-group">
                            <span className="input-group-text">×</span>
                            <input
                              className="form-control"
                              value={form.billing_paygo_price_multiplier}
                              onChange={(e) => setForm({ ...form, billing_paygo_price_multiplier: e.target.value })}
                              placeholder="1"
                              inputMode="decimal"
                            />
                          </div>
                          <div className="form-text small text-muted">留空表示恢复默认（×1）。最终计费倍率 = PayGO 倍率 × 最终成功渠道组倍率。</div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              </div>
            ) : null}
          </div>
        </SegmentedFrame>
      )}
    </div>
  );
}

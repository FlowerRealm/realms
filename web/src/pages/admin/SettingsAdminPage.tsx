import { useEffect, useMemo, useState } from 'react';

import { getAdminSettings, resetAdminSettings, updateAdminSettings, type AdminSettings, type FeatureBanItem, type UpdateAdminSettingsRequest } from '../../api/admin/settings';
import { TimeZoneInput } from '../../components/TimeZoneInput';

type TabKey = 'features' | 'base' | 'email' | 'billing';

function boolBadge(on: boolean): string {
  return on ? 'badge bg-success bg-opacity-10 text-success' : 'badge bg-secondary bg-opacity-10 text-secondary';
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
  if (item.forced_by_self_mode) return true;
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

  const showBillingTab = useMemo(() => {
    const v = settings?.features?.billing_disabled;
    return !(typeof v === 'boolean' && v);
  }, [settings]);

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

  useEffect(() => {
    void refresh();
  }, []);

  return (
    <div className="fade-in-up">
      <div className="d-flex justify-content-between align-items-center mb-4">
        <div>
          <h3 className="mb-1 fw-bold">系统设置</h3>
          <p className="text-muted small mb-0">少量运行期配置（持久化到数据库，优先于启动期默认值）。</p>
        </div>
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
        <div className="card border-0">
          <div className="card-body">
            <div className="d-flex flex-column flex-md-row align-items-center justify-content-between gap-2 mb-4">
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
                <button
                  type="button"
                  className="btn btn-primary btn-sm"
                  onClick={async () => {
                    if (!form) return;
                    setErr('');
                    setNotice('');
                    setSaving(true);
                    try {
                      const res = await updateAdminSettings(form);
                      if (!res.success) throw new Error(res.message || '保存失败');
                      setNotice(res.message || '已保存');
                      await refresh();
                    } catch (e) {
                      setErr(e instanceof Error ? e.message : '保存失败');
                    } finally {
                      setSaving(false);
                    }
                  }}
                  disabled={saving}
                >
                  {saving ? '保存中…' : '保存'}
                </button>
              </div>
            </div>

            {tab === 'features' ? (
              <div>
                <div className="mb-3">
                  <h5 className="fw-semibold mb-1">功能禁用</h5>
                  <div className="text-muted small">开关默认开启（功能启用）；关闭则表示禁用该功能（隐藏入口并在后端返回 404）。</div>
                </div>

                {settings.feature_ban_groups.map((g) => (
                  <div key={g.title} className="mb-4">
                    <h6 className="fw-medium text-dark border-bottom pb-2 mb-3">{g.title}</h6>
                    <div className="row g-3">
                      {g.items.map((item) => {
                        const enabled = !!form.feature_enabled[item.key];
                        const disabled = featureToggleDisabled(item);
                        return (
                          <div key={item.key} className="col-md-6 col-lg-4">
                            <div className="d-flex align-items-center justify-content-between p-3 border rounded bg-light bg-opacity-10 h-100">
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
                                  ) : item.forced_by_self_mode ? (
                                    <span className="badge bg-warning-subtle text-warning-emphasis border smaller">
                                      自用模式强制
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
                  <div className="card border-0 bg-light">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">基础设置</h5>

                      <div className="mb-3">
                        <label className="form-label fw-medium d-flex justify-content-between">
                          <span>Site Base URL</span>
                          {settings.site_base_url_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
                        <label className="form-label fw-medium d-flex justify-content-between">
                          <span>系统时区（Timezone）</span>
                          {settings.admin_time_zone_override ? (
                            <span className="badge bg-light text-dark border">界面覆盖</span>
                          ) : (
                            <span className="badge bg-light text-dark border">默认值</span>
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
                  <div className="card border-0 bg-light">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">启动期配置清单（只读）</h5>
                      <div className="form-text small text-muted mb-3">
                        以下配置项仅能通过环境变量（如 <code>.env</code> / <code>REALMS_*</code>）在启动前配置；修改后需要重启服务生效。
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
                  <div className="card border-0 bg-light">
                    <div className="card-body d-flex align-items-center justify-content-between">
                      <div>
                        <h5 className="fw-semibold mb-1">邮箱验证码</h5>
                        <p className="text-muted small mb-0">
                          开启后注册页将要求验证码，并开放 <code>/api/email/verification/send</code>。
                          {settings.email_verification_override ? <span className="badge bg-light text-dark border ms-2">界面覆盖</span> : <span className="badge bg-light text-dark border ms-2">启动默认</span>}
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
                  <div className="card border-0 bg-light">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">SMTP 配置</h5>

                      <div className="row g-3">
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Host</span>
                            {settings.smtp_server_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_server} onChange={(e) => setForm({ ...form, smtp_server: e.target.value })} placeholder="smtp.example.com" />
                        </div>
                        <div className="col-md-3">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Port</span>
                            {settings.smtp_port_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
                            {settings.smtp_ssl_enabled_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
                            {settings.smtp_account_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_account} onChange={(e) => setForm({ ...form, smtp_account: e.target.value })} placeholder="noreply@example.com" />
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>From Address</span>
                            {settings.smtp_from_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.smtp_from} onChange={(e) => setForm({ ...form, smtp_from: e.target.value })} placeholder="Name <addr@example.com>" />
                          <div className="form-text small text-muted">格式："Name &lt;addr@example.com&gt;"</div>
                        </div>
                        <div className="col-12">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>Password / Token</span>
                            {settings.smtp_token_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
                  <div className="card border-0 bg-light">
                    <div className="card-body">
                      <h5 className="fw-semibold mb-3">计费</h5>

                      <div className="mb-3 d-flex align-items-center justify-content-between">
                        <div>
                          <div className="fw-medium">按量计费</div>
                          <div className="text-muted small">
                            {settings.billing_enable_pay_as_you_go_override ? <span className="badge bg-light text-dark border me-2">界面覆盖</span> : null}
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
                            {settings.billing_min_topup_cny_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
                          </label>
                          <input className="form-control" value={form.billing_min_topup_cny} onChange={(e) => setForm({ ...form, billing_min_topup_cny: e.target.value })} placeholder="10.00" />
                        </div>
                        <div className="col-md-6">
                          <label className="form-label fw-medium d-flex justify-content-between">
                            <span>充值汇率（USD / CNY）</span>
                            {settings.billing_credit_usd_per_cny_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
                            {settings.billing_paygo_price_multiplier_override ? <span className="badge bg-light text-dark border">界面覆盖</span> : null}
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
        </div>
      )}
    </div>
  );
}

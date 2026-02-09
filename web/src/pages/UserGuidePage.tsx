import { useMemo } from 'react';
import { Link } from 'react-router-dom';

export function UserGuidePage() {
  const apiBaseURL = useMemo(() => `${window.location.origin}/v1`, []);

  return (
    <div className="fade-in-up">
      <div className="d-flex flex-wrap align-items-center justify-content-between gap-3 mb-4">
        <div>
          <h2 className="h4 fw-bold mb-1 d-flex align-items-center">
            <span className="me-2 material-symbols-rounded text-primary">menu_book</span>
            使用教程
          </h2>
          <p className="text-muted mb-0 small">面向用户侧的一站式接入说明：创建令牌、配置客户端并处理常见问题。</p>
        </div>
      </div>

      <div className="row g-4">
        <div className="col-12">
          <div className="card border-0 shadow-sm">
            <div className="card-body p-4">
              <h5 className="fw-semibold mb-3 d-flex align-items-center">
                <span className="me-2 material-symbols-rounded text-primary">looks_one</span>
                第一步：创建 API 令牌
              </h5>
              <p className="text-muted small mb-3">
                在用户面板进入「API 令牌」页面创建令牌。令牌仅展示一次，请妥善保存。
              </p>
              <Link to="/tokens" className="btn btn-primary btn-sm">
                <span className="me-1 material-symbols-rounded">key</span>
                前往 API 令牌
              </Link>
            </div>
          </div>
        </div>

        <div className="col-12">
          <div className="card border-0 shadow-sm">
            <div className="card-body p-4">
              <h5 className="fw-semibold mb-3 d-flex align-items-center">
                <span className="me-2 material-symbols-rounded text-primary">looks_two</span>
                第二步：配置客户端
              </h5>
              <p className="text-muted small mb-3">
                推荐使用 OpenAI 兼容方式接入。将基础地址指向 Realms，再填入上一步创建的令牌。
              </p>

              <div className="bg-dark rounded-3 p-3 mb-3 position-relative overflow-hidden">
                <div className="d-flex justify-content-between align-items-center mb-2">
                  <small className="text-secondary text-uppercase fw-bold smaller">终端配置</small>
                  <div className="d-flex gap-1">
                    <div className="rounded-circle bg-danger" style={{ width: 8, height: 8 }}></div>
                    <div className="rounded-circle bg-warning" style={{ width: 8, height: 8 }}></div>
                    <div className="rounded-circle bg-success" style={{ width: 8, height: 8 }}></div>
                  </div>
                </div>
                <pre className="mb-0 text-light overflow-auto smaller font-monospace" style={{ whiteSpace: 'pre-wrap' }}>
                  <code>
                    {'# Linux/macOS（bash/zsh）\n'}
                    {`export OPENAI_BASE_URL="${apiBaseURL}"\n`}
                    {'export OPENAI_API_KEY="sk_..."\n\n'}
                    {'# Windows（PowerShell）\n'}
                    {`$env:OPENAI_BASE_URL = "${apiBaseURL}"\n`}
                    {'$env:OPENAI_API_KEY = "sk_..."\n\n'}
                    {'# ~/.codex/config.toml（Windows: %USERPROFILE%\\\\.codex\\\\config.toml）\n'}
                    {'model_provider = "realms"\n\n'}
                    {'[model_providers.realms]\n'}
                    {'name = "Realms"\n'}
                    {`base_url = "${apiBaseURL}"\n`}
                    {'wire_api = "responses"\n'}
                    {'requires_openai_auth = true'}
                  </code>
                </pre>
              </div>

              <div className="alert alert-light border-0 bg-light small mb-0">
                <span className="me-1 material-symbols-rounded align-middle text-primary">info</span>
                API 基础地址：<strong className="user-select-all ms-1">{apiBaseURL}</strong>
              </div>
            </div>
          </div>
        </div>

        <div className="col-12">
          <div className="card border-0 shadow-sm">
            <div className="card-body p-4">
              <h5 className="fw-semibold mb-3 d-flex align-items-center">
                <span className="me-2 material-symbols-rounded text-primary">help</span>
                常见问题
              </h5>
              <ul className="mb-0 ps-3 text-muted small d-flex flex-column gap-2">
                <li>401 Unauthorized：通常是令牌错误或已撤销，请在「API 令牌」重新生成。</li>
                <li>模型不可用：请在「模型列表」确认当前账号是否可见该模型。</li>
                <li>余额或订阅限制：请在「订阅管理 / 余额充值」检查额度与套餐状态。</li>
              </ul>
              <div className="d-flex flex-wrap gap-2 mt-3">
                <Link to="/models" className="btn btn-outline-secondary btn-sm">
                  <span className="me-1 material-symbols-rounded">smart_toy</span>
                  模型列表
                </Link>
                <Link to="/subscription" className="btn btn-outline-secondary btn-sm">
                  <span className="me-1 material-symbols-rounded">credit_card</span>
                  订阅管理
                </Link>
                <Link to="/topup" className="btn btn-outline-secondary btn-sm">
                  <span className="me-1 material-symbols-rounded">account_balance_wallet</span>
                  余额充值
                </Link>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

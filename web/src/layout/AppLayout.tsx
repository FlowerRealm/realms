import { useEffect, useMemo, useState } from 'react';
import { Link, NavLink, Outlet } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import { ProjectFooter } from './ProjectFooter';

function userInitial(emailOrName: string | null | undefined): string {
  const s = (emailOrName || '').trim();
  if (!s) return '?';
  return s.slice(0, 1).toUpperCase();
}

export function AppLayout() {
  const { user, logout, loading } = useAuth();
  const [sidebarOpen, setSidebarOpen] = useState(false);

  useEffect(() => {
    document.documentElement.classList.remove('admin-html');
    document.body.classList.remove('admin-body');
    document.documentElement.classList.add('app-html');
    document.body.classList.add('app-body');
    return () => {
      document.documentElement.classList.remove('app-html');
      document.body.classList.remove('app-body');
    };
  }, []);

  const closeSidebar = () => setSidebarOpen(false);
  const displayEmail = user?.email || user?.username || '';
  const isRoot = user?.role === 'root';
  const features = user?.features;

  const sidebarVersionLabel = useMemo(() => (user ? 'v1.0.0' : ''), [user]);

  return (
    <div className="app-shell d-flex flex-grow-1">
      <aside className={`sidebar d-flex flex-column flex-shrink-0 p-3 ${sidebarOpen ? 'show' : ''}`} id="sidebarMenu">
        <Link to="/" className="d-flex align-items-center mb-4 mb-md-0 me-md-auto text-decoration-none px-2 mt-2" onClick={closeSidebar}>
          <div className="me-2 d-flex align-items-center justify-content-center flex-shrink-0" style={{ width: 36, height: 36 }}>
            <img src="/assets/realms_icon.svg" alt="Realms" style={{ width: 24, height: 24 }} />
          </div>
          <span className="fs-5 fw-bold text-body tracking-tight">Realms</span>
        </Link>
        <hr className="text-secondary opacity-50 my-3" />

        <ul className="nav flex-column sidebar-nav mb-0">
          <li className="nav-item">
            <NavLink to="/dashboard" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
              <span className="material-symbols-rounded">dashboard</span> 控制台
            </NavLink>
          </li>
          <li>
            <NavLink to="/guide" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
              <span className="material-symbols-rounded">menu_book</span> 使用教程
            </NavLink>
          </li>
          {features?.web_announcements_disabled ? null : (
            <li>
              <NavLink to="/announcements" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <span className="material-symbols-rounded">campaign</span> 公告
              </NavLink>
            </li>
          )}
          {features?.web_tokens_disabled ? null : (
            <li>
              <NavLink to="/tokens" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <span className="material-symbols-rounded">key</span> API 令牌
              </NavLink>
            </li>
          )}
          {features?.models_disabled ? null : (
            <li>
              <NavLink to="/models" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <span className="material-symbols-rounded">smart_toy</span> 模型列表
              </NavLink>
            </li>
          )}
          {features?.billing_disabled ? null : (
            <>
              <li>
                <NavLink to="/subscription" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                  <span className="material-symbols-rounded">credit_card</span> 订阅管理
                </NavLink>
              </li>
              <li>
                <NavLink to="/topup" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                  <span className="material-symbols-rounded">account_balance_wallet</span> 余额充值
                </NavLink>
              </li>
            </>
          )}
          {features?.web_usage_disabled ? null : (
            <li>
              <NavLink to="/usage" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <span className="material-symbols-rounded">monitoring</span> 用量统计
              </NavLink>
            </li>
          )}
          {features?.tickets_disabled ? null : (
            <li>
              <NavLink to="/tickets" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <span className="material-symbols-rounded">support_agent</span> 工单
              </NavLink>
            </li>
          )}
          <li>
            <NavLink to="/account" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
              <span className="material-symbols-rounded">manage_accounts</span> 账号设置
            </NavLink>
          </li>

          {isRoot ? (
            <>
              <li className="mt-4 mb-2 ms-2 text-uppercase text-muted sidebar-section-label">
                管理
              </li>
              <li>
                <NavLink to="/admin" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                  <span className="material-symbols-rounded">admin_panel_settings</span> 管理后台
                </NavLink>
              </li>
            </>
          ) : null}
        </ul>

        <div className="mt-auto px-2 pb-2">
          <span className="text-muted small opacity-50">{sidebarVersionLabel}</span>
        </div>
      </aside>

      <div className="main-wrapper w-100">
        <header className="top-header">
          <button
            className="btn btn-link text-body d-md-none p-0"
            type="button"
            onClick={() => setSidebarOpen((v) => !v)}
            aria-label="打开侧边栏"
          >
            <span className="fs-4 material-symbols-rounded">menu</span>
          </button>

          <div className="ms-auto d-flex align-items-center">
            <div className="dropdown">
              <a
                href="#"
                className="d-flex align-items-center text-body text-decoration-none dropdown-toggle"
                id="dropdownUser1"
                data-bs-toggle="dropdown"
                aria-expanded="false"
                onClick={(e) => e.preventDefault()}
              >
                <div
                  className="bg-primary bg-opacity-10 text-primary rounded-circle d-flex align-items-center justify-content-center me-2"
                  style={{ width: 40, height: 40 }}
                >
                  {userInitial(displayEmail)}
                </div>
                <span className="d-none d-sm-inline fw-medium small text-secondary">{displayEmail || '未登录'}</span>
              </a>

              <ul className="dropdown-menu dropdown-menu-end border-0 shadow-lg mt-2 p-2 rounded-4" aria-labelledby="dropdownUser1">
                <li>
                  <div className="dropdown-header">角色: {user?.role || '-'}</div>
                </li>
                <li>
                  <hr className="dropdown-divider" />
                </li>
                <li>
                  <Link className="dropdown-item rounded-2" to="/account">
                    <span className="me-2 material-symbols-rounded">manage_accounts</span>账号设置
                  </Link>
                </li>
                <li>
                  <hr className="dropdown-divider" />
                </li>
                <li>
                  <button className="dropdown-item rounded-2 text-danger" type="button" disabled={loading} onClick={() => void logout()}>
                    <span className="me-2 material-symbols-rounded">logout</span>退出登录
                  </button>
                </li>
              </ul>
            </div>
          </div>
        </header>

        <main className="content-scrollable">
          <div className="container-fluid" style={{ maxWidth: 1400 }}>
            <Outlet />
            <ProjectFooter variant="app" />
          </div>
        </main>
      </div>
    </div>
  );
}

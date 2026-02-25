import { useEffect, useMemo, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'

import { useAuth } from '../auth/AuthContext'
import { ProjectFooter } from './ProjectFooter'

function userEmail(userEmailValue: string | null | undefined, username: string | null | undefined): string {
  const email = (userEmailValue || '').trim()
  if (email) return email
  const u = (username || '').trim()
  if (u) return u
  return '未登录'
}

export function AdminLayoutSelf() {
  const { user, loading, refresh } = useAuth()
  const [sidebarOpen, setSidebarOpen] = useState(false)

  useEffect(() => {
    document.documentElement.classList.remove('app-html')
    document.body.classList.remove('app-body')
    document.documentElement.classList.add('admin-html')
    document.body.classList.add('admin-body')
    return () => {
      document.documentElement.classList.remove('admin-html')
      document.body.classList.remove('admin-body')
    }
  }, [])

  const features = user?.features
  const closeSidebar = () => setSidebarOpen(false)
  const loginLabel = useMemo(() => userEmail(user?.email, user?.username), [user?.email, user?.username])

  const logoutSelf = () => {
    localStorage.removeItem('self_mode_key')
    localStorage.removeItem('user')
    void refresh()
  }

  const showChannels = !(features?.admin_channels_disabled ?? false)
  const showUsage = !(features?.admin_usage_disabled ?? false)

  return (
    <div className="app-shell d-flex flex-grow-1">
      <aside className={`sidebar d-flex flex-column flex-shrink-0 p-3 ${sidebarOpen ? 'show' : ''}`} id="sidebarMenu">
        <Link to="/admin/channels" className="d-flex align-items-center mb-4 mb-md-0 me-md-auto text-decoration-none px-2 mt-2" onClick={closeSidebar}>
          <div className="me-2 d-flex align-items-center justify-content-center flex-shrink-0" style={{ width: 36, height: 36 }}>
            <img src="/assets/realms_icon.svg" alt="Realms" style={{ width: 24, height: 24 }} />
          </div>
          <span className="fs-5 fw-bold text-body tracking-tight">管理后台</span>
        </Link>
        <hr className="text-secondary opacity-50 my-3" />

        <ul className="nav flex-column sidebar-nav mb-0">
          {showChannels ? (
            <li>
              <NavLink to="/admin/channels" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <i className="ri-git-merge-line"></i> 上游渠道
              </NavLink>
            </li>
          ) : null}
          {showUsage ? (
            <li>
              <NavLink to="/admin/usage" className={({ isActive }) => `sidebar-link${isActive ? ' active' : ''}`} onClick={closeSidebar}>
                <i className="ri-line-chart-line"></i> 用量统计
              </NavLink>
            </li>
          ) : null}
        </ul>
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
                <span className="d-none d-sm-inline fw-medium small text-secondary">{loginLabel}</span>
              </a>

              <ul className="dropdown-menu dropdown-menu-end border-0 shadow-lg mt-2 p-2 rounded-4" aria-labelledby="dropdownUser1">
                <li>
                  <div className="dropdown-header">角色: root</div>
                </li>
                <li>
                  <hr className="dropdown-divider" />
                </li>
                <li>
                  <button className="dropdown-item rounded-2 text-danger" type="button" disabled={loading} onClick={logoutSelf}>
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
  )
}

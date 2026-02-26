import { useEffect, useMemo } from 'react'
import { Link, Outlet, useLocation } from 'react-router-dom'

import { useAuth } from '../../auth/AuthContext'
import { ProjectFooter } from '../ProjectFooter'

function userEmail(userEmailValue: string | null | undefined, username: string | null | undefined): string {
  const email = (userEmailValue || '').trim()
  if (email) return email
  const u = (username || '').trim()
  if (u) return u
  return '未登录'
}

export function AdminLayout() {
  const { user, loading, refresh } = useAuth()
  const location = useLocation()

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
  const loginLabel = useMemo(() => userEmail(user?.email, user?.username), [user?.email, user?.username])
  const channelsDisabled = features?.admin_channels_disabled ?? false
  const isSettingsPage = location.pathname.startsWith('/admin/settings')
  const homeTo = channelsDisabled ? '/admin/settings' : '/admin/channels'

  const logout = () => {
    localStorage.removeItem('personal_mode_key')
    localStorage.removeItem('user')
    void refresh()
  }

  const settingsButtonTo = isSettingsPage ? '/admin/channels' : '/admin/settings'
  const settingsButtonLabel = isSettingsPage ? '返回' : '设置'
  const settingsButtonIcon = isSettingsPage ? 'arrow_back' : 'settings'
  const showSettingsButton = !isSettingsPage || !channelsDisabled

  return (
    <div className="app-shell d-flex flex-grow-1">
      <div className="main-wrapper w-100">
        <header className="top-header">
          <Link to={homeTo} className="d-flex align-items-center text-decoration-none">
            <div className="me-2 d-flex align-items-center justify-content-center flex-shrink-0" style={{ width: 32, height: 32 }}>
              <img src="/assets/realms_icon.svg" alt="Realms" style={{ width: 20, height: 20 }} />
            </div>
            <span className="fw-bold text-body tracking-tight">管理后台</span>
          </Link>

          <div className="d-flex align-items-center gap-2">
            {showSettingsButton ? (
              <Link
                to={settingsButtonTo}
                className="rlm-personal-top-action"
                aria-label={settingsButtonLabel}
                title={settingsButtonLabel}
              >
                <span className="material-symbols-rounded">{settingsButtonIcon}</span>
                <span className="visually-hidden">{settingsButtonLabel}</span>
              </Link>
            ) : null}

            <div className="dropdown">
              <a
                href="#"
                className="rlm-personal-user-pill d-flex align-items-center text-decoration-none dropdown-toggle"
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
                  <button className="dropdown-item rounded-2 text-danger" type="button" disabled={loading} onClick={logout}>
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

import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'

import { ProjectFooter } from './ProjectFooter'
import { api } from '../api/client'
import type { APIResponse } from '../api/types'

export function PublicLayoutSelf() {
  const [selfModeKeySet, setSelfModeKeySet] = useState(false)

  useEffect(() => {
    document.documentElement.classList.remove('admin-html')
    document.body.classList.remove('admin-body')
    document.documentElement.classList.remove('app-html')
    document.body.classList.remove('app-body')
  }, [])

  useEffect(() => {
    let mounted = true
    ;(async () => {
      try {
        const res = await api.get<APIResponse<{ self_mode?: boolean; self_mode_key_set?: boolean }>>('/api/meta')
        if (!mounted) return
        if (res.data?.success && res.data?.data?.self_mode) {
          setSelfModeKeySet(!!res.data?.data?.self_mode_key_set)
        } else {
          setSelfModeKeySet(false)
        }
      } catch {
        // ignore
      }
    })()
    return () => {
      mounted = false
    }
  }, [])

  return (
    <div className="container-fluid d-flex flex-column min-vh-100 p-0">
      <header className="simple-header d-flex flex-wrap justify-content-center py-3 mb-4">
        <Link to="/" className="d-flex align-items-center mb-3 mb-md-0 me-md-auto text-body text-decoration-none ms-4">
          <div className="me-2 d-flex align-items-center justify-content-center flex-shrink-0" style={{ width: 32, height: 32 }}>
            <img src="/assets/realms_icon.svg" alt="Realms" style={{ width: 22, height: 22 }} />
          </div>
          <span className="fs-4 fw-bold tracking-tight">Realms</span>
        </Link>

        <ul className="nav nav-pills me-4">
          <li className="nav-item">
            <NavLink to="/login" className={({ isActive }) => `nav-link${isActive ? ' active rounded-pill px-4' : ' text-secondary'}`}>
              {selfModeKeySet ? '解锁' : '初始化'}
            </NavLink>
          </li>
        </ul>
      </header>

      <main className="flex-fill d-flex flex-column justify-content-center align-items-center">
        <div className="w-100" style={{ maxWidth: 520 }}>
          <Outlet />
        </div>
      </main>

      <ProjectFooter variant="public" />
    </div>
  )
}

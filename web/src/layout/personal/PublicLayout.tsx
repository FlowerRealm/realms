import type { Dispatch, SetStateAction } from 'react'
import { useEffect, useState } from 'react'
import { Link, NavLink, Outlet } from 'react-router-dom'

import { api } from '../../api/client'
import type { APIResponse } from '../../api/types'
import { ProjectFooter } from '../ProjectFooter'

export type PublicLayoutContext = {
  personalModeKeySet: boolean
  setPersonalModeKeySet: Dispatch<SetStateAction<boolean>>
}

export function PublicLayout() {
  const [personalModeKeySet, setPersonalModeKeySet] = useState(false)

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
        const res = await api.get<APIResponse<{ mode?: 'business' | 'personal'; personal_mode_key_set?: boolean }>>('/api/meta')
        if (!mounted) return
        const data = res.data?.data
        const isPersonal = data?.mode === 'personal'
        if (res.data?.success && isPersonal) {
          setPersonalModeKeySet(!!data?.personal_mode_key_set)
        } else {
          setPersonalModeKeySet(false)
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
              {personalModeKeySet ? '解锁' : '初始化'}
            </NavLink>
          </li>
        </ul>
      </header>

      <main className="flex-fill d-flex flex-column justify-content-center align-items-center">
        <div className="w-100" style={{ maxWidth: 520 }}>
          <Outlet context={{ personalModeKeySet, setPersonalModeKeySet } satisfies PublicLayoutContext} />
        </div>
      </main>

      <ProjectFooter variant="public" />
    </div>
  )
}


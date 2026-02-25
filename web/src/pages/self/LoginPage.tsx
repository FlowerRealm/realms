import { useMemo, useState } from 'react'
import { Navigate, useLocation, useNavigate, useOutletContext } from 'react-router-dom'

import { api } from '../../api/client'
import type { APIResponse } from '../../api/types'
import { useAuth } from '../../auth/AuthContext'
import { SegmentedFrame } from '../../components/SegmentedFrame'
import type { PublicLayoutContext } from '../../layout/self/PublicLayout'

type LocationState = {
  from?: string
  notice?: string
  error?: string
}

export function LoginPage() {
  const { user, loading, refresh } = useAuth()
  const navigate = useNavigate()
  const location = useLocation()
  const { selfModeKeySet, setSelfModeKeySet } = useOutletContext<PublicLayoutContext>()

  const [form, setForm] = useState({ key: '', confirm: '' })
  const [err, setErr] = useState('')

  const notice = useMemo(() => {
    const state = location.state as LocationState | null
    const v = (state?.notice || '').toString().trim()
    return v ? v : ''
  }, [location.state])

  const from = useMemo(() => {
    const state = location.state as LocationState | null
    const next = (state?.from || '').toString().trim()
    return next || '/admin/channels'
  }, [location.state])

  if (user) {
    return <Navigate to="/admin/channels" replace />
  }

  return (
    <SegmentedFrame>
      <div className="card border-0 mb-0">
        <div className="card-body p-4">
          <h2 className="h4 card-title text-center mb-4">{selfModeKeySet ? '解锁 Realms' : '初始化 Realms'}</h2>

          {notice ? (
            <div className="alert alert-success py-2" role="alert">
              <span className="me-1 material-symbols-rounded">check_circle</span> {notice}
            </div>
          ) : null}

          {err ? (
            <div className="alert alert-danger py-2" role="alert">
              <span className="me-1 material-symbols-rounded">warning</span> {err}
            </div>
          ) : null}

          <form
            onSubmit={async (e) => {
              e.preventDefault()
              setErr('')
              try {
                const key = (form.key || '').trim()
                if (!key) {
                  setErr('Key 不能为空')
                  return
                }
                if (!selfModeKeySet) {
                  const confirm = (form.confirm || '').trim()
                  if (!confirm) {
                    setErr('请再次输入 Key 确认')
                    return
                  }
                  if (confirm !== key) {
                    setErr('两次输入的 Key 不一致')
                    return
                  }
                }

                if (!selfModeKeySet) {
                  const res = await api.post<APIResponse<unknown>>('/api/self-mode/bootstrap', { key })
                  if (!res.data?.success) {
                    throw new Error(res.data?.message || '设置 Key 失败')
                  }
                  setSelfModeKeySet(true)
                }

                localStorage.setItem('self_mode_key', key)
                await refresh()
                navigate(from, { replace: true })
              } catch (e) {
                setErr(e instanceof Error ? e.message : '解锁失败')
              }
            }}
          >
            <div className="mb-3">
              <label className="form-label">{selfModeKeySet ? '管理 Key' : '设置管理 Key'}</label>
              <input
                className="form-control"
                name="key"
                type="password"
                autoComplete="off"
                required
                placeholder={selfModeKeySet ? '输入你设置的 Key' : '输入一个新的 Key'}
                value={form.key}
                onChange={(e) => setForm((p) => ({ ...p, key: e.target.value }))}
              />
              <div className="form-text">自用模式下使用 Key 作为鉴权，不需要账号系统。</div>
            </div>

            {!selfModeKeySet ? (
              <div className="mb-3">
                <label className="form-label">确认 Key</label>
                <input
                  className="form-control"
                  name="key_confirm"
                  type="password"
                  autoComplete="off"
                  required
                  placeholder="再次输入 Key"
                  value={form.confirm}
                  onChange={(e) => setForm((p) => ({ ...p, confirm: e.target.value }))}
                />
              </div>
            ) : null}

            <div className="d-grid mt-4">
              <button type="submit" className="btn btn-primary" disabled={loading}>
                {loading ? (selfModeKeySet ? '解锁中…' : '初始化中…') : selfModeKeySet ? '进入管理后台' : '完成初始化'}
              </button>
            </div>
          </form>
        </div>
      </div>
    </SegmentedFrame>
  )
}

import { Navigate, Route, Routes } from 'react-router-dom'

import { RequireAuth } from './auth/RequireAuth'
import { useAuth } from './auth/AuthContext'
import { AdminLayoutSelf } from './layout/AdminLayoutSelf'
import { PublicLayoutSelf } from './layout/PublicLayoutSelf'
import { AdminPageSelf } from './pages/AdminPageSelf'
import { LoginPageSelf } from './pages/LoginPageSelf'
import { NotFoundPage } from './pages/NotFoundPage'

function HomeRedirectSelf() {
  const { user, loading } = useAuth()
  if (loading) return null
  if (user) return <Navigate to="/admin/channels" replace />
  return <Navigate to="/login" replace state={{ from: '/admin/channels' }} />
}

export function AppSelf() {
  const { loading } = useAuth()
  if (loading) return null

  return (
    <Routes>
      <Route path="/" element={<HomeRedirectSelf />} />

      <Route element={<PublicLayoutSelf />}>
        <Route path="/login" element={<LoginPageSelf />} />
      </Route>

      <Route
        element={
          <RequireAuth>
            <AdminLayoutSelf />
          </RequireAuth>
        }
      >
        <Route path="/admin/*" element={<AdminPageSelf />} />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

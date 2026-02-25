import { Navigate, Route, Routes } from 'react-router-dom'

import { RequireAuth } from './auth/RequireAuth'
import { useAuth } from './auth/AuthContext'
import { AdminLayout } from './layout/self/AdminLayout'
import { PublicLayout } from './layout/self/PublicLayout'
import { AdminPage } from './pages/self/AdminPage'
import { LoginPage } from './pages/self/LoginPage'
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

      <Route element={<PublicLayout />}>
        <Route path="/login" element={<LoginPage />} />
      </Route>

      <Route
        element={
          <RequireAuth>
            <AdminLayout />
          </RequireAuth>
        }
      >
        <Route path="/admin/*" element={<AdminPage />} />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

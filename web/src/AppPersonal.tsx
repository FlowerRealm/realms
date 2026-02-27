import { Navigate, Route, Routes } from 'react-router-dom'

import { RequireAuth } from './auth/RequireAuth'
import { useAuth } from './auth/AuthContext'
import { AdminLayout } from './layout/personal/AdminLayout'
import { PublicLayout } from './layout/personal/PublicLayout'
import { McpServersPage } from './pages/McpServersPage'
import { AdminPage } from './pages/personal/AdminPage'
import { LoginPage } from './pages/personal/LoginPage'
import { NotFoundPage } from './pages/NotFoundPage'

function HomeRedirectPersonal() {
  const { user, loading } = useAuth()
  if (loading) return null
  if (user) return <Navigate to="/admin/channels" replace />
  return <Navigate to="/login" replace state={{ from: '/admin/channels' }} />
}

export function AppPersonal() {
  const { loading } = useAuth()
  if (loading) return null

  return (
    <Routes>
      <Route path="/" element={<HomeRedirectPersonal />} />

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
        <Route path="/mcp" element={<McpServersPage />} />
        <Route path="/admin/*" element={<AdminPage />} />
      </Route>

      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  )
}

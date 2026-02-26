import { Navigate, Route, Routes } from 'react-router-dom'

import { useAuth } from '../../auth/AuthContext'
import { ChannelsPage } from '../admin/ChannelsPage'
import { UsageAdminPage } from '../admin/UsageAdminPage'

export function AdminPage() {
  const { user } = useAuth()
  if (user?.role !== 'root') {
    return <Navigate to="/login" replace />
  }

  return (
    <Routes>
      <Route index element={<Navigate to="channels" replace />} />
      <Route path="channels" element={<ChannelsPage />} />
      <Route path="usage" element={<UsageAdminPage />} />
      <Route path="*" element={<Navigate to="/admin/channels" replace />} />
    </Routes>
  )
}


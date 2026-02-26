import { Navigate, Route, Routes } from 'react-router-dom'

import { useAuth } from '../../auth/AuthContext'
import { ChannelsPage } from '../admin/ChannelsPage'
import { SettingsAdminPage } from '../admin/SettingsAdminPage'

export function AdminPage() {
  const { user } = useAuth()
  if (user?.role !== 'root') {
    return <Navigate to="/login" replace />
  }

  const channelsDisabled = user?.features?.admin_channels_disabled ?? false
  const defaultRoute = channelsDisabled ? 'settings' : 'channels'

  return (
    <Routes>
      <Route index element={<Navigate to={defaultRoute} replace />} />

      <Route path="channels" element={channelsDisabled ? <Navigate to="/admin/settings" replace /> : <ChannelsPage />} />
      <Route path="settings" element={<SettingsAdminPage />} />

      <Route path="*" element={<Navigate to={`/admin/${defaultRoute}`} replace />} />
    </Routes>
  )
}

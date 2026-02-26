import { Navigate, Route, Routes, useLocation } from 'react-router-dom'

import { useAuth } from '../../auth/AuthContext'
import { ChannelsPage } from '../admin/ChannelsPage'
import { UsageAdminPage } from '../admin/UsageAdminPage'
import { PersonalAPIKeysPage } from './PersonalAPIKeysPage'

type PersonalAdminTab = 'channels' | 'usage' | 'api-keys'

function tabFromLocationSearch(search: string): PersonalAdminTab | '' {
  const tab = new URLSearchParams(search).get('tab')
  const v = (tab || '').trim()
  if (v === 'channels' || v === 'usage') return v
  if (v === 'settings' || v === 'api_keys' || v === 'api-keys') return 'api-keys'
  return ''
}

function AdminIndexRedirect({ defaultRoute }: { defaultRoute: PersonalAdminTab }) {
  const location = useLocation()
  const tab = tabFromLocationSearch(location.search)
  const target = tab || defaultRoute
  return <Navigate to={`/admin/${target}`} replace />
}

export function AdminPage() {
  const { user } = useAuth()
  if (user?.role !== 'root') {
    return <Navigate to="/login" replace />
  }

  const channelsDisabled = user?.features?.admin_channels_disabled ?? false
  const usageDisabled = user?.features?.admin_usage_disabled ?? false
  const defaultRoute: PersonalAdminTab = !channelsDisabled ? 'channels' : !usageDisabled ? 'usage' : 'api-keys'

  return (
    <Routes>
      <Route index element={<AdminIndexRedirect defaultRoute={defaultRoute} />} />

      <Route path="channels" element={channelsDisabled ? <Navigate to={`/admin/${defaultRoute}`} replace /> : <ChannelsPage />} />
      <Route path="usage" element={usageDisabled ? <Navigate to={`/admin/${defaultRoute}`} replace /> : <UsageAdminPage />} />
      <Route path="api-keys" element={<PersonalAPIKeysPage />} />
      <Route path="settings" element={<Navigate to="/admin/api-keys" replace />} />

      <Route path="*" element={<Navigate to={`/admin/${defaultRoute}`} replace />} />
    </Routes>
  )
}

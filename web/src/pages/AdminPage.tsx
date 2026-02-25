import { Navigate, Route, Routes } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import { SegmentedFrame } from '../components/SegmentedFrame';
import { AdminHomePage } from './admin/AdminHomePage';
import { AnnouncementsAdminPage } from './admin/AnnouncementsAdminPage';
import { ChannelsPage } from './admin/ChannelsPage';
import { ChannelGroupDetailPage } from './admin/ChannelGroupDetailPage';
import { ChannelGroupsPage } from './admin/ChannelGroupsPage';
import { ModelsAdminPage } from './admin/ModelsAdminPage';
import { MainGroupsPage } from './admin/MainGroupsPage';
import { OAuthAppDetailPage } from './admin/OAuthAppDetailPage';
import { OAuthAppsAdminPage } from './admin/OAuthAppsAdminPage';
import { OrdersPage } from './admin/OrdersPage';
import { PaymentChannelsPage } from './admin/PaymentChannelsPage';
import { SettingsAdminPage } from './admin/SettingsAdminPage';
import { SubscriptionEditPage } from './admin/SubscriptionEditPage';
import { SubscriptionsPage } from './admin/SubscriptionsPage';
import { TicketAdminDetailPage } from './admin/TicketAdminDetailPage';
import { TicketsAdminPage } from './admin/TicketsAdminPage';
import { UsageAdminPage } from './admin/UsageAdminPage';
import { UsersPage } from './admin/UsersPage';

export function AdminPage() {
  const { user } = useAuth();
  const isSelfMode = !!user?.self_mode;

  if (user?.role !== 'root') {
    return (
      <div className="fade-in-up">
        <SegmentedFrame>
          <div className="alert alert-danger mb-0" role="alert">
            <span className="me-2 material-symbols-rounded">report</span> 权限不足（需要 root）。
          </div>
        </SegmentedFrame>
      </div>
    );
  }

  return (
    <Routes>
      {isSelfMode ? (
        <>
          <Route index element={<Navigate to="channels" replace />} />
          <Route path="channels" element={<ChannelsPage />} />
          <Route path="usage" element={<UsageAdminPage />} />
          <Route path="*" element={<Navigate to="/admin/channels" replace />} />
        </>
      ) : (
        <>
          <Route index element={<AdminHomePage />} />
          <Route path="channels" element={<ChannelsPage />} />
          <Route path="channel-groups" element={<ChannelGroupsPage />} />
          <Route path="channel-groups/:id" element={<ChannelGroupDetailPage />} />
          <Route path="main-groups" element={<MainGroupsPage />} />
          <Route path="models" element={<ModelsAdminPage />} />
          <Route path="users" element={<UsersPage />} />
          <Route path="submissions" element={<Navigate to="/admin/subscriptions" replace />} />
          <Route path="subscriptions" element={<SubscriptionsPage />} />
          <Route path="subscriptions/:id" element={<SubscriptionEditPage />} />
          <Route path="orders" element={<OrdersPage />} />
          <Route path="payment-channels" element={<PaymentChannelsPage />} />
          <Route path="usage" element={<UsageAdminPage />} />
          <Route path="tickets" element={<TicketsAdminPage mode="all" />} />
          <Route path="tickets/open" element={<TicketsAdminPage mode="open" />} />
          <Route path="tickets/closed" element={<TicketsAdminPage mode="closed" />} />
          <Route path="tickets/:id" element={<TicketAdminDetailPage />} />
          <Route path="announcements" element={<AnnouncementsAdminPage />} />
          <Route path="oauth-apps" element={<OAuthAppsAdminPage />} />
          <Route path="oauth-apps/:id" element={<OAuthAppDetailPage />} />
          <Route path="settings" element={<SettingsAdminPage />} />
          <Route path="*" element={<Navigate to="/admin" replace />} />
        </>
      )}
    </Routes>
  );
}

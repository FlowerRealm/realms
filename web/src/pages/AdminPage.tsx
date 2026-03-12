import { Suspense, lazy } from 'react';
import { Navigate, Route, Routes } from 'react-router-dom';

import { useAuth } from '../auth/AuthContext';
import { SegmentedFrame } from '../components/SegmentedFrame';

const AdminHomePage = lazy(() => import('./admin/AdminHomePage').then((m) => ({ default: m.AdminHomePage })));
const AnnouncementsAdminPage = lazy(() => import('./admin/AnnouncementsAdminPage').then((m) => ({ default: m.AnnouncementsAdminPage })));
const ChannelsPage = lazy(() => import('./admin/ChannelsPage').then((m) => ({ default: m.ChannelsPage })));
const ChannelGroupDetailPage = lazy(() => import('./admin/ChannelGroupDetailPage').then((m) => ({ default: m.ChannelGroupDetailPage })));
const ChannelGroupsPage = lazy(() => import('./admin/ChannelGroupsPage').then((m) => ({ default: m.ChannelGroupsPage })));
const MainGroupsPage = lazy(() => import('./admin/MainGroupsPage').then((m) => ({ default: m.MainGroupsPage })));
const ModelsAdminPage = lazy(() => import('./admin/ModelsAdminPage').then((m) => ({ default: m.ModelsAdminPage })));
const OAuthAppDetailPage = lazy(() => import('./admin/OAuthAppDetailPage').then((m) => ({ default: m.OAuthAppDetailPage })));
const OAuthAppsAdminPage = lazy(() => import('./admin/OAuthAppsAdminPage').then((m) => ({ default: m.OAuthAppsAdminPage })));
const OrdersPage = lazy(() => import('./admin/OrdersPage').then((m) => ({ default: m.OrdersPage })));
const PaymentChannelsPage = lazy(() => import('./admin/PaymentChannelsPage').then((m) => ({ default: m.PaymentChannelsPage })));
const SettingsAdminPage = lazy(() => import('./admin/SettingsAdminPage').then((m) => ({ default: m.SettingsAdminPage })));
const SubscriptionEditPage = lazy(() => import('./admin/SubscriptionEditPage').then((m) => ({ default: m.SubscriptionEditPage })));
const SubscriptionsPage = lazy(() => import('./admin/SubscriptionsPage').then((m) => ({ default: m.SubscriptionsPage })));
const TicketAdminDetailPage = lazy(() => import('./admin/TicketAdminDetailPage').then((m) => ({ default: m.TicketAdminDetailPage })));
const TicketsAdminPage = lazy(() => import('./admin/TicketsAdminPage').then((m) => ({ default: m.TicketsAdminPage })));
const UsageAdminPage = lazy(() => import('./admin/UsageAdminPage').then((m) => ({ default: m.UsageAdminPage })));
const UsersPage = lazy(() => import('./admin/UsersPage').then((m) => ({ default: m.UsersPage })));

function AdminRouteFallback() {
  return <div className="text-muted">加载中…</div>;
}

export function AdminPage() {
  const { user } = useAuth();

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
    <Suspense fallback={<AdminRouteFallback />}>
      <Routes>
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
      </Routes>
    </Suspense>
  );
}

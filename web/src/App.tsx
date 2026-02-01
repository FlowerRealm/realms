import { Navigate, Route, Routes } from 'react-router-dom';

import { RequireAuth } from './auth/RequireAuth';
import { AdminLayout } from './layout/AdminLayout';
import { AppLayout } from './layout/AppLayout';
import { PublicLayout } from './layout/PublicLayout';
import { AdminPage } from './pages/AdminPage';
import { AccountPage } from './pages/AccountPage';
import { AnnouncementDetailPage } from './pages/AnnouncementDetailPage';
import { AnnouncementsPage } from './pages/AnnouncementsPage';
import { DashboardPage } from './pages/DashboardPage';
import { LoginPage } from './pages/LoginPage';
import { ModelsPage } from './pages/ModelsPage';
import { NotFoundPage } from './pages/NotFoundPage';
import { PayPage } from './pages/PayPage';
import { RegisterPage } from './pages/RegisterPage';
import { SubscriptionPage } from './pages/SubscriptionPage';
import { TicketDetailPage } from './pages/TicketDetailPage';
import { TicketNewPage } from './pages/TicketNewPage';
import { TicketsPage } from './pages/TicketsPage';
import { TokenCreatedPage } from './pages/TokenCreatedPage';
import { TokensPage } from './pages/TokensPage';
import { TopupPage } from './pages/TopupPage';
import { UsagePage } from './pages/UsagePage';
import { OAuthAuthorizePage } from './pages/OAuthAuthorizePage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/dashboard" replace />} />
      <Route element={<PublicLayout />}>
        <Route path="/login" element={<LoginPage />} />
        <Route path="/register" element={<RegisterPage />} />
      </Route>

      <Route
        path="/oauth/authorize"
        element={
          <RequireAuth>
            <OAuthAuthorizePage />
          </RequireAuth>
        }
      />

      <Route
        element={
          <RequireAuth>
            <AppLayout />
          </RequireAuth>
        }
      >
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/announcements" element={<AnnouncementsPage />} />
        <Route path="/announcements/:id" element={<AnnouncementDetailPage />} />
        <Route path="/tokens" element={<TokensPage />} />
        <Route path="/tokens/created" element={<TokenCreatedPage />} />
        <Route path="/models" element={<ModelsPage />} />
        <Route path="/usage" element={<UsagePage />} />
        <Route path="/account" element={<AccountPage />} />
        <Route path="/subscription" element={<SubscriptionPage />} />
        <Route path="/topup" element={<TopupPage />} />
        <Route path="/pay/:kind/:orderId" element={<PayPage />} />
        <Route path="/pay/:kind/:orderId/success" element={<PayPage />} />
        <Route path="/pay/:kind/:orderId/cancel" element={<PayPage />} />
        <Route path="/tickets" element={<TicketsPage mode="all" />} />
        <Route path="/tickets/open" element={<TicketsPage mode="open" />} />
        <Route path="/tickets/closed" element={<TicketsPage mode="closed" />} />
        <Route path="/tickets/new" element={<TicketNewPage />} />
        <Route path="/tickets/:id" element={<TicketDetailPage />} />
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
  );
}

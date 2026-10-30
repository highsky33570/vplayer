import { useEffect } from 'react';
import { BrowserRouter, Navigate, Outlet, Route, Routes, useLocation } from 'react-router-dom';
import { AdminBilling } from './admin/AdminBilling';
import { AdminCategories } from './admin/AdminCategories';
import { AdminCategoryDetail } from './admin/AdminCategoryDetail';
import { AdminDashboard } from './admin/AdminDashboard';
import { AdminGuard } from './admin/AdminGuard';
import { AdminLayout } from './admin/AdminLayout';
import { AdminLoginPage } from './admin/AdminLoginPage';
import { AdminOperationalRunDetail } from './admin/AdminOperationalRunDetail';
import { AdminPayments } from './admin/AdminPayments';
import { AdminSync } from './admin/AdminSync';
import { AdminSyncRunDetail } from './admin/AdminSyncRunDetail';
import { AdminUserDetail } from './admin/AdminUserDetail';
import { AdminUsers } from './admin/AdminUsers';
import { AdminVideoDetail } from './admin/AdminVideoDetail';
import { AdminVideos } from './admin/AdminVideos';
import { AuthProvider, useAuth } from './auth/AuthContext';
import { Header } from './components/Header';
import { MobileCornerDock } from './components/MobileCornerDock';
import { MobileTabBar } from './components/MobileTabBar';
import { stopAllHoverPreviews } from './components/HoverPreviewThumb';
import { LoginModal } from './components/LoginModal';
import { LoginReminder } from './components/LoginReminder';
import { ScrollToTop } from './components/ScrollToTop';
import { clearHoverPreviewCache } from './media/hoverPreviewResolve';
import { AccountPage } from './pages/AccountPage';
import { FavoritesPage } from './pages/FavoritesPage';
import { HistoryPage } from './pages/HistoryPage';
import { Home } from './pages/Home';
import { LoginPage } from './pages/Login';
import { MessagesPage } from './pages/MessagesPage';
import { VipPage } from './pages/VipPage';
import { VipPaymentResultPage } from './pages/VipPaymentResultPage';
import { Watch } from './pages/Watch';
import { ThemeProvider } from './theme/ThemeContext';

function ConsumerShell() {
  const location = useLocation();
  const { authRequired } = useAuth();
  useEffect(() => {
    stopAllHoverPreviews();
    clearHoverPreviewCache();
  }, [location.pathname, location.search]);

  return (
    <div className="shell">
      <Header />
      <Outlet />
      <ScrollToTop />
      <MobileCornerDock />
      <MobileTabBar />
      {authRequired ? <LoginModal /> : null}
      {authRequired ? <LoginReminder /> : null}
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <AuthProvider>
          <Routes>
            <Route path="/admin/login" element={<AdminLoginPage />} />
            <Route path="/admin" element={<AdminGuard />}>
              <Route element={<AdminLayout />}>
                <Route index element={<AdminDashboard />} />
                <Route path="videos" element={<AdminVideos />} />
                <Route path="videos/:id" element={<AdminVideoDetail />} />
                <Route path="categories" element={<AdminCategories />} />
                <Route path="categories/:id" element={<AdminCategoryDetail />} />
                <Route path="users" element={<AdminUsers />} />
                <Route path="users/:id" element={<AdminUserDetail />} />
                <Route path="billing" element={<AdminBilling />} />
                <Route path="payments" element={<AdminPayments />} />
                <Route path="sync" element={<AdminSync />} />
                <Route path="sync/runs/:id" element={<AdminSyncRunDetail />} />
                <Route path="sync/operations/:id" element={<AdminOperationalRunDetail />} />
              </Route>
            </Route>
            <Route element={<ConsumerShell />}>
              <Route path="/" element={<Home />} />
              <Route path="/watch/:id" element={<Watch />} />
              <Route path="/account" element={<AccountPage />} />
              <Route path="/vip" element={<VipPage />} />
              <Route path="/vip/payment/result" element={<VipPaymentResultPage />} />
              <Route path="/favorites" element={<FavoritesPage />} />
              <Route path="/history" element={<HistoryPage />} />
              <Route path="/messages" element={<MessagesPage />} />
              <Route path="/login" element={<LoginPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}

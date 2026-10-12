import { BrowserRouter, Navigate, Outlet, Route, Routes } from 'react-router-dom';
import { AdminCategories } from './admin/AdminCategories';
import { AdminCategoryDetail } from './admin/AdminCategoryDetail';
import { AdminDashboard } from './admin/AdminDashboard';
import { AdminGuard } from './admin/AdminGuard';
import { AdminLayout } from './admin/AdminLayout';
import { AdminUserDetail } from './admin/AdminUserDetail';
import { AdminUsers } from './admin/AdminUsers';
import { AdminVideoDetail } from './admin/AdminVideoDetail';
import { AdminVideos } from './admin/AdminVideos';
import { AuthProvider } from './auth/AuthContext';
import { Header } from './components/Header';
import { LoginModal } from './components/LoginModal';
import { LoginReminder } from './components/LoginReminder';
import { Home } from './pages/Home';
import { LoginPage } from './pages/Login';
import { Watch } from './pages/Watch';
import { ThemeProvider } from './theme/ThemeContext';

function ConsumerShell() {
  return (
    <div className="shell">
      <Header />
      <Outlet />
      <LoginModal />
      <LoginReminder />
    </div>
  );
}

export default function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <AuthProvider>
          <Routes>
            <Route path="/admin" element={<AdminGuard />}>
              <Route element={<AdminLayout />}>
                <Route index element={<AdminDashboard />} />
                <Route path="videos" element={<AdminVideos />} />
                <Route path="videos/:id" element={<AdminVideoDetail />} />
                <Route path="categories" element={<AdminCategories />} />
                <Route path="categories/:id" element={<AdminCategoryDetail />} />
                <Route path="users" element={<AdminUsers />} />
                <Route path="users/:id" element={<AdminUserDetail />} />
              </Route>
            </Route>
            <Route element={<ConsumerShell />}>
              <Route path="/" element={<Home />} />
              <Route path="/watch/:id" element={<Watch />} />
              <Route path="/login" element={<LoginPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Route>
          </Routes>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}

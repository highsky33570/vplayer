import { BrowserRouter, Navigate, Route, Routes } from 'react-router-dom';
import { AuthProvider } from './auth/AuthContext';
import { Header } from './components/Header';
import { LoginModal } from './components/LoginModal';
import { LoginReminder } from './components/LoginReminder';
import { Home } from './pages/Home';
import { LoginPage } from './pages/Login';
import { Watch } from './pages/Watch';
import { ThemeProvider } from './theme/ThemeContext';

export default function App() {
  return (
    <BrowserRouter>
      <ThemeProvider>
        <AuthProvider>
          <div className="shell">
            <Header />
            <Routes>
              <Route path="/" element={<Home />} />
              <Route path="/watch/:id" element={<Watch />} />
              <Route path="/login" element={<LoginPage />} />
              <Route path="*" element={<Navigate to="/" replace />} />
            </Routes>
            <LoginModal />
            <LoginReminder />
          </div>
        </AuthProvider>
      </ThemeProvider>
    </BrowserRouter>
  );
}

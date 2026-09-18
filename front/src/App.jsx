import { BrowserRouter as Router, Routes, Route, Navigate } from 'react-router-dom';
import { ThemeProvider, createTheme, CssBaseline } from '@mui/material';
import Login from './pages/Login';
import Layout from './pages/Layout';
import ServerManagement from './pages/ServerManagement';
import Users from './pages/Users';
import Groups from './pages/Groups';
import Resources from './pages/Resources';
import Certificates from './pages/Certificates';
import CertificateEvents from './pages/CertificateEvents';
import Logs from './pages/Logs';
import ProtectedRoute from './components/ProtectedRoute';
import Help from './pages/Help';
const theme = createTheme({
  palette: {
    primary: {
      main: '#1976d2',
    },
    secondary: {
      main: '#dc004e',
    },
  },
});

function App() {
  return (
    <ThemeProvider theme={theme}>
      <CssBaseline />
      <Router>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route
            path="/dashboard"
            element={
              <ProtectedRoute>
                <Layout />
              </ProtectedRoute>
            }
          >
            <Route path="servers" element={<ServerManagement />} />
            <Route path="certificates" element={<Certificates />} />
            <Route path="certificates/events" element={<CertificateEvents />} />
            <Route path="certificates/:caId" element={<Certificates />} />
            <Route path="users" element={<Users />} />
            <Route path="groups" element={<Groups />} />
            <Route path="resources" element={<Resources />} />
            <Route path="logs" element={<Logs />} />
            <Route path="/dashboard/help" element={<Help />} />
            <Route index element={<Navigate to="servers" replace />} />
          </Route>
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Router>
    </ThemeProvider>
  );
}

export default App;

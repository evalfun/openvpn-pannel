import React, { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import {
  Container,
  Paper,
  TextField,
  Button,
  Box,
  Typography,
  CircularProgress,
  Alert,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@mui/material';
import { userAPI } from '../api';

const Login = () => {
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  // MFA（多因素认证）二次验证弹框状态
  const [openMfaDialog, setOpenMfaDialog] = useState(false);
  const [mfaCode, setMfaCode] = useState('');
  const [mfaError, setMfaError] = useState('');

  // 登录成功后的后续处理：拉取用户信息、校验管理员权限并跳转
  const afterLoginSuccess = async () => {
    setMessage('登录成功！');
    const userInfoResponse = await userAPI.getUserInfo();

    if (userInfoResponse.status === 200) {
      const hasAdminGroup = userInfoResponse.data.groups?.some(
        (group) => group.name === 'admin'
      );

      if (hasAdminGroup) {
        localStorage.setItem('userInfo', JSON.stringify(userInfoResponse.data));
        setTimeout(() => {
          navigate('/dashboard');
        }, 1000);
      } else {
        setError('您没有权限访问该系统');
        setIsLoading(false);
      }
    }
  };

  // mfaCode 为空时表示普通登录；若后端返回 mfa_required，则弹出验证码输入框。
  const handleLogin = async (e, mfaCode = '') => {
    if (e) e.preventDefault();
    if (isLoading) return;
    setError('');
    setMessage('');
    setMfaError('');
    setIsLoading(true);

    try {
      const loginResponse = await userAPI.login(username, password, mfaCode);
      const data = loginResponse.data || {};

      if (data.result === 'mfa_required') {
        // 账号密码正确，但需要动态验证码
        setIsLoading(false);
        setMfaCode('');
        setMfaError('');
        setOpenMfaDialog(true);
        return;
      }

      if (loginResponse.status === 200 && data.result === 'success') {
        await afterLoginSuccess();
      } else {
        setError('登录失败：用户名或密码错误');
        setIsLoading(false);
      }
    } catch (err) {
      const data = err.response?.data || {};
      const code = data.error;
      if (code === 'mfa_invalid') {
        // 验证码错误：重新打开弹框并提示
        setMfaError('动态验证码错误或已过期，请重试');
        setOpenMfaDialog(true);
      } else if (code === 'mfa_cooldown') {
        // 验证码尝试过于频繁：保持弹框打开并提示剩余冷却时间
        setMfaError(data.message || '验证码尝试过于频繁，请稍后再试');
        setOpenMfaDialog(true);
      } else {
        setError('登录失败：' + (data.message || data.error || '用户名或密码错误'));
      }
      setIsLoading(false);
    }
  };

  const handleMfaSubmit = () => {
    if (!/^\d{6}$/.test(mfaCode.trim())) {
      setMfaError('请输入 6 位数字动态验证码');
      return;
    }
    setOpenMfaDialog(false);
    handleLogin(null, mfaCode.trim());
  };

  return (
    <Container maxWidth="sm">
      <Box
        display="flex"
        justifyContent="center"
        alignItems="center"
        minHeight="100vh"
      >
        <Paper elevation={3} sx={{ padding: 4, width: '100%' }}>
          <Typography variant="h4" gutterBottom align="center" sx={{ marginBottom: 3 }}>
            OpenVPN 管理系统
          </Typography>

          {error && <Alert severity="error" sx={{ marginBottom: 2 }}>{error}</Alert>}
          {message && <Alert severity="success" sx={{ marginBottom: 2 }}>{message}</Alert>}

          <form onSubmit={handleLogin}>
            <TextField
              fullWidth
              label="用户名"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              margin="normal"
              disabled={isLoading}
              required
            />
            <TextField
              fullWidth
              label="密码"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              margin="normal"
              disabled={isLoading}
              required
            />
            <Button
              fullWidth
              type="submit"
              variant="contained"
              color="primary"
              sx={{ marginTop: 3 }}
              disabled={isLoading}
            >
              {isLoading ? <CircularProgress size={24} /> : '登录'}
            </Button>
          </form>
        </Paper>
      </Box>

      {/* MFA 动态验证码弹框 */}
      <Dialog open={openMfaDialog} maxWidth="xs" fullWidth>
        <DialogTitle>动态验证码</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ marginBottom: 2 }}>
            该账号已启用 MFA（多因素认证），请输入认证器 App 中 30 秒刷新一次的 6 位验证码。
          </Typography>
          {mfaError && <Alert severity="error" sx={{ marginBottom: 2 }}>{mfaError}</Alert>}
          <TextField
            autoFocus
            fullWidth
            label="6 位验证码"
            value={mfaCode}
            onChange={(e) => setMfaCode(e.target.value.replace(/\D/g, '').slice(0, 6))}
            inputProps={{ inputMode: 'numeric', maxLength: 6, style: { letterSpacing: '6px', textAlign: 'center' } }}
            onKeyDown={(e) => { if (e.key === 'Enter') handleMfaSubmit(); }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setOpenMfaDialog(false); setIsLoading(false); }}>取消</Button>
          <Button variant="contained" onClick={handleMfaSubmit}>验证</Button>
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default Login;

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
} from '@mui/material';
import { userAPI } from '../api';

const Login = () => {
  const navigate = useNavigate();
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [message, setMessage] = useState('');

  const handleLogin = async (e) => {
    if (e) e.preventDefault();
    if (isLoading) return;
    setError('');
    setMessage('');
    setIsLoading(true);

    try {
      // 执行登录请求
      const loginResponse = await userAPI.login(username, password);

      if (loginResponse.status === 200 && loginResponse.data.result === 'success') {
        setMessage('登录成功！');
        // 立即请求用户信息以验证权限
        const userInfoResponse = await userAPI.getUserInfo();

        if (userInfoResponse.status === 200) {
          const hasAdminGroup = userInfoResponse.data.groups?.some(
            (group) => group.name === 'admin'
          );

          if (hasAdminGroup) {
            // 保存用户信息
            localStorage.setItem('userInfo', JSON.stringify(userInfoResponse.data));
            
            // 延迟导航以显示成功消息
            setTimeout(() => {
              navigate('/dashboard');
            }, 1000);
          } else {
            setError('您没有权限访问该系统');
            setIsLoading(false);
          }
        }
      } else {
        setError('登录失败：用户名或密码错误');
        setIsLoading(false);
      }
    } catch (err) {
      setError('登录失败：' + (err.response?.data?.error || '用户名或密码错误'));
      setIsLoading(false);
    }
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
    </Container>
  );
};

export default Login;

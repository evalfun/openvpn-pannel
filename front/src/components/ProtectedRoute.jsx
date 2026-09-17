import React, { useEffect, useState, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import { userAPI } from '../api';
import { CircularProgress, Box, Container } from '@mui/material';

const ProtectedRoute = ({ children }) => {
  const navigate = useNavigate();
  const [isLoading, setIsLoading] = useState(true);
  const [isAuthorized, setIsAuthorized] = useState(false);
  const hasCheckedRef = useRef(false);

  useEffect(() => {
    if (hasCheckedRef.current) return;
    hasCheckedRef.current = true;

    const checkAuth = async () => {
      try {
        const response = await userAPI.getUserInfo();
        
        // 检查是否返回200状态码
        if (response.status === 200 && response.data.result === 'success') {
          // 检查是否包含admin组
          const hasAdminGroup = response.data.groups?.some(group => group.name === 'admin');
          
          if (hasAdminGroup) {
            // 保存用户信息
            localStorage.setItem('userInfo', JSON.stringify(response.data));
            setIsAuthorized(true);
            setIsLoading(false);
          } else {
            // 没有admin权限，提示并跳转到登录
            alert('您没有权限访问该系统');
            //navigate('/login');
          }
        } else {
          // 返回其他状态，跳转到登录
          //navigate('/login');
        }
      } catch (error) {
        // 请求失败或401，跳转到登录
        if (error.response?.status === 401) {
          //navigate('/login');
        } else {
          //navigate('/login');
        }
      }
    };

    checkAuth();
  }, [navigate]);

  if (isLoading) {
    return (
      <Container maxWidth="sm">
        <Box
          display="flex"
          justifyContent="center"
          alignItems="center"
          minHeight="100vh"
        >
          <CircularProgress />
        </Box>
      </Container>
    );
  }

  return isAuthorized ? children : null;
};

export default ProtectedRoute;

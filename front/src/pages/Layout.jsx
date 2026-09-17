import React, { useState, useEffect, useRef } from 'react';
import {
  AppBar,
  Toolbar,
  Drawer,
  List,
  ListItem,
  ListItemIcon,
  ListItemText,
  Box,
  Typography,
  Button,
  Menu,
  MenuItem,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import MenuIcon from '@mui/icons-material/Menu';
import PeopleIcon from '@mui/icons-material/People';
import SecurityIcon from '@mui/icons-material/Security';
import StorageIcon from '@mui/icons-material/Storage';
import SettingsIcon from '@mui/icons-material/Settings';
import LogoutIcon from '@mui/icons-material/Logout';
import AccountCircleIcon from '@mui/icons-material/AccountCircle';
import HistoryIcon from '@mui/icons-material/History';
import HelpIcon from '@mui/icons-material/Help';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import { useNavigate, Outlet } from 'react-router-dom';
import { userAPI } from '../api';
const DRAWER_WIDTH = 240;

const Layout = () => {
  const navigate = useNavigate();
  const theme = useTheme();
  const isDesktop = useMediaQuery(theme.breakpoints.up('md'));
  const isWideScreen = useMediaQuery('(min-width:1300px)');
  const [drawerOpen, setDrawerOpen] = useState(isWideScreen);
  const [anchorEl, setAnchorEl] = useState(null);
  const [frontendVersion, setFrontendVersion] = useState('');
  const [backendVersion, setBackendVersion] = useState('');
  const hasFetchedBackendVersionRef = useRef(false);

  const userInfo = JSON.parse(localStorage.getItem('userInfo') || '{}');

  useEffect(() => {
    // 获取前端编译日期
    const buildDate = window.__BUILD_DATE__ || 'unknown';
    setFrontendVersion(buildDate);
  }, []);

  useEffect(() => {
    if (hasFetchedBackendVersionRef.current) return;
    hasFetchedBackendVersionRef.current = true;

    // 获取后端版本号
    const fetchBackendVersion = async () => {
      try {
        const response = await fetch('/api/builddate');
        const version = await response.text();
        setBackendVersion(version.trim());
      } catch (err) {
        console.error('获取后端版本号失败:', err);
        setBackendVersion('获取失败');
      }
    };
    fetchBackendVersion();
  }, []);

  const menuItems = [
    { label: '用户管理', icon: <PeopleIcon />, path: '/dashboard/users' },
    { label: '用户组管理', icon: <SecurityIcon />, path: '/dashboard/groups' },
    { label: '服务器管理', icon: <StorageIcon />, path: '/dashboard/servers' },
    { label: '证书管理', icon: <VpnKeyIcon />, path: '/dashboard/certificates' },
    { label: '资源管理', icon: <SettingsIcon />, path: '/dashboard/resources' },
    { label: '日志/状态', icon: <HistoryIcon />, path: '/dashboard/logs' },
    { label: '帮助信息', icon: <HelpIcon />, path: '/dashboard/help' },
  ];

  const handleLogout = async () => {
    try {
      // 调用logout API
      await userAPI.logout();
    } catch (err) {
      console.error('登出请求失败:', err);
    } finally {
      localStorage.removeItem('userInfo');
      localStorage.removeItem('authToken');
      setAnchorEl(null);
      navigate('/login');
    }
  };

  const handleMenuClick = (event) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleDrawerToggle = () => {
    if (!isWideScreen) {
      setDrawerOpen(!drawerOpen);
    }
  };

  // 监听窗口大小变化，宽度大于1300px时保持菜单打开
  useEffect(() => {
    setDrawerOpen(isWideScreen);
  }, [isWideScreen]);

  const drawer = (
    <Box
      sx={{
        padding: 2,
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <Typography variant="h6" sx={{ marginBottom: 3, fontWeight: 'bold' }}>
        系统管理
      </Typography>
      <List sx={{ flex: 1 }}>
        {menuItems.map((item) => (
          <ListItem
            button
            key={item.label}
            onClick={() => {
              navigate(item.path);
              if (!isWideScreen) {
                setDrawerOpen(false);
              }
            }}
          >
            <ListItemIcon>{item.icon}</ListItemIcon>
            <ListItemText primary={item.label} />
          </ListItem>
        ))}
      </List>
      <Box
        sx={{
          borderTop: '1px solid #e0e0e0',
          paddingTop: 2,
          marginTop: 2,
          fontSize: '0.75rem',
        }}
      >
        <Typography variant="caption" sx={{ color: '#999', display: 'block', textAlign: 'center' }}>
          前端版本
        </Typography>
        <Typography variant="caption" sx={{ color: '#666', display: 'block', textAlign: 'center', marginTop: 0.5 }}>
          {frontendVersion}
        </Typography>
        <Typography variant="caption" sx={{ color: '#999', display: 'block', textAlign: 'center', marginTop: 1.5 }}>
          后端版本
        </Typography>
        <Typography variant="caption" sx={{ color: '#666', display: 'block', textAlign: 'center', marginTop: 0.5 }}>
          {backendVersion}
        </Typography>
      </Box>
    </Box>
  );

  return (
    <Box sx={{ height: '100vh', display: 'flex', flexDirection: 'column' }}>
      <AppBar position="fixed" sx={{ zIndex: 1300 }}>
        <Toolbar>
          {!isWideScreen && (
            <Button
              color="inherit"
              onClick={handleDrawerToggle}
              sx={{ marginRight: 2 }}
            >
              <MenuIcon />
            </Button>
          )}
          <Typography variant="h6" sx={{ flex: 1 }}>
            OpenVPN 管理系统
          </Typography>
          <Button
            color="inherit"
            onClick={handleMenuClick}
            startIcon={<AccountCircleIcon />}
          >
            {userInfo.username || '用户'}
          </Button>
          <Menu
            anchorEl={anchorEl}
            open={Boolean(anchorEl)}
            onClose={handleMenuClose}
          >
            <MenuItem onClick={handleMenuClose} disabled>
              {userInfo.description || '管理员'}
            </MenuItem>
            <MenuItem onClick={handleLogout}>
              <LogoutIcon sx={{ marginRight: 1 }} />
              登出
            </MenuItem>
          </Menu>
        </Toolbar>
      </AppBar>

      <Box sx={{ display: 'flex', flex: 1, marginTop: '64px' }}>
        <Drawer
          anchor="left"
          variant={isWideScreen ? 'permanent' : 'temporary'}
          open={drawerOpen}
          onClose={() => !isWideScreen && setDrawerOpen(false)}
          sx={{
            width: DRAWER_WIDTH,
            flexShrink: 0,
            '& .MuiDrawer-paper': {
              width: DRAWER_WIDTH,
              height: '100%',
              boxSizing: 'border-box',
            },
          }}
        >
          {drawer}
        </Drawer>

        <Box
          component="main"
          sx={{
            flex: 1,
            overflow: 'auto',
            backgroundColor: '#f5f5f5',
            display: 'flex',
            flexDirection: 'column',
            alignItems: 'center',
            width: '100%',
          }}
        >
          <Box sx={{ flex: 1, width: '100%' }}>
            <Outlet />
          </Box>
        </Box>
      </Box>
    </Box>
  );
};

export default Layout;

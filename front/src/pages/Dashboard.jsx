import React, { useState, useEffect, useRef } from 'react';
import {
  Box,
  Typography,
  Paper,
  Grid,
  Card,
  CardContent,
  Stack,
  Chip,
  Button,
  Alert,
  CircularProgress,
  Divider,
  LinearProgress,
  useTheme,
} from '@mui/material';
import StorageIcon from '@mui/icons-material/Storage';
import PeopleIcon from '@mui/icons-material/People';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import SpeedIcon from '@mui/icons-material/Speed';
import PublicIcon from '@mui/icons-material/Public';
import SettingsIcon from '@mui/icons-material/Settings';
import RefreshIcon from '@mui/icons-material/Refresh';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import CancelIcon from '@mui/icons-material/Cancel';
import { useNavigate } from 'react-router-dom';
import { dashboardAPI } from '../api';

const EVENT_TYPE_MAP = {
  1: { label: '客户端上线', color: 'success' },
  2: { label: '客户端下线', color: 'warning' },
  3: { label: '认证失败', color: 'error' },
  4: { label: '认证成功', color: 'success' },
  5: { label: '服务端启动成功', color: 'success' },
  6: { label: '服务端停止成功', color: 'warning' },
  7: { label: '服务端启动失败', color: 'error' },
  8: { label: '服务端停止失败', color: 'error' },
  9: { label: '添加用户ACL', color: 'info' },
  10: { label: '删除用户ACL', color: 'info' },
  11: { label: '一次认证成功', color: 'success' },
  12: { label: '二次认证成功', color: 'success' },
  13: { label: '二次认证下线', color: 'warning' },
  14: { label: '一次认证下线', color: 'warning' },
  15: { label: '服务器已恢复', color: 'success' },
};

const formatTime = (ts) => {
  if (!ts) return '-';
  const d = new Date(Number(ts) * 1000);
  const pad = (n) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
};

const StatCard = ({ icon, label, value, sub, color, onClick }) => (
  <Card
    onClick={onClick}
    sx={{
      height: '100%',
      cursor: onClick ? 'pointer' : 'default',
      transition: 'box-shadow .2s, transform .2s',
      '&:hover': onClick ? { boxShadow: 4, transform: 'translateY(-2px)' } : {},
    }}
  >
    <CardContent sx={{ display: 'flex', alignItems: 'center', gap: 2, padding: 2 }}>
      <Box
        sx={{
          width: 52,
          height: 52,
          borderRadius: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          backgroundColor: `${color}.main`,
          color: '#fff',
          flexShrink: 0,
        }}
      >
        {icon}
      </Box>
      <Box sx={{ minWidth: 0 }}>
        <Typography variant="body2" color="textSecondary" noWrap>
          {label}
        </Typography>
        <Typography variant="h5" sx={{ fontWeight: 'bold', lineHeight: 1.2 }}>
          {value}
        </Typography>
        {sub && (
          <Typography variant="caption" color="textSecondary" noWrap>
            {sub}
          </Typography>
        )}
      </Box>
    </CardContent>
  </Card>
);

const Dashboard = () => {
  const navigate = useNavigate();
  const theme = useTheme();
  const initializedRef = useRef(false);
  const [data, setData] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);

  const fetchSummary = async () => {
    try {
      setLoading(true);
      setError(null);
      const res = await dashboardAPI.getSummary();
      if (res.data.result === 'success') {
        setData(res.data.data);
      } else {
        setError(res.data.error || '获取概览失败');
      }
    } catch (err) {
      setError('获取概览失败：' + (err.response?.data?.error || err.message));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (!initializedRef.current) {
      initializedRef.current = true;
      fetchSummary();
    }
  }, []);

  const summary = data?.summary || {};
  const servers = data?.servers || [];
  const events = data?.recent_events || [];
  const runningRatio = summary.server_total > 0 ? (summary.server_running / summary.server_total) * 100 : 0;
  const accentColor = theme.palette.primary.main;

  return (
    <Box sx={{ width: '100%', padding: { xs: 2, sm: 3 } }}>
      {/* 顶部标题 */}
      <Box
        sx={{
          mb: 3,
          p: 3,
          borderRadius: 2,
          color: '#fff',
          background: `linear-gradient(135deg, ${accentColor} 0%, #21a1f1 100%)`,
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          flexWrap: 'wrap',
          gap: 2,
        }}
      >
        <Box>
          <Typography variant="h4" sx={{ fontWeight: 'bold' }}>
            OpenVPN 管理面板
          </Typography>
          <Typography variant="body2" sx={{ opacity: 0.9, mt: 0.5 }}>
            服务运行概览与最新动态
          </Typography>
        </Box>
        <Button
          variant="contained"
          color="inherit"
          startIcon={<RefreshIcon />}
          onClick={fetchSummary}
          disabled={loading}
          sx={{ color: accentColor, backgroundColor: '#fff', '&:hover': { backgroundColor: '#f0f0f0' } }}
        >
          刷新
        </Button>
      </Box>

      {error && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {loading && !data ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', padding: 6 }}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          {/* 关键指标 */}
          <Grid container spacing={2} sx={{ mb: 3 }}>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <StatCard
                icon={<StorageIcon />}
                label="服务器"
                value={`${summary.server_running ?? 0} / ${summary.server_total ?? 0}`}
                sub="运行 / 总数"
                color="primary"
                onClick={() => navigate('/dashboard/servers')}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <StatCard
                icon={<PublicIcon />}
                label="在线客户端"
                value={summary.online_clients ?? 0}
                sub="当前在线连接"
                color="success"
                onClick={() => navigate('/dashboard/logs')}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <StatCard
                icon={<PeopleIcon />}
                label="用户"
                value={summary.user_total ?? 0}
                sub={`${summary.group_total ?? 0} 个用户组`}
                color="info"
                onClick={() => navigate('/dashboard/users')}
              />
            </Grid>
            <Grid size={{ xs: 12, sm: 6, md: 3 }}>
              <StatCard
                icon={<VpnKeyIcon />}
                label="证书"
                value={(summary.cert_ca_total ?? 0) + (summary.cert_server_total ?? 0) + (summary.cert_client_total ?? 0)}
                sub={`CA ${summary.cert_ca_total ?? 0} · 服务端 ${summary.cert_server_total ?? 0} · 客户端 ${summary.cert_client_total ?? 0}`}
                color="secondary"
                onClick={() => navigate('/dashboard/certificates')}
              />
            </Grid>
          </Grid>

          <Grid container spacing={3}>
            {/* 左侧：服务器状态 */}
            <Grid size={{ xs: 12, lg: 7 }}>
              <Paper sx={{ p: 2.5, height: '100%' }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
                  <Typography variant="h6">服务器状态</Typography>
                  <Button size="small" onClick={() => navigate('/dashboard/servers')}>
                    管理
                  </Button>
                </Box>
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
                  <LinearProgress
                    variant="determinate"
                    value={runningRatio}
                    color={runningRatio === 100 ? 'success' : runningRatio === 0 ? 'error' : 'warning'}
                    sx={{ flex: 1, height: 8, borderRadius: 4 }}
                  />
                  <Typography variant="caption" color="textSecondary">
                    {runningRatio.toFixed(0)}%
                  </Typography>
                </Box>
                {servers.length === 0 ? (
                  <Typography color="textSecondary" sx={{ py: 3, textAlign: 'center' }}>
                    暂无服务器，请先在「服务器管理」中创建
                  </Typography>
                ) : (
                  <Stack divider={<Divider />} spacing={0}>
                    {servers.map((s) => (
                      <Box
                        key={s.id}
                        sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', py: 1.2 }}
                      >
                        <Box sx={{ minWidth: 0 }}>
                          <Typography variant="body1" sx={{ fontWeight: 500 }} noWrap>
                            {s.name}
                          </Typography>
                          <Typography variant="caption" color="textSecondary" noWrap>
                            {s.proto}/{s.dev} · {s.local || '0.0.0.0'}:{s.port} · {s.server_cidr}
                          </Typography>
                        </Box>
                        <Chip
                          size="small"
                          icon={s.running ? <CheckCircleIcon /> : <CancelIcon />}
                          label={s.running ? '运行中' : '已停止'}
                          color={s.running ? 'success' : 'default'}
                          variant={s.running ? 'filled' : 'outlined'}
                        />
                      </Box>
                    ))}
                  </Stack>
                )}
              </Paper>
            </Grid>

            {/* 右侧：系统信息 + 达量限速 */}
            <Grid size={{ xs: 12, lg: 5 }}>
              <Stack spacing={3} sx={{ height: '100%' }}>
                <Paper sx={{ p: 2.5 }}>
                  <Typography variant="h6" sx={{ mb: 1.5 }}>
                    系统信息
                  </Typography>
                  <Stack spacing={1.2}>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                      <Typography variant="body2" color="textSecondary">当前资源集</Typography>
                      <Typography variant="body2" sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
                        <SettingsIcon fontSize="small" color="action" />
                        {summary.active_resource_set_name || summary.active_resource_set || '-'}
                      </Typography>
                    </Box>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                      <Typography variant="body2" color="textSecondary">后端版本</Typography>
                      <Typography variant="body2">{summary.backend_build_date || '-'}</Typography>
                    </Box>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                      <Typography variant="body2" color="textSecondary">累计事件</Typography>
                      <Typography variant="body2">{summary.event_total ?? 0}</Typography>
                    </Box>
                    <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
                      <Typography variant="body2" color="textSecondary">服务器时间</Typography>
                      <Typography variant="body2">{formatTime(summary.server_time)}</Typography>
                    </Box>
                  </Stack>
                </Paper>

                <Paper
                  sx={{ p: 2.5, flex: 1, cursor: 'pointer' }}
                  onClick={() => navigate('/dashboard/ratelimit')}
                >
                  <Typography variant="h6" sx={{ mb: 1.5, display: 'flex', alignItems: 'center', gap: 0.5 }}>
                    <SpeedIcon fontSize="small" color="action" /> 达量限速
                  </Typography>
                  <Stack direction="row" spacing={3}>
                    <Box>
                      <Typography variant="h5" sx={{ fontWeight: 'bold' }}>
                        {summary.ratelimit_plan_total ?? 0}
                      </Typography>
                      <Typography variant="caption" color="textSecondary">限速方案</Typography>
                    </Box>
                    <Box>
                      <Typography variant="h5" sx={{ fontWeight: 'bold' }}>
                        {summary.ratelimit_user_total ?? 0}
                      </Typography>
                      <Typography variant="caption" color="textSecondary">关联用户</Typography>
                    </Box>
                  </Stack>
                </Paper>
              </Stack>
            </Grid>

            {/* 最新动态 */}
            <Grid size={{ xs: 12 }}>
              <Paper sx={{ p: 2.5 }}>
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1.5 }}>
                  <Typography variant="h6">最新动态</Typography>
                  <Button size="small" onClick={() => navigate('/dashboard/logs')}>
                    查看全部
                  </Button>
                </Box>
                {events.length === 0 ? (
                  <Typography color="textSecondary" sx={{ py: 3, textAlign: 'center' }}>
                    暂无事件记录
                  </Typography>
                ) : (
                  <Stack divider={<Divider />} spacing={0}>
                    {events.map((e) => {
                      const info = EVENT_TYPE_MAP[e.event_type] || { label: '未知', color: 'default' };
                      return (
                        <Box
                          key={e.id}
                          sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', py: 1, gap: 2 }}
                        >
                          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0 }}>
                            <Chip label={info.label} color={info.color} size="small" sx={{ flexShrink: 0 }} />
                            <Typography variant="body2" noWrap sx={{ minWidth: 0 }}>
                              {e.event_data || '-'}
                            </Typography>
                          </Box>
                          <Stack direction="row" spacing={2} sx={{ flexShrink: 0 }}>
                            <Typography variant="caption" color="textSecondary">
                              {e.server_name || `服务器 ${e.server_id}`}
                            </Typography>
                            <Typography variant="caption" color="textSecondary">
                              {formatTime(e.event_time)}
                            </Typography>
                          </Stack>
                        </Box>
                      );
                    })}
                  </Stack>
                )}
              </Paper>
            </Grid>
          </Grid>
        </>
      )}
    </Box>
  );
};

export default Dashboard;

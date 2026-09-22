import React, { useState, useEffect, useRef } from 'react';
import {
  Box,
  Typography,
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Pagination,
  Button,
  TextField,
  Alert,
  CircularProgress,
  Stack,
  Chip,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  IconButton,
  Card,
  CardContent,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import {
  ArrowBack as ArrowBackIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import { certificateAPI } from '../api';

const CERT_EVENT_TYPE_MAP = {
  1: { label: '生成CA', color: 'success' },
  2: { label: '导入证书', color: 'info' },
  3: { label: '签发证书', color: 'success' },
  4: { label: '删除证书', color: 'error' },
  5: { label: '下载证书', color: 'info' },
  6: { label: '下载私钥', color: 'warning' },
  7: { label: '服务器引用', color: 'primary' },
  8: { label: '客户端引用证书', color: 'primary' },
  9: { label: '清空事件', color: 'warning' },
};

const formatTime = (unix) => {
  if (!unix) return '-';
  return new Date(unix * 1000).toLocaleString();
};

const CertificateEvents = () => {
  const navigate = useNavigate();
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

  const [list, setList] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [type, setType] = useState('');
  const [query, setQuery] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const seqRef = useRef(0);

  const loadEvents = async (override = {}) => {
    const p = override.page ?? page;
    const ps = override.pageSize ?? pageSize;
    const t = override.type ?? type;
    const q = override.query ?? query;
    const seq = ++seqRef.current;
    setLoading(true);
    setError('');
    try {
      const res = await certificateAPI.listEvents({ type: t, query: q, page: p + 1, pageSize: ps });
      if (seq !== seqRef.current) return;
      setList(res.data.data || []);
      setTotal(res.data.total || 0);
      setPage(p);
      setPageSize(ps);
      setType(t);
      setQuery(q);
    } catch (err) {
      if (seq === seqRef.current) {
        setError('加载证书事件失败：' + (err.response?.data?.error || err.message));
      }
    } finally {
      if (seq === seqRef.current) setLoading(false);
    }
  };

  useEffect(() => {
    loadEvents({ page: 0 });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleClear = async () => {
    if (!window.confirm('确定要清空所有证书事件吗？此操作不可恢复。')) return;
    try {
      await certificateAPI.clearEvents();
      loadEvents({ page: 0 });
    } catch (err) {
      setError('清空证书事件失败：' + (err.response?.data?.error || err.message));
    }
  };

  // 移动端卡片视图
  const MobileEventCard = ({ event }) => {
    const info = CERT_EVENT_TYPE_MAP[event.event_type] || { label: '未知', color: 'default' };
    return (
      <Card sx={{ marginBottom: 2 }}>
        <CardContent>
          <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={1} sx={{ marginBottom: 1 }}>
            <Chip label={info.label} size="small" color={info.color} variant="outlined" />
            <Typography variant="caption" color="textSecondary">
              {formatTime(event.event_time)}
            </Typography>
          </Stack>
          <Stack spacing={0.5}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
              <Typography variant="body2" color="textSecondary" sx={{ flexShrink: 0 }}>
                操作人:
              </Typography>
              <Typography variant="body2" sx={{ textAlign: 'right', wordBreak: 'break-all' }}>
                {event.operator_username || '-'}
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
              <Typography variant="body2" color="textSecondary" sx={{ flexShrink: 0 }}>
                证书:
              </Typography>
              <Typography variant="body2" sx={{ textAlign: 'right', wordBreak: 'break-all' }}>
                {event.cert_name || '-'}
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
              <Typography variant="body2" color="textSecondary" sx={{ flexShrink: 0 }}>
                服务器:
              </Typography>
              <Typography variant="body2" sx={{ textAlign: 'right', wordBreak: 'break-all' }}>
                {event.server_name || '-'}
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', gap: 2 }}>
              <Typography variant="body2" color="textSecondary" sx={{ flexShrink: 0 }}>
                对端地址:
              </Typography>
              <Typography variant="body2" sx={{ textAlign: 'right', wordBreak: 'break-all' }}>
                {event.real_ip_addr || '-'}
              </Typography>
            </Box>
          </Stack>
          <Typography
            variant="body2"
            sx={{ marginTop: 1, fontFamily: 'monospace', fontSize: 12, wordBreak: 'break-all', whiteSpace: 'pre-wrap' }}
          >
            {event.event_data}
          </Typography>
        </CardContent>
      </Card>
    );
  };

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Stack
        direction="row"
        alignItems="center"
        spacing={1}
        sx={{ marginBottom: 2, flexWrap: 'wrap', rowGap: 1 }}
      >
        <Stack direction="row" alignItems="center" spacing={1} sx={{ flexGrow: 1, minWidth: 180 }}>
          <IconButton onClick={() => navigate('/dashboard/certificates')}>
            <ArrowBackIcon />
          </IconButton>
          <Typography variant="h5" sx={{ wordBreak: 'break-word' }}>
            证书操作事件
          </Typography>
        </Stack>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, justifyContent: 'flex-end' }}>
          <Button variant="outlined" startIcon={<RefreshIcon />} onClick={() => loadEvents()} disabled={loading}>
            刷新
          </Button>
          <Button variant="outlined" color="error" startIcon={<DeleteIcon />} onClick={handleClear} disabled={total === 0}>
            清空
          </Button>
        </Box>
      </Stack>

      {error && (
        <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      )}

      <Paper sx={{ padding: 1 }}>
        <Stack
          direction={{ xs: 'column', sm: 'row' }}
          spacing={1}
          sx={{ marginBottom: 2, padding: 1 }}
        >
          <FormControl size="small" fullWidth={isMobile} sx={{ minWidth: 150 }}>
            <InputLabel>事件类型</InputLabel>
            <Select
              value={type}
              label="事件类型"
              onChange={(e) => loadEvents({ type: e.target.value, page: 0 })}
            >
              <MenuItem value="">全部</MenuItem>
              {Object.entries(CERT_EVENT_TYPE_MAP).map(([value, info]) => (
                <MenuItem key={value} value={value}>
                  {info.label}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <TextField
            size="small"
            fullWidth={isMobile}
            label="搜索（证书/服务器/内容）"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') loadEvents({ query: e.target.value, page: 0 });
            }}
          />
          <Button variant="outlined" fullWidth={isMobile} onClick={() => loadEvents({ query, page: 0 })}>
            查询
          </Button>
        </Stack>

        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
            <CircularProgress />
          </Box>
        ) : isMobile ? (
          <Box>
            {list.length === 0 ? (
              <Box sx={{ padding: 3, textAlign: 'center' }}>
                <Typography color="textSecondary">暂无证书事件</Typography>
              </Box>
            ) : (
              list.map((event) => <MobileEventCard key={event.id} event={event} />)
            )}
          </Box>
        ) : (
          <TableContainer>
            <Table>
              <TableHead>
                <TableRow sx={{ backgroundColor: '#f5f5f5' }}>
                  <TableCell sx={{ fontWeight: 'bold' }}>时间</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>类型</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>操作人</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>证书</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>对端地址</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>事件数据</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {list.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={7} align="center" sx={{ padding: 3 }}>
                      暂无证书事件
                    </TableCell>
                  </TableRow>
                ) : (
                  list.map((event) => {
                    const info = CERT_EVENT_TYPE_MAP[event.event_type] || { label: '未知', color: 'default' };
                    return (
                      <TableRow key={event.id} hover>
                        <TableCell sx={{ whiteSpace: 'nowrap' }}>{formatTime(event.event_time)}</TableCell>
                        <TableCell>
                          <Chip label={info.label} size="small" color={info.color} variant="outlined" />
                        </TableCell>
                        <TableCell>{event.operator_username || '-'}</TableCell>
                        <TableCell>{event.cert_name || '-'}</TableCell>
                        <TableCell>{event.real_ip_addr || '-'}</TableCell>
                        <TableCell sx={{ wordBreak: 'break-all', fontFamily: 'monospace', fontSize: 12 }}>
                          {event.event_data}
                        </TableCell>
                      </TableRow>
                    );
                  })
                )}
              </TableBody>
            </Table>
          </TableContainer>
        )}

        <Stack
          direction="row"
          spacing={2}
          alignItems="center"
          sx={{ marginTop: 1, marginBottom: 1, paddingX: 1, flexWrap: 'wrap', rowGap: 1 }}
        >
          <FormControl sx={{ minWidth: 120 }}>
            <InputLabel>每页数量</InputLabel>
            <Select
              value={pageSize}
              label="每页数量"
              onChange={(e) => loadEvents({ pageSize: e.target.value, page: 0 })}
            >
              <MenuItem value={10}>10</MenuItem>
              <MenuItem value={20}>20</MenuItem>
              <MenuItem value={50}>50</MenuItem>
              <MenuItem value={100}>100</MenuItem>
            </Select>
          </FormControl>
          <Typography>总共 {total} 条事件</Typography>
          <Pagination
            count={Math.max(1, Math.ceil(total / pageSize))}
            page={page + 1}
            onChange={(e, value) => loadEvents({ page: value - 1 })}
            color="primary"
          />
        </Stack>
      </Paper>
    </Box>
  );
};

export default CertificateEvents;

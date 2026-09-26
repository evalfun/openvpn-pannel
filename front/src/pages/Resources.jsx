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
  Button,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Alert,
  CircularProgress,
  Stack,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material';
import { Edit as EditIcon, Refresh as RefreshIcon, SwapHoriz as SwapHorizIcon } from '@mui/icons-material';
import client from '../api/client';

const Resources = () => {
  const initializedRef = useRef(false);
  const [resources, setResources] = useState([]);
  const [sets, setSets] = useState([]);
  const [activeSet, setActiveSet] = useState('');
  const [viewSet, setViewSet] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [dialogError, setDialogError] = useState(null);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedResource, setSelectedResource] = useState(null);
  const [resourceContent, setResourceContent] = useState('');
  const [saving, setSaving] = useState(false);
  const [successMessage, setSuccessMessage] = useState('');
  // allow_edit 由后端 config.json 的 allow_edit_resource 决定
  const [allowEdit, setAllowEdit] = useState(false);

  // 获取资源列表（含资源集）
  const fetchResources = async () => {
    try {
      setLoading(true);
      setError(null);
      const response = await client.get('/resource/list');
      setResources(response.data.data || []);
      setSets(response.data.sets || []);
      setActiveSet(response.data.active_set || '');
      // 默认查看当前启用的资源集
      setViewSet((prev) => prev || response.data.active_set || '');
      setAllowEdit(!!response.data.allow_edit);
    } catch (err) {
      setError('加载资源列表失败：' + (err.response?.data?.error || err.message));
    } finally {
      setLoading(false);
    }
  };

  // 切换当前启用的资源集
  const handleActivateSet = async (setId) => {
    if (!window.confirm('确认切换到该资源集吗？切换后所有服务器将共用新资源集，请重启相关服务器进程使其生效。')) {
      return;
    }
    try {
      setLoading(true);
      setError(null);
      await client.post('/resource/set/active', { set_id: setId });
      setSuccessMessage('已切换资源集，请重启相关服务器进程使其生效');
      setActiveSet(setId);
      setViewSet(setId);
      // 重新加载（sets 中的 active 标记需要刷新）
      const response = await client.get('/resource/list');
      setResources(response.data.data || []);
      setSets(response.data.sets || []);
      setActiveSet(response.data.active_set || '');
      setAllowEdit(!!response.data.allow_edit);
    } catch (err) {
      setError('切换资源集失败：' + (err.response?.data?.error || err.message));
    } finally {
      setLoading(false);
    }
  };

  // 获取资源内容
  const openEditDialog = async (resource) => {
    try {
      setDialogError(null);
      const response = await client.get(`/resource/get?id=${resource.id}&set=${viewSet}`, {
        responseType: 'text',
      });
      setSelectedResource(resource);
      setResourceContent(response.data);
      setEditDialogOpen(true);
    } catch (err) {
      setError('加载资源内容失败：' + (err.response?.data?.error || err.message));
    }
  };

  // 保存资源
  const handleSave = async () => {
    try {
      setSaving(true);
      setDialogError(null);
      await client.post('/resource/write', {
        set_id: viewSet,
        id: selectedResource.id,
        content: resourceContent,
      });
      setSuccessMessage('资源已成功保存');
      setEditDialogOpen(false);
    } catch (err) {
      setDialogError('保存资源失败：' + (err.response?.data?.error || err.message));
    } finally {
      setSaving(false);
    }
  };

  // 重置资源
  const handleReset = async () => {
    if (window.confirm('确认要将该资源重置为当前资源集的内置默认值吗？')) {
      try {
        setSaving(true);
        setDialogError(null);
        await client.post('/resource/delete', {
          set_id: viewSet,
          id: selectedResource.id,
        });
        setSuccessMessage('资源已成功重置为内置默认值');
        setEditDialogOpen(false);
      } catch (err) {
        setDialogError('重置资源失败：' + (err.response?.data?.error || err.message));
      } finally {
        setSaving(false);
      }
    }
  };

  // 关闭对话框
  const handleCloseDialog = () => {
    setEditDialogOpen(false);
    setSelectedResource(null);
    setResourceContent('');
    setDialogError(null);
  };

  // 初始加载 - 使用useRef防止StrictMode双重调用
  useEffect(() => {
    if (!initializedRef.current) {
      initializedRef.current = true;
      fetchResources();
    }
  }, []);

  const viewSetInfo = sets.find((s) => s.id === viewSet);
  const isViewingActive = viewSet === activeSet;

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Typography variant="h5" sx={{ marginBottom: 3 }}>
        资源管理
      </Typography>

      {successMessage && (
        <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setSuccessMessage('')}>
          {successMessage}
        </Alert>
      )}

      {error && (
        <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      {!loading && !allowEdit && (
        <Alert severity="warning" sx={{ marginBottom: 2 }}>
          资源编辑已禁用。如需修改、重置或切换资源集，请在 config.json 中设置
          {' '}<code>allow_edit_resource: true</code>{' '}后重启服务。
        </Alert>
      )}

      {/* 资源集选择 */}
      <Paper sx={{ padding: 2, marginBottom: 2 }}>
        <Typography variant="h6" sx={{ marginBottom: 1 }}>
          资源集
        </Typography>
        <Typography variant="body2" color="textSecondary" sx={{ marginBottom: 2 }}>
          每套资源集包含全部脚本与页面文件。当前启用：
          <strong>{sets.find((s) => s.id === activeSet)?.name || activeSet || '-'}</strong>
          ，所有服务器共用。切换资源集后需<strong>重启服务器进程</strong>才能生效。
        </Typography>
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2} alignItems={{ sm: 'center' }}>
          <FormControl sx={{ minWidth: 320 }} size="small">
            <InputLabel id="view-set-label">查看的资源集</InputLabel>
            <Select
              labelId="view-set-label"
              label="查看的资源集"
              value={viewSet}
              onChange={(e) => setViewSet(e.target.value)}
            >
              {sets.map((s) => (
                <MenuItem key={s.id} value={s.id}>
                  {s.name}{s.active ? '（当前启用）' : ''}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          <Button
            variant="contained"
            startIcon={<SwapHorizIcon />}
            disabled={!allowEdit || isViewingActive || !viewSet}
            onClick={() => handleActivateSet(viewSet)}
          >
            启用该资源集
          </Button>
          {viewSetInfo && (
            <Typography variant="body2" color="textSecondary">
              {viewSetInfo.description}
            </Typography>
          )}
        </Stack>
      </Paper>

      <Paper sx={{ padding: 1 }}>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
            <CircularProgress />
          </Box>
        ) : (
          <TableContainer sx={{ padding: 0 }}>
            <Table>
              <TableHead>
                <TableRow sx={{ backgroundColor: '#f5f5f5' }}>
                  <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>资源ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>描述</TableCell>
                  <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>
                    操作
                  </TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {resources.map((resource) => (
                  <TableRow key={resource.id} hover>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>{resource.id}</TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>{resource.description}</TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<EditIcon />}
                        onClick={() => openEditDialog(resource)}
                        disabled={!allowEdit}
                        sx={{ marginRight: 1, paddingTop: 0.5, paddingBottom: 0.5 }}
                      >
                        编辑
                      </Button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Paper>

      {/* 编辑对话框 */}
      <Dialog open={editDialogOpen} onClose={handleCloseDialog} maxWidth="md" fullWidth>
        <DialogTitle>
          编辑资源：{selectedResource?.id}
          <Typography variant="caption" color="textSecondary" sx={{ marginLeft: 1 }}>
            （资源集：{viewSetInfo?.name || viewSet}）
          </Typography>
        </DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {dialogError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setDialogError(null)}>
              {dialogError}
            </Alert>
          )}
          <TextField
            fullWidth
            multiline
            rows={15}
            variant="outlined"
            value={resourceContent}
            onChange={(e) => setResourceContent(e.target.value)}
            placeholder="输入资源内容"
            sx={{ fontFamily: 'monospace' }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleReset} color="error" disabled={saving}>
            <RefreshIcon sx={{ marginRight: 1 }} />
            重置为内置默认值
          </Button>
          <Button onClick={handleCloseDialog} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSave} variant="contained" disabled={saving}>
            {saving ? <CircularProgress size={24} /> : '保存'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Resources;

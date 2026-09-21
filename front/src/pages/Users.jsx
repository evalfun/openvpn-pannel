import React, { useState, useEffect, useCallback, useMemo } from 'react';
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
  CircularProgress,
  Alert,
  Stack,
  Pagination,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Tooltip,
  Modal,
  Tabs,
  Tab,
  Chip,
  Checkbox,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import EditIcon from '@mui/icons-material/Edit';
import VisibilityIcon from '@mui/icons-material/Visibility';
import DeleteForeverIcon from '@mui/icons-material/DeleteForever';
import UploadFileIcon from '@mui/icons-material/UploadFile';
import DownloadIcon from '@mui/icons-material/Download';
import { userManageAPI, userServerPermAPI } from '../api';

// 用户限速策略，取值与后端 models.RATE_LIMIT_TYPE_* 一致
const RATE_LIMIT_OPTIONS = [
  { value: 1, label: '依据活跃用户组的最低速率（默认）' },
  { value: 2, label: '依据活跃用户组的最高速率' },
  { value: 3, label: '固定限速' },
  { value: 0, label: '不设置任何限速策略（不限速）' },
];

const RATE_LIMIT_LABELS = {
  0: '不限速',
  1: '活跃组最低',
  2: '活跃组最高',
  3: '固定',
};

// 列表里展示用户的限速策略摘要
const formatRateLimit = (user) => {
  const label = RATE_LIMIT_LABELS[user.rate_limit_type] ?? '活跃组最低';
  if (user.rate_limit_type === 3) {
    return `${label} ↑${user.upload_limit_kb || 0} ↓${user.download_limit_kb || 0} KB/s`;
  }
  return label;
};

// 限速单位 KB/s；上传=服务器->客户端，下载=客户端->服务器；0=不限速
const RateLimitFields = ({ value, onChange }) => (  <Stack spacing={2}>
    <TextField
      select
      fullWidth
      label="限速策略"
      value={value.rate_limit_type ?? 1}
      onChange={(e) => onChange({ ...value, rate_limit_type: Number(e.target.value) })}
    >
      {RATE_LIMIT_OPTIONS.map((option) => (
        <MenuItem key={option.value} value={option.value}>
          {option.label}
        </MenuItem>
      ))}
    </TextField>
    {value.rate_limit_type === 3 && (
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
        <TextField
          fullWidth
          type="number"
          label="上传限速 (KB/s)"
          helperText="服务器 -> 客户端，0=不限速"
          value={value.upload_limit_kb ?? 0}
          onChange={(e) =>
            onChange({ ...value, upload_limit_kb: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })
          }
        />
        <TextField
          fullWidth
          type="number"
          label="下载限速 (KB/s)"
          helperText="客户端 -> 服务器，0=不限速"
          value={value.download_limit_kb ?? 0}
          onChange={(e) =>
            onChange({ ...value, download_limit_kb: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })
          }
        />
      </Stack>
    )}
  </Stack>
);

const Users = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  
  const [users, setUsers] = useState([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [createUserError, setCreateUserError] = useState('');
  const [editUserError, setEditUserError] = useState('');
  const [userInfoError, setUserInfoError] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [totalCount, setTotalCount] = useState(0);
  // reloadKey 用于在不改变页码（例如在第 1 页搜索/刷新）时强制重新加载列表。
  const [reloadKey, setReloadKey] = useState(0);
  const [queryString, setQueryString] = useState('');
  const [selectedUserIds, setSelectedUserIds] = useState([]);
  
  const [openDialog, setOpenDialog] = useState(false);
  const [openEditDialog, setOpenEditDialog] = useState(false);
  const [openBatchDialog, setOpenBatchDialog] = useState(false);
  const [batchCsv, setBatchCsv] = useState('');
  const [batchFileName, setBatchFileName] = useState('');
  const [batchError, setBatchError] = useState('');
  const [batchResults, setBatchResults] = useState(null);
  const [batchSubmitLoading, setBatchSubmitLoading] = useState(false);
  const [openDescriptionModal, setOpenDescriptionModal] = useState(false);
  const [selectedDescription, setSelectedDescription] = useState('');
  const [selectedUserNameForDesc, setSelectedUserNameForDesc] = useState('');
  
  const [openUserInfoDialog, setOpenUserInfoDialog] = useState(false);
  const [userInfoTabValue, setUserInfoTabValue] = useState(0);
  const [selectedUser, setSelectedUser] = useState(null);
  const [userGroups, setUserGroups] = useState([]);
  const [userServerPerms, setUserServerPerms] = useState({});
  const [userInfoLoading, setUserInfoLoading] = useState(false);
  const [aclModalOpen, setAclModalOpen] = useState(false);
  const [selectedAclContent, setSelectedAclContent] = useState('');
  const [serverAclMap, setServerAclMap] = useState({});
  
  const [formData, setFormData] = useState({
    username: '',
    password: '',
    description: '',
    rate_limit_type: 1,
    upload_limit_kb: 0,
    download_limit_kb: 0,
  });
  
  const [editingUser, setEditingUser] = useState({
    username: '',
    password: '',
    description: '',
    rate_limit_type: 1,
    upload_limit_kb: 0,
    download_limit_kb: 0,
  });

  const loadUsers = async (p = 1, ps = pageSize) => {
    setIsLoading(true);
    setError('');
    try {
      const response = await userManageAPI.getUserList(p, ps, queryString);
      if (response.data.result === 'success') {
        setUsers(response.data.data || []);
        setTotalCount(response.data.count || 0);
        setSelectedUserIds([]);
      }
    } catch (err) {
      setError('加载用户列表失败');
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  // 触发一次列表重新加载（用于搜索/刷新/增删改后，即使页码未变也能刷新）
  const reloadUsers = () => setReloadKey((k) => k + 1);

  useEffect(() => {
    loadUsers(page, pageSize);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [page, pageSize, reloadKey]);

  const handleCreateUser = async () => {
    if (!formData.username || !formData.password) {
      setCreateUserError('用户名和密码不能为空');
      return;
    }
    try {
      setIsLoading(true);
      const response = await userManageAPI.createUser(formData);
      if (response.data.result === 'success') {
        setSuccess('用户创建成功');
        setFormData({ username: '', password: '', description: '', rate_limit_type: 1, upload_limit_kb: 0, download_limit_kb: 0 });
        setOpenDialog(false);
        setPage(1);
        reloadUsers();
      } else {
        setCreateUserError(response.data.error || '创建用户失败');
      }
    } catch (err) {
      setCreateUserError(err.response?.data?.error || '创建用户失败');
    } finally {
      setIsLoading(false);
    }
  };

  // ===== 批量添加用户 =====
  const downloadUserTemplate = () => {
    // 首行表头：用户名,密码,用户备注,用户组；备注与用户组可留空。
    const csv = '\ufeff用户名,密码,用户备注,用户组\nzhangsan,password123,张三,vip\nlisi,password456,,\n';
    const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' });
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = '用户批量添加模板.csv';
    document.body.appendChild(a);
    a.click();
    a.remove();
    window.URL.revokeObjectURL(url);
  };

  const openBatchAddDialog = () => {
    setBatchCsv('');
    setBatchFileName('');
    setBatchError('');
    setBatchResults(null);
    setOpenBatchDialog(true);
  };

  const closeBatchDialog = () => {
    setOpenBatchDialog(false);
    setBatchFileName('');
  };

  const handleBatchFileChange = (e) => {
    const file = e.target.files?.[0];
    e.target.value = ''; // 允许重复选择同一文件
    if (!file) return;
    if (file.size > 1024 * 1024) {
      setBatchError('CSV 文件不能超过 1 MB');
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      setBatchCsv(typeof reader.result === 'string' ? reader.result : '');
      setBatchFileName(file.name);
      setBatchError('');
      setBatchResults(null);
    };
    reader.onerror = () => setBatchError('读取文件失败');
    reader.readAsText(file, 'utf-8');
  };

  const handleBatchSubmit = async () => {
    if (!batchCsv.trim()) {
      setBatchError('请先选择或粘贴 CSV 内容');
      return;
    }
    try {
      setBatchSubmitLoading(true);
      setBatchError('');
      const response = await userManageAPI.createUsersBatch(batchCsv);
      if (response.data.result === 'success') {
        const items = response.data.data || [];
        setBatchResults({
          items,
          successCount: response.data.success_count || 0,
          failedCount: response.data.failed_count || 0,
        });
        if ((response.data.success_count || 0) > 0) {
          setPage(1);
          reloadUsers();
        }
      } else {
        setBatchError(response.data.error || '批量添加失败');
      }
    } catch (err) {
      setBatchError(err.response?.data?.error || '批量添加失败');
    } finally {
      setBatchSubmitLoading(false);
    }
  };

  const handleEditUser = (user) => {
    setEditingUser({
      username: user.username,
      password: '',
      description: user.description || '',
      rate_limit_type: user.rate_limit_type ?? 1,
      upload_limit_kb: user.upload_limit_kb ?? 0,
      download_limit_kb: user.download_limit_kb ?? 0,
    });
    setEditUserError('');
    setOpenEditDialog(true);
  };

  const handleUpdateUser = async () => {
    try {
      setIsLoading(true);
      const updateData = {
        username: editingUser.username,
        description: editingUser.description,
        rate_limit_type: editingUser.rate_limit_type,
        upload_limit_kb: editingUser.upload_limit_kb,
        download_limit_kb: editingUser.download_limit_kb,
      };
      if (editingUser.password) {
        updateData.password = editingUser.password;
      }
      const response = await userManageAPI.updateUserInfo(updateData);
      if (response.data.result === 'success') {
        setSuccess('用户信息更新成功');
        setOpenEditDialog(false);
        reloadUsers();
      } else {
        setEditUserError(response.data.error || '更新失败');
      }
    } catch (err) {
      setEditUserError(err.response?.data?.error || '更新失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteUsers = async () => {
    if (selectedUserIds.length === 0) {
      setError('请选择要删除的用户');
      return;
    }
    if (!window.confirm(`确定要删除选中的 ${selectedUserIds.length} 个用户吗？`)) {
      return;
    }
    try {
      setIsLoading(true);
      const response = await userManageAPI.deleteUsers(selectedUserIds);
      if (response.data.result === 'success') {
        setSuccess('用户删除成功');
        setSelectedUserIds([]);
        setPage(1);
        reloadUsers();
      } else {
        setError(response.data.error || '删除失败');
      }
    } catch (err) {
      setError(err.response?.data?.error || '删除失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleResetTraffic = async (user) => {
    if (!window.confirm(`确定要清除用户 ${user.username} 的流量记录吗？`)) {
      return;
    }
    try {
      setIsLoading(true);
      const response = await userManageAPI.resetTraffic(user.id);
      if (response.data.result === 'success') {
        setSuccess('流量记录已清除');
        reloadUsers();
      } else {
        setError(response.data.error || '清除流量记录失败');
      }
    } catch (err) {
      setError(err.response?.data?.error || '清除流量记录失败');
    } finally {
      setIsLoading(false);
    }
  };

  // 稳定化事件处理函数
  const handleQueryChange = useCallback((e) => {
    setQueryString(e.target.value);
  }, []);

  const handleSearch = () => {
    setPage(1);
    reloadUsers();
  };

  const handleReset = () => {
    setQueryString('');
    setPage(1);
    reloadUsers();
  };

  const handleSelectUser = useCallback((userId) => {
    setSelectedUserIds(prevIds => 
      prevIds.includes(userId) 
        ? prevIds.filter(id => id !== userId)
        : [...prevIds, userId]
    );
  }, []);

  const handleSelectAllUsers = useCallback((checked) => {
    setSelectedUserIds(checked ? users.map(u => u.id) : []);
  }, [users]);

  const truncateText = (text, length = 50) => {
    if (!text) return '';
    return text.length > length ? text.substring(0, length) + '...' : text;
  };

  const formatTraffic = (bytes) => {
    if (bytes === 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    let unitIndex = 0;
    let value = bytes;
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex++;
    }
    return `${value.toFixed(2)} ${units[unitIndex]}`;
  };

  const handleViewDescription = (user) => {
    setSelectedDescription(user.description || '');
    setSelectedUserNameForDesc(user.username);
    setOpenDescriptionModal(true);
  };

  const handleOpenUserInfo = async (user) => {
    setSelectedUser(user);
    // 保持上次查看的 tab，如果上次查看的是服务器列表，就自动切到服务器列表
    setUserInfoError('');
    setOpenUserInfoDialog(true);
    setUserInfoLoading(true);
    try {
      // 获取用户组信息
      const userInfoResponse = await userManageAPI.getUserDetailInfo(user.username);
      if (userInfoResponse.data.result === 'success') {
        setUserGroups(userInfoResponse.data.groups || []);
      }
      
      // 获取用户服务器权限信息（包含acl）
      const permResponse = await userServerPermAPI.getUserServerPermissions(user.id);
      if (permResponse.data.result === 'success') {
        setUserServerPerms(permResponse.data.data || {});
        // 从响应中获取acl数据
        if (permResponse.data.acl) {
          setServerAclMap(permResponse.data.acl);
        }
      }
    } catch (err) {
      console.error('加载用户信息失败', err);
      setUserInfoError('加载用户信息失败');
    } finally {
      setUserInfoLoading(false);
    }
  };

  const renderServerPermissions = () => {
    const allowServers = userServerPerms[1] || [];
    const denyServers = userServerPerms[2] || [];
    const groupAllowServers = userServerPerms[3] || [];
    const groupDenyServers = userServerPerms[4] || [];

    const allServers = new Map();
    
    // 收集所有服务器
    const addServersToMap = (servers, type) => {
      servers.forEach(server => {
        if (!allServers.has(server.id)) {
          allServers.set(server.id, { ...server, types: [] });
        }
        allServers.get(server.id).types.push(type);
      });
    };

    addServersToMap(allowServers, 'allow');
    addServersToMap(denyServers, 'deny');
    addServersToMap(groupAllowServers, 'groupAllow');
    addServersToMap(groupDenyServers, 'groupDeny');

    // 判断规则优先级和是否应该灰显
    const getRuleStatus = (serverId, types) => {
      const hasUserDeny = types.includes('deny');
      const hasUserAllow = types.includes('allow');
      const hasGroupDeny = types.includes('groupDeny');
      const hasGroupAllow = types.includes('groupAllow');

      // 优先级: 用户拒绝 > 用户允许 > 组拒绝 > 组允许
      const status = {
        userDeny: hasUserDeny,
        userAllow: hasUserAllow,
        groupDeny: hasGroupDeny,
        groupAllow: hasGroupAllow,
        groupDisabled: false, // 是否禁用组规则显示
      };

      // 如果有用户规则（允许或拒绝），组规则应该灰显
      if (hasUserDeny || hasUserAllow) {
        status.groupDisabled = true;
      }

      // 如果用户允许且有用户拒绝，用户允许应该灰显
      if (hasUserAllow && hasUserDeny) {
        status.userAllowDisabled = true;
      }

      // 如果只有组规则（没有用户规则）且有组拒绝，组允许应该灰显
      if (!hasUserDeny && !hasUserAllow && hasGroupDeny && hasGroupAllow) {
        status.groupAllowDisabledByGroupDeny = true;
      }

      return status;
    };

    // 渲染ACL列
    const renderAclCell = (server) => {
      const acl = serverAclMap[server.id] || [];
      if (!acl || acl.length === 0) {
        return '-';
      }
      
      return (
        <Box sx={{ position: 'relative' }}>
          <Tooltip 
            title={
              <Box sx={{ whiteSpace: 'pre-wrap', maxHeight: '300px', overflow: 'auto' }}>
                {acl.join('\n')}
              </Box>
            }
            arrow
          >
            <Typography
              variant="body2"
              sx={{
                cursor: 'pointer',
                textDecoration: 'underline',
                color: 'primary.main',
                maxWidth: '200px',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap'
              }}
              onClick={() => {
                setSelectedAclContent(acl.join('\n'));
                setAclModalOpen(true);
              }}
            >
              {acl[0]}
            </Typography>
          </Tooltip>
        </Box>
      );
    };

    return (
      <TableContainer component={Paper}>
        <Table size="small">
          <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
            <TableRow>
              <TableCell sx={{ fontWeight: 'bold' }}>服务器ID</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>服务器名称</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>协议/端口</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>权限类型</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>ACL信息</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {allServers.size > 0 ? (
              Array.from(allServers.values()).map((server) => {
                const ruleStatus = getRuleStatus(server.id, server.types);
                return (
                  <TableRow key={server.id} hover>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>{server.id}</TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>{server.name}</TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>{server.proto}/{server.port}</TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                      <Stack spacing={0.5} direction="row" flexWrap="wrap" useFlexGap>
                        {ruleStatus.userDeny && (
                          <Chip label="用户拒绝" size="small" color="error" variant="filled" />
                        )}
                        {ruleStatus.userAllow && (
                          <Tooltip title={ruleStatus.userAllowDisabled ? '被用户拒绝策略覆盖，此条目不生效' : ''}>
                            <span>
                              <Chip
                                label="用户允许"
                                size="small"
                                variant={ruleStatus.userAllowDisabled ? 'outlined' : 'filled'}
                                sx={{
                                  textDecoration: ruleStatus.userAllowDisabled ? 'line-through' : 'none',
                                  opacity: ruleStatus.userAllowDisabled ? 0.5 : 1,
                                  backgroundColor: ruleStatus.userAllowDisabled ? 'transparent' : '#FFD700',
                                  color: ruleStatus.userAllowDisabled ? '#999' : '#000',
                                }}
                              />
                            </span>
                          </Tooltip>
                        )}
                        {ruleStatus.groupDeny && (
                          <Tooltip title={ruleStatus.groupDisabled ? '被用户规则覆盖，此条目不生效' : ''}>
                            <span>
                              <Chip
                                label="组拒绝"
                                size="small"
                                color="error"
                                variant={ruleStatus.groupDisabled ? 'outlined' : 'filled'}
                                sx={{
                                  textDecoration: ruleStatus.groupDisabled ? 'line-through' : 'none',
                                  opacity: ruleStatus.groupDisabled ? 0.5 : 1,
                                }}
                              />
                            </span>
                          </Tooltip>
                        )}
                        {ruleStatus.groupAllow && (
                          <Tooltip title={ruleStatus.groupDisabled ? '被用户规则覆盖，此条目不生效' : ruleStatus.groupAllowDisabledByGroupDeny ? '被组拒绝规则覆盖，此条目不生效' : ''}>
                            <span>
                              <Chip
                                label="组允许"
                                size="small"
                                color="info"
                                variant={ruleStatus.groupDisabled || ruleStatus.groupAllowDisabledByGroupDeny ? 'outlined' : 'filled'}
                                sx={{
                                  textDecoration: ruleStatus.groupDisabled || ruleStatus.groupAllowDisabledByGroupDeny ? 'line-through' : 'none',
                                  opacity: ruleStatus.groupDisabled || ruleStatus.groupAllowDisabledByGroupDeny ? 0.5 : 1,
                                }}
                              />
                            </span>
                          </Tooltip>
                        )}
                      </Stack>
                    </TableCell>
                    <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                      {renderAclCell(server)}
                    </TableCell>
                  </TableRow>
                );
              })
            ) : (
              <TableRow>
                <TableCell colSpan={5} align="center" sx={{ paddingTop: 2, paddingBottom: 2 }}>
                  无服务器权限
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    );
  };

  const totalPages = Math.ceil(totalCount / pageSize);

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Typography variant="h5" sx={{ marginBottom: 3 }}>
        用户管理
      </Typography>

      {error && (
        <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError('')}>
          {error}
        </Alert>
      )}
      {success && (
        <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setSuccess('')}>
          {success}
        </Alert>
      )}

      <Stack direction="row" spacing={2} sx={{ marginBottom: 2 }}>
        <TextField
          placeholder="搜索用户名"
          size="small"
          value={queryString}
          onChange={handleQueryChange}
          onKeyPress={(e) => {
            if (e.key === 'Enter') {
              handleSearch();
            }
          }}
          sx={{ minWidth: 200 }}
        />
        <Button
          variant="contained"
          startIcon={<RefreshIcon />}
          onClick={handleSearch}
          disabled={isLoading}
        >
          搜索
        </Button>
        <Button
          variant="contained"
          startIcon={<RefreshIcon />}
          onClick={handleReset}
          disabled={isLoading}
        >
          刷新
        </Button>
        {selectedUserIds.length > 0 && (
          <Button
            variant="contained"
            color="error"
            onClick={handleDeleteUsers}
            disabled={isLoading}
          >
            批量删除 ({selectedUserIds.length})
          </Button>
        )}
        <Button
          variant="contained"
          color="success"
          startIcon={<AddIcon />}
          onClick={() => {
            setCreateUserError('');
            setOpenDialog(true);
          }}
        >
          创建用户
        </Button>
        <Button
          variant="outlined"
          startIcon={<UploadFileIcon />}
          onClick={openBatchAddDialog}
          disabled={isLoading}
        >
          批量添加
        </Button>
        {isLoading && <CircularProgress />}
      </Stack>

      

      {/* 表格内容 - 使用 useMemo 缓存，只在用户列表改变时重新渲染 */}
      {useMemo(() => (
        <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
          <Table>
            <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold', width: '50px' }}>
                  <Checkbox
                    indeterminate={selectedUserIds.length > 0 && selectedUserIds.length < users.length}
                    checked={users.length > 0 && selectedUserIds.length === users.length}
                    onChange={(e) => handleSelectAllUsers(e.target.checked)}
                  />
                </TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>限速</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>流量(历史/当前)</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }} >
                  操作
                </TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {users.length > 0 ? (
                users.map((user) => (
                  <TableRow key={user.id} hover>
                    <TableCell sx={{paddingTop:0, paddingBottom:0, width: '50px'}}>
                      <Checkbox
                        checked={selectedUserIds.includes(user.id)}
                        onChange={() => handleSelectUser(user.id)}
                      />
                    </TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>{user.id}</TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>{user.username}</TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>
                      <Tooltip title={user.description || '无描述'}>
                        <Box
                          onClick={() => handleViewDescription(user)}
                          sx={{
                            cursor: 'pointer',
                            color: user.description ? '#1976d2' : '#999',
                            textDecoration: 'underline',
                          }}
                        >
                          {truncateText(user.description, 50) || '-'}
                        </Box>
                      </Tooltip>
                    </TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>
                      <Typography variant="body2" sx={{ fontSize: '0.75rem' }}>
                        {formatRateLimit(user)}
                      </Typography>
                    </TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>
                      <Box>
                        <Typography variant="body2" sx={{ fontSize: '0.75rem' }}>
                          ↑ {formatTraffic(user.upload_traffic || 0)} ({formatTraffic(user.connected_upload_traffic || 0)})
                        </Typography>
                        <Typography variant="body2" sx={{ fontSize: '0.75rem' }}>
                          ↓ {formatTraffic(user.download_traffic || 0)} ({formatTraffic(user.connected_download_traffic || 0)})
                        </Typography>
                      </Box>
                    </TableCell>
                    <TableCell sx={{paddingTop:0, paddingBottom:0}}>
                      <Stack direction="row" spacing={1}>
                        <Button
                          size="small"
                          variant="outlined"
                          startIcon={<VisibilityIcon />}
                          onClick={() => handleOpenUserInfo(user)}
                        >
                          查看
                        </Button>
                        <Button
                          size="small"
                          variant="outlined"
                          startIcon={<EditIcon />}
                          onClick={() => handleEditUser(user)}
                        >
                          编辑
                        </Button>
                        <Button
                          size="small"
                          variant="outlined"
                          color="error"
                          startIcon={<DeleteForeverIcon />}
                          onClick={() => handleResetTraffic(user)}
                        >
                          清除流量
                        </Button>
                      </Stack>
                    </TableCell>
                  </TableRow>
                ))
              ) : (
                <TableRow>
                  <TableCell colSpan={8} align="center" >
                    暂无用户
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        </TableContainer>
      ), [users, selectedUserIds, handleSelectUser, handleSelectAllUsers])}


      <Stack direction="row" spacing={2} alignItems="center"  sx={{ marginBottom: 3}}>
        <FormControl sx={{ minWidth: 120 }}>
          <InputLabel>每页数量</InputLabel>
          <Select
            value={pageSize}
            label="每页数量"
            onChange={(e) => {
              setPageSize(e.target.value);
              setPage(1);
            }}
          >
            <MenuItem value={10}>10</MenuItem>
            <MenuItem value={20}>20</MenuItem>
            <MenuItem value={50}>50</MenuItem>
            <MenuItem value={100}>100</MenuItem>
            <MenuItem value={200}>200</MenuItem>
          </Select>
        </FormControl>
        <Typography>
          总共 {totalCount} 个用户
        </Typography>
        <Pagination
          count={totalPages}
          page={page}
          onChange={(e, value) => setPage(value)}
          color="primary"
        />
      </Stack>

      <Box sx={{ display: "inline-block", justifyContent: 'center', marginBottom: 3 }}>
        
      </Box>

      {/* 创建用户对话框 */}
      <Dialog  maxWidth={isMobile ? 'lg' : 'md'} open={openDialog} onClose={() => { setOpenDialog(false); setCreateUserError(''); }} fullWidth fullScreen={isMobile}>
        <DialogTitle>创建新用户</DialogTitle>
        <DialogContent >
          {createUserError && (
            <Alert severity="error" sx={{ marginTop: 1 }} onClose={() => setCreateUserError('')}>
              {createUserError}
            </Alert>
          )}
          <Stack spacing={2} sx={{ paddingTop: 1 }}>
            <TextField
              fullWidth
              label="用户名"
              value={formData.username}
              onChange={(e) => setFormData({ ...formData, username: e.target.value })}
            />
            <TextField
              fullWidth
              label="密码"
              type="password"
              value={formData.password}
              onChange={(e) => setFormData({ ...formData, password: e.target.value })}
            />
            <TextField
              fullWidth
              label="描述"
              multiline
              rows={3}
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            />
            <RateLimitFields value={formData} onChange={setFormData} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setOpenDialog(false); setCreateUserError(''); }}>取消</Button>
          <Button onClick={handleCreateUser} variant="contained" disabled={isLoading}>
            创建
          </Button>
        </DialogActions>
      </Dialog>

      {/* 批量添加用户对话框 */}
      <Dialog maxWidth={isMobile ? 'lg' : 'md'} open={openBatchDialog} onClose={closeBatchDialog} fullWidth fullScreen={isMobile}>
        <DialogTitle>批量添加用户</DialogTitle>
        <DialogContent>
          {batchError && (
            <Alert severity="error" sx={{ marginTop: 1 }} onClose={() => setBatchError('')}>
              {batchError}
            </Alert>
          )}

          {!batchResults ? (
            <>
              <Alert severity="info" sx={{ marginTop: 1 }}>
                CSV 首行为表头，需包含「用户名」「密码」列，可选「用户备注」「用户组」列；备注写入用户描述，
                用户组留空表示不加入任何用户组。可先下载模板按格式填写。
              </Alert>
              <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} alignItems={{ xs: 'stretch', sm: 'center' }} sx={{ marginTop: 2, marginBottom: 1, flexWrap: 'wrap' }}>
                <Button variant="outlined" startIcon={<DownloadIcon />} onClick={downloadUserTemplate}>
                  下载模板
                </Button>
                <Button variant="outlined" component="label" startIcon={<UploadFileIcon />}>
                  选择 CSV 文件
                  <input hidden type="file" accept=".csv,text/csv" onChange={handleBatchFileChange} />
                </Button>
                {batchFileName && (
                  <Typography variant="body2" color="textSecondary" sx={{ wordBreak: 'break-all' }}>
                    已选择：{batchFileName}
                  </Typography>
                )}
              </Stack>
              <TextField
                fullWidth
                multiline
                rows={8}
                label="CSV 内容（可粘贴或修改）"
                value={batchCsv}
                onChange={(e) => {
                  setBatchCsv(e.target.value);
                  setBatchError('');
                }}
                placeholder={'用户名,密码,用户备注,用户组\nuser1,password1,张三,groupA\nuser2,password2,,'}
                sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
              />
            </>
          ) : (
            <Box sx={{ marginTop: 1 }}>
              <Alert severity={batchResults.failedCount > 0 ? 'warning' : 'success'}>
                成功 {batchResults.successCount} 个，失败 {batchResults.failedCount} 个
                {batchResults.failedCount > 0 ? '，失败详情见下表' : ''}
              </Alert>
              <TableContainer sx={{ marginTop: 1, maxHeight: 360 }}>
                <Table size="small" stickyHeader>
                  <TableHead>
                    <TableRow>
                      <TableCell>行号</TableCell>
                      <TableCell>用户名</TableCell>
                      <TableCell>用户备注</TableCell>
                      <TableCell>用户组</TableCell>
                      <TableCell>结果</TableCell>
                      <TableCell>原因</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {batchResults.items.map((item, idx) => (
                      <TableRow key={`${item.line}-${idx}`}>
                        <TableCell>{item.line}</TableCell>
                        <TableCell sx={{ wordBreak: 'break-all' }}>{item.username || '-'}</TableCell>
                        <TableCell sx={{ wordBreak: 'break-all' }}>{item.description || '-'}</TableCell>
                        <TableCell sx={{ wordBreak: 'break-all' }}>{item.group_name || '-'}</TableCell>
                        <TableCell>
                          {item.success ? (
                            <Chip size="small" color="success" label="成功" />
                          ) : (
                            <Chip size="small" color="error" label="失败" />
                          )}
                        </TableCell>
                        <TableCell sx={{ color: item.success ? 'text.secondary' : 'error.main', wordBreak: 'break-all' }}>
                          {item.success ? '-' : item.error || '未知原因'}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          {batchResults ? (
            <>
              <Button onClick={() => { setBatchResults(null); setBatchCsv(''); setBatchFileName(''); }}>继续添加</Button>
              <Button variant="contained" onClick={closeBatchDialog}>完成</Button>
            </>
          ) : (
            <>
              <Button onClick={closeBatchDialog}>取消</Button>
              <Button variant="contained" onClick={handleBatchSubmit} disabled={batchSubmitLoading || !batchCsv.trim()}>
                {batchSubmitLoading ? <CircularProgress size={20} /> : '批量添加'}
              </Button>
            </>
          )}
        </DialogActions>
      </Dialog>

      {/* 编辑用户对话框 */}
      <Dialog  maxWidth={isMobile ? 'lg' : 'md'} open={openEditDialog} onClose={() => { setOpenEditDialog(false); setEditUserError(''); }} fullWidth fullScreen={isMobile}>
        <DialogTitle>更新用户信息 - {editingUser.username}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {editUserError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setEditUserError('')}>
              {editUserError}
            </Alert>
          )}
          <Stack spacing={2}>
            <TextField
              fullWidth
              label="新密码（留空表示不修改）"
              type="password"
              value={editingUser.password}
              onChange={(e) => setEditingUser({ ...editingUser, password: e.target.value })}
            />
            <TextField
              fullWidth
              label="描述"
              multiline
              rows={3}
              value={editingUser.description}
              onChange={(e) => setEditingUser({ ...editingUser, description: e.target.value })}
            />
            <RateLimitFields value={editingUser} onChange={setEditingUser} />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setOpenEditDialog(false); setEditUserError(''); }}>取消</Button>
          <Button onClick={handleUpdateUser} variant="contained" disabled={isLoading}>
            更新
          </Button>
        </DialogActions>
      </Dialog>

      {/* 查看完整描述对话框 */}
      <Dialog 
        open={openDescriptionModal} 
        onClose={() => setOpenDescriptionModal(false)} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>完整描述 - {selectedUserNameForDesc}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          <TextField
            fullWidth
            multiline
            rows={18}
            value={selectedDescription}
            InputProps={{
              readOnly: true,
            }}
            variant="outlined"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDescriptionModal(false)}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 用户信息对话框 */}
      <Dialog 
        open={openUserInfoDialog} 
        onClose={() => {
          setOpenUserInfoDialog(false);
          setUserInfoError('');
        }} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        
       fullScreen={isMobile}>
        <DialogTitle>用户信息 - {selectedUser?.username}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {userInfoError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setUserInfoError('')}>
              {userInfoError}
            </Alert>
          )}
          {userInfoLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', padding: 2 }}>
              <CircularProgress />
            </Box>
          ) : (
            <>
              <Tabs value={userInfoTabValue} onChange={(e, newValue) => setUserInfoTabValue(newValue)}>
                <Tab label="所属用户组" />
                <Tab label="服务器权限" />
              </Tabs>

              {/* 用户组标签 */}
              {userInfoTabValue === 0 && (
                <Box sx={{ marginTop: 2 }}>
                  {userGroups.length > 0 ? (
                    <TableContainer component={Paper}>
                      <Table size="small">
                        <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                          <TableRow>
                            <TableCell sx={{ fontWeight: 'bold' }}>用户组ID</TableCell>
                            <TableCell sx={{ fontWeight: 'bold' }}>用户组名称</TableCell>
                            <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                          </TableRow>
                        </TableHead>
                        <TableBody>
                          {userGroups.map((group) => (
                            <TableRow key={group.ID} hover>
                              <TableCell sx={{ paddingTop: 0.7, paddingBottom: 0.7 }}>{group.ID}</TableCell>
                              <TableCell sx={{ paddingTop: 0.7, paddingBottom: 0.7 }}>{group.name}</TableCell>
                              <TableCell sx={{ paddingTop: 0.7, paddingBottom: 0.7, maxWidth: '200px', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{group.description || '-'}</TableCell>
                            </TableRow>
                          ))}
                        </TableBody>
                      </Table>
                    </TableContainer>
                  ) : (
                    <Typography sx={{ paddingTop: 2 }}>用户未加入任何用户组</Typography>
                  )}
                </Box>
              )}

              {/* 服务器权限标签 */}
              {userInfoTabValue === 1 && (
                <Box sx={{ marginTop: 2 }}>
                  {renderServerPermissions()}
                </Box>
              )}
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setOpenUserInfoDialog(false);
            setUserInfoError('');
          }}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* ACL 内容模态框 */}
      <Modal
        open={aclModalOpen}
        onClose={() => setAclModalOpen(false)}
        sx={{
          display: 'flex',
          justifyContent: 'center',
          alignItems: 'center'
        }}
      >
        <Box
          sx={{
            backgroundColor: 'white',
            borderRadius: '8px',
            padding: 3,
            maxWidth: '500px',
            maxHeight: '80vh',
            overflow: 'auto',
            boxShadow: '0 3px 5px -1px rgba(0,0,0,0.2)'
          }}
        >
          <Typography variant="h6" sx={{ marginBottom: 2 }}>
            ACL 信息
          </Typography>
          <TextField
            fullWidth
            multiline
            rows={10}
            value={selectedAclContent}
            readOnly
            variant="outlined"
            sx={{ fontFamily: 'monospace', fontSize: '12px' }}
          />
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', marginTop: 2 }}>
            <Button
              variant="contained"
              onClick={() => setAclModalOpen(false)}
            >
              关闭
            </Button>
          </Box>
        </Box>
      </Modal>
    </Box>
  );
};

export default Users;

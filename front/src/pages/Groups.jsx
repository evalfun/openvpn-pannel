import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
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
  Tabs,
  Tab,
  Checkbox,
  FormGroup,
  FormControlLabel,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  Chip,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import ManageIcon from '@mui/icons-material/Build';
import DeleteIcon from '@mui/icons-material/Delete';
import { groupAPI, userManageAPI } from '../api';

const Groups = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const [groups, setGroups] = useState([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [totalCount, setTotalCount] = useState(0);
  const [queryString, setQueryString] = useState('');
  
  const [openDialog, setOpenDialog] = useState(false);
  const [openManageDialog, setOpenManageDialog] = useState(false);
  const [tabValue, setTabValue] = useState(0);
  
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    upload_limit_kb: 0,
    download_limit_kb: 0,
  });
  
  const [selectedGroup, setSelectedGroup] = useState(null);
  const [groupUsers, setGroupUsers] = useState([]);
  const [groupACLs, setGroupACLs] = useState([]);
  const [allUsers, setAllUsers] = useState([]);
  
  const [openAddUserDialog, setOpenAddUserDialog] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [addUserPage, setAddUserPage] = useState(1);
  const [addUserPageSize, setAddUserPageSize] = useState(10);
  const [availableUsersCount, setAvailableUsersCount] = useState(0);
  const [availableUsersLoading, setAvailableUsersLoading] = useState(false);
  const [userSearchQuery, setUserSearchQuery] = useState('');
  const [groupManageError, setGroupManageError] = useState(''); // 新增：管理用户组内的错误提示
  const [groupManageSuccess, setGroupManageSuccess] = useState(''); // 新增：管理用户组内的成功提示
  const [createGroupError, setCreateGroupError] = useState(''); // 创建用户组弹窗内的错误提示
  const [addUserDialogError, setAddUserDialogError] = useState(''); // 添加用户弹窗内的错误提示
  const [addACLDialogError, setAddACLDialogError] = useState(''); // 添加ACL弹窗内的错误提示
  
  const [openAddACLDialog, setOpenAddACLDialog] = useState(false);
  const [aclFormData, setAclFormData] = useState({
    type: 4,
    value: '',
  });
  
  const [selectedACLs, setSelectedACLs] = useState([]);
  
  // 描述弹窗状态
  const [openDescDialog, setOpenDescDialog] = useState(false);
  const [selectedDescriptionText, setSelectedDescriptionText] = useState('');
  const [selectedGroupNameForDesc, setSelectedGroupNameForDesc] = useState('');
  
  // 修改描述表单状态
  const [editDescData, setEditDescData] = useState('');
  // 修改限速表单状态（单位 KB/s，0=不限速）
  const [editUploadLimitKB, setEditUploadLimitKB] = useState(0);
  const [editDownloadLimitKB, setEditDownloadLimitKB] = useState(0);
  
  // 删除用户组状态
  const [deleteConfirmLoading, setDeleteConfirmLoading] = useState(false);

  const loadGroups = async (p = 1, ps = pageSize) => {
    setIsLoading(true);
    setError('');
    try {
      const response = await groupAPI.getGroupList(p, ps, queryString);
      if (response.data.result === 'success') {
        setGroups(response.data.data || []);
        setTotalCount(response.data.count || 0);
      }
    } catch (err) {
      setError('加载用户组列表失败');
      console.error(err);
    } finally {
      setIsLoading(false);
    }
  };

  const loadGroupUsers = async (groupName) => {
    try {
      const response = await groupAPI.getGroupUsers(groupName);
      if (response.data.data) {
        setGroupUsers(response.data.data);
      }
    } catch (err) {
      setGroupManageError('加载用户组用户失败');
    }
  };

  useEffect(() => {
    loadGroups(page, pageSize);
  }, [page, pageSize]); // 添加pageSize到依赖数组

  const loadGroupACLs = async (groupName) => {
    try {
      const response = await groupAPI.getGroupACLs(groupName);
      if (response.data.result === 'success') {
        setGroupACLs(response.data.data || []);
      }
    } catch (err) {
      setGroupManageError('加载用户组ACL失败');
    }
  };

  const loadAvailableUsers = async (pageNum = 1, pageSize = 10, query = '') => {
    try {
      setAvailableUsersLoading(true);
      
      // 使用 exclude_group_id 参数直接在后端筛选用户
      const response = await userManageAPI.getUserList(pageNum, pageSize, query, selectedGroup.ID);
      if (response.data.result === 'success') {
        setAllUsers(response.data.data || []);
        setAvailableUsersCount(response.data.count || 0);
      }
    } catch (err) {
      console.error('加载可用用户列表失败', err);
    } finally {
      setAvailableUsersLoading(false);
    }
  };

  const handleCreateGroup = async () => {
    if (!formData.name) {
      setCreateGroupError('用户组名称不能为空');
      return;
    }
    try {
      setIsLoading(true);
      const response = await groupAPI.createGroup(formData);
      if (response.data.result === 'success') {
        setSuccess('用户组创建成功');
        setFormData({ name: '', description: '', upload_limit_kb: 0, download_limit_kb: 0 });
        setOpenDialog(false);
        setPage(1);
        loadGroups(1, pageSize);
      } else {
        setCreateGroupError(response.data.error || '创建用户组失败');
      }
    } catch (err) {
      setCreateGroupError(err.response?.data?.error || '创建用户组失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleManageGroup = async (group) => {
    setSelectedGroup(group);
    setEditDescData(group.description || '');
    setEditUploadLimitKB(group.upload_limit_kb || 0);
    setEditDownloadLimitKB(group.download_limit_kb || 0);
    //setTabValue(0);
    setOpenManageDialog(true);
    await loadGroupUsers(group.name);
    await loadGroupACLs(group.name);
  };

  const handleAddUsers = async () => {
    if (selectedUsers.length === 0) {
      setAddUserDialogError('请至少选择一个用户');
      return;
    }
    try {
      setIsLoading(true);
      // 获取选中用户的用户名列表
      const selectedUsernames = selectedUsers
        .map(userId => allUsers.find(u => u.id === userId)?.username)
        .filter(Boolean);
      
      // 尝试使用批量添加接口，如果后端支持
      try {
        const response = await groupAPI.addUsersToGroup({
          group: selectedGroup.name,
          users: selectedUsernames,
        });
        if (response.data.result === 'success') {
          setGroupManageSuccess('用户添加成功');
          setSelectedUsers([]);
          setOpenAddUserDialog(false);
          await loadGroupUsers(selectedGroup.name);
        }
      } catch (batchError) {
        // 如果批量接口不存在，降级为逐个添加
        for (const username of selectedUsernames) {
          await groupAPI.addUserToGroup({
            group: selectedGroup.name,
            user: username,
          });
        }
        setGroupManageSuccess('用户添加成功');
        setSelectedUsers([]);
        setOpenAddUserDialog(false);
        await loadGroupUsers(selectedGroup.name);
      }
    } catch (err) {
      setAddUserDialogError(err.response?.data?.error || '添加用户失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleRemoveUser = async (username) => {
    try {
      setIsLoading(true);
      const response = await groupAPI.removeUserFromGroup({
        group: selectedGroup.name,
        user: username,
      });
      if (response.data.result === 'success') {
        setGroupManageSuccess('用户移除成功');
        await loadGroupUsers(selectedGroup.name);
      }
    } catch (err) {
      setGroupManageError(err.response?.data?.error || '移除用户失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddACL = async () => {
    if (!aclFormData.value) {
      setAddACLDialogError('ACL值不能为空');
      return;
    }
    try {
      setIsLoading(true);
      const response = await groupAPI.addGroupACL({
        group: selectedGroup.name,
        type: aclFormData.type,
        value: aclFormData.value,
      });
      if (response.data.result === 'success') {
        setGroupManageSuccess('ACL添加成功');
        setAclFormData({ type: 4, value: '' });
        setOpenAddACLDialog(false);
        await loadGroupACLs(selectedGroup.name);
      }
    } catch (err) {
      setAddACLDialogError(err.response?.data?.error || '添加ACL失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteACLs = async () => {
    if (selectedACLs.length === 0) {
      setGroupManageError('请至少选择一个ACL');
      return;
    }
    try {
      setIsLoading(true);
      for (const aclId of selectedACLs) {
        await groupAPI.deleteGroupACL({ acl_id: aclId });
      }
      setGroupManageSuccess('ACL删除成功');
      setSelectedACLs([]);
      await loadGroupACLs(selectedGroup.name);
    } catch (err) {
      setGroupManageError(err.response?.data?.error || '删除ACL失败');
    } finally {
      setIsLoading(false);
    }
  };

  const totalPages = Math.ceil(totalCount / pageSize);

  // 稳定化搜索输入处理器
  const handleUserSearchChange = useCallback((e) => {
    setUserSearchQuery(e.target.value);
  }, []);

  const handleSearchUsers = useCallback(() => {
    setAddUserPage(1);
    loadAvailableUsers(1, addUserPageSize, userSearchQuery);
  }, [userSearchQuery, addUserPageSize, loadAvailableUsers]);

  // 稳定化用户选择处理器
  const handleSelectAllAvailableUsers = useCallback((e) => {
    setSelectedUsers(e.target.checked ? allUsers.map(u => u.id) : []);
  }, [allUsers]);

  const handleSelectUser = useCallback((userId) => {
    setSelectedUsers(prevIds =>
      prevIds.includes(userId)
        ? prevIds.filter(id => id !== userId)
        : [...prevIds, userId]
    );
  }, []);

  // 稳定化事件处理函数
  const handleQueryChange = useCallback((e) => {
    setQueryString(e.target.value);
  }, []);

  const handleSearch = () => {
    setPage(1);
    loadGroups(1, pageSize);
  };

  const handleReset = () => {
    setQueryString('');
    setPage(1);
    loadGroups(1, pageSize);
  };

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Typography variant="h5" sx={{ marginBottom: 3 }}>
        用户组管理
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
          placeholder="搜索用户组名"
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
        <Button
          variant="contained"
          color="success"
          startIcon={<AddIcon />}
          onClick={() => {
            setCreateGroupError('');
            setOpenDialog(true);
          }}
        >
          创建用户组
        </Button>
        {isLoading && <CircularProgress />}
      </Stack>

      

      <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
        <Table>
          <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
            <TableRow>
              <TableCell sx={{ fontWeight: 'bold' }}>用户组ID</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>用户组名称</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }}>限速</TableCell>
              <TableCell sx={{ fontWeight: 'bold' }} >
                操作
              </TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {groups.length > 0 ? (
              groups.map((group) => (
                <TableRow key={group.ID} hover>
                  <TableCell sx={{paddingTop:1, paddingBottom:1}} >{group.ID}</TableCell>
                  <TableCell sx={{paddingTop:1, paddingBottom:1}} >{group.name}</TableCell>
                  <TableCell 
                    sx={{
                      paddingTop:1, 
                      paddingBottom:1,
                      maxWidth: '160px',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      cursor: group.description ? 'pointer' : 'default',
                      '&:hover': group.description ? {
                        textDecoration: 'underline',
                        color: 'primary.main'
                      } : {}
                    }}
                    onClick={() => {
                      if (group.description) {
                        setSelectedDescriptionText(group.description);
                        setSelectedGroupNameForDesc(group.name);
                        setOpenDescDialog(true);
                      }
                    }}
                  >
                    {group.description || '-'}
                  </TableCell>
                  <TableCell sx={{paddingTop:1, paddingBottom:1}}>
                    {(!group.upload_limit_kb && !group.download_limit_kb)
                      ? '不限速'
                      : `↑${group.upload_limit_kb || 0} ↓${group.download_limit_kb || 0} KB/s`}
                  </TableCell>
                  <TableCell sx={{paddingTop:1, paddingBottom:1}}  >
                    <Button
                      size="small"
                      variant="outlined"
                      startIcon={<ManageIcon />}
                      onClick={() => handleManageGroup(group)}
                    >
                      管理
                    </Button>
                  </TableCell>
                </TableRow>
              ))
            ) : (
              <TableRow>
                <TableCell colSpan={5} align="center">
                  暂无用户组
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>

      <Stack direction="row" spacing={2} alignItems="center" sx={{ marginBottom: 3 }}>
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
          总共 {totalCount} 个用户组
        </Typography>
        <Pagination
          count={totalPages}
          page={page}
          onChange={(e, value) => setPage(value)}
          color="primary"
        />
      </Stack>

      <Box sx={{ display: 'flex', justifyContent: 'center', marginBottom: 3 }}>
        
      </Box>

      {/* 查看完整描述对话框 */}
      <Dialog 
        open={openDescDialog} 
        onClose={() => setOpenDescDialog(false)} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
      >
        <DialogTitle>用户组描述 - {selectedGroupNameForDesc}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          <TextField
            fullWidth
            multiline
            rows={18}
            value={selectedDescriptionText}
            InputProps={{
              readOnly: true,
            }}
            variant="outlined"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenDescDialog(false)}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 创建用户组对话框 */}
      <Dialog 
        open={openDialog} 
        onClose={() => {
          setOpenDialog(false);
          setCreateGroupError('');
        }} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        
      >
        <DialogTitle>创建新用户组</DialogTitle>
        <DialogContent sx={{ padding: isMobile ? '12px' : '24px' }}>
          {createGroupError && (
            <Alert severity="error" sx={{ marginBottom: 2, marginTop: 1 }} onClose={() => setCreateGroupError('')}>
              {createGroupError}
            </Alert>
          )}
          <Stack spacing={2} sx={{ paddingTop: 1 }}>
            <TextField
              fullWidth
              label="用户组名称"
              value={formData.name}
              onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            />
            <TextField
              fullWidth
              label="描述"
              multiline
              rows={3}
              value={formData.description}
              onChange={(e) => setFormData({ ...formData, description: e.target.value })}
            />
            <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
              <TextField
                fullWidth
                type="number"
                label="上传限速 (KB/s)"
                helperText="服务器 -> 客户端，0=不限速"
                value={formData.upload_limit_kb}
                onChange={(e) =>
                  setFormData({ ...formData, upload_limit_kb: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })
                }
              />
              <TextField
                fullWidth
                type="number"
                label="下载限速 (KB/s)"
                helperText="客户端 -> 服务器，0=不限速"
                value={formData.download_limit_kb}
                onChange={(e) =>
                  setFormData({ ...formData, download_limit_kb: Math.max(0, parseInt(e.target.value || '0', 10) || 0) })
                }
              />
            </Stack>
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setOpenDialog(false);
            setCreateGroupError('');
          }}>取消</Button>
          <Button onClick={handleCreateGroup} variant="contained" disabled={isLoading}>
            创建
          </Button>
        </DialogActions>
      </Dialog>

      {/* 管理用户组对话框 */}
      <Dialog 
        open={openManageDialog} 
        onClose={() => setOpenManageDialog(false)} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        sx={{
          '& .MuiDialog-paper': {
            //margin: isMobile ? '0' : '0',
            margin: 0,
          }
        }}
      >
        <DialogTitle>管理用户组 - {selectedGroup?.name}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {groupManageError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setGroupManageError('')}>
              {groupManageError}
            </Alert>
          )}
          {groupManageSuccess && (
            <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setGroupManageSuccess('')}>
              {groupManageSuccess}
            </Alert>
          )}
          <Tabs value={tabValue} onChange={(e, newValue) => setTabValue(newValue)}>
            <Tab label="管理用户" />
            <Tab label="管理ACL" />
            <Tab label="修改信息" />
            <Tab label="删除用户组" />
          </Tabs>

          {/* 用户管理Tab */}
          {tabValue === 0 && (
            <Box sx={{ marginTop: 2 }}>
              <Stack direction="row" spacing={2} sx={{ marginBottom: 2 }}>
                <Button
                  variant="contained"
                  color="success"
                  startIcon={<AddIcon />}
                  onClick={async () => {
                    setAddUserDialogError('');
                    setOpenAddUserDialog(true);
                    setAddUserPage(1);
                    setAvailableUsersCount(0);
                    await loadAvailableUsers(1, addUserPageSize);
                  }}
                >
                  添加用户
                </Button>
                <Button
                  variant="contained"
                  startIcon={<RefreshIcon />}
                  onClick={() => loadGroupUsers(selectedGroup.name)}
                  disabled={isLoading}
                >
                  刷新
                </Button>
              </Stack>

              <TableContainer component={Paper}>
                <Table>
                  <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>
                        操作
                      </TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {groupUsers.length > 0 ? (
                      groupUsers.map((user) => (
                        <TableRow key={user.ID} hover>
                          <TableCell  sx={{paddingTop:1, paddingBottom:1}} >{user.ID}</TableCell>
                          <TableCell  sx={{paddingTop:1, paddingBottom:1}} >{user.username}</TableCell>
                          <TableCell  sx={{paddingTop:1, paddingBottom:1}} >{user.description || '-'}</TableCell>
                          <TableCell  sx={{paddingTop:1, paddingBottom:1}}  >
                            <Button
                              size="small"
                              variant="outlined"
                              color="error"
                              startIcon={<DeleteIcon />}
                              onClick={() => handleRemoveUser(user.username)}
                            >
                              删除
                            </Button>
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={4} align="center">
                          该用户组中暂无用户
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          )}

          {/* ACL管理Tab */}
          {tabValue === 1 && (
            <Box sx={{ marginTop: 2 }}>
              <Stack direction="row" spacing={2} sx={{ marginBottom: 2,marginTop: 2 }}>
                <Button
                  variant="contained"
                  color="success"
                  startIcon={<AddIcon />}
                  onClick={() => {
                    setAddACLDialogError('');
                    setOpenAddACLDialog(true);
                  }}
                >
                  新增ACL
                </Button>
                <Button
                  variant="contained"
                  startIcon={<RefreshIcon />}
                  onClick={() => loadGroupACLs(selectedGroup.name)}
                  disabled={isLoading}
                >
                  刷新
                </Button>
                {selectedACLs.length > 0 && (
                  <Button
                    variant="contained"
                    color="error"
                    startIcon={<DeleteIcon />}
                    onClick={handleDeleteACLs}
                  >
                    删除选中 ({selectedACLs.length})
                  </Button>
                )}
              </Stack>

              <TableContainer component={Paper}>
                <Table>
                  <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 'bold' }}>选择</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>ACL ID</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>类型</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>值</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {groupACLs.length > 0 ? (
                      groupACLs.map((acl) => (
                        <TableRow key={acl.id} hover>
                          <TableCell sx={{padding: 0}}>
                            <Checkbox
                              checked={selectedACLs.includes(acl.id)}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setSelectedACLs([...selectedACLs, acl.id]);
                                } else {
                                  setSelectedACLs(selectedACLs.filter(id => id !== acl.id));
                                }
                              }}
                            />
                          </TableCell>
                          <TableCell  sx={{padding: 1}}>{acl.id}</TableCell>
                          <TableCell  sx={{padding: 1}}>
                            <Chip
                              label={acl.type === 4 ? 'IPv4' : 'IPv6'}
                              size="small"
                              color={acl.type === 4 ? 'primary' : 'info'}
                            />
                          </TableCell>
                          <TableCell  sx={{padding: 0}}>{acl.value}</TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={4} align="center">
                          该用户组中暂无ACL
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            </Box>
          )}

          {/* 修改描述Tab */}
          {tabValue === 2 && (
            <Box sx={{ marginTop: 2 }}>
              <Stack spacing={2}>
                <Typography variant="body2" sx={{ color: '#666' }}>
                  修改用户组描述信息
                </Typography>
                <TextField
                  fullWidth
                  multiline
                  rows={6}
                  label="用户组描述"
                  value={editDescData}
                  onChange={(e) => setEditDescData(e.target.value)}
                  placeholder="输入用户组描述"
                />
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
                  <TextField
                    fullWidth
                    type="number"
                    label="上传限速 (KB/s)"
                    helperText="服务器 -> 客户端，0=不限速"
                    value={editUploadLimitKB}
                    onChange={(e) => setEditUploadLimitKB(Math.max(0, parseInt(e.target.value || '0', 10) || 0))}
                  />
                  <TextField
                    fullWidth
                    type="number"
                    label="下载限速 (KB/s)"
                    helperText="客户端 -> 服务器，0=不限速"
                    value={editDownloadLimitKB}
                    onChange={(e) => setEditDownloadLimitKB(Math.max(0, parseInt(e.target.value || '0', 10) || 0))}
                  />
                </Stack>
                <Box sx={{ display: 'flex', gap: 2 }}>
                  <Button
                    variant="contained"
                    color="primary"
                    onClick={async () => {
                      try {
                        setIsLoading(true);
                        const response = await groupAPI.updateGroupDesc({
                          id: selectedGroup.ID,
                          desc: editDescData,
                          upload_limit_kb: editUploadLimitKB,
                          download_limit_kb: editDownloadLimitKB,
                        });
                        if (response.data.result === 'success') {
                          setGroupManageSuccess('信息更新成功');
                          await loadGroups(page, pageSize);
                        }
                      } catch (err) {
                        setGroupManageError(err.response?.data?.error || '更新失败');
                      } finally {
                        setIsLoading(false);
                      }
                    }}
                    disabled={isLoading}
                  >
                    保存
                  </Button>
                </Box>
              </Stack>
            </Box>
          )}

          {/* 删除用户组Tab */}
          {tabValue === 3 && (
            <Box sx={{ marginTop: 2 }}>
              <Alert severity="warning" sx={{ marginBottom: 2 }}>
                警告：删除用户组是不可逆操作，请谨慎操作！
              </Alert>
              <Stack spacing={2}>
                <Typography variant="body2" sx={{ color: '#666' }}>
                  用户组名称：{selectedGroup?.name}
                </Typography>
                <Button
                  variant="contained"
                  color="error"
                  startIcon={<DeleteIcon />}
                  onClick={async () => {
                    if (!window.confirm(`确认删除用户组 "${selectedGroup?.name}" 吗？此操作不可逆！`)) {
                      return;
                    }
                    try {
                      setDeleteConfirmLoading(true);
                      const response = await groupAPI.deleteGroup({
                        name: selectedGroup.name,
                      });
                      if (response.data.result === 'success') {
                        setGroupManageSuccess('用户组删除成功');
                        setOpenManageDialog(false);
                        setPage(1);
                        await loadGroups(1, pageSize);
                      }
                    } catch (err) {
                      setGroupManageError(err.response?.data?.error || '删除用户组失败');
                    } finally {
                      setDeleteConfirmLoading(false);
                    }
                  }}
                  disabled={deleteConfirmLoading}
                >
                  {deleteConfirmLoading ? '删除中...' : '删除用户组'}
                </Button>
              </Stack>
            </Box>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setOpenManageDialog(false)}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 添加用户对话框 */}
      <Dialog 
        open={openAddUserDialog} 
        onClose={() => {
          setOpenAddUserDialog(false);
          setSelectedUsers([]);
          setAddUserPage(1);
          setAvailableUsersCount(0);
          setUserSearchQuery('');
          setAddUserDialogError('');
        }} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        
      >
        <DialogTitle>添加用户到 {selectedGroup?.name}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {addUserDialogError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setAddUserDialogError('')}>
              {addUserDialogError}
            </Alert>
          )}
          <Typography variant="body2" sx={{ marginBottom: 2, color: '#666' }}>
            选择要添加的用户（已在此用户组中的用户已被过滤）
          </Typography>
          
          <Stack direction="row" spacing={1} sx={{ marginBottom: 2 }}>
            <TextField
              placeholder="搜索用户名"
              size="small"
              value={userSearchQuery}
              onChange={handleUserSearchChange}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  handleSearchUsers();
                }
              }}
              sx={{ minWidth: 200, flex: 1 }}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={handleSearchUsers}
              disabled={availableUsersLoading}
            >
              搜索
            </Button>
          </Stack>
          
          {availableUsersLoading && <CircularProgress sx={{ marginBottom: 2 }} />}
          
          <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
            <Table>
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', width: '50px' }}>
                    <Checkbox
                      indeterminate={selectedUsers.length > 0 && selectedUsers.length < allUsers.length}
                      checked={allUsers.length > 0 && selectedUsers.length === allUsers.length}
                      onChange={handleSelectAllAvailableUsers}
                    />
                  </TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {allUsers.length > 0 ? (
                  allUsers.map((user) => (
                    <TableRow key={user.id} hover>
                      <TableCell sx={{padding: 0, width: '50px'}}>
                        <Checkbox
                          checked={selectedUsers.includes(user.id)}
                          onChange={() => handleSelectUser(user.id)}
                        />
                      </TableCell>
                      <TableCell  sx={{padding: 0}}>{user.id}</TableCell>
                      <TableCell  sx={{padding: 0}}>{user.username}</TableCell>
                      <TableCell  sx={{padding: 0}}>{user.description || '无'}</TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={4} align="center">
                      {availableUsersLoading ? '加载中...' : '没有可添加的用户'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {availableUsersCount > 0 && (
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControl sx={{ minWidth: 120 }}>
                <InputLabel>每页数量</InputLabel>
                <Select
                  value={addUserPageSize}
                  label="每页数量"
                  onChange={async (e) => {
                    setAddUserPageSize(e.target.value);
                    setAddUserPage(1);
                    await loadAvailableUsers(1, e.target.value, userSearchQuery);
                  }}
                  size="small"
                  disabled={availableUsersLoading}
                >
                  <MenuItem value={5}>5</MenuItem>
                  <MenuItem value={10}>10</MenuItem>
                  <MenuItem value={20}>20</MenuItem>
                </Select>
              </FormControl>
              <Box sx={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
                <Pagination
                  count={Math.ceil(availableUsersCount / addUserPageSize)}
                  page={addUserPage}
                  onChange={async (e, value) => {
                    setAddUserPage(value);
                    await loadAvailableUsers(value, addUserPageSize, userSearchQuery);
                  }}
                  color="primary"
                  size="small"
                  disabled={availableUsersLoading}
                />
              </Box>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setOpenAddUserDialog(false);
            setSelectedUsers([]);
            setAddUserPage(1);
            setAvailableUsersCount(0);
            setUserSearchQuery('');
            setAddUserDialogError('');
          }}>取消</Button>
          <Button onClick={handleAddUsers} variant="contained" disabled={isLoading || selectedUsers.length === 0}>
            添加 ({selectedUsers.length})
          </Button>
        </DialogActions>
      </Dialog>

      {/* 添加ACL对话框 */}
      <Dialog 
        open={openAddACLDialog} 
        onClose={() => {
          setOpenAddACLDialog(false);
          setAddACLDialogError('');
        }} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
      >
        <DialogTitle>新增ACL</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {addACLDialogError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setAddACLDialogError('')}>
              {addACLDialogError}
            </Alert>
          )}
          <Stack spacing={2} sx={{ paddingTop: 2 }}>
            <FormControl fullWidth>
              <InputLabel>类型</InputLabel>
              <Select
                value={aclFormData.type}
                label="类型"
                onChange={(e) => setAclFormData({ ...aclFormData, type: e.target.value })}
              >
                <MenuItem value={4}>IPv4</MenuItem>
                <MenuItem value={6}>IPv6</MenuItem>
              </Select>
            </FormControl>
            <TextField
              fullWidth
              label="值 (CIDR格式或其他数据)"
              value={aclFormData.value}
              onChange={(e) => setAclFormData({ ...aclFormData, value: e.target.value })}
              placeholder="例如：172.31.0.0/16"
            />
          </Stack>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setOpenAddACLDialog(false); setAclFormData({ type: 4, value: '' }); setAddACLDialogError(''); }}>取消</Button>
          <Button onClick={handleAddACL} variant="contained" disabled={isLoading}>
            添加
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Groups;

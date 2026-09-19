import React, { useState, useEffect, useRef, useCallback } from 'react';
import {
  Box,
  Typography,
  Paper,
  Tabs,
  Tab,
  Button,
  TextField,
  Alert,
  CircularProgress,
  Stack,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Checkbox,
  FormControl,
  FormControlLabel,
  InputLabel,
  Select,
  MenuItem,
  InputAdornment,
  Chip,
  Pagination,
  Card,
  CardContent,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import {
  Add as AddIcon,
  Edit as EditIcon,
  Delete as DeleteIcon,
  Refresh as RefreshIcon,
  People as PeopleIcon,
  Speed as SpeedIcon,
} from '@mui/icons-material';
import { rateLimitAPI, userManageAPI } from '../api';

const PERIOD_UNITS = [
  { label: '分钟', seconds: 60 },
  { label: '小时', seconds: 3600 },
  { label: '天', seconds: 86400 },
];

const BYTE_UNITS = [
  { label: 'MB', bytes: 1024 * 1024 },
  { label: 'GB', bytes: 1024 * 1024 * 1024 },
  { label: 'TB', bytes: 1024 * 1024 * 1024 * 1024 },
];

const formatBytes = (bytes) => {
  if (!bytes || bytes <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  let value = bytes;
  let idx = 0;
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024;
    idx += 1;
  }
  return `${value.toFixed(value >= 100 || idx === 0 ? 0 : 1)} ${units[idx]}`;
};

const formatRate = (kb) => {
  if (!kb || kb <= 0) return '不限速';
  return `${kb} KB/s`;
};

// 把秒数拆成“值 + 单位”，优先使用能整除的最大单位
const splitPeriod = (seconds) => {
  for (let i = PERIOD_UNITS.length - 1; i >= 0; i -= 1) {
    const unit = PERIOD_UNITS[i];
    if (seconds >= unit.seconds && seconds % unit.seconds === 0) {
      return { value: String(seconds / unit.seconds), unit: unit.seconds };
    }
  }
  return { value: String(seconds || 60), unit: 60 };
};

const formatPeriod = (seconds) => {
  if (!seconds) return '-';
  const { value, unit } = splitPeriod(seconds);
  const label = PERIOD_UNITS.find((u) => u.seconds === unit)?.label || '秒';
  return `${value} ${label}`;
};

// 把字节数拆成“值 + 单位”，优先使用能整除的最大单位
const splitBytes = (bytes) => {
  for (let i = BYTE_UNITS.length - 1; i >= 0; i -= 1) {
    const unit = BYTE_UNITS[i];
    if (bytes >= unit.bytes && bytes % unit.bytes === 0) {
      return { value: String(bytes / unit.bytes), unit: unit.bytes };
    }
  }
  return { value: '1', unit: 1024 * 1024 * 1024 };
};

const ruleToForm = (rule) => {
  const upload = splitBytes(rule.upload_threshold_bytes || 0);
  const download = splitBytes(rule.download_threshold_bytes || 0);
  return {
    priority: rule.priority ?? 0,
    upload_threshold_value: upload.value,
    upload_threshold_unit: upload.unit,
    download_threshold_value: download.value,
    download_threshold_unit: download.unit,
    allow_connect: rule.allow_connect,
    limit_upload_kb: rule.limit_upload_kb || 0,
    limit_download_kb: rule.limit_download_kb || 0,
  };
};

const defaultRule = () => ({
  priority: 1,
  upload_threshold_value: '100',
  upload_threshold_unit: 1024 * 1024 * 1024,
  download_threshold_value: '100',
  download_threshold_unit: 1024 * 1024 * 1024,
  allow_connect: true,
  limit_upload_kb: 0,
  limit_download_kb: 0,
});

// 规则描述：优先级 + 上传/下载流量大于阈值时触发
const describeRule = (rule) =>
  `优先级 ${rule.priority}：上传 > ${formatBytes(rule.upload_threshold_bytes)} 或 下载 > ${formatBytes(rule.download_threshold_bytes)}`;

// 剩余时间格式化
const formatRemaining = (sec) => {
  if (!sec || sec <= 0) return '-';
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const minutes = Math.floor((sec % 3600) / 60);
  const parts = [];
  if (days > 0) parts.push(`${days}天`);
  if (hours > 0) parts.push(`${hours}小时`);
  if (minutes > 0 && days === 0) parts.push(`${minutes}分钟`);
  return parts.length ? parts.join('') : '不足1分钟';
};

// 流量阈值输入框：数值 + 单位（KB 内嵌在下拉中）
const ThresholdField = ({ label, value, unit, onValueChange, onUnitChange }) => (
  <TextField
    type="number"
    size="small"
    label={label}
    value={value}
    onChange={(e) => onValueChange(e.target.value)}
    fullWidth
    InputProps={{
      endAdornment: (
        <InputAdornment position="end">
          <Select
            variant="standard"
            disableUnderline
            value={unit}
            onChange={(e) => onUnitChange(Number(e.target.value))}
            sx={{ minWidth: 56 }}
          >
            {BYTE_UNITS.map((u) => (
              <MenuItem key={u.bytes} value={u.bytes}>{u.label}</MenuItem>
            ))}
          </Select>
        </InputAdornment>
      ),
    }}
  />
);

const RateLimit = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

  const [tab, setTab] = useState(0);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');

  // 限速方案
  const [plans, setPlans] = useState([]);
  const [plansLoading, setPlansLoading] = useState(false);
  const [planDialog, setPlanDialog] = useState({ open: false, editId: 0 });
  const [planForm, setPlanForm] = useState({ name: '', description: '', period_value: '1', period_unit: 86400, rules: [defaultRule()] });
  const [planBusy, setPlanBusy] = useState(false);
  const [planError, setPlanError] = useState('');

  // 方案用户管理
  const [userDialog, setUserDialog] = useState({ open: false, plan: null });
  const [planUsers, setPlanUsers] = useState([]);
  const [planUserTotal, setPlanUserTotal] = useState(0);
  const [planUserPage, setPlanUserPage] = useState(1);
  const [planUserPageSize, setPlanUserPageSize] = useState(10);
  const [planUserQuery, setPlanUserQuery] = useState('');
  const [planUserSearch, setPlanUserSearch] = useState('');
  const [planUserSelected, setPlanUserSelected] = useState([]);
  const [userBusy, setUserBusy] = useState(false);
  const [userError, setUserError] = useState('');
  const userSeqRef = useRef(0);

  // 添加用户（参考用户组管理页面）
  const [addUserDialog, setAddUserDialog] = useState({ open: false, plan: null });
  const [availableUsers, setAvailableUsers] = useState([]);
  const [availableTotal, setAvailableTotal] = useState(0);
  const [availablePage, setAvailablePage] = useState(1);
  const [availablePageSize, setAvailablePageSize] = useState(10);
  const [availableQuery, setAvailableQuery] = useState('');
  const [availableSearch, setAvailableSearch] = useState('');
  const [availableSelected, setAvailableSelected] = useState([]);
  const [availableLoading, setAvailableLoading] = useState(false);
  const [availableError, setAvailableError] = useState('');
  const availableSeqRef = useRef(0);

  // 用户状态
  const [userStatus, setUserStatus] = useState([]);
  const [userStatusTotal, setUserStatusTotal] = useState(0);
  const [userStatusPage, setUserStatusPage] = useState(1);
  const [userStatusPageSize, setUserStatusPageSize] = useState(20);
  const [userStatusLoading, setUserStatusLoading] = useState(false);
  const [userStatusError, setUserStatusError] = useState('');
  const userStatusSeqRef = useRef(0);

  const loadPlans = useCallback(async () => {
    setPlansLoading(true);
    setError('');
    try {
      const res = await rateLimitAPI.listPlans();
      setPlans(res.data.data || []);
    } catch (err) {
      setError('加载限速方案失败：' + (err.response?.data?.error || err.message));
    } finally {
      setPlansLoading(false);
    }
  }, []);

  const loadUserStatus = useCallback(async (page = 1, pageSize = userStatusPageSize) => {
    const seq = ++userStatusSeqRef.current;
    setUserStatusLoading(true);
    setUserStatusError('');
    try {
      const res = await rateLimitAPI.listUserStatus(page, pageSize);
      if (seq !== userStatusSeqRef.current) return;
      setUserStatus(res.data.data || []);
      setUserStatusTotal(res.data.count || 0);
      setUserStatusPage(page);
    } catch (err) {
      if (seq === userStatusSeqRef.current) {
        setUserStatusError('加载用户状态失败：' + (err.response?.data?.error || err.message));
      }
    } finally {
      if (seq === userStatusSeqRef.current) setUserStatusLoading(false);
    }
  }, [userStatusPageSize]);

  useEffect(() => {
    loadPlans();
  }, [loadPlans]);

  useEffect(() => {
    if (tab !== 1) return undefined;
    loadUserStatus(1, userStatusPageSize);
    const timer = setInterval(() => loadUserStatus(userStatusPage, userStatusPageSize), 15000);
    return () => clearInterval(timer);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [tab, loadUserStatus]);

  const resetUserCycle = async (item) => {
    if (!window.confirm(`确定要立即重置用户「${item.username}」本周期内的流量吗？`)) return;
    try {
      await rateLimitAPI.resetUserCycle(item.user_id);
      setSuccess(`已重置用户 ${item.username} 的周期流量`);
      loadUserStatus(userStatusPage, userStatusPageSize);
    } catch (err) {
      setUserStatusError('重置失败：' + (err.response?.data?.error || err.message));
    }
  };

  // ===== 方案编辑 =====
  const openCreatePlan = () => {
    setPlanForm({ name: '', description: '', period_value: '1', period_unit: 86400, rules: [defaultRule()] });
    setPlanError('');
    setPlanDialog({ open: true, editId: 0 });
  };

  const openEditPlan = (plan) => {
    const period = splitPeriod(plan.period_seconds);
    // 编辑时规则按优先级从小到大展示（后端存储顺序为从大到小，便于匹配）
    const sortedRules = [...(plan.rules || [])].sort(
      (a, b) => (a.priority ?? 0) - (b.priority ?? 0)
    );
    setPlanForm({
      name: plan.name,
      description: plan.description || '',
      period_value: period.value,
      period_unit: period.unit,
      rules: sortedRules.length ? sortedRules.map(ruleToForm) : [defaultRule()],
    });
    setPlanError('');
    setPlanDialog({ open: true, editId: plan.id });
  };

  const updateRule = (index, patch) => {
    setPlanForm((prev) => {
      const rules = prev.rules.map((r, i) => (i === index ? { ...r, ...patch } : r));
      return { ...prev, rules };
    });
  };

  const addRule = () =>
    setPlanForm((prev) => {
      const maxPriority = prev.rules.reduce((m, r) => Math.max(m, Number(r.priority) || 0), 0);
      return { ...prev, rules: [...prev.rules, { ...defaultRule(), priority: maxPriority + 1 }] };
    });
  const removeRule = (index) =>
    setPlanForm((prev) => ({ ...prev, rules: prev.rules.filter((_, i) => i !== index) }));

  const submitPlan = async () => {
    setPlanError('');
    const periodValue = Number(planForm.period_value);
    if (!planForm.name.trim()) {
      setPlanError('请填写方案名称');
      return;
    }
    if (!periodValue || periodValue <= 0) {
      setPlanError('周期必须大于 0');
      return;
    }
    if (!planForm.rules.length) {
      setPlanError('至少需要一条限速规则');
      return;
    }
    const rules = [];
    for (let i = 0; i < planForm.rules.length; i += 1) {
      const r = planForm.rules[i];
      const priority = Number(r.priority);
      const uploadThreshold = Number(r.upload_threshold_value);
      const downloadThreshold = Number(r.download_threshold_value);
      if (Number.isNaN(priority) || priority < 0) {
        setPlanError(`第 ${i + 1} 条规则的优先级无效`);
        return;
      }
      if (Number.isNaN(uploadThreshold) || uploadThreshold < 0) {
        setPlanError(`第 ${i + 1} 条规则的上传流量无效`);
        return;
      }
      if (Number.isNaN(downloadThreshold) || downloadThreshold < 0) {
        setPlanError(`第 ${i + 1} 条规则的下载流量无效`);
        return;
      }
      rules.push({
        priority,
        upload_threshold_bytes: Math.round(uploadThreshold * r.upload_threshold_unit),
        download_threshold_bytes: Math.round(downloadThreshold * r.download_threshold_unit),
        limit_upload_kb: Number(r.limit_upload_kb) || 0,
        limit_download_kb: Number(r.limit_download_kb) || 0,
        allow_connect: !!r.allow_connect,
      });
    }
    const payload = {
      name: planForm.name.trim(),
      description: planForm.description,
      period_seconds: Math.round(periodValue * planForm.period_unit),
      rules,
    };
    setPlanBusy(true);
    try {
      if (planDialog.editId) {
        await rateLimitAPI.updatePlan({ id: planDialog.editId, ...payload });
        setSuccess('限速方案已更新');
      } else {
        await rateLimitAPI.createPlan(payload);
        setSuccess('限速方案已创建');
      }
      setPlanDialog({ open: false, editId: 0 });
      loadPlans();
    } catch (err) {
      setPlanError(err.response?.data?.error || err.message);
    } finally {
      setPlanBusy(false);
    }
  };

  const deletePlan = async (plan) => {
    if (!window.confirm(`确定要删除限速方案「${plan.name}」吗？关联用户的周期数据会被清除。`)) return;
    try {
      await rateLimitAPI.deletePlan(plan.id);
      setSuccess('限速方案已删除');
      loadPlans();
    } catch (err) {
      setError('删除失败：' + (err.response?.data?.error || err.message));
    }
  };

  // ===== 方案用户管理 =====
  const loadPlanUsers = useCallback(async (planId, page, pageSize, query) => {
    const seq = ++userSeqRef.current;
    setUserBusy(true);
    setUserError('');
    try {
      const res = await rateLimitAPI.listPlanUsers(planId, page, pageSize, query);
      if (seq !== userSeqRef.current) return;
      setPlanUsers(res.data.data || []);
      setPlanUserTotal(res.data.count || 0);
      setPlanUserPage(page);
      setPlanUserPageSize(pageSize);
      setPlanUserQuery(query);
      setPlanUserSelected([]);
    } catch (err) {
      if (seq === userSeqRef.current) {
        setUserError('加载方案用户失败：' + (err.response?.data?.error || err.message));
      }
    } finally {
      if (seq === userSeqRef.current) setUserBusy(false);
    }
  }, []);

  const loadAvailableUsers = useCallback(async (planId, page, pageSize, query) => {
    const seq = ++availableSeqRef.current;
    setAvailableLoading(true);
    setAvailableError('');
    try {
      const res = await userManageAPI.getUserList(page, pageSize, query, '', planId);
      if (seq !== availableSeqRef.current) return;
      setAvailableUsers(res.data.data || []);
      setAvailableTotal(res.data.count || 0);
      setAvailablePage(page);
      setAvailablePageSize(pageSize);
      setAvailableQuery(query);
      setAvailableSelected([]);
    } catch (err) {
      if (seq === availableSeqRef.current) {
        setAvailableError('加载用户列表失败：' + (err.response?.data?.error || err.message));
      }
    } finally {
      if (seq === availableSeqRef.current) setAvailableLoading(false);
    }
  }, []);

  const openUserDialog = (plan) => {
    setUserDialog({ open: true, plan });
    setPlanUserSearch('');
    setPlanUserSelected([]);
    setUserError('');
    loadPlanUsers(plan.id, 1, 10, '');
  };

  const openAddUserDialog = (plan) => {
    setAddUserDialog({ open: true, plan });
    setAvailableSearch('');
    setAvailableQuery('');
    setAvailableSelected([]);
    setAvailableError('');
    loadAvailableUsers(plan.id, 1, 10, '');
  };

  const handleAddUsers = async () => {
    if (!availableSelected.length) return;
    setAvailableLoading(true);
    setAvailableError('');
    try {
      const usernames = availableUsers
        .filter((u) => availableSelected.includes(u.id))
        .map((u) => u.username);
      await rateLimitAPI.addPlanUsers(addUserDialog.plan.id, usernames);
      setSuccess(`已关联 ${usernames.length} 个用户`);
      const plan = addUserDialog.plan;
      setAddUserDialog({ open: false, plan: null });
      if (userDialog.open && userDialog.plan && userDialog.plan.id === plan.id) {
        loadPlanUsers(plan.id, planUserPage, planUserPageSize, planUserQuery);
      }
      loadPlans();
    } catch (err) {
      setAvailableError(err.response?.data?.error || err.message);
    } finally {
      setAvailableLoading(false);
    }
  };

  const removeSelectedUsers = async () => {
    if (!planUserSelected.length) return;
    if (!window.confirm(`确定要移除选中的 ${planUserSelected.length} 个用户吗？`)) return;
    setUserBusy(true);
    setUserError('');
    try {
      await rateLimitAPI.removePlanUsers(userDialog.plan.id, planUserSelected);
      setSuccess('已移除所选用户');
      loadPlanUsers(userDialog.plan.id, planUserPage, planUserPageSize, planUserQuery);
      loadPlans();
    } catch (err) {
      setUserError(err.response?.data?.error || err.message);
    } finally {
      setUserBusy(false);
    }
  };

  // ===== 渲染 =====
  const renderPlans = () => (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2, flexWrap: 'wrap', gap: 1 }}>
        <Typography variant="h6">限速方案</Typography>
        <Button variant="contained" startIcon={<AddIcon />} onClick={openCreatePlan}>
          创建方案
        </Button>
      </Stack>

      {plansLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
          <CircularProgress />
        </Box>
      ) : plans.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center', color: 'text.secondary' }}>暂无限速方案</Paper>
      ) : isMobile ? (
        plans.map((plan) => (
          <Card key={plan.id} sx={{ mb: 2 }}>
            <CardContent>
              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography variant="subtitle1" sx={{ fontWeight: 'bold' }}>{plan.name}</Typography>
                <Chip label={`周期 ${formatPeriod(plan.period_seconds)}`} size="small" />
              </Stack>
              {plan.description ? (
                <Typography variant="body2" color="textSecondary" sx={{ mt: 1 }}>{plan.description}</Typography>
              ) : null}
              <Typography variant="body2" sx={{ mt: 1 }}>关联用户：{plan.user_count} 人</Typography>
              <Box sx={{ mt: 1 }}>
                {(plan.rules || []).map((rule, i) => (
                  <Typography key={rule.id || i} variant="caption" sx={{ display: 'block' }}>
                    {describeRule(rule)}：{rule.allow_connect ? `${formatRate(rule.limit_upload_kb)} / ${formatRate(rule.limit_download_kb)}` : '禁止连接'}
                  </Typography>
                ))}
              </Box>
              <Stack direction="row" spacing={1} sx={{ mt: 2, flexWrap: 'wrap', gap: 1 }}>
                <Button size="small" variant="outlined" startIcon={<PeopleIcon />} onClick={() => openUserDialog(plan)}>
                  用户 ({plan.user_count})
                </Button>
                <Button size="small" variant="outlined" startIcon={<EditIcon />} onClick={() => openEditPlan(plan)}>
                  编辑
                </Button>
                <Button size="small" variant="outlined" color="error" startIcon={<DeleteIcon />} onClick={() => deletePlan(plan)}>
                  删除
                </Button>
              </Stack>
            </CardContent>
          </Card>
        ))
      ) : (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold' }}>名称</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>周期</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>限速规则</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>关联用户</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>操作</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {plans.map((plan) => (
                <TableRow key={plan.id} hover>
                  <TableCell>{plan.name}</TableCell>
                  <TableCell>{formatPeriod(plan.period_seconds)}</TableCell>
                  <TableCell>
                    <Chip label={`${(plan.rules || []).length} 条`} size="small" variant="outlined" />
                  </TableCell>
                  <TableCell>{plan.user_count}</TableCell>
                  <TableCell sx={{ maxWidth: 240, wordBreak: 'break-word' }}>{plan.description || '-'}</TableCell>
                  <TableCell>
                    <Stack direction="row" spacing={1}>
                      <Button size="small" startIcon={<PeopleIcon />} onClick={() => openUserDialog(plan)}>用户</Button>
                      <Button size="small" startIcon={<EditIcon />} onClick={() => openEditPlan(plan)}>编辑</Button>
                      <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={() => deletePlan(plan)}>删除</Button>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );

  const renderLimit = (item) => {
    if (!item.allow_connect) return '拒绝连接';
    if (item.has_rule) {
      return `上传 ${formatRate(item.limit_upload_kb)} / 下载 ${formatRate(item.limit_download_kb)}`;
    }
    return '依据用户/用户组限速';
  };

  const renderUserStatus = () => (
    <Box>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2, flexWrap: 'wrap', gap: 1 }}>
        <Typography variant="h6">用户状态</Typography>
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={() => loadUserStatus(userStatusPage)}
          disabled={userStatusLoading}
        >
          刷新
        </Button>
      </Stack>

      {userStatusError && (
        <Alert severity="error" sx={{ mb: 2 }} onClose={() => setUserStatusError('')}>{userStatusError}</Alert>
      )}
      {userStatusLoading && userStatus.length === 0 ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
          <CircularProgress />
        </Box>
      ) : userStatus.length === 0 ? (
        <Paper sx={{ p: 3, textAlign: 'center', color: 'text.secondary' }}>暂无已关联限速方案的用户</Paper>
      ) : isMobile ? (
        userStatus.map((item) => (
          <Card key={item.user_id} sx={{ mb: 2 }}>
            <CardContent>
              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Typography variant="subtitle1" sx={{ fontWeight: 'bold', wordBreak: 'break-all' }}>{item.username}</Typography>
                <Chip label={item.plan_name || '无方案'} size="small" color={item.plan_name ? 'primary' : 'default'} />
              </Stack>
              <Typography variant="body2" sx={{ mt: 1 }}>周期已用：↑ {formatBytes(item.cycle_upload)} / ↓ {formatBytes(item.cycle_download)}</Typography>
              <Typography variant="body2">当前规则：{item.has_rule ? `优先级 ${item.rule_priority}` : '无'}</Typography>
              <Typography variant="body2">限制：{renderLimit(item)}</Typography>
              <Typography variant="body2">周期剩余：{formatRemaining(item.reset_remaining_sec)}</Typography>
              <Button size="small" variant="outlined" sx={{ mt: 1 }} onClick={() => resetUserCycle(item)}>
                重置周期流量
              </Button>
            </CardContent>
          </Card>
        ))
      ) : (
        <TableContainer component={Paper}>
          <Table size="small">
            <TableHead>
              <TableRow>
                <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>限速方案</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>周期已用流量</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>当前规则</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>限制</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>周期剩余</TableCell>
                <TableCell sx={{ fontWeight: 'bold' }}>操作</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {userStatus.map((item) => (
                <TableRow key={item.user_id} hover>
                  <TableCell>{item.username}</TableCell>
                  <TableCell>{item.plan_name || '-'}</TableCell>
                  <TableCell>
                    <Typography variant="body2">↑ {formatBytes(item.cycle_upload)}</Typography>
                    <Typography variant="body2">↓ {formatBytes(item.cycle_download)}</Typography>
                  </TableCell>
                  <TableCell>{item.has_rule ? `优先级 ${item.rule_priority}` : '无'}</TableCell>
                  <TableCell>{renderLimit(item)}</TableCell>
                  <TableCell>{formatRemaining(item.reset_remaining_sec)}</TableCell>
                  <TableCell>
                    <Button size="small" onClick={() => resetUserCycle(item)}>重置</Button>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {userStatusTotal > 0 && (
        <Stack direction="row" spacing={2} alignItems="center" sx={{ mt: 2, flexWrap: 'wrap', gap: 1 }}>
          <FormControl sx={{ minWidth: 120 }} size="small">
            <InputLabel>每页数量</InputLabel>
            <Select
              label="每页数量"
              value={userStatusPageSize}
              onChange={(e) => {
                const size = e.target.value;
                setUserStatusPageSize(size);
                loadUserStatus(1, size);
              }}
            >
              <MenuItem value={10}>10</MenuItem>
              <MenuItem value={20}>20</MenuItem>
              <MenuItem value={50}>50</MenuItem>
              <MenuItem value={100}>100</MenuItem>
              <MenuItem value={200}>200</MenuItem>
            </Select>
          </FormControl>
          <Typography>总共 {userStatusTotal} 个用户</Typography>
          <Pagination
            count={Math.ceil(userStatusTotal / userStatusPageSize) || 1}
            page={userStatusPage}
            onChange={(e, value) => loadUserStatus(value, userStatusPageSize)}
            color="primary"
            size="small"
          />
        </Stack>
      )}
    </Box>
  );

  return (
    <Box sx={{ p: isMobile ? 2 : 3, width: '100%' }}>
      <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 2 }}>
        <SpeedIcon color="primary" />
        <Typography variant="h5" sx={{ fontWeight: 'bold' }}>达量限速</Typography>
      </Stack>

      {error && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>{error}</Alert>}
      {success && <Alert severity="success" sx={{ mb: 2 }} onClose={() => setSuccess('')}>{success}</Alert>}

      <Paper sx={{ mb: 2 }}>
        <Tabs value={tab} onChange={(e, v) => setTab(v)} variant="fullWidth">
          <Tab label="限速方案" />
          <Tab label="用户状态" />
        </Tabs>
      </Paper>

      {tab === 0 ? renderPlans() : renderUserStatus()}

      {/* 方案编辑对话框 */}
      <Dialog open={planDialog.open} onClose={() => setPlanDialog({ open: false, editId: 0 })} fullWidth maxWidth="md" fullScreen={isMobile}>
        <DialogTitle>{planDialog.editId ? '编辑限速方案' : '创建限速方案'}</DialogTitle>
        <DialogContent>
          {planError && <Alert severity="error" sx={{ mb: 2 }}>{planError}</Alert>}
          <TextField
            fullWidth
            margin="normal"
            label="方案名称"
            value={planForm.name}
            onChange={(e) => setPlanForm({ ...planForm, name: e.target.value })}
          />
          <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
            <TextField
              label="周期"
              type="number"
              value={planForm.period_value}
              onChange={(e) => setPlanForm({ ...planForm, period_value: e.target.value })}
              sx={{ width: 160 }}
            />
            <Select
              value={planForm.period_unit}
              onChange={(e) => setPlanForm({ ...planForm, period_unit: Number(e.target.value) })}
              sx={{ minWidth: 120 }}
            >
              {PERIOD_UNITS.map((u) => (
                <MenuItem key={u.seconds} value={u.seconds}>{u.label}</MenuItem>
              ))}
            </Select>
          </Stack>
          <TextField
            fullWidth
            margin="normal"
            multiline
            minRows={2}
            label="描述"
            value={planForm.description}
            onChange={(e) => setPlanForm({ ...planForm, description: e.target.value })}
          />

          <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mt: 3, mb: 1 }}>
            <Box>
              <Typography variant="subtitle1">限速规则</Typography>
              <Typography variant="caption" color="textSecondary">
                上传或下载任一超过阈值即触发，按优先级从大到小匹配
              </Typography>
            </Box>
            <Button size="small" variant="outlined" startIcon={<AddIcon />} onClick={addRule}>添加规则</Button>
          </Stack>
          {(() => {
            const priorities = planForm.rules.map((r) => Number(r.priority));
            const hasDuplicate = priorities.some((p, i) => priorities.indexOf(p) !== i);
            return hasDuplicate ? (
              <Alert severity="warning" sx={{ mb: 1 }}>
                检测到重复的优先级：不建议多条规则使用相同优先级，可能产生你不希望的效果。
              </Alert>
            ) : (
              <Typography variant="caption" color="textSecondary" sx={{ display: 'block' }}>
                提示：不建议多条规则使用相同优先级，可能产生你不希望的效果。
              </Typography>
            );
          })()}
          {planForm.rules.map((rule, index) => (
            <Paper key={index} variant="outlined" sx={{ p: 2, mt: 1.5 }}>
              <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.5 }}>
                <Chip label={`规则 #${index + 1}`} size="small" variant="outlined" />
                <IconButton size="small" color="error" onClick={() => removeRule(index)} disabled={planForm.rules.length <= 1}>
                  <DeleteIcon fontSize="small" />
                </IconButton>
              </Stack>

              <Box
                sx={{
                  display: 'grid',
                  gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr', lg: 'repeat(5, minmax(0, 1fr))' },
                  gap: 1.5,
                }}
              >
                <TextField
                  type="number"
                  size="small"
                  label="优先级"
                  value={rule.priority}
                  onChange={(e) => updateRule(index, { priority: e.target.value })}
                  helperText="越大越先匹配"
                  fullWidth
                />
                <ThresholdField
                  label="上传流量大于"
                  value={rule.upload_threshold_value}
                  unit={rule.upload_threshold_unit}
                  onValueChange={(v) => updateRule(index, { upload_threshold_value: v })}
                  onUnitChange={(u) => updateRule(index, { upload_threshold_unit: u })}
                />
                <ThresholdField
                  label="下载流量大于"
                  value={rule.download_threshold_value}
                  unit={rule.download_threshold_unit}
                  onValueChange={(v) => updateRule(index, { download_threshold_value: v })}
                  onUnitChange={(u) => updateRule(index, { download_threshold_unit: u })}
                />
                <TextField
                  size="small"
                  type="number"
                  label="上传限速 (KB/s)"
                  value={rule.limit_upload_kb}
                  onChange={(e) => updateRule(index, { limit_upload_kb: e.target.value })}
                  helperText="0 = 不限速"
                  fullWidth
                />
                <TextField
                  size="small"
                  type="number"
                  label="下载限速 (KB/s)"
                  value={rule.limit_download_kb}
                  onChange={(e) => updateRule(index, { limit_download_kb: e.target.value })}
                  helperText="0 = 不限速"
                  fullWidth
                />
              </Box>

              <FormControlLabel
                sx={{ mt: 1 }}
                control={
                  <Checkbox
                    checked={rule.allow_connect}
                    onChange={(e) => updateRule(index, { allow_connect: e.target.checked })}
                  />
                }
                label="允许连接（不勾选则匹配到该规则时直接拒绝认证）"
              />
            </Paper>
          ))}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setPlanDialog({ open: false, editId: 0 })}>取消</Button>
          <Button variant="contained" onClick={submitPlan} disabled={planBusy}>
            {planBusy ? '保存中…' : '保存'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* 方案用户管理对话框 */}
      <Dialog
        open={userDialog.open}
        onClose={() => setUserDialog({ open: false, plan: null })}
        fullWidth
        maxWidth="md"
        fullScreen={isMobile}
      >
        <DialogTitle>管理用户 - {userDialog.plan?.name}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {userError && <Alert severity="error" sx={{ mb: 2 }} onClose={() => setUserError('')}>{userError}</Alert>}
          <Typography variant="body2" sx={{ marginBottom: 2, color: '#666' }}>
            当前已关联 {planUserTotal} 个用户
          </Typography>

          <Stack direction="row" spacing={1} sx={{ marginBottom: 2, flexWrap: 'wrap', gap: 1 }}>
            <TextField
              placeholder="搜索用户名"
              size="small"
              value={planUserSearch}
              onChange={(e) => setPlanUserSearch(e.target.value)}
              onKeyPress={(e) => {
                if (e.key === 'Enter') loadPlanUsers(userDialog.plan.id, 1, planUserPageSize, planUserSearch);
              }}
              sx={{ minWidth: 200, flex: 1 }}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={() => loadPlanUsers(userDialog.plan.id, 1, planUserPageSize, planUserSearch)}
              disabled={userBusy}
            >
              搜索
            </Button>
            <Button
              variant="contained"
              size="small"
              startIcon={<AddIcon />}
              onClick={() => openAddUserDialog(userDialog.plan)}
            >
              添加用户
            </Button>
          </Stack>

          {userBusy && <CircularProgress sx={{ marginBottom: 2 }} />}

          <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
            <Table size="small">
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', width: '50px' }}>
                    <Checkbox
                      indeterminate={planUserSelected.length > 0 && planUserSelected.length < planUsers.length}
                      checked={planUsers.length > 0 && planUserSelected.length === planUsers.length}
                      onChange={(e) => setPlanUserSelected(e.target.checked ? planUsers.map((u) => u.id) : [])}
                    />
                  </TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {planUsers.length > 0 ? (
                  planUsers.map((u) => (
                    <TableRow key={u.id} hover>
                      <TableCell sx={{ padding: 0, width: '50px' }}>
                        <Checkbox
                          checked={planUserSelected.includes(u.id)}
                          onChange={() =>
                            setPlanUserSelected((prev) =>
                              prev.includes(u.id) ? prev.filter((id) => id !== u.id) : [...prev, u.id]
                            )
                          }
                        />
                      </TableCell>
                      <TableCell sx={{ padding: 0 }}>{u.id}</TableCell>
                      <TableCell sx={{ padding: 0 }}>{u.username}</TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={3} align="center">
                      {userBusy ? '加载中...' : '暂无用户'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {planUserTotal > 0 && (
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControl sx={{ minWidth: 120 }}>
                <InputLabel>每页数量</InputLabel>
                <Select
                  value={planUserPageSize}
                  label="每页数量"
                  onChange={(e) => loadPlanUsers(userDialog.plan.id, 1, e.target.value, planUserQuery)}
                  size="small"
                  disabled={userBusy}
                >
                  <MenuItem value={5}>5</MenuItem>
                  <MenuItem value={10}>10</MenuItem>
                  <MenuItem value={20}>20</MenuItem>
                </Select>
              </FormControl>
              <Box sx={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
                <Pagination
                  count={Math.ceil(planUserTotal / planUserPageSize)}
                  page={planUserPage}
                  onChange={(e, value) => loadPlanUsers(userDialog.plan.id, value, planUserPageSize, planUserQuery)}
                  color="primary"
                  size="small"
                  disabled={userBusy}
                />
              </Box>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setUserDialog({ open: false, plan: null })}>关闭</Button>
          <Button
            color="error"
            variant="outlined"
            onClick={removeSelectedUsers}
            disabled={userBusy || planUserSelected.length === 0}
          >
            移除所选 ({planUserSelected.length})
          </Button>
        </DialogActions>
      </Dialog>

      {/* 添加用户到方案对话框（参考用户组管理） */}
      <Dialog
        open={addUserDialog.open}
        onClose={() => {
          setAddUserDialog({ open: false, plan: null });
          setAvailableSelected([]);
          setAvailableError('');
        }}
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        fullScreen={isMobile}
      >
        <DialogTitle>添加用户到 {addUserDialog.plan?.name}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {availableError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setAvailableError('')}>
              {availableError}
            </Alert>
          )}
          <Typography variant="body2" sx={{ marginBottom: 2, color: '#666' }}>
            选择要添加的用户（已在此限速方案中的用户已被过滤）
          </Typography>

          <Stack direction="row" spacing={1} sx={{ marginBottom: 2 }}>
            <TextField
              placeholder="搜索用户名"
              size="small"
              value={availableSearch}
              onChange={(e) => setAvailableSearch(e.target.value)}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  loadAvailableUsers(addUserDialog.plan.id, 1, availablePageSize, availableSearch);
                }
              }}
              sx={{ minWidth: 200, flex: 1 }}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={() => loadAvailableUsers(addUserDialog.plan.id, 1, availablePageSize, availableSearch)}
              disabled={availableLoading}
            >
              搜索
            </Button>
          </Stack>

          {availableLoading && <CircularProgress sx={{ marginBottom: 2 }} />}

          <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
            <Table size="small">
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', width: '50px' }}>
                    <Checkbox
                      indeterminate={availableSelected.length > 0 && availableSelected.length < availableUsers.length}
                      checked={availableUsers.length > 0 && availableSelected.length === availableUsers.length}
                      onChange={(e) => setAvailableSelected(e.target.checked ? availableUsers.map((u) => u.id) : [])}
                    />
                  </TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {availableUsers.length > 0 ? (
                  availableUsers.map((user) => (
                    <TableRow key={user.id} hover>
                      <TableCell sx={{ padding: 0, width: '50px' }}>
                        <Checkbox
                          checked={availableSelected.includes(user.id)}
                          onChange={() =>
                            setAvailableSelected((prev) =>
                              prev.includes(user.id) ? prev.filter((id) => id !== user.id) : [...prev, user.id]
                            )
                          }
                        />
                      </TableCell>
                      <TableCell sx={{ padding: 0 }}>{user.id}</TableCell>
                      <TableCell sx={{ padding: 0 }}>{user.username}</TableCell>
                      <TableCell sx={{ padding: 0 }}>{user.description || '无'}</TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={4} align="center">
                      {availableLoading ? '加载中...' : '没有可添加的用户'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {availableTotal > 0 && (
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControl sx={{ minWidth: 120 }}>
                <InputLabel>每页数量</InputLabel>
                <Select
                  value={availablePageSize}
                  label="每页数量"
                  onChange={(e) => loadAvailableUsers(addUserDialog.plan.id, 1, e.target.value, availableQuery)}
                  size="small"
                  disabled={availableLoading}
                >
                  <MenuItem value={5}>5</MenuItem>
                  <MenuItem value={10}>10</MenuItem>
                  <MenuItem value={20}>20</MenuItem>
                </Select>
              </FormControl>
              <Box sx={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
                <Pagination
                  count={Math.ceil(availableTotal / availablePageSize)}
                  page={availablePage}
                  onChange={(e, value) => loadAvailableUsers(addUserDialog.plan.id, value, availablePageSize, availableQuery)}
                  color="primary"
                  size="small"
                  disabled={availableLoading}
                />
              </Box>
            </Stack>
          )}
        </DialogContent>
        <DialogActions>
          <Button
            onClick={() => {
              setAddUserDialog({ open: false, plan: null });
              setAvailableSelected([]);
              setAvailableError('');
            }}
          >
            取消
          </Button>
          <Button onClick={handleAddUsers} variant="contained" disabled={availableLoading || availableSelected.length === 0}>
            添加 ({availableSelected.length})
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default RateLimit;

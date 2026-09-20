import React, { useState, useEffect, useRef } from 'react';
import {
  Paper,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
  Box,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  CircularProgress,
  Alert,
  Typography,
  Stack,
  FormControlLabel,
  Checkbox,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Chip,
  Card,
  CardContent,
  CardActions,
  useMediaQuery,
  useTheme,
  FormGroup,
  Pagination,
  TablePagination,
  Tabs,
  Tab,
  Menu,
  Autocomplete,
} from '@mui/material';
import AddIcon from '@mui/icons-material/Add';
import RefreshIcon from '@mui/icons-material/Refresh';
import EditIcon from '@mui/icons-material/Edit';
import DeleteIcon from '@mui/icons-material/Delete';
import PlayArrowIcon from '@mui/icons-material/PlayArrow';
import StopIcon from '@mui/icons-material/Stop';
import SecurityIcon from '@mui/icons-material/Security';
import { serverAPI, permissionAPI, userManageAPI, groupAPI, certificateAPI } from '../api';

const CERT_REF_PREFIX = 'cert-stor:';
const CERT_SELECT_PAGE_SIZE = 10;

// 数据加密算法（data-ciphers）推荐顺序，从高到低
const DATA_CIPHER_OPTIONS = [
  'AES-256-GCM',
  'AES-128-GCM',
  'CHACHA20-POLY1305',
  'AES-256-CBC',
  'AES-192-CBC',
  'DES-CBC',
  'AES-128-CBC',
  'BF-CBC',
  'DES-EDE3-CBC',
  'none',
  'DESX-CBC',
];

// 解析证书引用 cert-stor:<id>/cert 或 cert-stor:<id>/key
const parseCertRef = (value) => {
  const s = String(value || '').trim();
  if (!s.startsWith(CERT_REF_PREFIX)) return null;
  const rest = s.slice(CERT_REF_PREFIX.length);
  const idx = rest.indexOf('/');
  if (idx < 0) return null;
  const id = Number(rest.slice(0, idx));
  const part = rest.slice(idx + 1);
  if (!id || (part !== 'cert' && part !== 'key')) return null;
  return { id, part };
};

// 从 "C=CN, ST=..., CN=xxx" 形式的主题中提取 CN
const subjectCommonName = (subject) => {
  for (const part of String(subject || '').split(',')) {
    const idx = part.indexOf('=');
    if (idx < 0) continue;
    if (part.slice(0, idx).trim() === 'CN') return part.slice(idx + 1).trim();
  }
  return '';
};

// 证书类型标签（与后端 models.Certificate.Type 对应）
const certTypeLabel = (type) => {
  switch (type) {
    case 1:
      return 'CA 证书';
    case 2:
      return '服务器证书';
    case 3:
      return '客户端证书';
    case 4:
      return '未指定';
    default:
      return '未知';
  }
};

// 证书字段的只读摘要：显示所选证书信息，或手动填写证书的解析结果
const CertSummary = ({ value, info, manual, kind = 'cert' }) => {
  const ref = parseCertRef(value);
  const hasValue = String(value || '').trim() !== '';
  let text;
  let severity = 'info';
  if (ref) {
    if (info) {
      const expires = info.not_after ? new Date(info.not_after * 1000).toLocaleDateString() : '-';
      text = `已选择「${info.name}」（${info.subject || ''}，到期 ${expires}，${info.has_key ? '含私钥' : '无私钥'}）`;
    } else {
      text = `已引用证书 ${value}`;
    }
  } else if (!hasValue) {
    text = '未选择';
    severity = 'warning';
  } else if (kind === 'key') {
    text = '已手动填写私钥（点击“手动填写”可查看或修改）';
  } else if (manual?.status === 'ok' && manual.info) {
    const m = manual.info;
    const expires = m.not_after ? new Date(m.not_after * 1000).toLocaleDateString() : '-';
    text = `已手动填写「${m.subject || ''}」（${m.cert_type || '证书'}，到期 ${expires}）`;
  } else if (manual?.status === 'error') {
    text = '已手动填写 无法解析证书信息，请自行确保证书有效性';
    severity = 'warning';
  } else {
    text = '已手动填写，正在解析证书信息…';
  }
  return <Alert severity={severity} sx={{ marginTop: 0.5, marginBottom: 0.5 }}>{text}</Alert>;
};

const filenameFromDisposition = (disposition, fallback) => {
  if (!disposition) return fallback;
  const match = /filename\*?=(?:UTF-8'')?["']?([^"';]+)/i.exec(disposition);
  if (!match) return fallback;
  try {
    return decodeURIComponent(match[1]);
  } catch {
    return match[1];
  }
};

const ServerManagement = () => {
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));
  const initializedRef = useRef(false);
  const loadSeqRef = useRef(0);
  
  const [servers, setServers] = useState([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState('');
  const [success, setSuccess] = useState('');
  const [openDialog, setOpenDialog] = useState(false);
  const [editingId, setEditingId] = useState(null);
  const [actionLoading, setActionLoading] = useState(() => new Set()); // 正在执行操作的服务器 id 集合
  const [formData, setFormData] = useState({
    name: '',
    local: '',
    port: 28190,
    proto: 'udp',
    dev: '',
    ca: '',
    cert: '',
    key: '',
    dh: '',
    data_cipher: 'AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305',
    topology: 'subnet',
    server_cidr: '100.66.10.0 255.255.255.0',
    duplicate_cn: true,
    keepalive: '10 50',
    tls_auth: '',
    other_config: '',
    auto_start: true,
    server_route: [],
    export_host: '',
    export_port: 1194,
  });

  // 权限管理相关状态
  const [permissionDialog, setPermissionDialog] = useState(false);
  const [selectedServer, setSelectedServer] = useState(null);
  const [permissions, setPermissions] = useState({ GroupPermission: [], UserPermission: [] });
  const [selectedPermissions, setSelectedPermissions] = useState([]); // 用于统一选择权限项
  const [selectedUserPermissions, setSelectedUserPermissions] = useState([]); // 专门用于用户权限选择
  const [selectedGroupPermissions, setSelectedGroupPermissions] = useState([]); // 专门用于用户组权限选择
  const [allUsers, setAllUsers] = useState([]);
  const [allGroups, setAllGroups] = useState([]);
  const [addUserDialog, setAddUserDialog] = useState(false);
  const [addGroupDialog, setAddGroupDialog] = useState(false);
  const [selectedUsers, setSelectedUsers] = useState([]);
  const [selectedGroups, setSelectedGroups] = useState([]);
  const [addUserPermPage, setAddUserPermPage] = useState(1);
  const [addUserPermPageSize, setAddUserPermPageSize] = useState(10);
  const [addGroupPermPage, setAddGroupPermPage] = useState(1);
  const [addGroupPermPageSize, setAddGroupPermPageSize] = useState(10);
  const [availableUsersCount, setAvailableUsersCount] = useState(0);
  const [availableGroupsCount, setAvailableGroupsCount] = useState(0);
  const [availableUsersLoading, setAvailableUsersLoading] = useState(false);
  const [availableGroupsLoading, setAvailableGroupsLoading] = useState(false);
  const [addUserSearchQuery, setAddUserSearchQuery] = useState(''); // 添加用户搜索查询
  const [addGroupSearchQuery, setAddGroupSearchQuery] = useState(''); // 添加用户组搜索查询
  const [permissionTabValue, setPermissionTabValue] = useState(0); // 添加权限管理标签页状态

  // 客户端配置管理相关状态
  const [clientConfigDialog, setClientConfigDialog] = useState(false);
  const [selectedServerConfigs, setSelectedServerConfigs] = useState([]);
  const [selectedConfigs, setSelectedConfigs] = useState([]);
  const [fullConfigContent, setFullConfigContent] = useState('');
  const [showFullConfigDialog, setShowFullConfigDialog] = useState(false);
  const [addConfigDialog, setAddConfigDialog] = useState(false);
  const [configFormData, setConfigFormData] = useState({
    client_cert_name: '',
    config: '',
  });
  // 当服务器证书来自证书库时，可选的客户端证书 CN 列表
  const [clientCertCNs, setClientCertCNs] = useState([]);
  // 添加客户端配置管理专用的提示状态
  const [clientConfigError, setClientConfigError] = useState('');
  const [clientConfigSuccess, setClientConfigSuccess] = useState('');
  // 添加权限管理专用的提示状态
  const [permissionError, setPermissionError] = useState('');
  const [permissionSuccess, setPermissionSuccess] = useState('');
  // 服务器编辑对话框专用错误
  const [serverFormError, setServerFormError] = useState('');

  // 证书选择 / DH 生成 / 客户端导出 相关状态
  const [certSelect, setCertSelect] = useState({
    open: false, target: 'ca', certs: [], loading: false, page: 1, pageSize: CERT_SELECT_PAGE_SIZE, total: 0, caId: null,
  });
  // 证书字段显示模式：display（只显示所选证书信息）/ manual（显示输入框）
  const [certFieldMode, setCertFieldMode] = useState({ ca: 'display', cert: 'display', key: 'display' });
  // 已引用证书的信息缓存：id -> 证书信息
  const [certRefInfo, setCertRefInfo] = useState({});
  // 手动填写证书的解析结果：ca/cert -> { status, value, info }
  const [manualCertInfo, setManualCertInfo] = useState({ ca: null, cert: null });
  const [dhDialog, setDhDialog] = useState({ open: false, bits: 2048, generating: false });
  const [tlsGenerating, setTlsGenerating] = useState(false);
  const [exportDialog, setExportDialog] = useState({
    open: false,
    servers: [],
    clientCerts: [],
    serverId: '',
    certId: '',
    mode: 'select',
    caId: null,
    manualCert: '',
    manualKey: '',
    extraConfig: '',
    host: '',
    port: 1194,
    loading: false,
    error: '',
  });
  // 添加用户/用户组权限对话框专用错误
  const [addPermissionError, setAddPermissionError] = useState('');
  // 添加/编辑客户端配置对话框专用错误
  const [configFormError, setConfigFormError] = useState('');
  
  // 添加dropdown菜单状态
  const [dropdownAnchor, setDropdownAnchor] = useState(null);
  const [dropdownServerId, setDropdownServerId] = useState(null);

  const loadServers = async () => {
    const seq = ++loadSeqRef.current;
    setIsLoading(true);
    setError('');
    try {
      const response = await serverAPI.getServerList();
      // 丢弃过期的响应，避免并发刷新时旧数据覆盖新数据
      if (seq !== loadSeqRef.current) return;
      if (response.data.result === 'success') {
        setServers(response.data.data || []);
      }
    } catch (err) {
      if (seq === loadSeqRef.current) setError('加载服务器列表失败');
    } finally {
      if (seq === loadSeqRef.current) setIsLoading(false);
    }
  };

  const markActionLoading = (serverId, loading) => {
    setActionLoading((prev) => {
      const next = new Set(prev);
      if (loading) next.add(serverId);
      else next.delete(serverId);
      return next;
    });
  };

  useEffect(() => {
    if (!initializedRef.current) {
      initializedRef.current = true;
      loadServers();
    }
  }, []);

  // 手动填写的 CA / 服务器证书：尝试解析证书信息用于展示（防抖 400ms）
  useEffect(() => {
    if (!openDialog) return undefined;
    const timer = setTimeout(() => {
      ['ca', 'cert'].forEach(async (field) => {
        const value = String(formData[field] || '').trim();
        if (!value || parseCertRef(value)) {
          setManualCertInfo((prev) => (prev[field] ? { ...prev, [field]: null } : prev));
          return;
        }
        if (!value.includes('BEGIN CERTIFICATE') || !value.includes('END CERTIFICATE')) {
          setManualCertInfo((prev) => ({ ...prev, [field]: { status: 'error', value } }));
          return;
        }
        setManualCertInfo((prev) => ({ ...prev, [field]: { status: 'loading', value } }));
        try {
          const res = await certificateAPI.parse(value);
          setManualCertInfo((prev) => ({ ...prev, [field]: { status: 'ok', value, info: res.data.data } }));
        } catch (err) {
          setManualCertInfo((prev) => ({ ...prev, [field]: { status: 'error', value } }));
        }
      });
    }, 400);
    return () => clearTimeout(timer);
  }, [formData.ca, formData.cert, openDialog]);

  // 只返回与当前字段值匹配的手动解析结果，避免异步竞态显示旧信息
  const certManual = (field) => {
    const m = manualCertInfo[field];
    if (!m) return null;
    return m.value === String(formData[field] || '').trim() ? m : { status: 'loading' };
  };

  const handleAddServer = () => {
    setEditingId(null);
    setServerFormError('');
    setFormData({
      name: '',
      local: '',
      port: 28190,
      proto: 'udp',
      dev: '',
      ca: '',
      cert: '',
      key: '',
      dh: '',
      data_cipher: 'AES-256-GCM:AES-128-GCM:CHACHA20-POLY1305',
      topology: 'subnet',
      server_cidr: '100.66.10.0 255.255.255.0',
      duplicate_cn: true,
      keepalive: '10 50',
      tls_auth: '',
      other_config: '',
      auto_start: true,
      server_route: [],
      export_host: '',
      export_port: 1194,
    });
    setCertFieldMode({ ca: 'display', cert: 'display', key: 'display' });
    setOpenDialog(true);
  };

  const handleEditServer = async (serverId) => {
    try {
      const response = await serverAPI.getServerInfo(serverId);
      if (response.data.result === 'success') {
        const data = response.data.data;
        setEditingId(serverId);
        setFormData(data);
        setCertFieldMode({ ca: 'display', cert: 'display', key: 'display' });
        ['ca', 'cert', 'key'].forEach((field) => {
          const ref = parseCertRef(data[field]);
          if (ref) ensureCertRefInfo(ref.id);
        });
        setServerFormError('');
        setOpenDialog(true);
      }
    } catch (err) {
      setError('获取服务器信息失败');
    }
  };

  const handleSaveServer = async () => {
    setServerFormError('');
    try {
      if (editingId) {
        await serverAPI.updateServer(editingId, formData);
      } else {
        await serverAPI.createServer(formData);
      }
      setOpenDialog(false);
      loadServers();
    } catch (err) {
      setServerFormError(err.response?.data?.error || '保存失败');
    }
  };

  const handleDeleteServer = async (serverId) => {
    if (window.confirm('确定要删除该服务器吗？')) {
      try {
        await serverAPI.deleteServer(serverId);
        loadServers();
      } catch (err) {
        setError('删除失败');
      }
    }
  };

  const handleStartServer = async (serverId) => {
    try {
      markActionLoading(serverId, true);
      setError('');
      await serverAPI.startServer(serverId);
      setSuccess('服务器启动成功');
      setServers((prev) => prev.map((s) => (s.id === serverId ? { ...s, running: true } : s)));
      loadServers();
    } catch (err) {
      const errorMsg = err.response?.data?.error || '启动失败';
      setError(errorMsg);
    } finally {
      markActionLoading(serverId, false);
    }
  };

  const handleStopServer = async (serverId) => {
    try {
      markActionLoading(serverId, true);
      setError('');
      await serverAPI.stopServer(serverId);
      setSuccess('服务器停止成功');
      setServers((prev) => prev.map((s) => (s.id === serverId ? { ...s, running: false } : s)));
      loadServers();
    } catch (err) {
      const errorMsg = err.response?.data?.error || '停止失败';
      setError(errorMsg);
    } finally {
      markActionLoading(serverId, false);
    }
  };

  const handleFormChange = (field, value) => {
    setFormData((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  // 打开证书选择：target 为 ca / cert。选择服务器证书时会同时选中其私钥。
  // target=cert 时，若已选择 CA，则只列出该 CA 签发的服务器证书或未指定证书。
  const openCertSelect = async (target, page = 1) => {
    let caId = null;
    if (target === 'cert') {
      const ref = parseCertRef(formData.ca);
      if (ref) caId = ref.id;
    }
    const params = { page, pageSize: CERT_SELECT_PAGE_SIZE };
    if (target === 'ca') {
      params.type = 1;
    } else {
      params.types = [2, 4];
      params.hasKey = 1;
      if (caId) params.parentId = caId;
    }
    setCertSelect((prev) => ({ ...prev, open: true, target, certs: [], loading: true, page, caId }));
    try {
      const res = await certificateAPI.list(params);
      setCertSelect({
        open: true,
        target,
        certs: res.data.data || [],
        loading: false,
        page,
        pageSize: CERT_SELECT_PAGE_SIZE,
        total: res.data.total || 0,
        caId,
      });
    } catch (err) {
      setServerFormError('加载证书列表失败：' + (err.response?.data?.error || err.message));
      setCertSelect((prev) => ({ ...prev, open: false, loading: false }));
    }
  };

  const ensureCertRefInfo = async (id) => {
    if (!id) return;
    try {
      const res = await certificateAPI.getInfo(id);
      setCertRefInfo((prev) => ({ ...prev, [id]: res.data.data }));
    } catch (err) {
      // 引用可能已被删除，忽略
    }
  };

  const applyCertSelect = (cert) => {
    setCertRefInfo((prev) => ({ ...prev, [cert.id]: cert }));
    if (certSelect.target === 'ca') {
      handleFormChange('ca', `${CERT_REF_PREFIX}${cert.id}/cert`);
      setCertFieldMode((prev) => ({ ...prev, ca: 'display' }));
    } else if (certSelect.target === 'cert') {
      // 证书与私钥一起选中
      handleFormChange('cert', `${CERT_REF_PREFIX}${cert.id}/cert`);
      handleFormChange('key', `${CERT_REF_PREFIX}${cert.id}/key`);
      setCertFieldMode((prev) => ({ ...prev, cert: 'display', key: 'display' }));
    }
    setCertSelect((prev) => ({ ...prev, open: false }));
  };

  // 切换证书字段的 手动填写/收起。从证书管理引用切到手动填写时清空输入框，
  // 避免把 cert-stor:<id>/... 这样的内部引用显示在输入框里。
  const toggleCertFieldMode = (field) => {
    const enteringManual = certFieldMode[field] !== 'manual';
    if (enteringManual) {
      if (parseCertRef(formData[field])) {
        handleFormChange(field, '');
      }
      // 服务器证书与私钥是成对选择的，改用手动填写时一并清空私钥引用
      if (field === 'cert' && parseCertRef(formData.key)) {
        handleFormChange('key', '');
      }
    }
    setCertFieldMode((prev) => ({ ...prev, [field]: enteringManual ? 'manual' : 'display' }));
  };

  const handleGenerateDH = async () => {
    try {
      setDhDialog((prev) => ({ ...prev, generating: true }));
      const res = await certificateAPI.generateDH(dhDialog.bits);
      handleFormChange('dh', res.data.data.dh);
      setDhDialog({ open: false, bits: dhDialog.bits, generating: false });
      setSuccess(`DH 参数（${dhDialog.bits} 位）已生成`);
    } catch (err) {
      setServerFormError('生成 DH 参数失败：' + (err.response?.data?.error || err.message));
      setDhDialog((prev) => ({ ...prev, generating: false }));
    }
  };

  const handleGenerateTLSAuth = async () => {
    if (formData.tls_auth && !window.confirm('将覆盖当前 TLS-Auth 密钥，确认生成？')) {
      return;
    }
    try {
      setTlsGenerating(true);
      const res = await certificateAPI.generateTLSAuth();
      handleFormChange('tls_auth', res.data.data.tls_auth);
      setSuccess('ta.key 已生成');
    } catch (err) {
      setServerFormError('生成 ta.key 失败：' + (err.response?.data?.error || err.message));
    } finally {
      setTlsGenerating(false);
    }
  };

  const openExportDialog = async () => {
    setExportDialog((prev) => ({
      ...prev,
      open: true,
      loading: true,
      error: '',
      servers: [],
      clientCerts: [],
      serverId: '',
      certId: '',
      mode: 'select',
      caId: null,
      manualCert: '',
      manualKey: '',
      extraConfig: '',
    }));
    try {
      const serverRes = await serverAPI.getServerList();
      setExportDialog((prev) => ({
        ...prev,
        open: true,
        servers: serverRes.data.data || [],
        loading: false,
      }));
    } catch (err) {
      setExportDialog((prev) => ({
        ...prev,
        loading: false,
        error: '加载数据失败：' + (err.response?.data?.error || err.message),
      }));
    }
  };

  // 选择服务器后决定导出模式：
  // - 服务器 CA 为证书管理引用：仅列出由该 CA 签发的客户端证书
  // - 服务器 CA 为手动填写：改为手动输入客户端证书/私钥
  const handleExportServerChange = async (serverId) => {
    if (!serverId) {
      setExportDialog((prev) => ({ ...prev, serverId: '', certId: '', mode: 'select', caId: null, clientCerts: [] }));
      return;
    }
    setExportDialog((prev) => ({ ...prev, serverId, certId: '', clientCerts: [], loading: true, error: '' }));
    try {
      const res = await serverAPI.getServerInfo(serverId);
      const data = res.data.data || {};
      const ref = parseCertRef(data.ca);
      if (ref) {
        const certRes = await certificateAPI.list({ types: [3, 4], parentId: ref.id });
        setExportDialog((prev) => ({
          ...prev,
          serverId,
          host: data.export_host || prev.host,
          port: data.export_port || prev.port,
          mode: 'select',
          caId: ref.id,
          clientCerts: certRes.data.data || [],
          extraConfig: data.export_extra_config || '',
          loading: false,
        }));
      } else {
        setExportDialog((prev) => ({
          ...prev,
          serverId,
          host: data.export_host || prev.host,
          port: data.export_port || prev.port,
          mode: 'manual',
          caId: null,
          clientCerts: [],
          extraConfig: data.export_extra_config || '',
          loading: false,
        }));
      }
    } catch (err) {
      setExportDialog((prev) => ({
        ...prev,
        loading: false,
        error: '加载服务器信息失败：' + (err.response?.data?.error || err.message),
      }));
    }
  };

  const handleExportClientConfig = async () => {
    try {
      setExportDialog((prev) => ({ ...prev, loading: true, error: '' }));
      const payload = {
        server_id: Number(exportDialog.serverId),
        host: exportDialog.host,
        port: Number(exportDialog.port),
        extra_config: exportDialog.extraConfig,
      };
      if (exportDialog.mode === 'select') {
        payload.cert_id = Number(exportDialog.certId);
      } else {
        payload.client_cert = exportDialog.manualCert;
        payload.client_key = exportDialog.manualKey;
      }
      const res = await serverAPI.exportClientConfig(payload);
      const blob = new Blob([res.data], { type: 'application/x-openvpn-profile' });
      const fileName = filenameFromDisposition(res.headers?.['content-disposition'], 'client.ovpn');
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = fileName;
      document.body.appendChild(a);
      a.click();
      a.remove();
      window.URL.revokeObjectURL(url);
      setExportDialog((prev) => ({ ...prev, loading: false, open: false }));
      setSuccess('客户端配置已导出');
    } catch (err) {
      setExportDialog((prev) => ({ ...prev, loading: false, error: '导出失败：' + (err.response?.data?.error || err.message) }));
    }
  };

  // 权限管理函数
  const handleOpenPermissionDialog = async (server) => {
    setSelectedServer(server);
    setPermissionDialog(true);
    setPermissionError('');
    setPermissionSuccess('');
    // setPermissionTabValue(0); // 不要重置到第一个标签页
    await loadPermissions(server.id);
    await loadAllUsersAndGroups();
  };

  const loadPermissions = async (serverId) => {
    try {
      setIsLoading(true);
      const response = await permissionAPI.getPermissions(serverId);
      if (response.data.result === 'success') {
        setPermissions(response.data.data || { GroupPermission: [], UserPermission: [] });
        setSelectedPermissions([]);
      }
    } catch (err) {
      setPermissionError('加载权限失败');
    } finally {
      setIsLoading(false);
    }
  };

  const loadAllUsersAndGroups = async () => {
    try {
      // 先获取第一页，获取总数
      const [usersFirstRes, groupsFirstRes] = await Promise.all([
        userManageAPI.getUserList(1, 1),
        groupAPI.getGroupList(1, 1),
      ]);
      
      let usersTotalCount = 0;
      let groupsTotalCount = 0;
      
      if (usersFirstRes.data.result === 'success') {
        usersTotalCount = usersFirstRes.data.count || 0;
      }
      if (groupsFirstRes.data.result === 'success') {
        groupsTotalCount = groupsFirstRes.data.count || 0;
      }
      
      setAvailableUsersCount(usersTotalCount);
      setAvailableGroupsCount(groupsTotalCount);
    } catch (err) {
      console.error('加载用户和用户组失败', err);
    }
  };

  // 修改加载可用用户的函数，支持搜索查询
  const loadAvailableUsers = async (pageNum = 1, pageSize = 10, query = '') => {
    try {
      setAvailableUsersLoading(true);
      const usedUserIds = selectedPermissions
        .filter(p => p.type === 'user')
        .map(p => parseInt(p.user_id));
      
      // 加载指定页的数据
      const response = await userManageAPI.getUserList(pageNum, pageSize, query);
      if (response.data.result === 'success') {
        // 过滤掉已被分配权限的用户 似乎不会过滤 无所谓了
        const filteredUsers = (response.data.data || []).filter(u => !usedUserIds.includes(u.ID));
        setAllUsers(filteredUsers);
        
        // 每次加载第一页时，更新总数
        if (pageNum === 1) {
          const totalCount = response.data.count || 0;
          const availableCount = Math.max(0, totalCount - usedUserIds.length);
          setAvailableUsersCount(availableCount);
        }
      }
    } catch (err) {
      console.error('加载可用用户列表失败', err);
    } finally {
      setAvailableUsersLoading(false);
    }
  };

  // 修改加载可用用户组的函数，支持搜索查询
  const loadAvailableGroups = async (pageNum = 1, pageSize = 10, query = '') => {
    try {
      setAvailableGroupsLoading(true);
      const usedGroupIds = selectedPermissions
        .filter(p => p.type === 'group')
        .map(p => parseInt(p.group_id));
      
      // 加载指定页的数据
      const response = await groupAPI.getGroupList(pageNum, pageSize, query);
      if (response.data.result === 'success') {
        // 过滤掉已被分配权限的用户组  似乎不会过滤 无所谓了
        const filteredGroups = (response.data.data || []).filter(g => !usedGroupIds.includes(g.ID));
        setAllGroups(filteredGroups);
        
        // 每次加载第一页时，更新总数
        if (pageNum === 1) {
          const totalCount = response.data.count || 0;
          const availableCount = Math.max(0, totalCount - usedGroupIds.length);
          setAvailableGroupsCount(availableCount);
        }
      }
    } catch (err) {
      console.error('加载可用用户组列表失败', err);
    } finally {
      setAvailableGroupsLoading(false);
    }
  };

  const handleAddUserPermission = async (actionValue = 1) => {
    if (selectedUsers.length === 0) {
      setAddPermissionError('请至少选择一个用户');
      return;
    }
    try {
      setIsLoading(true);
      for (const userId of selectedUsers) {
        await permissionAPI.addPermission({
          server_id: selectedServer.id,
          obj_type: 1,
          obj_id: userId,
          action: actionValue,
        });
      }
      setPermissionSuccess(actionValue === 1 ? '用户权限添加成功' : '用户权限拒绝添加成功');
      setSelectedUsers([]);
      setAddUserDialog(false);
      await loadPermissions(selectedServer.id);
    } catch (err) {
      setAddPermissionError(err.response?.data?.error || '添加权限失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddGroupPermission = async (actionValue = 1) => {
    if (selectedGroups.length === 0) {
      setAddPermissionError('请至少选择一个用户组');
      return;
    }
    try {
      setIsLoading(true);
      for (const groupId of selectedGroups) {
        await permissionAPI.addPermission({
          server_id: selectedServer.id,
          obj_type: 2,
          obj_id: groupId,
          action: actionValue,
        });
      }
      setPermissionSuccess(actionValue === 1 ? '用户组权限添加成功' : '用户组权限拒绝添加成功');
      setSelectedGroups([]);
      setAddGroupDialog(false);
      await loadPermissions(selectedServer.id);
    } catch (err) {
      setAddPermissionError(err.response?.data?.error || '添加权限失败');
    } finally {
      setIsLoading(false);
    }
  };

  // 处理删除权限的函数
  const handleDeletePermissions = async () => {
    if ((selectedUserPermissions.length === 0 && selectedGroupPermissions.length === 0) || 
        (permissionTabValue === 0 && selectedUserPermissions.length === 0) ||
        (permissionTabValue === 1 && selectedGroupPermissions.length === 0)) {
      setPermissionError('请至少选择一个权限');
      return;
    }
    
    try {
      setIsLoading(true);
      const idsToDelete = permissionTabValue === 0 ? selectedUserPermissions : selectedGroupPermissions;
      
      await permissionAPI.deletePermissions({
        id_list: idsToDelete,
      });
      
      setPermissionSuccess(permissionTabValue === 0 ? '用户权限删除成功' : '用户组权限删除成功');
      
      // 重置对应标签页的选择状态
      if (permissionTabValue === 0) {
        setSelectedUserPermissions([]);
      } else {
        setSelectedGroupPermissions([]);
      }
      
      await loadPermissions(selectedServer.id);
    } catch (err) {
      setPermissionError(err.response?.data?.error || '删除权限失败');
    } finally {
      setIsLoading(false);
    }
  };

  const getUsedUserIds = () => {
    return (permissions.UserPermission || []).map(p => p.ObjID);
  };

  const getUsedGroupIds = () => {
    return (permissions.GroupPermission || []).map(p => p.ObjID);
  };

  // 客户端配置管理函数
  const handleOpenClientConfigDialog = async (serverId) => {
    try {
      setIsLoading(true);
      const response = await serverAPI.getClientConfigList(serverId);
      if (response.data.result === 'success') {
        setSelectedServerConfigs(response.data.data || []);
        setSelectedConfigs([]);
        setClientConfigDialog(true);
        // 保存当前服务器ID，用于后续操作
        setSelectedServer({ id: serverId });
        setClientCertCNs([]);
        // 若该服务器的证书来自证书库，加载同 CA 已签发的客户端证书 CN 供选择
        try {
          const infoRes = await serverAPI.getServerInfo(serverId);
          const info = infoRes.data?.data;
          if (info && parseCertRef(info.cert)) {
            const caRef = parseCertRef(info.ca);
            const params = { types: [3, 4], pageSize: 0 };
            if (caRef) params.parentId = caRef.id;
            const certRes = await certificateAPI.list(params);
            const cns = (certRes.data.data || [])
              .map((c) => subjectCommonName(c.subject))
              .filter(Boolean);
            setClientCertCNs([...new Set(cns)]);
          }
        } catch (certErr) {
          // 加载失败时退化为普通输入框，不阻断配置管理
          setClientCertCNs([]);
        }
      }
    } catch (err) {
      setError('加载客户端配置失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleDeleteConfigs = async () => {
    if (selectedConfigs.length === 0) {
      setClientConfigError('请至少选择一个配置'); // 使用独立的错误状态
      return;
    }
    
    if (!window.confirm(`确定要删除 ${selectedConfigs.length} 个客户端配置吗？`)) {
      return;
    }
    
    try {
      setIsLoading(true);
      await serverAPI.deleteClientConfigs({ id_list: selectedConfigs });
      setClientConfigSuccess('客户端配置删除成功'); // 使用独立的成功状态
      setSelectedConfigs([]);
      await loadClientConfigs(selectedServer.id);
    } catch (err) {
      setClientConfigError(err.response?.data?.error || '删除配置失败'); // 使用独立的错误状态
    } finally {
      setIsLoading(false);
    }
  };

  // 修改loadClientConfigs函数，增加参数验证
  const loadClientConfigs = async (serverId) => {
    if (!serverId) {
      setClientConfigError('服务器ID不能为空');
      return;
    }
    
    try {
      setIsLoading(true);
      const response = await serverAPI.getClientConfigList(serverId);
      if (response.data.result === 'success') {
        setSelectedServerConfigs(response.data.data || []);
        setSelectedConfigs([]);
      }
    } catch (err) {
      setClientConfigError('加载客户端配置失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleAddConfig = async () => {
    if (!configFormData.client_cert_name || !configFormData.config) {
      setConfigFormError('客户端证书名称和配置内容不能为空');
      return;
    }
    
    try {
      setIsLoading(true);
      await serverAPI.addClientConfig({
        server_id: selectedServer.id,
        ...configFormData,
      });
      setClientConfigSuccess('客户端配置添加成功'); // 使用独立的成功状态
      setConfigFormData({ client_cert_name: '', config: '' });
      setAddConfigDialog(false);
      await loadClientConfigs(selectedServer.id);
    } catch (err) {
      setConfigFormError(err.response?.data?.error || '添加配置失败');
    } finally {
      setIsLoading(false);
    }
  };

  const handleEditConfig = (config) => {
    setConfigFormData({
      client_cert_name: config.client_cert_name,
      config: config.config,
    });
    setConfigFormError('');
    setAddConfigDialog(true);
  };

  const truncateConfig = (config, maxLength = 50) => {
    if (!config) return '';
    return config.length > maxLength ? `${config.substring(0, maxLength)}...` : config;
  };

  const truncateText = (text, maxLength = 30) => {
    if (!text) return '';
    return text.length > maxLength ? `${text.substring(0, maxLength)}...` : text;
  };

  // 移动端卡片视图
  const MobileServerCard = ({ server }) => (
    <Card sx={{ marginBottom: 2 }}>
      <CardContent>
        <Typography variant="h6" sx={{ marginBottom: 1 }}>
          {server.name}
        </Typography>
        <Stack spacing={1}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              端口:
            </Typography>
            <Typography variant="body2">{server.port}</Typography>
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              协议:
            </Typography>
            <Typography variant="body2">{server.proto}</Typography>
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              网卡:
            </Typography>
            <Typography variant="body2">{server.dev}</Typography>
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              网段:
            </Typography>
            <Typography variant="body2" sx={{ textAlign: 'right' }}>
              {server.server_cidr}
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              自动启动:
            </Typography>
            <Chip
              label={server.auto_start ? '是' : '否'}
              size="small"
              color={server.auto_start ? 'success' : 'default'}
            />
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              状态:
            </Typography>
            <Chip
              label={server.running ? '运行中' : '已停止'}
              size="small"
              color={server.running ? 'success' : 'error'}
            />
          </Box>
        </Stack>
      </CardContent>
      <CardActions sx={{ flexWrap: 'wrap', gap: 1 }}>
        {!server.running ? (
          <Button
            size="small"
            startIcon={<PlayArrowIcon />}
            onClick={() => handleStartServer(server.id)}
            variant="contained"
            color="success"
            disabled={actionLoading.has(server.id)}
          >
            {actionLoading.has(server.id) ? '启动中...' : '启动'}
          </Button>
        ) : (
          <Button
            size="small"
            startIcon={<StopIcon />}
            onClick={() => handleStopServer(server.id)}
            variant="contained"
            color="error"
            disabled={actionLoading.has(server.id)}
          >
            {actionLoading.has(server.id) ? '停止中...' : '停止'}
          </Button>
        )}
        <Button
          size="small"
          startIcon={<EditIcon />}
          onClick={() => handleEditServer(server.id)}
          variant="outlined"
        >
          编辑配置
        </Button>
        <Button
          size="small"
          startIcon={<SecurityIcon />}
          onClick={() => handleOpenPermissionDialog(server)}
          variant="outlined"
          color="info"
        >
          权限管理
        </Button>
        <Button
          size="small"
          startIcon={<SecurityIcon />}
          onClick={() => handleOpenClientConfigDialog(server.id)}
          variant="outlined"
          color="secondary"
        >
          客户端配置
        </Button>
        <Button
          size="small"
          startIcon={<DeleteIcon />}
          onClick={() => handleDeleteServer(server.id)}
          variant="outlined"
          color="error"
        >
          删除
        </Button>
      </CardActions>
    </Card>
  );

  if (isLoading && servers.length === 0) {
    return (
      <Box sx={{ width: '100%', padding: 3, display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Box sx={{ marginBottom: 3 }}>
        <Typography variant="h5" sx={{ marginBottom: 2 }}>
          服务器管理
        </Typography>
        {error && <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError('')}>{error}</Alert>}
        {success && <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setSuccess('')}>{success}</Alert>}
        <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
          <Button
            variant="contained"
            color="primary"
            startIcon={<AddIcon />}
            onClick={handleAddServer}
            fullWidth={isMobile}
          >
            创建服务器
          </Button>
          <Button
            variant="outlined"
            startIcon={<RefreshIcon />}
            onClick={loadServers}
            disabled={isLoading}
            fullWidth={isMobile}
          >
            刷新
          </Button>
          <Button
            variant="outlined"
            onClick={openExportDialog}
            fullWidth={isMobile}
          >
            导出客户端配置
          </Button>
        </Stack>
      </Box>

      {/* 移动端显示卡片 */}
      {isMobile ? (
        <Box>
          {servers.map((server) => (
            <MobileServerCard key={server.id} server={server} />
          ))}
        </Box>
      ) : (
        /* 桌面端显示表格 */
        <TableContainer component={Paper}>
          <Table>
            <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
              <TableRow>
                <TableCell>服务器名称</TableCell>
                <TableCell align="right">端口</TableCell>
                <TableCell>协议</TableCell>
                <TableCell>网卡</TableCell>
                <TableCell>网段</TableCell>
                <TableCell>自动启动</TableCell>
                <TableCell>运行状态</TableCell>
                <TableCell align="right">操作</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {servers.map((server) => (
                <TableRow key={server.id}>
                  <TableCell>{server.name}</TableCell>
                  <TableCell align="right">{server.port}</TableCell>
                  <TableCell>{server.proto}</TableCell>
                  <TableCell>{server.dev}</TableCell>
                  <TableCell>{server.server_cidr}</TableCell>
                  <TableCell>
                    <Chip
                      label={server.auto_start ? '是' : '否'}
                      size="small"
                      color={server.auto_start ? 'success' : 'default'}
                    />
                  </TableCell>
                  <TableCell>
                    <Chip
                      label={server.running ? '运行中' : '已停止'}
                      size="small"
                      color={server.running ? 'success' : 'error'}
                    />
                  </TableCell>
                  <TableCell align="right">
                    <Stack direction="row" spacing={1}>
                      {!server.running ? (
                        <Button
                          size="small"
                          startIcon={<PlayArrowIcon />}
                          onClick={() => handleStartServer(server.id)}
                          variant="outlined"
                          disabled={actionLoading.has(server.id)}
                        >
                          {actionLoading.has(server.id) ? '启动中...' : '启动'}
                        </Button>
                      ) : (
                        <Button
                          size="small"
                          startIcon={<StopIcon />}
                          onClick={() => handleStopServer(server.id)}
                          variant="outlined"
                          color="error"
                          disabled={actionLoading.has(server.id)}
                        >
                          {actionLoading.has(server.id) ? '停止中...' : '停止'}
                        </Button>
                      )}
                      <Button
                        size="small"
                        startIcon={<EditIcon />}
                        onClick={(e) => {
                          setDropdownAnchor(e.currentTarget);
                          setDropdownServerId(server.id);
                        }}
                        variant="outlined"
                      >
                        编辑
                      </Button>
                      <Menu
                        anchorEl={dropdownAnchor}
                        open={Boolean(dropdownAnchor) && dropdownServerId === server.id}
                        onClose={() => {
                          setDropdownAnchor(null);
                          setDropdownServerId(null);
                        }}
                        anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
                        transformOrigin={{ vertical: 'top', horizontal: 'right' }}
                      >
                        <MenuItem
                          onClick={() => {
                            handleEditServer(server.id);
                            setDropdownAnchor(null);
                            setDropdownServerId(null);
                          }}
                        >
                          编辑配置
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            handleOpenPermissionDialog(server);
                            setDropdownAnchor(null);
                            setDropdownServerId(null);
                          }}
                        >
                          权限管理
                        </MenuItem>
                        <MenuItem
                          onClick={() => {
                            handleOpenClientConfigDialog(server.id);
                            setDropdownAnchor(null);
                            setDropdownServerId(null);
                          }}
                        >
                          客户端配置
                        </MenuItem>
                      </Menu>
                      <Button
                        size="small"
                        startIcon={<DeleteIcon />}
                        onClick={() => handleDeleteServer(server.id)}
                        variant="outlined"
                        color="error"
                      >
                        删除
                      </Button>
                    </Stack>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      {/* 服务器编辑对话框 */}
      <Dialog 
        open={openDialog} 
        onClose={() => {
          setOpenDialog(false);
          setServerFormError('');
        }} 
        fullWidth
        maxWidth={isMobile ? 'lg' : 'md'}
       fullScreen={isMobile}>
        <DialogTitle>
          {editingId ? '编辑服务器' : '创建新服务器'}
        </DialogTitle>
        <DialogContent sx={{ paddingTop: 2, maxHeight: '70vh', overflow: 'auto' }}>
          {serverFormError && <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setServerFormError('')}>{serverFormError}</Alert>}
          <TextField
            fullWidth
            label="服务器名称"
            value={formData.name}
            onChange={(e) => handleFormChange('name', e.target.value)}
            margin="normal"
            required
          />
          <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
            <TextField
              label="监听端口"
              type="number"
              value={formData.port}
              onChange={(e) => handleFormChange('port', parseInt(e.target.value))}
              margin="normal"
              sx={{ width: '100%', '@media (min-width:926px)': { width: '49%' } }}
            />

            <TextField
              label="网卡名称"
              value={formData.dev}
              onChange={(e) => handleFormChange('dev', e.target.value)}
              margin="normal"
              sx={{ width: '100%', '@media (min-width:926px)': { width: '49%' } }}
            />

            <FormControl margin="normal" sx={{ width: '100%', '@media (min-width:926px)': { width: '49%' } }}>
              <InputLabel>协议</InputLabel>
              <Select
                value={formData.proto}
                onChange={(e) => handleFormChange('proto', e.target.value)}
                label="协议"
              >
                <MenuItem value="tcp">tcp</MenuItem>
                <MenuItem value="udp">udp</MenuItem>
                <MenuItem value="tcp4">tcp4</MenuItem>
                <MenuItem value="udp4">udp4</MenuItem>
                <MenuItem value="tcp6">tcp6</MenuItem>
                <MenuItem value="udp6">udp6</MenuItem>
              </Select>
            </FormControl>

            <FormControl margin="normal" sx={{ width: '100%', '@media (min-width:926px)': { width: '49%' } }}>
              <InputLabel>拓扑</InputLabel>
              <Select
                value={formData.topology}
                onChange={(e) => handleFormChange('topology', e.target.value)}
                label="拓扑"
              >
                <MenuItem value="subnet">subnet</MenuItem>
                <MenuItem value="net30">net30</MenuItem>
              </Select>
            </FormControl>
            <TextField
            fullWidth
            label="服务器网段 (网络地址 掩码)"
            value={formData.server_cidr}
            onChange={(e) => handleFormChange('server_cidr', e.target.value)}
            margin="normal"
            sx={{  width: '100%', '@media (min-width:926px)': { width: '49%' } }}
          />
          
          <TextField
            fullWidth
            label="心跳配置 (间隔 阈值)"
            value={formData.keepalive}
            onChange={(e) => handleFormChange('keepalive', e.target.value)}
            margin="normal"
            sx={{ width: '100%', '@media (min-width:926px)': { width: '49%' } }}
          />
          </Box>

          <Autocomplete
            multiple
            freeSolo
            options={DATA_CIPHER_OPTIONS}
            value={
              formData.data_cipher
                ? String(formData.data_cipher).split(':').filter(Boolean)
                : []
            }
            onChange={(e, newValue) => handleFormChange('data_cipher', newValue.join(':'))}
            renderInput={(params) => (
              <TextField
                {...params}
                fullWidth
                margin="normal"
                label="数据加密算法 (data-ciphers)"
                helperText="按推荐顺序从高到低多选，也可手动输入；提交格式以 : 分隔，例如 AES-256-GCM:AES-128-GCM"
              />
            )}
          />
          
          <FormControlLabel
            control={
              <Checkbox
                checked={formData.auto_start}
                onChange={(e) => handleFormChange('auto_start', e.target.checked)}
              />
            }
            label="自动启动"
            sx={{ marginTop: 1, marginRight: 8 }}
          />
          <FormControlLabel
            control={
              <Checkbox
                checked={formData.duplicate_cn}
                onChange={(e) => handleFormChange('duplicate_cn', e.target.checked)}
              />
            }
            label="同证书多次登录"
            sx={{ marginTop: 1 }}
          />
          <Box sx={{ marginTop: 2 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ marginBottom: 1, flexWrap: 'wrap' }}>
              <Typography variant="subtitle2">CA 证书</Typography>
              <Button size="small" variant="outlined" onClick={() => openCertSelect('ca', 1)}>从证书管理选择</Button>
              <Button size="small" onClick={() => toggleCertFieldMode('ca')}>
                {certFieldMode.ca === 'manual' ? '收起' : '手动填写'}
              </Button>
            </Stack>
            {certFieldMode.ca === 'manual' ? (
              <TextField
                fullWidth
                value={formData.ca}
                onChange={(e) => handleFormChange('ca', e.target.value)}
                multiline
                rows={6}
                placeholder="-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----"
              />
            ) : (
              <CertSummary value={formData.ca} info={certRefInfo[parseCertRef(formData.ca)?.id]} manual={certManual('ca')} />
            )}
          </Box>
          <Box sx={{ marginTop: 2 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ marginBottom: 1, flexWrap: 'wrap' }}>
              <Typography variant="subtitle2">服务器证书</Typography>
              <Button size="small" variant="outlined" onClick={() => openCertSelect('cert', 1)}>从证书管理选择</Button>
              <Button size="small" onClick={() => toggleCertFieldMode('cert')}>
                {certFieldMode.cert === 'manual' ? '收起' : '手动填写'}
              </Button>
            </Stack>
            {certFieldMode.cert === 'manual' ? (
              <TextField
                fullWidth
                value={formData.cert}
                onChange={(e) => handleFormChange('cert', e.target.value)}
                multiline
                rows={6}
                placeholder="-----BEGIN CERTIFICATE-----
...
-----END CERTIFICATE-----"
              />
            ) : (
              <CertSummary value={formData.cert} info={certRefInfo[parseCertRef(formData.cert)?.id]} manual={certManual('cert')} />
            )}
          </Box>
          <Box sx={{ marginTop: 2 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ marginBottom: 1, flexWrap: 'wrap' }}>
              <Typography variant="subtitle2">服务器证书私钥</Typography>
              <Button size="small" onClick={() => toggleCertFieldMode('key')}>
                {certFieldMode.key === 'manual' ? '收起' : '手动填写'}
              </Button>
            </Stack>
            {certFieldMode.key === 'manual' ? (
              <TextField
                fullWidth
                value={formData.key}
                onChange={(e) => handleFormChange('key', e.target.value)}
                multiline
                rows={6}
                placeholder="-----BEGIN PRIVATE KEY-----
...
-----END PRIVATE KEY-----"
              />
            ) : (
              <CertSummary value={formData.key} info={certRefInfo[parseCertRef(formData.key)?.id]} kind="key" />
            )}
          </Box>
          <Box sx={{ marginTop: 2 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ marginBottom: 1 }}>
              <Typography variant="subtitle2">DH 参数</Typography>
              <Button size="small" variant="outlined" onClick={() => setDhDialog({ open: true, bits: 2048, generating: false })}>
                一键生成
              </Button>
            </Stack>
            <TextField
              fullWidth
              value={formData.dh}
              onChange={(e) => handleFormChange('dh', e.target.value)}
              multiline
              rows={6}
              placeholder="-----BEGIN DH PARAMETERS-----
...
-----END DH PARAMETERS-----"
            />
          </Box>
          <Box sx={{ marginTop: 2 }}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ marginBottom: 1 }}>
              <Typography variant="subtitle2">TLS-Auth 密钥</Typography>
              <Button size="small" variant="outlined" disabled={tlsGenerating} onClick={handleGenerateTLSAuth}>
                {tlsGenerating ? '生成中...' : '用 openvpn 生成 ta.key'}
              </Button>
            </Stack>
            <TextField
              fullWidth
              value={formData.tls_auth}
              onChange={(e) => handleFormChange('tls_auth', e.target.value)}
              multiline
              rows={6}
            />
          </Box>
          <TextField
            fullWidth
            label="其他配置"
            value={formData.other_config}
            onChange={(e) => handleFormChange('other_config', e.target.value)}
            margin="normal"
            multiline
            rows={5}
            helperText='可添加 remote-cert-tls client 限制客户端证书类型（要求客户端证书含 clientAuth 用途）。'
            placeholder='mssfix 1308
push "route 10.0.2.0 255.255.255.0"
push "dhcp-option DNS [IP 網址]"
push "redirect-gateway def1"'
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setOpenDialog(false);
            setServerFormError('');
          }}>取消</Button>
          <Button onClick={handleSaveServer} variant="contained" color="primary">
            保存
          </Button>
        </DialogActions>
      </Dialog>

      {/* 权限管理对话框 */}
      <Dialog 
        open={permissionDialog} 
        onClose={() => setPermissionDialog(false)} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
        
       fullScreen={isMobile}>
        <DialogTitle sx={{paddingBottom: 0}}>权限管理 - {selectedServer?.name}</DialogTitle>
        <DialogContent sx={{ paddingTop: 1 }}>
          {permissionError && <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setPermissionError('')}>{permissionError}</Alert>}
          {permissionSuccess && <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setPermissionSuccess('')}>{permissionSuccess}</Alert>}
          {/* 添加标签页 */}
          <Tabs 
            value={permissionTabValue} 
            onChange={(e, newValue) => setPermissionTabValue(newValue)}
            sx={{ marginBottom: 1 }}
          >
            <Tab label="用户权限" />
            <Tab label="用户组权限" />
          </Tabs>

          {/* 用户权限标签页 */}
          {permissionTabValue === 0 && (
            <React.Fragment>
              <Button
                  variant="contained"
                  color="success"
                  startIcon={<AddIcon />}
                  onClick={async () => {
                    setAddPermissionError('');
                    setAddUserDialog(true);
                    setAddUserPermPage(1);
                    setAvailableUsersCount(0);
                    await loadAvailableUsers(1, addUserPermPageSize);
                  }}
                >
                  添加用户权限
                </Button>
                {selectedUserPermissions.length > 0 && (
                  <Button
                    variant="contained"
                    color="error"
                    startIcon={<DeleteIcon />}
                    onClick={handleDeletePermissions}
                    sx={{marginLeft: 2}}
                  >
                    删除选中 ({selectedUserPermissions.length})
                  </Button>
                )}
              
              
              <Stack direction="row" spacing={2} sx={{ marginTop: 1 }}>
                <Typography variant="h6" sx={{ marginBottom: 0, marginTop: 0 }}>
                用户权限列表
              </Typography>
                
              </Stack>
              <TableContainer component={Paper} sx={{ marginBottom: 3 }}>
                <Table size="small">
                  <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 'bold' }}>选择</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户ID</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户名</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>权限</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {(permissions.UserPermission || []).length > 0 ? (
                      (permissions.UserPermission || []).map((perm) => (
                        <TableRow key={perm.PermissionID} hover sx={{ padding: 0 }}>
                          <TableCell  sx={{padding: 0}}>
                            <Checkbox
                              checked={selectedUserPermissions.includes(perm.PermissionID)}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setSelectedUserPermissions([...selectedUserPermissions, perm.PermissionID]);
                                } else {
                                  setSelectedUserPermissions(selectedUserPermissions.filter(id => id !== perm.PermissionID));
                                }
                              }}
                            />
                          </TableCell>
                          <TableCell  sx={{padding: 0}}>{perm.ObjID}</TableCell>
                          <TableCell  sx={{padding: 0}}>{perm.ObjName}</TableCell>
                          <TableCell  sx={{padding: 0}}>
                            <Chip
                              label={perm.Action === 1 ? '允许' : '禁止'}
                              size="small"
                              color={perm.Action === 1 ? 'success' : 'error'}
                            />
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={4} align="center">
                          暂无用户权限
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            </React.Fragment>
          )}

          {/* 用户组权限标签页 */}
          {permissionTabValue === 1 && (
            <React.Fragment>
              
              <Button
                  variant="contained"
                  color="success"
                  startIcon={<AddIcon />}
                  onClick={async () => {
                    setAddPermissionError('');
                    setAddGroupDialog(true);
                    setAddGroupPermPage(1);
                    setAvailableGroupsCount(0);
                    await loadAvailableGroups(1, addGroupPermPageSize);
                  }}
                >
                  添加用户组权限
                </Button>
                {selectedGroupPermissions.length > 0 && (
                  <Button
                    variant="contained"
                    color="error"
                    startIcon={<DeleteIcon />}
                    onClick={handleDeletePermissions}
                    sx={{marginLeft: 1}}
                  >
                    删除选中 ({selectedGroupPermissions.length})
                  </Button>
                )}
                
              <Stack direction="row" spacing={2} sx={{ marginTop: 1 }}>
                <Typography variant="h6" sx={{ marginBottom: 0 }}>
                用户组权限列表
              </Typography>
              </Stack>
              <TableContainer component={Paper}>
                <Table size="small">
                  <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                    <TableRow>
                      <TableCell sx={{ fontWeight: 'bold' }}>选择</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户组ID</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>用户组名</TableCell>
                      <TableCell sx={{ fontWeight: 'bold' }}>权限</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {(permissions.GroupPermission || []).length > 0 ? (
                      (permissions.GroupPermission || []).map((perm) => (
                        <TableRow key={perm.PermissionID} hover>
                          <TableCell sx={{padding: 0}}>
                            <Checkbox
                              checked={selectedGroupPermissions.includes(perm.PermissionID)}
                              onChange={(e) => {
                                if (e.target.checked) {
                                  setSelectedGroupPermissions([...selectedGroupPermissions, perm.PermissionID]);
                                } else {
                                  setSelectedGroupPermissions(selectedGroupPermissions.filter(id => id !== perm.PermissionID));
                                }
                              }}
                            />
                          </TableCell>
                          <TableCell sx={{padding: 0}}>{perm.ObjID}</TableCell>
                          <TableCell sx={{padding: 0}}>{perm.ObjName}</TableCell>
                          <TableCell sx={{padding: 0}}>
                            <Chip
                              label={perm.Action === 1 ? '允许' : '禁止'}
                              size="small"
                              color={perm.Action === 1 ? 'success' : 'error'}
                            />
                          </TableCell>
                        </TableRow>
                      ))
                    ) : (
                      <TableRow>
                        <TableCell colSpan={4} align="center">
                          暂无用户组权限
                        </TableCell>
                      </TableRow>
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
            </React.Fragment>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setPermissionDialog(false);
            setPermissionError('');
            setPermissionSuccess('');
          }}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 添加用户权限对话框 */}
      <Dialog 
        open={addUserDialog} 
        onClose={() => {
          setAddUserDialog(false);
          setSelectedUsers([]);
          setAddUserPermPage(1);
          setAvailableUsersCount(0);
          setAddUserSearchQuery(''); // 重置搜索查询
          setAddPermissionError('');
        }} 
        
        fullWidth
        maxWidth={isMobile ? 'lg' : 'md'}
       fullScreen={isMobile}>
        <DialogTitle>添加用户权限</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {addPermissionError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setAddPermissionError('')}>
              {addPermissionError}
            </Alert>
          )}
          <Typography variant="body2" sx={{ marginBottom: 2, color: '#666' }}>
            选择要添加权限的用户
          </Typography>
          
          <Stack direction="row" spacing={1} sx={{ marginBottom: 2 }}>
            <TextField
              placeholder="搜索用户名"
              size="small"
              value={addUserSearchQuery}
              onChange={(e) => setAddUserSearchQuery(e.target.value)}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  setAddUserPermPage(1);
                  loadAvailableUsers(1, addUserPermPageSize, addUserSearchQuery);
                }
              }}
              sx={{ minWidth: 200, flex: 1 }}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={() => {
                setAddUserPermPage(1);
                loadAvailableUsers(1, addUserPermPageSize, addUserSearchQuery);
              }}
              disabled={availableUsersLoading}
            >
              搜索
            </Button>
          </Stack>
          
          {availableUsersLoading && <CircularProgress sx={{ marginBottom: 2 }} />}
          
          <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
            <Table>
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow >
                  <TableCell sx={{ fontWeight: 'bold'}}>选择</TableCell>
                  <TableCell sx={{ fontWeight: 'bold'}}>用户ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold'}}>用户名</TableCell>
                  <TableCell sx={{ fontWeight: 'bold'}}>描述</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {allUsers.length > 0 ? (
                  allUsers.map((user) => (
                    <TableRow key={user.ID} hover>
                      <TableCell  sx={{ padding: 0 }}>
                        <Checkbox
                          checked={selectedUsers.includes(user.ID)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedUsers([...selectedUsers, user.ID]);
                            } else {
                              setSelectedUsers(selectedUsers.filter(id => id !== user.ID));
                            }
                          }}
                        />
                      </TableCell>
                      <TableCell sx={{ padding: 1 }}>{user.ID}</TableCell>
                      <TableCell sx={{ padding: 1 }}>{user.username}</TableCell>
                      <TableCell sx={{ padding: 1 }}>{user.description ? truncateText(user.description, 30) : '无'}</TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={4} align="center" sx={{ padding: 0 }}>
                      {availableUsersLoading ? '加载中...' : '没有可添加的用户'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          {(availableUsersCount > 0 || availableUsersLoading) && (
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControl sx={{ minWidth: 120 }}>
                <InputLabel>每页数量</InputLabel>
                <Select
                  value={addUserPermPageSize}
                  label="每页数量"
                  onChange={async (e) => {
                    setAddUserPermPageSize(e.target.value);
                    setAddUserPermPage(1);
                    await loadAvailableUsers(1, e.target.value, addUserSearchQuery);
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
                  count={Math.ceil(availableUsersCount / addUserPermPageSize)}
                  page={addUserPermPage}
                  onChange={async (e, value) => {
                    setAddUserPermPage(value);
                    await loadAvailableUsers(value, addUserPermPageSize, addUserSearchQuery);
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
            setAddUserDialog(false);
            setSelectedUsers([]);
            setAddUserPermPage(1);
            setAvailableUsersCount(0);
            setAddUserSearchQuery(''); // 重置搜索查询
            setAddPermissionError('');
          }}>取消</Button>
          <Button onClick={() => handleAddUserPermission(0)} variant="contained" color="error" disabled={isLoading || selectedUsers.length === 0}>
            添加拒绝
          </Button>
          <Button onClick={() => handleAddUserPermission(1)} variant="contained" color="success" disabled={isLoading || selectedUsers.length === 0}>
            添加允许
          </Button>
        </DialogActions>
      </Dialog>

      {/* 添加用户组权限对话框 */}
      <Dialog 
        open={addGroupDialog} 
        onClose={() => {
          setAddGroupDialog(false);
          setSelectedGroups([]);
          setAddGroupPermPage(1);
          setAvailableGroupsCount(0);
          setAddGroupSearchQuery(''); // 重置搜索查询
          setAddPermissionError('');
        }} 
        
        fullWidth
        maxWidth={isMobile ? 'lg' : 'md'}
       fullScreen={isMobile}>
        <DialogTitle>添加用户组权限</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {addPermissionError && (
            <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setAddPermissionError('')}>
              {addPermissionError}
            </Alert>
          )}
          <Typography variant="body2" sx={{ marginBottom: 2, color: '#666' }}>
            选择要添加权限的用户组
          </Typography>
          
          <Stack direction="row" spacing={1} sx={{ marginBottom: 2 }}>
            <TextField
              placeholder="搜索用户组名"
              size="small"
              value={addGroupSearchQuery}
              onChange={(e) => setAddGroupSearchQuery(e.target.value)}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  setAddGroupPermPage(1);
                  loadAvailableGroups(1, addGroupPermPageSize, addGroupSearchQuery);
                }
              }}
              sx={{ minWidth: 200, flex: 1 }}
            />
            <Button
              variant="outlined"
              size="small"
              onClick={() => {
                setAddGroupPermPage(1);
                loadAvailableGroups(1, addGroupPermPageSize, addGroupSearchQuery);
              }}
              disabled={availableGroupsLoading}
            >
              搜索
            </Button>
          </Stack>
          
          {availableGroupsLoading && <CircularProgress sx={{ marginBottom: 2 }} />}
          
          <TableContainer component={Paper} sx={{ marginBottom: 2 }}>
            <Table>
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold' }}>选择</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户组ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>用户组名</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>描述</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {allGroups.length > 0 ? (
                  allGroups.map((group) => (
                    <TableRow key={group.ID} hover>
                      <TableCell  sx={{padding:0}}>
                        <Checkbox
                          checked={selectedGroups.includes(group.ID)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedGroups([...selectedGroups, group.ID]);
                            } else {
                              setSelectedGroups(selectedGroups.filter(id => id !== group.ID));
                            }
                          }}
                        />
                      </TableCell>
                      <TableCell  sx={{padding:1}}>{group.ID}</TableCell>
                      <TableCell  sx={{padding:1}}>{group.name}</TableCell>
                      <TableCell  sx={{padding:1}}>{group.description ? truncateText(group.description, 30) : '无'}</TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={4} align="center"  sx={{padding:0}}>
                      {availableGroupsLoading ? '加载中...' : '没有可添加的用户组'}
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>

          
          {(availableGroupsCount > 0 || availableGroupsLoading) && (
            <Stack direction="row" spacing={2} alignItems="center">
              <FormControl sx={{ minWidth: 120 }}>
                <InputLabel>每页数量</InputLabel>
                <Select
                  value={addGroupPermPageSize}
                  label="每页数量"
                  onChange={async (e) => {
                    setAddGroupPermPageSize(e.target.value);
                    setAddGroupPermPage(1);
                    await loadAvailableGroups(1, e.target.value, addGroupSearchQuery);
                  }}
                  size="small"
                  disabled={availableGroupsLoading}
                >
                  <MenuItem value={5}>5</MenuItem>
                  <MenuItem value={10}>10</MenuItem>
                  <MenuItem value={20}>20</MenuItem>
                </Select>
              </FormControl>
              <Box sx={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
                <Pagination
                  count={Math.ceil(availableGroupsCount / addGroupPermPageSize)}
                  page={addGroupPermPage}
                  onChange={async (e, value) => {
                    setAddGroupPermPage(value);
                    await loadAvailableGroups(value, addGroupPermPageSize, addGroupSearchQuery);
                  }}
                  color="primary"
                  size="small"
                  disabled={availableGroupsLoading}
                />
              </Box>
            </Stack>
          )}
          
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setAddGroupDialog(false);
            setSelectedGroups([]);
            setAddGroupPermPage(1);
            setAvailableGroupsCount(0);
            setAddGroupSearchQuery(''); // 重置搜索查询
            setAddPermissionError('');
          }}>取消</Button>
          <Button onClick={() => handleAddGroupPermission(0)} variant="contained" color="error" disabled={isLoading || selectedGroups.length === 0}>
            添加拒绝
          </Button>
          <Button onClick={() => handleAddGroupPermission(1)} variant="contained" color="success" disabled={isLoading || selectedGroups.length === 0}>
            添加允许
          </Button>
        </DialogActions>
      </Dialog>

      {/* 客户端配置管理对话框 */}
      <Dialog 
        open={clientConfigDialog} 
        onClose={() => setClientConfigDialog(false)} 
        maxWidth={isMobile ? 'lg' : 'md'}
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>客户端配置管理</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {/* 使用独立的错误和成功状态变量 */}
          {clientConfigError && <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setClientConfigError('')}>{clientConfigError}</Alert>}
          {clientConfigSuccess && <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setClientConfigSuccess('')}>{clientConfigSuccess}</Alert>}

          <Stack direction="row" spacing={2} sx={{ marginBottom: 2 }}>
            <Button
              variant="contained"
              color="success"
              startIcon={<AddIcon />}
              onClick={() => {
                setConfigFormData({ client_cert_name: '', config: '' });
                setConfigFormError('');
                setAddConfigDialog(true);
              }}
            >
              添加配置
            </Button>
            {selectedConfigs.length > 0 && (
              <Button
                variant="contained"
                color="error"
                startIcon={<DeleteIcon />}
                onClick={handleDeleteConfigs}
                sx={{marginLeft: 2}}
              >
                删除选中 ({selectedConfigs.length})
                
              </Button>
            )}
            <Button
              variant="outlined"
              startIcon={<RefreshIcon />}
              onClick={() => loadClientConfigs(selectedServer?.id)}
              disabled={isLoading}
            >
              刷新
            </Button>
          </Stack>

          <TableContainer component={Paper}>
            <Table size="small">
              <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                <TableRow>
                  <TableCell sx={{ fontWeight: 'bold', width: '50px' }}>选择</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>ID</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>客户端证书名称</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>配置内容</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }} align="right">操作</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {selectedServerConfigs.length > 0 ? (
                  selectedServerConfigs.map((config) => (
                    <TableRow key={config.ID} hover>
                      <TableCell sx={{ padding: 0, width: '50px' }}>
                        <Checkbox
                          checked={selectedConfigs.includes(config.ID)}
                          onChange={(e) => {
                            if (e.target.checked) {
                              setSelectedConfigs([...selectedConfigs, config.ID]);
                            } else {
                              setSelectedConfigs(selectedConfigs.filter(id => id !== config.ID));
                            }
                          }}
                        />
                      </TableCell>
                      <TableCell>{config.ID}</TableCell>
                      <TableCell>{config.client_cert_name}</TableCell>
                      <TableCell 
                        onClick={() => {
                          setFullConfigContent(config.config);
                          setShowFullConfigDialog(true);
                        }}
                        style={{ cursor: 'pointer', textDecoration: 'underline' }}
                      >
                        {truncateConfig(config.config, 50)}
                      </TableCell>
                      <TableCell align="right">
                        <Stack direction="row" spacing={1}>
                          <Button
                            size="small"
                            variant="outlined"
                            onClick={() => handleEditConfig(config)}
                          >
                            编辑
                          </Button>
                          <Button
                            size="small"
                            variant="outlined"
                            color="error"
                            onClick={async () => {
                              if (window.confirm('确定要删除该客户端配置吗？')) {
                                try {
                                  setIsLoading(true);
                                  await serverAPI.deleteClientConfigs({ id_list: [config.ID] });
                                  setClientConfigSuccess('客户端配置删除成功'); // 使用独立的成功状态
                                  await loadClientConfigs(selectedServer.id);
                                } catch (err) {
                                  setClientConfigError(err.response?.data?.error || '删除配置失败'); // 使用独立的错误状态
                                } finally {
                                  setIsLoading(false);
                                }
                              }
                            }}
                          >
                            删除
                          </Button>
                        </Stack>
                      </TableCell>
                    </TableRow>
                  ))
                ) : (
                  <TableRow>
                    <TableCell colSpan={5} align="center">
                      暂无客户端配置
                    </TableCell>
                  </TableRow>
                )}
              </TableBody>
            </Table>
          </TableContainer>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setClientConfigDialog(false)}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 查看完整配置内容对话框 */}
      <Dialog 
        open={showFullConfigDialog} 
        onClose={() => setShowFullConfigDialog(false)} 
        maxWidth="md"
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>完整配置内容 - {selectedServerConfigs.find(c => c.config === fullConfigContent)?.client_cert_name || ''}</DialogTitle>
        <DialogContent>
          <TextField
            fullWidth
            multiline
            rows={10}
            value={fullConfigContent}
            variant="outlined"
            disabled
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setShowFullConfigDialog(false)}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 添加/编辑配置对话框 */}
      <Dialog 
        open={addConfigDialog} 
        onClose={() => {
          setAddConfigDialog(false);
          setConfigFormData({ client_cert_name: '', config: '' });
          setConfigFormError('');
        }} 
        maxWidth="md"
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>
          {configFormData.ID ? '编辑客户端配置' : '添加客户端配置'}
        </DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {configFormError && <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setConfigFormError('')}>{configFormError}</Alert>}
          {clientCertCNs.length > 0 ? (
            <Autocomplete
              freeSolo
              options={clientCertCNs}
              value={configFormData.client_cert_name}
              onChange={(e, newValue) =>
                setConfigFormData((prev) => ({ ...prev, client_cert_name: newValue || '' }))
              }
              onInputChange={(e, newInputValue) =>
                setConfigFormData((prev) => ({ ...prev, client_cert_name: newInputValue }))
              }
              renderInput={(params) => (
                <TextField
                  {...params}
                  fullWidth
                  label="客户端证书名称"
                  margin="normal"
                  required
                  helperText="可输入自定义名称，或从该服务器 CA 已签发的客户端证书中选择 CN"
                />
              )}
            />
          ) : (
            <TextField
              fullWidth
              label="客户端证书名称"
              value={configFormData.client_cert_name}
              onChange={(e) => setConfigFormData({ ...configFormData, client_cert_name: e.target.value })}
              margin="normal"
              required
            />
          )}
          <TextField
            fullWidth
            label="配置内容"
            value={configFormData.config}
            onChange={(e) => setConfigFormData({ ...configFormData, config: e.target.value })}
            margin="normal"
            multiline
            rows={6}
            placeholder="#指定客户端ip地址&#10;ifconfig-push 100.66.10.73 255.255.255.0&#10;#指定通向客户端的路由&#10;iroute 192.168.17.0 255.255.255.0&#10;#为客户端推送指定路由&#10;push &quot;route 192.168.1.0 255.255.255.0&quot;"
            required
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setAddConfigDialog(false);
            setConfigFormData({ client_cert_name: '', config: '' });
            setConfigFormError('');
          }}>取消</Button>
          <Button onClick={handleAddConfig} variant="contained" color="primary">
            保存
          </Button>
        </DialogActions>
      </Dialog>

      {/* 从证书管理选择 */}
      <Dialog
        open={certSelect.open}
        onClose={() => setCertSelect((prev) => ({ ...prev, open: false }))}
        maxWidth="md"
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>
          {certSelect.target === 'ca' ? '选择 CA 证书' : '选择服务器证书'}
        </DialogTitle>
        <DialogContent>
          {certSelect.target === 'cert' && (
            certSelect.caId ? (
              <Alert severity="info" sx={{ marginTop: 1, marginBottom: 1 }}>
                仅列出所选 CA 签发的服务器证书或未指定证书；选择后会同时使用其私钥。
              </Alert>
            ) : (
              <Alert severity="warning" sx={{ marginTop: 1, marginBottom: 1 }}>
                未选择 CA 证书，以下列出所有服务器证书与未指定证书。请确保所选证书是 CA 证书签发的服务器证书，否则无法工作。
              </Alert>
            )
          )}
          {certSelect.loading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}><CircularProgress /></Box>
          ) : (
            <>
              <TableContainer>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>名称</TableCell>
                      <TableCell>类型</TableCell>
                      <TableCell>主题</TableCell>
                      <TableCell>到期时间</TableCell>
                      <TableCell>操作</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {certSelect.certs.length === 0 ? (
                      <TableRow><TableCell colSpan={5} align="center">暂无可选证书</TableCell></TableRow>
                    ) : (
                      certSelect.certs.map((cert) => (
                        <TableRow key={cert.id} hover>
                          <TableCell>{cert.name}</TableCell>
                          <TableCell>{certTypeLabel(cert.type)}</TableCell>
                          <TableCell sx={{ maxWidth: 240, wordBreak: 'break-all' }}>{cert.subject}</TableCell>
                          <TableCell>{cert.not_after ? new Date(cert.not_after * 1000).toLocaleDateString() : '-'}</TableCell>
                          <TableCell><Button size="small" onClick={() => applyCertSelect(cert)}>选择</Button></TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
              </TableContainer>
              <TablePagination
                component="div"
                count={certSelect.total}
                page={Math.max(certSelect.page - 1, 0)}
                onPageChange={(e, newPage) => openCertSelect(certSelect.target, newPage + 1)}
                rowsPerPage={certSelect.pageSize}
                rowsPerPageOptions={[CERT_SELECT_PAGE_SIZE]}
                labelRowsPerPage="每页"
              />
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCertSelect((prev) => ({ ...prev, open: false }))}>取消</Button>
        </DialogActions>
      </Dialog>

      {/* DH 参数生成 */}
      <Dialog open={dhDialog.open} onClose={() => setDhDialog({ ...dhDialog, open: false })} maxWidth="xs" fullWidth fullScreen={isMobile}>
        <DialogTitle>生成 DH 参数</DialogTitle>
        <DialogContent>
          {formData.dh && <Alert severity="warning" sx={{ marginTop: 1, marginBottom: 1 }}>当前已有 DH 参数，生成后将覆盖现有值。</Alert>}
          <FormControl fullWidth margin="normal">
            <InputLabel>位数</InputLabel>
            <Select value={dhDialog.bits} label="位数" onChange={(e) => setDhDialog({ ...dhDialog, bits: Number(e.target.value) })}>
              <MenuItem value={2048}>2048 位（推荐）</MenuItem>
              <MenuItem value={3072}>3072 位</MenuItem>
              <MenuItem value={4096}>4096 位</MenuItem>
            </Select>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDhDialog({ ...dhDialog, open: false })} disabled={dhDialog.generating}>取消</Button>
          <Button variant="contained" onClick={handleGenerateDH} disabled={dhDialog.generating}>
            {dhDialog.generating ? <CircularProgress size={24} /> : '生成'}
          </Button>
        </DialogActions>
      </Dialog>

      {/* 导出客户端配置 */}
      <Dialog open={exportDialog.open} onClose={() => setExportDialog((prev) => ({ ...prev, open: false }))} maxWidth="sm" fullWidth fullScreen={isMobile}>
        <DialogTitle>导出客户端配置</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {exportDialog.error && <Alert severity="error" sx={{ marginBottom: 2 }}>{exportDialog.error}</Alert>}
          {exportDialog.loading && !exportDialog.servers.length ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}><CircularProgress /></Box>
          ) : (
            <>
              <TextField
                select
                fullWidth
                margin="normal"
                label="服务器"
                value={exportDialog.serverId}
                onChange={(e) => handleExportServerChange(e.target.value)}
              >
                {exportDialog.servers.map((s) => (
                  <MenuItem key={s.id} value={s.id}>{s.name}</MenuItem>
                ))}
              </TextField>

              {!exportDialog.serverId && (
                <Alert severity="info" sx={{ marginTop: 1 }}>请先选择服务器。</Alert>
              )}

              {exportDialog.serverId && exportDialog.mode === 'select' && (
                <>
                  <Alert severity="info" sx={{ marginTop: 1 }}>
                    仅列出该服务器 CA 签发的客户端证书或未指定证书。
                  </Alert>
                  <TextField
                    select
                    fullWidth
                    margin="normal"
                    label="客户端证书"
                    value={exportDialog.certId}
                    onChange={(e) => setExportDialog((prev) => ({ ...prev, certId: e.target.value }))}
                  >
                    {exportDialog.clientCerts.length === 0 ? (
                      <MenuItem value="" disabled>该 CA 下没有可用的客户端证书或未指定证书</MenuItem>
                    ) : (
                      exportDialog.clientCerts.map((cert) => (
                        <MenuItem key={cert.id} value={cert.id}>
                          {cert.name}{cert.type === 4 ? '（未指定）' : ''}{cert.has_key ? '' : '（无私钥）'}
                        </MenuItem>
                      ))
                    )}
                  </TextField>
                </>
              )}

              {exportDialog.serverId && exportDialog.mode === 'manual' && (
                <>
                  <Alert severity="info" sx={{ marginTop: 1 }}>
                    该服务器使用手动填写的 CA 证书，请手动填写客户端证书与私钥；也可以留空，导出后自行修改配置文件。
                  </Alert>
                  <TextField
                    fullWidth
                    margin="normal"
                    label="客户端证书 (PEM，可留空)"
                    multiline
                    rows={6}
                    value={exportDialog.manualCert}
                    onChange={(e) => setExportDialog((prev) => ({ ...prev, manualCert: e.target.value }))}
                    placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
                    sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
                  />
                  <TextField
                    fullWidth
                    margin="normal"
                    label="客户端私钥 (PEM，可留空)"
                    multiline
                    rows={6}
                    value={exportDialog.manualKey}
                    onChange={(e) => setExportDialog((prev) => ({ ...prev, manualKey: e.target.value }))}
                    placeholder="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
                    sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
                  />
                </>
              )}

              <TextField
                fullWidth
                margin="normal"
                label="服务器地址"
                value={exportDialog.host}
                onChange={(e) => setExportDialog((prev) => ({ ...prev, host: e.target.value }))}
              />
              <TextField
                fullWidth
                margin="normal"
                label="服务器端口"
                type="number"
                value={exportDialog.port}
                onChange={(e) => setExportDialog((prev) => ({ ...prev, port: e.target.value }))}
              />
              <TextField
                fullWidth
                margin="normal"
                label="客户端附加配置"
                multiline
                rows={5}
                value={exportDialog.extraConfig}
                onChange={(e) => setExportDialog((prev) => ({ ...prev, extraConfig: e.target.value }))}
                placeholder={'将追加到导出配置末尾，例如：\nmssfix 1308\nredirect-gateway def1'}
                helperText="随服务器地址一起保存，下次导出自动回填；可添加 remote-cert-tls server 限制服务端证书类型（要求服务端证书含 serverAuth 用途）"
                sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
              />
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setExportDialog((prev) => ({ ...prev, open: false }))} disabled={exportDialog.loading}>取消</Button>
          <Button
            variant="contained"
            onClick={handleExportClientConfig}
            disabled={
              exportDialog.loading ||
              !exportDialog.serverId ||
              !exportDialog.host ||
              !exportDialog.port ||
              (exportDialog.mode === 'select' && !exportDialog.certId)
            }
          >
            {exportDialog.loading ? <CircularProgress size={24} /> : '导出'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default ServerManagement;

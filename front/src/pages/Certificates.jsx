import React, { useState, useEffect, useCallback } from 'react';
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
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
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
  Tooltip,
  Card,
  CardContent,
  CardActions,
  useMediaQuery,
  useTheme,
} from '@mui/material';
import {
  Add as AddIcon,
  Upload as UploadIcon,
  ArrowBack as ArrowBackIcon,
  Delete as DeleteIcon,
  Visibility as ViewIcon,
  Settings as ManageIcon,
  Download as DownloadIcon,
  History as HistoryIcon,
} from '@mui/icons-material';
import { useNavigate, useParams } from 'react-router-dom';
import { certificateAPI } from '../api';

export const CERT_TYPE_CA = 1;
export const CERT_TYPE_SERVER = 2;
export const CERT_TYPE_CLIENT = 3;

const certTypeLabel = (type) => {
  switch (type) {
    case CERT_TYPE_CA:
      return 'CA 证书';
    case CERT_TYPE_SERVER:
      return '服务器证书';
    case CERT_TYPE_CLIENT:
      return '客户端证书';
    default:
      return '未知';
  }
};

const formatTime = (unix) => {
  if (!unix) return '-';
  return new Date(unix * 1000).toLocaleString();
};

// 把 "C=CN, ST=JiangSu, CN=xxx" 形式的主题解析为字段数组
const parseSubject = (subject) => {
  return String(subject || '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => {
      const idx = part.indexOf('=');
      if (idx < 0) return { key: '', value: part };
      return { key: part.slice(0, idx).trim(), value: part.slice(idx + 1).trim() };
    });
};

const subjectCN = (subject) => {
  const found = parseSubject(subject).find((f) => f.key === 'CN');
  return found ? found.value : String(subject || '');
};

const subjectOthers = (subject) => parseSubject(subject).filter((f) => f.key !== 'CN');

// 主题/颁发者多行显示文本
const subjectLines = (subject) =>
  parseSubject(subject)
    .map((f) => (f.key ? `${f.key} = ${f.value}` : f.value))
    .join('\n');

// 从 Content-Disposition 响应头解析文件名
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

const saveBlobResponse = (res, fallbackName) => {
  const blob = res.data instanceof Blob ? res.data : new Blob([res.data]);
  const fileName = filenameFromDisposition(res.headers?.['content-disposition'], fallbackName);
  const url = window.URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = fileName;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.URL.revokeObjectURL(url);
};

const blobErrorMessage = async (err, fallback) => {
  const data = err.response?.data;
  if (data instanceof Blob) {
    try {
      const parsed = JSON.parse(await data.text());
      if (parsed?.error) return parsed.error;
    } catch {
      // 忽略解析失败
    }
  }
  return err.response?.data?.error || err.message || fallback;
};

const initialKeyOptions = { key_type: 'rsa', rsa_bits: 2048, ec_curve: 'P256' };

const KeyOptionsFields = ({ value, onChange }) => (
  <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
    <FormControl margin="normal" sx={{ minWidth: 140 }}>
      <InputLabel>密钥类型</InputLabel>
      <Select
        value={value.key_type}
        label="密钥类型"
        onChange={(e) => onChange({ ...value, key_type: e.target.value })}
      >
        <MenuItem value="rsa">RSA</MenuItem>
        <MenuItem value="ec">ECDSA</MenuItem>
      </Select>
    </FormControl>
    {value.key_type === 'rsa' ? (
      <FormControl margin="normal" sx={{ minWidth: 140 }}>
        <InputLabel>RSA 位数</InputLabel>
        <Select
          value={value.rsa_bits}
          label="RSA 位数"
          onChange={(e) => onChange({ ...value, rsa_bits: Number(e.target.value) })}
        >
          <MenuItem value={2048}>2048</MenuItem>
          <MenuItem value={3072}>3072</MenuItem>
          <MenuItem value={4096}>4096</MenuItem>
        </Select>
      </FormControl>
    ) : (
      <FormControl margin="normal" sx={{ minWidth: 140 }}>
        <InputLabel>EC 曲线</InputLabel>
        <Select
          value={value.ec_curve}
          label="EC 曲线"
          onChange={(e) => onChange({ ...value, ec_curve: e.target.value })}
        >
          <MenuItem value="P256">P-256</MenuItem>
          <MenuItem value="P384">P-384</MenuItem>
          <MenuItem value="P521">P-521</MenuItem>
        </Select>
      </FormControl>
    )}
  </Box>
);

const Certificates = () => {
  const navigate = useNavigate();
  const { caId } = useParams();
  const isLevel2 = !!caId;
  const theme = useTheme();
  const isMobile = useMediaQuery(theme.breakpoints.down('md'));

  const [certs, setCerts] = useState([]);
  const [caCert, setCaCert] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState(null);
  const [success, setSuccess] = useState('');
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);

  const [viewDialog, setViewDialog] = useState({ open: false, data: null });
  const [subjectDialog, setSubjectDialog] = useState({ open: false, cert: null });
  const [genCADialog, setGenCADialog] = useState(false);
  const [signDialog, setSignDialog] = useState({ open: false, certType: CERT_TYPE_SERVER });
  const [importDialog, setImportDialog] = useState({ open: false, certType: CERT_TYPE_SERVER });
  const [busy, setBusy] = useState(false);
  const [dialogError, setDialogError] = useState('');

  const [keyOptions, setKeyOptions] = useState(initialKeyOptions);
  const [form, setForm] = useState({
    name: '',
    common_name: '',
    org: '',
    organizational_unit: '',
    country: '',
    province: '',
    locality: '',
    email_address: '',
    days: 3650,
    description: '',
  });
  const [importForm, setImportForm] = useState({ name: '', cert: '', key: '', description: '', cert_type: CERT_TYPE_SERVER });

  const loadCerts = useCallback(async () => {
    try {
      setLoading(true);
      setError(null);
      const listParams = { page: page + 1, pageSize };
      if (isLevel2) {
        listParams.parentId = caId;
        const [childRes, caRes] = await Promise.all([
          certificateAPI.list(listParams),
          certificateAPI.getInfo(caId),
        ]);
        setCerts(childRes.data.data || []);
        setTotal(childRes.data.total || 0);
        setCaCert(caRes.data.data || null);
      } else {
        listParams.type = CERT_TYPE_CA;
        const res = await certificateAPI.list(listParams);
        setCerts(res.data.data || []);
        setTotal(res.data.total || 0);
        setCaCert(null);
      }
    } catch (err) {
      setError('加载证书列表失败：' + (err.response?.data?.error || err.message));
    } finally {
      setLoading(false);
    }
  }, [caId, isLevel2, page, pageSize]);

  useEffect(() => {
    setPage(0);
  }, [caId]);

  useEffect(() => {
    loadCerts();
  }, [loadCerts]);

  const resetForm = () => {
    setForm({ name: '', common_name: '', org: '', organizational_unit: '', country: '', province: '', locality: '', email_address: '', days: 3650, description: '' });
    setKeyOptions(initialKeyOptions);
    setDialogError('');
  };

  const handleGenerateCA = async () => {
    try {
      setBusy(true);
      setDialogError('');
      await certificateAPI.generateCA({ ...form, ...keyOptions });
      setSuccess('CA 证书已生成');
      setGenCADialog(false);
      resetForm();
      loadCerts();
    } catch (err) {
      setDialogError('生成 CA 失败：' + (err.response?.data?.error || err.message));
    } finally {
      setBusy(false);
    }
  };

  const handleSign = async () => {
    try {
      setBusy(true);
      setDialogError('');
      await certificateAPI.sign({ ...form, ...keyOptions, ca_id: Number(caId), cert_type: signDialog.certType });
      setSuccess('证书已签发');
      setSignDialog({ open: false, certType: CERT_TYPE_SERVER });
      resetForm();
      loadCerts();
    } catch (err) {
      setDialogError('签发证书失败：' + (err.response?.data?.error || err.message));
    } finally {
      setBusy(false);
    }
  };

  const handleImport = async () => {
    try {
      setBusy(true);
      setDialogError('');
      const payload = {
        name: importForm.name,
        cert: importForm.cert,
        key: importForm.key,
        description: importForm.description,
      };
      if (isLevel2) {
        payload.cert_type = importForm.cert_type;
        payload.parent_id = Number(caId);
      } else {
        payload.cert_type = CERT_TYPE_CA;
      }
      await certificateAPI.importCert(payload);
      setSuccess('证书已导入');
      setImportDialog({ open: false, certType: CERT_TYPE_SERVER });
      setImportForm({ name: '', cert: '', key: '', description: '', cert_type: CERT_TYPE_SERVER });
      setDialogError('');
      loadCerts();
    } catch (err) {
      setDialogError('导入证书失败：' + (err.response?.data?.error || err.message));
    } finally {
      setBusy(false);
    }
  };

  const handleDelete = async (cert) => {
    const message = cert.type === CERT_TYPE_CA
      ? `确认删除 CA「${cert.name}」吗？若其下仍有服务器证书将无法删除；删除时会一并删除其下的客户端证书。`
      : `确认删除证书「${cert.name}」吗？`;
    if (!window.confirm(message)) return;
    try {
      await certificateAPI.delete(cert.id);
      setSuccess('证书已删除');
      loadCerts();
    } catch (err) {
      setError('删除证书失败：' + (err.response?.data?.error || err.message));
    }
  };

  const handleView = async (cert) => {
    try {
      const res = await certificateAPI.getInfo(cert.id);
      setViewDialog({ open: true, data: res.data.data });
    } catch (err) {
      setError('获取证书内容失败：' + (err.response?.data?.error || err.message));
    }
  };

  const handleDownload = async (cert, kind) => {
    if (!cert) return;
    try {
      const res = kind === 'key'
        ? await certificateAPI.downloadKey(cert.id)
        : await certificateAPI.downloadCert(cert.id);
      saveBlobResponse(res, `${cert.name || 'cert'}.${kind === 'key' ? 'key' : 'cert'}`);
    } catch (err) {
      setError('下载失败：' + (await blobErrorMessage(err, '下载失败')));
    }
  };

  const caHasKey = !!caCert && (caCert.has_key ?? !!String(caCert.key || '').trim());
  const canSign = !isLevel2 || caHasKey;

  // 移动端卡片视图
  const MobileCertCard = ({ cert }) => (
    <Card sx={{ marginBottom: 2 }}>
      <CardContent>
        <Typography variant="h6" sx={{ marginBottom: 1, wordBreak: 'break-all' }}>
          {cert.name}
        </Typography>
        <Stack spacing={1}>
          {isLevel2 && (
            <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
              <Typography variant="body2" color="textSecondary">
                类型:
              </Typography>
              <Typography variant="body2">{certTypeLabel(cert.type)}</Typography>
            </Box>
          )}
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              主题 (CN):
            </Typography>
            <Box
              component="span"
              onClick={() => setSubjectDialog({ open: true, cert })}
              sx={{
                cursor: 'pointer',
                textDecoration: 'underline dotted',
                wordBreak: 'break-all',
                textAlign: 'right',
              }}
            >
              {subjectCN(cert.subject) || '-'}
            </Box>
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              密钥:
            </Typography>
            {cert.has_key ? (
              <Chip size="small" color="success" label={cert.key_type || '已有私钥'} />
            ) : (
              <Chip size="small" color="default" label="无私钥" />
            )}
          </Box>
          <Box sx={{ display: 'flex', justifyContent: 'space-between' }}>
            <Typography variant="body2" color="textSecondary">
              到期时间:
            </Typography>
            <Typography variant="body2" sx={{ textAlign: 'right' }}>
              {formatTime(cert.not_after)}
            </Typography>
          </Box>
        </Stack>
      </CardContent>
      <CardActions sx={{ flexWrap: 'wrap', gap: 1 }}>
        <Button size="small" startIcon={<ViewIcon />} onClick={() => handleView(cert)} variant="outlined">
          查看
        </Button>
        {!isLevel2 && (
          <Button
            size="small"
            startIcon={<ManageIcon />}
            onClick={() => navigate(`/dashboard/certificates/${cert.id}`)}
            variant="outlined"
          >
            管理
          </Button>
        )}
        <Button
          size="small"
          color="error"
          startIcon={<DeleteIcon />}
          onClick={() => handleDelete(cert)}
          variant="outlined"
        >
          删除
        </Button>
      </CardActions>
    </Card>
  );

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      <Stack
        direction="row"
        alignItems="center"
        spacing={1}
        sx={{ marginBottom: 2, flexWrap: 'wrap', rowGap: 1 }}
      >
        <Stack direction="row" alignItems="center" spacing={1} sx={{ flexGrow: 1, minWidth: 200 }}>
          {isLevel2 && (
            <IconButton onClick={() => navigate('/dashboard/certificates')}>
              <ArrowBackIcon />
            </IconButton>
          )}
          <Typography variant="h5" sx={{ wordBreak: 'break-word' }}>
            {isLevel2 ? `管理证书 - ${caCert?.name || ''}` : '证书管理'}
          </Typography>
        </Stack>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 1, justifyContent: 'flex-end' }}>
          {!isLevel2 && (
            <>
              <Button variant="contained" startIcon={<AddIcon />} onClick={() => { resetForm(); setGenCADialog(true); }}>
                生成新 CA
              </Button>
              <Button variant="outlined" startIcon={<UploadIcon />} onClick={() => { setDialogError(''); setImportDialog({ open: true, certType: CERT_TYPE_CA }); }}>
                导入 CA
              </Button>
            </>
          )}
          {isLevel2 && (
            <>
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                disabled={!canSign}
                onClick={() => { resetForm(); setSignDialog({ open: true, certType: CERT_TYPE_SERVER }); }}
              >
                签发服务器证书
              </Button>
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                disabled={!canSign}
                onClick={() => { resetForm(); setSignDialog({ open: true, certType: CERT_TYPE_CLIENT }); }}
              >
                签发客户端证书
              </Button>
              <Button variant="outlined" startIcon={<UploadIcon />} onClick={() => { setDialogError(''); setImportDialog({ open: true, certType: CERT_TYPE_SERVER }); }}>
                导入证书
              </Button>
            </>
          )}
          <Button variant="outlined" startIcon={<HistoryIcon />} onClick={() => navigate('/dashboard/certificates/events')}>
            操作事件
          </Button>
        </Box>
      </Stack>

      {isLevel2 && caCert && !caHasKey && (
        <Alert severity="warning" sx={{ marginBottom: 2 }}>
          该 CA 证书没有私钥，无法签发新证书（仍可查看与导入由它签发的证书）。
        </Alert>
      )}
      {success && (
        <Alert severity="success" sx={{ marginBottom: 2 }} onClose={() => setSuccess('')}>
          {success}
        </Alert>
      )}
      {error && (
        <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError(null)}>
          {error}
        </Alert>
      )}

      <Paper sx={{ padding: 1 }}>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
            <CircularProgress />
          </Box>
        ) : isMobile ? (
          <Box>
            {certs.length === 0 ? (
              <Box sx={{ padding: 3, textAlign: 'center' }}>
                <Typography color="textSecondary">暂无证书</Typography>
              </Box>
            ) : (
              certs.map((cert) => <MobileCertCard key={cert.id} cert={cert} />)
            )}
          </Box>
        ) : (
          <TableContainer>
            <Table size="small">
              <TableHead>
                <TableRow sx={{ backgroundColor: '#f5f5f5' }}>
                  <TableCell sx={{ fontWeight: 'bold' }}>名称</TableCell>
                  {isLevel2 && <TableCell sx={{ fontWeight: 'bold' }}>类型</TableCell>}
                  <TableCell sx={{ fontWeight: 'bold' }}>主题</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>密钥</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>到期时间</TableCell>
                  <TableCell sx={{ fontWeight: 'bold' }}>操作</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {certs.length === 0 ? (
                  <TableRow>
                    <TableCell colSpan={isLevel2 ? 6 : 5} align="center">
                      暂无证书
                    </TableCell>
                  </TableRow>
                ) : (
                  certs.map((cert) => (
                    <TableRow key={cert.id} hover>
                      <TableCell>{cert.name}</TableCell>
                      {isLevel2 && <TableCell>{certTypeLabel(cert.type)}</TableCell>}
                      <TableCell>
                        <Tooltip
                          placement="right"
                          title={
                            <Box>
                              {subjectOthers(cert.subject).length === 0 ? (
                                <span>无其他字段</span>
                              ) : (
                                subjectOthers(cert.subject).map((f, i) => (
                                  <div key={i}>{f.key ? `${f.key}=${f.value}` : f.value}</div>
                                ))
                              )}
                            </Box>
                          }
                        >
                          <Box
                            component="span"
                            onClick={() => setSubjectDialog({ open: true, cert })}
                            sx={{ cursor: 'pointer', textDecoration: 'underline dotted', wordBreak: 'break-all' }}
                          >
                            {subjectCN(cert.subject) || '-'}
                          </Box>
                        </Tooltip>
                      </TableCell>
                      <TableCell>
                        {cert.has_key ? (
                          <Chip size="small" color="success" label={cert.key_type || '已有私钥'} />
                        ) : (
                          <Chip size="small" color="default" label="无私钥" />
                        )}
                      </TableCell>
                      <TableCell>{formatTime(cert.not_after)}</TableCell>
                      <TableCell>
                        <Tooltip title="查看内容">
                          <Button size="small" startIcon={<ViewIcon />} onClick={() => handleView(cert)}>
                            查看
                          </Button>
                        </Tooltip>
                        {!isLevel2 && (
                          <Button
                            size="small"
                            startIcon={<ManageIcon />}
                            onClick={() => navigate(`/dashboard/certificates/${cert.id}`)}
                          >
                            管理
                          </Button>
                        )}
                        <Button size="small" color="error" startIcon={<DeleteIcon />} onClick={() => handleDelete(cert)}>
                          删除
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
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
             size="small"
              value={pageSize}
              label="每页数量"
              onChange={(e) => {
                setPageSize(e.target.value);
                setPage(0);
              }}
            >
              <MenuItem value={10}>10</MenuItem>
              <MenuItem value={20}>20</MenuItem>
              <MenuItem value={50}>50</MenuItem>
              <MenuItem value={100}>100</MenuItem>
            </Select>
          </FormControl>
          <Typography>总共 {total} 个证书</Typography>
          <Pagination
            count={Math.max(1, Math.ceil(total / pageSize))}
            page={page + 1}
            onChange={(e, value) => setPage(value - 1)}
            color="primary"
          />
        </Stack>
      </Paper>

      {/* 查看详情 */}
      <Dialog open={viewDialog.open} onClose={() => setViewDialog({ open: false, data: null })} maxWidth="md" fullWidth fullScreen={isMobile}>
        <DialogTitle>证书详情：{viewDialog.data?.name}</DialogTitle>
        <DialogContent dividers>
          {(() => {
            const d = viewDialog.data;
            if (!d) return null;
            const rows = [
              { label: '名称', value: d.name },
              { label: '类型', value: certTypeLabel(d.type) },
              { label: '主题', value: subjectLines(d.subject), pre: true },
              { label: '颁发者', value: subjectLines(d.issuer), pre: true },
              { label: '序列号', value: d.serial_number, mono: true },
              { label: '生效时间', value: formatTime(d.not_before) },
              { label: '失效时间', value: formatTime(d.not_after) },
              { label: '证书 SHA-256', value: d.cert_sha256, mono: true },
              { label: '公钥 SHA-256', value: d.public_key_sha256, mono: true },
              { label: '密钥', value: d.has_key ? d.key_type || '已有私钥' : '无私钥' },
              { label: '备注', value: d.description || '-' },
              { label: '创建时间', value: formatTime(d.created_at) },
            ];
            return rows.map((row) => (
              <Box
                key={row.label}
                sx={{
                  display: 'flex',
                  flexDirection: isMobile ? 'column' : 'row',
                  gap: isMobile ? 0.5 : 2,
                  marginBottom: isMobile ? 1.5 : 1,
                  alignItems: 'flex-start',
                }}
              >
                <Typography
                  variant="body2"
                  color="textSecondary"
                  sx={{ minWidth: isMobile ? 0 : 120, flexShrink: 0 }}
                >
                  {row.label}:
                </Typography>
                <Typography
                  variant="body2"
                  sx={{
                    whiteSpace: row.pre ? 'pre-wrap' : 'normal',
                    wordBreak: 'break-all',
                    fontFamily: row.mono ? 'monospace' : 'inherit',
                  }}
                >
                  {row.value || '-'}
                </Typography>
              </Box>
            ));
          })()}
        </DialogContent>
        <DialogActions>
          <Button
            startIcon={<DownloadIcon />}
            onClick={() => handleDownload(viewDialog.data, 'cert')}
            disabled={!viewDialog.data}
          >
            下载证书
          </Button>
          <Button
            startIcon={<DownloadIcon />}
            onClick={() => handleDownload(viewDialog.data, 'key')}
            disabled={!viewDialog.data?.has_key}
          >
            下载私钥
          </Button>
          <Button onClick={() => setViewDialog({ open: false, data: null })}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 主题字段详情 */}
      <Dialog
        open={subjectDialog.open}
        onClose={() => setSubjectDialog({ open: false, cert: null })}
        maxWidth="xs"
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>证书主题：{subjectDialog.cert?.name}</DialogTitle>
        <DialogContent>
          {parseSubject(subjectDialog.cert?.subject).length === 0 ? (
            <Typography variant="body2">-</Typography>
          ) : (
            parseSubject(subjectDialog.cert?.subject).map((f, i) => (
              <Typography
                key={i}
                variant="body2"
                sx={{ fontFamily: 'monospace', wordBreak: 'break-all', marginBottom: 0.5 }}
              >
                {f.key ? `${f.key} = ${f.value}` : f.value}
              </Typography>
            ))
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setSubjectDialog({ open: false, cert: null })}>关闭</Button>
        </DialogActions>
      </Dialog>

      {/* 生成 CA / 签发证书 */}
      {(() => {
        const isSign = signDialog.open;
        const open = genCADialog || signDialog.open;
        const title = isSign
          ? (signDialog.certType === CERT_TYPE_SERVER ? '签发服务器证书' : '签发客户端证书')
          : '生成新 CA';
        const submit = isSign ? handleSign : handleGenerateCA;
        const close = () => {
          if (isSign) setSignDialog({ open: false, certType: CERT_TYPE_SERVER });
          else setGenCADialog(false);
          setDialogError('');
        };
        return (
          <Dialog open={open} onClose={close} maxWidth="md" fullWidth fullScreen={isMobile}>
            <DialogTitle>{title}</DialogTitle>
            <DialogContent sx={{ paddingTop: 2 }}>
              {dialogError && <Alert severity="error" sx={{ marginBottom: 2 }}>{dialogError}</Alert>}
              <TextField fullWidth margin="normal" label="名称" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
              <TextField fullWidth margin="normal" label="Common Name (CN)" required value={form.common_name} onChange={(e) => setForm({ ...form, common_name: e.target.value })} />
              <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2 }}>
                <TextField margin="normal" label="组织 (O)" value={form.org} onChange={(e) => setForm({ ...form, org: e.target.value })} sx={{ minWidth: 220 }} />
                <TextField margin="normal" label="组织单位 (OU)" value={form.organizational_unit} onChange={(e) => setForm({ ...form, organizational_unit: e.target.value })} sx={{ minWidth: 220 }} />
                <TextField margin="normal" label="国家 (C)" value={form.country} onChange={(e) => setForm({ ...form, country: e.target.value })} sx={{ minWidth: 120 }} />
                <TextField margin="normal" label="省/州 (ST)" value={form.province} onChange={(e) => setForm({ ...form, province: e.target.value })} sx={{ minWidth: 160 }} />
                <TextField margin="normal" label="城市 (L)" value={form.locality} onChange={(e) => setForm({ ...form, locality: e.target.value })} sx={{ minWidth: 160 }} />
                <TextField margin="normal" label="邮箱 (emailAddress)" value={form.email_address} onChange={(e) => setForm({ ...form, email_address: e.target.value })} sx={{ minWidth: 220 }} />
                <TextField margin="normal" label="有效期(天)" type="number" value={form.days} onChange={(e) => setForm({ ...form, days: Number(e.target.value) })} sx={{ minWidth: 140 }} />
                <KeyOptionsFields value={keyOptions} onChange={setKeyOptions} />
              </Box>
              
              <TextField fullWidth margin="normal" label="备注" value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} />
            </DialogContent>
            <DialogActions>
              <Button onClick={close} disabled={busy}>取消</Button>
              <Button onClick={submit} variant="contained" disabled={busy || !form.name || !form.common_name}>
                {busy ? <CircularProgress size={24} /> : '确定'}
              </Button>
            </DialogActions>
          </Dialog>
        );
      })()}

      {/* 导入证书 */}
      <Dialog
        open={importDialog.open}
        onClose={() => { setImportDialog({ open: false, certType: CERT_TYPE_SERVER }); setDialogError(''); }}
        maxWidth="md"
        fullWidth
       fullScreen={isMobile}>
        <DialogTitle>{isLevel2 ? '导入证书' : '导入 CA 证书'}</DialogTitle>
        <DialogContent sx={{ paddingTop: 2 }}>
          {dialogError && <Alert severity="error" sx={{ marginBottom: 2 }}>{dialogError}</Alert>}
          <TextField
            fullWidth
            margin="normal"
            label="名称"
            required
            value={importForm.name}
            onChange={(e) => setImportForm({ ...importForm, name: e.target.value })}
          />
          {isLevel2 && (
            <FormControl margin="normal" sx={{ minWidth: 200 }}>
              <InputLabel>证书类型</InputLabel>
              <Select
                value={importForm.cert_type}
                label="证书类型"
                onChange={(e) => setImportForm({ ...importForm, cert_type: Number(e.target.value) })}
              >
                <MenuItem value={CERT_TYPE_SERVER}>服务器证书</MenuItem>
                <MenuItem value={CERT_TYPE_CLIENT}>客户端证书</MenuItem>
              </Select>
            </FormControl>
          )}
          <TextField
            fullWidth
            margin="normal"
            label="证书 (PEM)"
            required
            multiline
            rows={8}
            value={importForm.cert}
            onChange={(e) => setImportForm({ ...importForm, cert: e.target.value })}
            placeholder="-----BEGIN CERTIFICATE-----\n...\n-----END CERTIFICATE-----"
            sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
          />
          <TextField
            fullWidth
            margin="normal"
            label="私钥 (PEM，可选)"
            multiline
            rows={8}
            value={importForm.key}
            onChange={(e) => setImportForm({ ...importForm, key: e.target.value })}
            placeholder="-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----"
            sx={{ '& textarea': { fontFamily: 'monospace', fontSize: 12 } }}
          />
          <TextField
            fullWidth
            margin="normal"
            label="备注"
            value={importForm.description}
            onChange={(e) => setImportForm({ ...importForm, description: e.target.value })}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setImportDialog({ open: false, certType: CERT_TYPE_SERVER }); setDialogError(''); }} disabled={busy}>
            取消
          </Button>
          <Button onClick={handleImport} variant="contained" disabled={busy || !importForm.name || !importForm.cert}>
            {busy ? <CircularProgress size={24} /> : '导入'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Certificates;

import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import {
    Box,
    Typography,
    Paper,
    Button,
    Dialog,
    DialogTitle,
    DialogContent,
    FormControl,
    InputLabel,
    Select,
    MenuItem,
    CircularProgress,
    Alert,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    Stack,
    Card,
    CardContent,
    TextField,
    Chip,
    Tabs,
    Tab,
    Pagination,
    useMediaQuery,
    useTheme,
    Modal,
    Tooltip,
} from '@mui/material';
import client from '../api/client';
import { eventAPI, serverAPI } from '../api';

// 事件类型映射
const EVENT_TYPE_MAP = {
    1: { label: '客户端上线', color: 'success' },
    2: { label: '客户端下线', color: 'warning' },
    3: { label: '认证失败', color: 'error' },
    4: { label: '认证成功', color: 'success' },
    5: { label: '服务端启动成功', color: 'success' },
    6: { label: '服务端停止成功', color: 'warning' },
    7: { label: '服务端启动失败', color: 'error' },
    8: { label: '服务端启动失败', color: 'error' },
    9: { label: '添加用户ACL', color: 'info' },
    10: { label: '删除用户ACL', color: 'info' },
    11: { label: '一次认证成功', color: 'success' },
    12: { label: '二次认证成功', color: 'success' },
    13: { label: '二次认证下线', color: 'warning' },
    14: { label: '一次认证下线', color: 'warning' },
    15: { label: '服务器已恢复', color: 'success' },
};

// 格式化字节大小
const formatBytes = (bytes) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return Math.round((bytes / Math.pow(k, i)) * 100) / 100 + ' ' + sizes[i];
};

const Logs = () => {
    const theme = useTheme();
    const isMobile = useMediaQuery(theme.breakpoints.down('md'));
    const initializedRef = useRef(false);
    const hasLoadedRef = useRef(new Map());
    const eventLoadingRef = useRef(false);
    const [servers, setServers] = useState([]);
    const [selectedServer, setSelectedServer] = useState(null);
    const [serverDialogOpen, setServerDialogOpen] = useState(true); // 修改: 默认为true，使对话框自动打开
    const [logType, setLogType] = useState('status'); // 'status', 'server', 'script' 或 'event'
    const [tabValue, setTabValue] = useState(0); // 0: status, 1: logs, 2: events
    const [maxLines, setMaxLines] = useState(200);
    const [logs, setLogs] = useState([]); // { lineNumber: number, content: string }[]
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [info, setInfo] = useState('');
    const [lastLoadDirection, setLastLoadDirection] = useState(null); // 用于记录最后一次加载的方向

    // 状态相关
    const [statusData, setStatusData] = useState(null);
    const [killLoading, setKillLoading] = useState(null); // 记录正在断开连接的客户端地址
    const [aclModalOpen, setAclModalOpen] = useState(false);
    const [selectedAclContent, setSelectedAclContent] = useState('');

    // 用于跟踪日志行数范围
    const logsRangeRef = useRef({ startLine: 0, endLine: 0 });
    const tableContainerRef = useRef(null);
    const targetLineNumberRef = useRef(null); // 用于记录加载更早日志后需要滚动到的行号

    // 事件相关状态
    const [events, setEvents] = useState([]);
    const [eventLoading, setEventLoading] = useState(false);
    const [eventStartDate, setEventStartDate] = useState(new Date(Date.now() - 2 * 24 * 60 * 60 * 1000).toISOString().split('T')[0]); // 修改: 默认为前天
    const [eventEndDate, setEventEndDate] = useState(new Date(Date.now() + 1 * 24 * 60 * 60 * 1000).toISOString().split('T')[0]); // 修改: 默认为明天
    const [eventIPFilter, setEventIPFilter] = useState('');
    const [eventPage, setEventPage] = useState(1);
    const [eventPageSize, setEventPageSize] = useState(20);
    const [eventTotal, setEventTotal] = useState(0);

    // 获取服务器列表
    useEffect(() => {
        if (!initializedRef.current) {
            initializedRef.current = true;
            const fetchServers = async () => {
                try {
                    const response = await client.get('/server/list');
                    setServers(response.data.data || []);
                } catch (err) {
                    setError('加载服务器列表失败：' + (err.response?.data?.error || err.message));
                }
            };
            fetchServers();
        }
    }, []);

    // 选择服务器
    const handleSelectServer = (server) => {
        setSelectedServer(server);
        setServerDialogOpen(false);
        setLogs([]);
        logsRangeRef.current = { startLine: 0, endLine: 0 };
        setInfo('');
        setStatusData(null);
    };

    // 获取服务器状态
    const loadServerStatus = async (serverToUse = selectedServer) => {
        if (!serverToUse) return;

        try {
            setLoading(true);
            setError(null);
            
            const response = await client.get('/server/status', {
                params: {
                    id: serverToUse.id
                }
            });

            if (response.data.result === 'success') {
                setStatusData(response.data.data);
            } else {
                setError('获取状态失败：' + (response.data.error || '未知错误'));
            }
        } catch (err) {
            setError('获取状态失败：' + (err.response?.data?.error || err.message));
            console.error('获取状态错误:', err);
        } finally {
            setLoading(false);
        }
    };

    // 断开客户端连接
    const handleKillClient = async (realIpAddr) => {
        if (!selectedServer) return;

        if (!window.confirm(`确定要断开客户端连接 "${realIpAddr}" 吗？`)) {
            return;
        }

        try {
            setKillLoading(realIpAddr);
            setError(null);
            
            const response = await client.post('/server/client/kill', {
                id: selectedServer.id,
                read_ip_addr: realIpAddr
            });

            if (response.data.result === 'success') {
                // 刷新状态数据
                await loadServerStatus();
            } else {
                setError('断开连接失败：' + (response.data.error || '未知错误'));
            }
        } catch (err) {
            setError('断开连接失败：' + (err.response?.data?.error || err.message));
            console.error('断开连接错误:', err);
        } finally {
            setKillLoading(null);
        }
    };

    // 加载日志
    const loadLogs = async (direction = 'initial', serverToUse = selectedServer) => {
        if (!serverToUse) return;
        
        // 如果是事件查看模式，不调用此接口
        if (logType === 'event' ) return;

        try {
            setLoading(true);
            setError(null);

            let startLine = logsRangeRef.current.startLine;
            let endLine = logsRangeRef.current.endLine;

            if (direction === 'initial') {
                startLine = 0;
                endLine = 0;
            } else if (direction === 'earlier') {
                // 加载更早的日志
                // 如果第一行日志是1，不能加载更早的日志
                if (logs.length === 0 || logs[0].lineNumber <= 1) {
                    setLoading(false);
                    return;
                }
                // 记录当前第一条日志的行号，用于加载后滚动回该位置
                targetLineNumberRef.current = logs[0].lineNumber;
                const firstLineNumber = logs[0].lineNumber;
                startLine = Math.max(0, firstLineNumber - 1 - Math.floor(maxLines / 2));
                endLine = firstLineNumber - 1;
            } else if (direction === 'newer') {
                // 加载更新的日志
                if (logs.length === 0) {
                    setLoading(false);
                    return;
                }
                const lastLineNumber = logs[logs.length - 1].lineNumber;
                startLine = lastLineNumber + 1;
                endLine = lastLineNumber + Math.floor(maxLines / 2);
            }

            const response = await client.get('/server/log', {
                params: {
                    id: serverToUse.id,
                    type: logType,
                    start_line: startLine,
                    end_line: endLine,
                },
            });

            const { content, start_line, end_line } = response.data.data;
            const newLogs = content
                .split('\n')
                .filter((line) => line.trim())
                .map((line, index) => ({
                    lineNumber: start_line + index,
                    content: line,
                }));

            // 如果没有返回新日志，说明已经到底或到顶
            if (newLogs.length === 0) {
                setLoading(false);
                return;
            }

            // 更新日志范围
            logsRangeRef.current = { startLine: start_line, endLine: end_line };

            // 合并日志并管理数量
            let mergedLogs = [...logs];
            if (direction === 'initial') {
                mergedLogs = newLogs;
            } else if (direction === 'earlier') {
                mergedLogs = [...newLogs, ...logs];
            } else if (direction === 'newer') {
                mergedLogs = [...logs, ...newLogs];
            }

            // 确保不超过最大显示数量
            if (mergedLogs.length > maxLines) {
                if (direction === 'earlier' || direction === 'initial') {
                    // 删除末尾的日志
                    mergedLogs = mergedLogs.slice(0, maxLines);
                    // 更新endLine，删除了末尾的日志
                    if (mergedLogs.length > 0) {
                        logsRangeRef.current.endLine = mergedLogs[mergedLogs.length - 1].lineNumber;
                    }
                } else if (direction === 'newer') {
                    // 删除开头的日志
                    mergedLogs = mergedLogs.slice(mergedLogs.length - maxLines);
                    // 更新startLine，删除了开头的日志
                    if (mergedLogs.length > 0) {
                        logsRangeRef.current.startLine = mergedLogs[0].lineNumber;
                    }
                }
            }

            setLogs(mergedLogs);
            setInfo(`共显示 ${mergedLogs.length} 条日志（最多 ${maxLines} 条）`);
            setLastLoadDirection(direction);
            setLoading(false);
        } catch (err) {
            setError('加载日志失败：' + (err.response?.data?.error || err.message));
            setLoading(false);
        }
    };

    // 刷新日志
    const handleRefresh = () => {
        if (selectedServer) {
            loadLogs('initial', selectedServer);
        }
    };

    // 清除日志
    const handleClear = async () => {
        if (!selectedServer) return;

        if (!window.confirm(`确定要清除服务器 "${selectedServer.name}" 的${logType === 'server' ? '服务器' : '脚本'}日志吗？`)) {
            return;
        }

        try {
            setLoading(true);
            setError(null);
            
            const logTypeMap = {
                'server': 'server',
                'script': 'script'
            };

            const response = await client.post('/server/log/clear', {
                id: selectedServer.id,
                log_type: logTypeMap[logType]
            });

            if (response.data.result === 'success' || response.status === 200) {
                setLogs([]);
                logsRangeRef.current = { startLine: 0, endLine: 0 };
                setInfo('日志已清除');
            }
        } catch (err) {
            setError('清除日志失败：' + (err.response?.data?.error || err.message));
            console.error('清除日志错误:', err);
        } finally {
            setLoading(false);
        }
    };

    // 处理最大显示数量变化
    const handleMaxLinesChange = (newMaxLines) => {
        setMaxLines(newMaxLines);
        if (logs.length > newMaxLines) {
            // 保留最新的日志
            const newLogs = logs.slice(logs.length - newMaxLines);
            setLogs(newLogs);
            // 同步更新日志范围
            if (newLogs.length > 0) {
                logsRangeRef.current.startLine = newLogs[0].lineNumber;
                logsRangeRef.current.endLine = newLogs[newLogs.length - 1].lineNumber;
            }
        }
    };

    // 当logType变动时立即刷新
    useEffect(() => {
        if (!selectedServer) return;
        
        const key = `${selectedServer.id}-${logType}`;
        if (hasLoadedRef.current.has(key)) return;
        hasLoadedRef.current.set(key, true);
        
        if (logType === 'status') {
            loadServerStatus();
        } else if (logType === 'event') {
            // 切换到事件时，自动加载事件列表
            loadEvents(1, 20);
        } else {
            // 切换到日志时，自动加载初始日志
            loadLogs('initial', selectedServer);
        }
    }, [logType, selectedServer]);

    // 加载更早日志后，滚动到之前的第一条日志位置
    useEffect(() => {
        if (lastLoadDirection === 'earlier' && targetLineNumberRef.current && logs.length > 0) {
            // 查找目标行在新日志中的位置
            const targetIndex = logs.findIndex(log => log.lineNumber === targetLineNumberRef.current);
            if (targetIndex !== -1) {
                // 计算该行在 DOM 中的位置并滚动
                setTimeout(() => {
                    const rows = document.querySelectorAll('table tbody tr');
                    if (rows[targetIndex]) {
                        rows[targetIndex].scrollIntoView({ behavior: 'auto', block: 'start' });
                    }
                    targetLineNumberRef.current = null;
                    setLastLoadDirection(null);
                }, 0);
            }
        }
    }, [logs, lastLoadDirection]);

    // 回到顶部
    const scrollToTop = () => {
        window.scrollTo({ top: 0, behavior: 'smooth' });
    };

    // 回到底部
    const scrollToBottom = () => {
        window.scrollTo({ top: document.documentElement.scrollHeight, behavior: 'smooth' });
    };

    // 加载事件列表
    const loadEvents = async (page = 1, eventPageSize = 20) => {
        if (!selectedServer || eventLoadingRef.current) return;

        eventLoadingRef.current = true;
        try {
            setEventLoading(true);
            setError(null);
            
            // 将日期转换为时间戳
            const startDate = new Date(eventStartDate);
            const endDate = new Date(eventEndDate);
            endDate.setHours(23, 59, 59, 999); // 设置为当天结束时间
            
            const startTime = Math.floor(startDate.getTime() / 1000);
            const endTime = Math.floor(endDate.getTime() / 1000);
            
            const response = await eventAPI.getEventList(selectedServer.id, startTime, endTime, eventIPFilter, page, eventPageSize);
            if (response.data.data) {
                // 服务器端直接返回分页数据
                setEvents(response.data.data);
                setEventTotal(response.data.total || 0);
                setEventPage(page);
            }
        } catch (err) {
            setError('加载事件列表失败：' + (err.response?.data?.error || err.message));
        } finally {
            setEventLoading(false);
            eventLoadingRef.current = false;
        }
    };

    // 清空事件
    const handleClearEvents = async () => {
        if (!selectedServer) return;

        if (!window.confirm(`确定要清空服务器 "${selectedServer.name}" 的所有事件吗？此操作不可恢复。`)) {
            return;
        }

        try {
            setEventLoading(true);
            setError(null);
            
            const response = await eventAPI.clearEvents(selectedServer.id);
            if (response.data.result === 'success' || response.status === 200) {
                setEvents([]);
                setEventTotal(0);
                setEventPage(1);
                setError(null);
                // 显示成功消息（可选）
                setTimeout(() => {
                    setError('');
                }, 2000);
            }
        } catch (err) {
            setError('清空事件失败：' + (err.response?.data?.error || err.message));
        } finally {
            setEventLoading(false);
        }
    };

    // 稳定化事件过滤处理器
    const handleEventStartDateChange = useCallback((e) => {
        setEventStartDate(e.target.value);
    }, []);

    const handleEventEndDateChange = useCallback((e) => {
        setEventEndDate(e.target.value);
    }, []);

    const handleEventIPFilterChange = useCallback((e) => {
        setEventIPFilter(e.target.value);
    }, []);

    const handleLoadEvents = useCallback(() => {
        loadEvents(1, 20);
    }, [loadEvents]);

    return (
        <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
            <Typography variant="h5" sx={{ marginBottom: 3 }}>
                日志/状态
            </Typography>

            {error && (
                <Alert severity="error" sx={{ marginBottom: 2 }} onClose={() => setError(null)}>
                    {error}
                </Alert>
            )}

            {/* 控制栏 */}
            <Paper sx={{ padding: 1.5, marginBottom: 2 }}>
                <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1} sx={{ alignItems: 'flex-start' }}>
                    {/* 选择服务器 */}
                    <Button
                        variant="outlined"
                        size="small"
                        onClick={() => setServerDialogOpen(true)}
                        sx={{ height: "40px" }}
                    >
                        {selectedServer ? `服务器: ${selectedServer.name}` : '选择服务器'}
                    </Button>

                    {/* 日志类别 */}
                    <FormControl sx={{ minWidth: 130 }} size="small">
                        <InputLabel>查看类别</InputLabel>
                        <Select value={logType} label="查看类别" onChange={(e) => {
                            setLogType(e.target.value);
                            if (e.target.value === 'event' && selectedServer) {
                                loadEvents(1, 20);
                            }
                        }}>
                            <MenuItem value="status">状态</MenuItem>
                            <MenuItem value="server">服务器日志</MenuItem>
                            <MenuItem value="script">脚本日志</MenuItem>
                            <MenuItem value="event">事件</MenuItem>
                        </Select>
                    </FormControl>

                    {logType !== 'status' && logType !== 'event' && (
                        <>
                            {/* 最多显示数量 */}
                            <FormControl sx={{ minWidth: 110 }} size="small">
                                <InputLabel>显示数量</InputLabel>
                                <Select value={maxLines} label="显示数量" onChange={(e) => handleMaxLinesChange(e.target.value)}>
                                    <MenuItem value={200}>200</MenuItem>
                                    <MenuItem value={500}>500</MenuItem>
                                    <MenuItem value={1000}>1000</MenuItem>
                                    <MenuItem value={10000}>10000</MenuItem>
                                    <MenuItem value={20000}>20000</MenuItem>
                                </Select>
                            </FormControl>

                            {/* 刷新按钮 */}
                            <Button
                                variant="outlined"
                                size="small"
                                sx={{ height: "40px" }}
                                onClick={handleRefresh}
                                disabled={!selectedServer || loading}
                            >
                                🔄 刷新
                            </Button>

                            {/* 清除按钮 */}
                            <Button
                                variant="outlined"
                                color="error"
                                size="small"
                                sx={{ height: "40px" }}
                                onClick={handleClear}
                                disabled={logs.length === 0}
                            >
                                🗑️ 清除
                            </Button>
                        </>
                    )}

                    {logType === 'event' && (
                        <>
                            <TextField
                                type="date"
                                value={eventStartDate}
                                onChange={handleEventStartDateChange}
                                size="small"
                                label="开始日期"
                                InputLabelProps={{ shrink: true }}
                            />
                            <TextField
                                type="date"
                                value={eventEndDate}
                                onChange={handleEventEndDateChange}
                                size="small"
                                label="结束日期"
                                InputLabelProps={{ shrink: true }}
                            />
                            <TextField
                                placeholder="搜索事件"
                                value={eventIPFilter}
                                onChange={handleEventIPFilterChange}
                                size="small"
                                sx={{ minWidth: 120 }}
                            />
                            <Button
                                variant="outlined"
                                size="small"
                                sx={{ height: "40px" }}
                                onClick={handleLoadEvents}
                                disabled={!selectedServer || eventLoading}
                            >
                                🔍 查询
                            </Button>
                            <Button
                                variant="outlined"
                                color="error"
                                size="small"
                                sx={{ height: "40px" }}
                                onClick={handleClearEvents}
                                disabled={!selectedServer || eventLoading || events.length === 0}
                            >
                                🗑️ 清空
                            </Button>
                        </>
                    )}

                    {logType === 'status' && (
                        <Button
                            variant="outlined"
                            size="small"
                            sx={{ height: "40px" }}
                            onClick={() => loadServerStatus()}
                            disabled={!selectedServer || loading}
                        >
                            🔄 刷新
                        </Button>
                    )}
                </Stack>
            </Paper>

            {/* 内容区域 */}
            {logType === 'status' ? (
                // 状态页面
                selectedServer ? (
                    <Paper sx={{ padding: 2 }}>
                        {loading ? (
                            <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
                                <CircularProgress />
                            </Box>
                        ) : statusData ? (
                            <Box>
                                {/* 版本信息 */}
                                <Box sx={{ marginBottom: 3 }}>
                                    <Typography variant="h6" sx={{ marginBottom: 1 }}>
                                        服务器信息
                                    </Typography>
                                    <Stack spacing={1}>
                                        <Typography variant="body2">
                                            <strong>服务器版本：</strong> {statusData.version || 'N/A'}
                                        </Typography>
                                        <Typography variant="body2">
                                            <strong>管理接口版本：</strong> {statusData.management_version || 'N/A'}
                                        </Typography>
                                    </Stack>
                                </Box>

                                {/* 客户端列表 */}
                                <Box>
                                    <Typography variant="h6" sx={{ marginBottom: 1 }}>
                                        客户端连接列表 ({statusData.ClientList?.length || 0})
                                    </Typography>
                                    {statusData.ClientList && statusData.ClientList.length > 0 ? (
                                        isMobile ? (
                                            // 移动设备：卡片布局
                                            <Stack spacing={1.5}>
                                                {statusData.ClientList.map((client, index) => (
                                                    <Card key={index} sx={{ padding: 2 }}>
                                                        <CardContent sx={{ padding: 0, '&:last-child': { paddingBottom: 0 } }}>
                                                            <Stack spacing={1.5}>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">证书名称</Typography>
                                                                    <Typography variant="body2">{client.common_name}</Typography>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">真实地址</Typography>
                                                                    <Typography variant="body2">{client.real_ip_addr}</Typography>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">虚拟地址</Typography>
                                                                    <Typography variant="body2">{client.virtual_ip_addr?.join(', ') || 'N/A'}</Typography>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">数据流量</Typography>
                                                                    <Box sx={{ display: 'flex', justifyContent: 'space-between', marginTop: 0.5 }}>
                                                                        <Typography variant="body2">↑ {formatBytes(client.last_byte_sent)}</Typography>
                                                                        <Typography variant="body2">↓ {formatBytes(client.last_byte_received)}</Typography>
                                                                    </Box>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">连接时间</Typography>
                                                                    <Typography variant="body2">{client.connected_since}</Typography>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">用户名</Typography>
                                                                    <Typography variant="body2">{client.username || 'N/A'}</Typography>
                                                                </Box>
                                                                <Box>
                                                                    <Typography variant="caption" color="textSecondary">ACL信息</Typography>
                                                                    {client.acl_list && client.acl_list.length > 0 ? (
                                                                        <Box sx={{ position: 'relative', marginTop: 0.5 }}>
                                                                            <Tooltip 
                                                                                title={
                                                                                    <Box sx={{ whiteSpace: 'pre-wrap', maxHeight: '300px', overflow: 'auto' }}>
                                                                                        {client.acl_list.join('\n')}
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
                                                                                    }}
                                                                                    onClick={() => {
                                                                                        setSelectedAclContent(client.acl_list.join('\n'));
                                                                                        setAclModalOpen(true);
                                                                                    }}
                                                                                >
                                                                                    {client.acl_list[0]}
                                                                                </Typography>
                                                                            </Tooltip>
                                                                        </Box>
                                                                    ) : (
                                                                        <Typography variant="body2">-</Typography>
                                                                    )}
                                                                </Box>
                                                                <Button
                                                                    variant="contained"
                                                                    color="error"
                                                                    size="small"
                                                                    fullWidth
                                                                    onClick={() => handleKillClient(client.real_ip_addr)}
                                                                    disabled={killLoading === client.real_ip_addr}
                                                                >
                                                                    {killLoading === client.real_ip_addr ? (
                                                                        <CircularProgress size={20} />
                                                                    ) : (
                                                                        '断开连接'
                                                                    )}
                                                                </Button>
                                                            </Stack>
                                                        </CardContent>
                                                    </Card>
                                                ))}
                                            </Stack>
                                        ) : (
                                            <TableContainer>
                                                <Table size="small">
                                                    <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                                                        <TableRow>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>证书名称</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>真实地址</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>虚拟地址</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }} align="right">发送/接收</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>连接时间</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>用户名</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>ACL信息</TableCell>
                                                            <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }} align="center">操作</TableCell>
                                                        </TableRow>
                                                    </TableHead>
                                                    <TableBody>
                                                        {statusData.ClientList.map((client, index) => (
                                                            <TableRow key={index} hover>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.common_name}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.real_ip_addr}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.virtual_ip_addr?.join(', ') || 'N/A'}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }} align="right">
                                                                    <Typography variant="body2">
                                                                        ↑ {formatBytes(client.last_byte_sent)}
                                                                    </Typography>
                                                                    <Typography variant="body2">
                                                                        ↓ {formatBytes(client.last_byte_received)}
                                                                    </Typography>
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.connected_since}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.username || 'N/A'}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                                    {client.acl_list && client.acl_list.length > 0 ? (
                                                                        <Box sx={{ position: 'relative' }}>
                                                                            <Tooltip 
                                                                                title={
                                                                                    <Box sx={{ whiteSpace: 'pre-wrap', maxHeight: '300px', overflow: 'auto' }}>
                                                                                        {client.acl_list.join('\n')}
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
                                                                                        maxWidth: '150px',
                                                                                        overflow: 'hidden',
                                                                                        textOverflow: 'ellipsis',
                                                                                        whiteSpace: 'nowrap'
                                                                                    }}
                                                                                    onClick={() => {
                                                                                        setSelectedAclContent(client.acl_list.join('\n'));
                                                                                        setAclModalOpen(true);
                                                                                    }}
                                                                                >
                                                                                    {client.acl_list[0]}
                                                                                </Typography>
                                                                            </Tooltip>
                                                                        </Box>
                                                                    ) : (
                                                                        '-'
                                                                    )}
                                                                </TableCell>
                                                                <TableCell sx={{ paddingTop: 0.5, paddingBottom: 0.5 }} align="center">
                                                                    <Button
                                                                        variant="contained"
                                                                        color="error"
                                                                        size="small"
                                                                        onClick={() => handleKillClient(client.real_ip_addr)}
                                                                        disabled={killLoading === client.real_ip_addr}
                                                                        sx={{ minWidth: '80px' }}
                                                                    >
                                                                        {killLoading === client.real_ip_addr ? (
                                                                            <CircularProgress size={20} />
                                                                        ) : (
                                                                            '断开'
                                                                        )}
                                                                    </Button>
                                                                </TableCell>
                                                            </TableRow>
                                                        ))}
                                                    </TableBody>
                                                </Table>
                                            </TableContainer>
                                        )
                                    ) : (
                                        <Typography color="textSecondary">暂无客户端连接</Typography>
                                    )}
                                </Box>
                            </Box>
                        ) : (
                            <Typography color="textSecondary" align="center" sx={{ paddingTop: 3 }}>
                                暂无状态信息
                            </Typography>
                        )}
                    </Paper>
                ) : (
                    <Paper sx={{ padding: 3, textAlign: 'center' }}>
                        <Typography color="textSecondary">请先选择服务器</Typography>
                    </Paper>
                )
            ) : logType !== 'event' ? (
                // 日志页面
                selectedServer ? (
                    <Paper sx={{ padding: 0, position: 'relative', minHeight: '400px' }}>
                        {logs.length === 0 && !loading ? (
                            <Typography color="textSecondary" align="center" sx={{ paddingTop: 3 }}>
                                暂无日志
                            </Typography>
                        ) : (
                            <>
                                {/* 加载更早的日志按钮 */}
                                <Box sx={{ textAlign: 'center', marginBottom: 1, minHeight: '32px' }}>
                                    {loading ? (
                                        <CircularProgress size={24} />
                                    ) : logs.length > 0 && logs[0].lineNumber > 1 ? (
                                        <Button
                                            variant="text"
                                            size="small"
                                            onClick={() => loadLogs('earlier')}
                                            disabled={loading || logs[0].lineNumber <= 1}
                                        >
                                            加载更早的日志
                                        </Button>
                                    ) : null}
                                </Box>

                                {/* 日志表格 */}
                                {logs.length > 0 && (
                                    <TableContainer ref={tableContainerRef} sx={{ marginBottom: 1 }}>
                                        <Table size="small">
                                            <TableHead sx={{ position: 'sticky', top: 0, backgroundColor: '#f5f5f5' }}>
                                                <TableRow>
                                                    <TableCell sx={{ fontWeight: 'bold', width: '80px', paddingTop: 0.5, paddingBottom: 0.5 }}>行号</TableCell>
                                                    <TableCell sx={{ fontWeight: 'bold', paddingTop: 0.5, paddingBottom: 0.5 }}>日志内容</TableCell>
                                                </TableRow>
                                            </TableHead>
                                            <TableBody>
                                                {logs.map((log) => (
                                                    <TableRow key={log.lineNumber} hover>
                                                        <TableCell sx={{ width: '80px', fontFamily: 'monospace', fontSize: '0.85em', paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                            {log.lineNumber}
                                                        </TableCell>
                                                        <TableCell sx={{ fontFamily: 'monospace', fontSize: '0.85em', whiteSpace: 'pre-wrap', paddingTop: 0.5, paddingBottom: 0.5 }}>
                                                            {log.content}
                                                        </TableCell>
                                                    </TableRow>
                                                ))}
                                            </TableBody>
                                        </Table>
                                    </TableContainer>
                                )}
                                
                                {/* 加载更新的日志按钮 */}
                                <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', marginTop: 1, minHeight: '32px', paddingX: 2, position: 'relative' }}>
                                    {logs.length > 0 && !loading && (
                                        <Button
                                            variant="text"
                                            size="small"
                                            onClick={() => loadLogs('newer')}
                                            disabled={loading}
                                        >
                                            加载更新的日志
                                        </Button>
                                    )}
                                    <Box sx={{ position: 'absolute', right: 16 }}>
                                        {info && (
                                            <Typography variant="caption" sx={{ color: 'textSecondary' }}>
                                                {info}
                                            </Typography>
                                        )}
                                    </Box>
                                </Box>
                            </>
                        )}

                        {/* 回到顶部和底部按钮 */}
                        {logs.length > 0 && (
                            <Box sx={{ position: 'fixed', bottom: 32, right: 32, display: 'flex', flexDirection: 'column', gap: 1 }}>
                                <Button
                                    variant="contained"
                                    size="small"
                                    onClick={scrollToTop}
                                    sx={{ minWidth: '40px', padding: '8px', borderRadius: '50%' }}
                                    title="回到顶部"
                                >
                                    ▲
                                </Button>
                                <Button
                                    variant="contained"
                                    size="small"
                                    onClick={scrollToBottom}
                                    sx={{ minWidth: '40px', padding: '8px', borderRadius: '50%' }}
                                    title="回到底部"
                                >
                                    ▼
                                </Button>
                            </Box>
                        )}
                    </Paper>
                ) : (
                    <Paper sx={{ padding: 3, textAlign: 'center' }}>
                        <Typography color="textSecondary">请先选择服务器</Typography>
                    </Paper>
                )
            ) : null}

            {logType === 'event' && (
                selectedServer ? (
                    <Paper sx={{ padding: 2 }}>
                        {eventLoading ? (
                            <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
                                <CircularProgress />
                            </Box>
                        ) : (
                            <>
                                <TableContainer>
                                    <Table>
                                        <TableHead sx={{ backgroundColor: '#f5f5f5' }}>
                                            <TableRow>
                                                <TableCell sx={{ fontWeight: 'bold' }}>时间</TableCell>
                                                <TableCell sx={{ fontWeight: 'bold' }}>类型</TableCell>
                                                <TableCell sx={{ fontWeight: 'bold' }}>对端地址</TableCell>
                                                <TableCell sx={{ fontWeight: 'bold' }}>事件数据</TableCell>
                                            </TableRow>
                                        </TableHead>
                                        <TableBody>
                                            {events.length > 0 ? (
                                                events.map((event) => {
                                                    const eventInfo = EVENT_TYPE_MAP[event.event_type] || { label: '未知', color: 'default' };
                                                    const eventTime = new Date(event.event_time * 1000).toLocaleString();
                                                    
                                                    return (
                                                        <TableRow key={event.id} hover>
                                                            <TableCell sx={{ paddingTop: 1, paddingBottom: 1 }}>{eventTime}</TableCell>
                                                            <TableCell sx={{ paddingTop: 1, paddingBottom: 1 }}>
                                                                <Chip
                                                                    label={eventInfo.label}
                                                                    size="small"
                                                                    color={eventInfo.color}
                                                                    variant="outlined"
                                                                />
                                                            </TableCell>
                                                            <TableCell sx={{ paddingTop: 1, paddingBottom: 1 }}>
                                                                {event.real_ip_addr || '-'}
                                                            </TableCell>
                                                            <TableCell sx={{ paddingTop: 1, paddingBottom: 1 }}>
                                                                {event.event_data}
                                                            </TableCell>
                                                        </TableRow>
                                                    );
                                                })
                                            ) : (
                                                <TableRow>
                                                    <TableCell colSpan={4} align="center" sx={{ paddingTop: 3, paddingBottom: 3 }}>
                                                        暂无事件数据
                                                    </TableCell>
                                                </TableRow>
                                            )}
                                        </TableBody>
                                    </Table>
                                </TableContainer>

                                {eventTotal > 0 && (
                                    <Stack direction="row" spacing={2} alignItems="center" sx={{ marginTop: 2, justifyContent: 'center' }}>
                                        <FormControl sx={{ minWidth: 110 }} size="small">
                                            <InputLabel>每页数量</InputLabel>
                                            <Select
                                                value={eventPageSize}
                                                label="每页数量"
                                                onChange={(e) => {
                                                    setEventPageSize(e.target.value);
                                                    setEventPage(1);
                                                    loadEvents(1, e.target.value);
                                                }}
                                            >
                                                <MenuItem value={10}>10</MenuItem>
                                                <MenuItem value={20}>20</MenuItem>
                                                <MenuItem value={50}>50</MenuItem>
                                                <MenuItem value={100}>100</MenuItem>
                                                <MenuItem value={200}>200</MenuItem>
                                                <MenuItem value={500}>500</MenuItem>
                                            </Select>
                                        </FormControl>
                                        <Typography variant="body2">
                                            总共 {eventTotal} 条事件
                                        </Typography>
                                        <Pagination
                                            count={Math.ceil(eventTotal / eventPageSize)} // 修改: 修正分页计算
                                            page={eventPage}
                                            onChange={(e, page) => {
                                                setEventPage(page);
                                                loadEvents(page, eventPageSize); // 修改: 分页改变时加载对应页的数据
                                            }}
                                            color="primary"
                                        />
                                    </Stack>
                                )}
                            </>
                        )}
                    </Paper>
                ) : (
                    <Paper sx={{ padding: 3, textAlign: 'center' }}>
                        <Typography color="textSecondary">请先选择服务器</Typography>
                    </Paper>
                )
            )}

            {/* 服务器选择对话框 */}
            <Dialog open={serverDialogOpen} onClose={() => setServerDialogOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>选择服务器</DialogTitle>
                <DialogContent>
                    <Stack spacing={1} sx={{ marginTop: 1 }}>
                        {servers.map((server) => (
                            <Button
                                key={server.id}
                                fullWidth
                                variant={selectedServer?.id === server.id ? 'contained' : 'outlined'}
                                onClick={() => handleSelectServer(server)}
                                sx={{ justifyContent: 'flex-start', padding: 2 }}
                            >
                                <Box sx={{ width: '100%', textAlign: 'left' }}>
                                    <Typography variant="subtitle2">{server.name}</Typography>
                                    <Typography variant="caption" color="textSecondary">
                                        {server.proto}/{server.port} | 运行状态: {server.running ? '运行中' : '未运行'}
                                    </Typography>
                                </Box>
                            </Button>
                        ))}
                    </Stack>
                </DialogContent>
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

export default Logs;

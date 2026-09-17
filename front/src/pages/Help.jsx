import React, { useState, useEffect } from 'react';
import {
  Box,
  Typography,
  Paper,
  CircularProgress,
  Alert,
} from '@mui/material';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';

const Help = () => {
  const [helpContent, setHelpContent] = useState('');
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');

  useEffect(() => {
    const fetchHelpContent = async () => {
      try {
        setLoading(true);
        const response = await fetch('/api/resource/get?id=help');
        if (!response.ok) {
          throw new Error('获取帮助信息失败');
        }
        const text = await response.text();
        setHelpContent(text);
        setError('');
      } catch (err) {
        console.error('加载帮助信息失败:', err);
        setError('加载帮助信息失败，请重试');
      } finally {
        setLoading(false);
      }
    };

    fetchHelpContent();
  }, []);

  const markdownComponents = {
    h1: ({ node, ...props }) => (
      <Typography variant="h4" sx={{ marginTop: 3, marginBottom: 2, fontWeight: 'bold' }} {...props} />
    ),
    h2: ({ node, ...props }) => (
      <Typography variant="h5" sx={{ marginTop: 2.5, marginBottom: 1.5, fontWeight: 'bold' }} {...props} />
    ),
    h3: ({ node, ...props }) => (
      <Typography variant="h6" sx={{ marginTop: 2, marginBottom: 1, fontWeight: 'bold' }} {...props} />
    ),
    h4: ({ node, ...props }) => (
      <Typography variant="subtitle1" sx={{ marginTop: 1.5, marginBottom: 1, fontWeight: 'bold' }} {...props} />
    ),
    p: ({ node, ...props }) => (
      <Typography variant="body2" sx={{ marginBottom: 1.5, lineHeight: 1.8 }} {...props} />
    ),
    code: ({ node, ...props }) => (
      <Typography
        component="code"
        sx={{
          backgroundColor: '#f0f0f0',
          padding: '2px 6px',
          borderRadius: '3px',
          fontFamily: 'monospace',
          fontSize: '0.9em',
        }}
        {...props}
      />
    ),
    pre: ({ node, ...props }) => (
      <Paper
        component="pre"
        sx={{
          backgroundColor: '#f5f5f5',
          padding: 2,
          marginY: 2,
          overflow: 'auto',
          border: '1px solid #e0e0e0',
          fontFamily: 'monospace',
          fontSize: '12px',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
        }}
        {...props}
      />
    ),
    li: ({ node, ...props }) => (
      <Box
        component="li"
        sx={{
          marginLeft: 2,
          marginBottom: 0.5,
        }}
      >
        <Typography variant="body2" {...props} />
      </Box>
    ),
    ul: ({ node, ...props }) => (
      <Box
        component="ul"
        sx={{
          listStyleType: 'disc',
          paddingLeft: 0,
        }}
        {...props}
      />
    ),
    ol: ({ node, ...props }) => (
      <Box
        component="ol"
        sx={{
          listStyleType: 'decimal',
          paddingLeft: 0,
        }}
        {...props}
      />
    ),
    blockquote: ({ node, ...props }) => (
      <Paper
        sx={{
          borderLeft: '4px solid #1976d2',
          paddingLeft: 2,
          marginY: 2,
          paddingY: 1,
          backgroundColor: '#f9f9f9',
        }}
        {...props}
      />
    ),
    hr: ({ node, ...props }) => (
      <Box
        sx={{
          borderTop: '1px solid #e0e0e0',
          marginY: 2,
        }}
      />
    ),
    table: ({ node, ...props }) => (
      <Paper
        component="table"
        sx={{
          width: '100%',
          borderCollapse: 'collapse',
          marginY: 2,
        }}
        {...props}
      />
    ),
    th: ({ node, ...props }) => (
      <Typography
        component="th"
        sx={{
          padding: 1,
          backgroundColor: '#f5f5f5',
          borderBottom: '1px solid #e0e0e0',
          fontWeight: 'bold',
          textAlign: 'left',
        }}
        {...props}
      />
    ),
    td: ({ node, ...props }) => (
      <Typography
        component="td"
        sx={{
          padding: 1,
          borderBottom: '1px solid #e0e0e0',
        }}
        {...props}
      />
    ),
    a: ({ node, ...props }) => (
      <Typography
        component="a"
        sx={{
          color: '#1976d2',
          textDecoration: 'none',
          '&:hover': {
            textDecoration: 'underline',
          },
        }}
        {...props}
      />
    ),
  };

  return (
    <Box sx={{ width: '100%', padding: { xs: 1, sm: 2, md: 3 } }}>
      {error && (
        <Alert severity="error" sx={{ marginBottom: 2 }}>
          {error}
        </Alert>
      )}

      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', padding: 3 }}>
          <CircularProgress />
        </Box>
      ) : (
        <Paper sx={{ padding: 3, backgroundColor: '#fafafa' }}>
          <Box sx={{ lineHeight: 1.8 }}>
            <ReactMarkdown components={markdownComponents} remarkPlugins={[remarkGfm]}>
              {helpContent}
            </ReactMarkdown>
          </Box>
        </Paper>
      )}
    </Box>
  );
};

export default Help;

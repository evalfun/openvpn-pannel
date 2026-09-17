import axios from 'axios';

const client = axios.create({
  baseURL: '/api',
  withCredentials: true, // 允许跨域请求时携带 cookies
  headers: {
    'Content-Type': 'application/json',
  },
});

// 请求拦截器 - 确保每个请求都携带 cookies
client.interceptors.request.use(
  (config) => {
    // 确保请求会发送 cookies
    config.withCredentials = true;
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// 响应拦截器 - 自动保存 cookies（由浏览器处理）
client.interceptors.response.use(
  (response) => {
    // cookies 会自动由浏览器保存和发送
    return response;
  },
  (error) => {
    if (error.response?.status === 401) {
      // 处理未授权情况
      localStorage.removeItem('userInfo');
      // 清除可能存储的 token
      localStorage.removeItem('userInfo');
      window.location.href = '/login';
    }
    return Promise.reject(error);
  }
);

export default client;

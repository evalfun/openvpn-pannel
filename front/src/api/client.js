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
      const url = error.config?.url || '';
      const onLoginPage = window.location.pathname === '/login';
      // 登录接口返回 401（账号密码错误）或已在登录页时，不要跳转，避免刷新页面丢失错误提示
      if (!url.includes('/user/login') && !onLoginPage) {
        localStorage.removeItem('userInfo');
        window.location.href = '/login';
      }
    }
    return Promise.reject(error);
  }
);

export default client;

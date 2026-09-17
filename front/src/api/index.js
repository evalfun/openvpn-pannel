import client from './client';

export const userAPI = {
  // 获取用户信息
  getUserInfo: () => client.get('/user/info'),
  
  // 登录
  login: (username, password) => client.post('/user/login', { username, password }),

  // 登出
  logout: () => client.post('/user/logout'),
};

export const serverAPI = {
  // 获取服务器列表
  getServerList: () => client.get('/server/list'),
  
  // 创建服务器
  createServer: (data) => client.post('/server/create', data),
  
  // 更新服务器
  updateServer: (id, data) => client.post('/server/update', { id, ...data }),
  
  // 获取服务器信息
  getServerInfo: (id) => client.get(`/server/info?id=${id}`),
  
  // 启动服务器
  startServer: (id) => client.post('/server/start', { id }),
  
  // 停止服务器
  stopServer: (id) => client.post('/server/stop', { id }),
  
  // 删除服务器
  deleteServer: (id) => client.post('/server/delete', { id }),
  
  // 获取客户端配置列表
  getClientConfigList: (id) => client.get(`/server/client_config/list?id=${id}`),
  
  // 删除客户端配置
  deleteClientConfigs: (data) => client.post('/server/client_config/delete', data),
  
  // 添加/更新客户端配置
  addClientConfig: (data) => client.post('/server/client_config/add', data),

  // 导出客户端配置（下载 .ovpn）
  exportClientConfig: (data) =>
    client.post('/server/client_export', data, { responseType: 'blob' }),
};

export const certificateAPI = {
  // 列出证书：支持 type / parentId / hasKey 过滤与 page / pageSize 分页
  list: (params = {}) => {
    const query = new URLSearchParams();
    if (params.parentId !== undefined && params.parentId !== null) query.append('parent_id', params.parentId);
    if (params.type !== undefined && params.type !== null) query.append('type', params.type);
    if (params.hasKey !== undefined && params.hasKey !== null) query.append('has_key', params.hasKey ? 1 : 0);
    if (params.page !== undefined && params.page !== null) query.append('page', params.page);
    if (params.pageSize !== undefined && params.pageSize !== null) query.append('page_size', params.pageSize);
    const qs = query.toString();
    return client.get(`/certificate/list${qs ? `?${qs}` : ''}`);
  },

  // 获取证书完整内容
  getInfo: (id) => client.get(`/certificate/info?id=${id}`),

  // 解析手动填写的 PEM 证书（不保存），可选传入私钥校验是否匹配
  parse: (cert, key = '') => client.post('/certificate/parse', { cert, key }),

  // 生成自签名 CA
  generateCA: (data) => client.post('/certificate/ca/generate', data),

  // 使用 CA 签发服务器/客户端证书
  sign: (data) => client.post('/certificate/sign', data),

  // 导入证书
  importCert: (data) => client.post('/certificate/import', data),

  // 删除证书
  delete: (id) => client.post('/certificate/delete', { id }),

  // 生成 DH 参数
  generateDH: (bits) => client.post('/certificate/dh/generate', { bits }),

  // 生成 ta.key
  generateTLSAuth: () => client.post('/certificate/tls_auth/generate'),
};

export const groupAPI = {
  // 获取用户组列表
  getGroupList: (page = 1, pageSize = 20, query = '') => {
    let url = `/group/list?page=${page}&page_size=${pageSize}`;
    if (query) {
      url += `&query=${encodeURIComponent(query)}`;
    }
    return client.get(url);
  },
  
  // 创建用户组
  createGroup: (data) => client.post('/group/create', data),
  
  // 获取用户组中的用户列表
  getGroupUsers: (group) => client.get(`/group/user/list?group=${group}`),
  
  // 将用户添加到用户组
  addUserToGroup: (data) => client.post('/group/user/add', data),
  
  // 批量将用户添加到用户组
  addUsersToGroup: (data) => client.post('/group/user/add/batch', data),
  
  // 从用户组移除用户
  removeUserFromGroup: (data) => client.post('/group/user/remove', data),
  
  // 获取用户组的ACL列表
  getGroupACLs: (group) => client.get(`/group/acl/list?group=${group}`),
  
  // 添加ACL到用户组
  addGroupACL: (data) => client.post('/group/acl/add', data),
  
  // 删除用户组的ACL
  deleteGroupACL: (data) => client.post('/group/acl/delete', data),
  
  // 更新用户组描述
  updateGroupDesc: (data) => client.post('/group/update', data),
  
  // 删除用户组
  deleteGroup: (data) => client.post('/group/delete', data),
};

export const userManageAPI = {
  // 获取用户列表
  getUserList: (page = 1, pageSize = 20, query = '', excludeGroupId = '') => {
    let url = `/user/list?page=${page}&page_size=${pageSize}`;
    if (query) {
      url += `&query=${encodeURIComponent(query)}`;
    }
    if (excludeGroupId) {
      url += `&exclude_group_id=${excludeGroupId}`;
    }
    return client.get(url);
  },
  
  // 创建用户
  createUser: (data) => client.post('/user/create', data),
  
  // 更新用户信息
  updateUserInfo: (data) => client.post('/user/info', data),
  
  // 删除用户
  deleteUser: (id) => client.post('/user/delete', { id }),
  
  // 批量删除用户
  deleteUsers: (idList) => client.post('/user/delete', { id_list: idList }),
  
  // 获取用户详细信息（包括所属组等）
  getUserDetailInfo: (username) => client.get(`/user/info?username=${username}`),
  
  // 清除用户流量
  resetTraffic: (id) => client.post('/user/reset_traffic', { id }),
};

export const permissionAPI = {
  // 获取权限列表
  getPermissions: (id) => client.get(`/permission/list?id=${id}`),
  
  // 添加权限
  addPermission: (data) => client.post('/permission/add', data),
  
  // 删除权限
  deletePermissions: (data) => client.post('/permission/delete', data),
};

export const userServerPermAPI = {
  // 获取用户的服务器权限列表
  getUserServerPermissions: (userId) => client.get(`/server/list/user_perm?user_id=${userId}`),
};

export const eventAPI = {
  // 获取事件列表
  getEventList: (serverId, startTime, endTime, ip = '', page = 1, pageSize = 20) => {
    let url = `/event/list?id=${serverId}&start=${startTime}&end=${endTime}&page=${page}&page_size=${pageSize}`;
    if (ip) {
      url += `&ip=${encodeURIComponent(ip)}`;
    }
    return client.get(url);
  },
  
  // 清空事件
  clearEvents: (serverId) => client.post('/event/clear', { id: serverId }),
};

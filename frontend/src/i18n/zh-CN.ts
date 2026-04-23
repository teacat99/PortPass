export default {
  app: {
    title: 'PortPass',
    subtitle: '按需临时开放服务器端口'
  },
  menu: {
    home: '首页',
    rules: '规则',
    history: '历史',
    settings: '设置'
  },
  action: {
    submit: '提交',
    cancel: '取消',
    confirm: '确认',
    create: '创建规则',
    terminate: '提前终止',
    extend: '延长',
    duplicate: '复制',
    delete: '删除',
    edit: '编辑',
    save: '保存',
    refresh: '刷新',
    login: '登录',
    logout: '退出',
    search: '搜索'
  },
  home: {
    clientIP: '当前客户端 IP',
    sourceMode: '来源 IP',
    sourceCurrent: '使用当前 IP',
    sourceAny: '全部 IP (0.0.0.0/0)',
    sourceManual: '手动输入 CIDR',
    port: '端口',
    portPlaceholder: '1-65535',
    presets: '预设',
    protocol: '协议',
    duration: '有效期',
    durationCustom: '自定义到期时刻',
    note: '备注',
    notePlaceholder: '可选，用于识别该规则',
    submitted: '规则已下发',
    countdown: '剩余时间'
  },
  rules: {
    title: '活跃规则',
    empty: '暂无活跃规则',
    id: 'ID',
    source: '来源',
    port: '端口',
    protocol: '协议',
    remaining: '剩余',
    createdAt: '创建时间',
    note: '备注',
    actions: '操作',
    terminateConfirm: '确定要立即终止该规则吗？',
    extendDialog: '延长有效期',
    extendAmount: '延长时长'
  },
  history: {
    title: '历史记录',
    status: '状态',
    actor: '操作者 IP',
    terminatedAt: '终止时间',
    duration: '持续时长',
    filterFrom: '起始时间',
    filterTo: '结束时间'
  },
  settings: {
    title: '设置',
    tabPresets: '预设端口',
    tabDefaults: '默认参数',
    tabProxies: '可信代理',
    tabAuth: '鉴权模式',
    authMode: '当前鉴权模式',
    firewallDriver: '防火墙驱动',
    maxDuration: '单条规则最大有效期（小时）',
    historyRetention: '历史保留天数',
    trustedProxies: '可信反代 CIDR 列表'
  },
  login: {
    title: '登录 PortPass',
    password: '管理员密码',
    passwordPlaceholder: '输入密码进入管理界面',
    failed: '登录失败'
  },
  status: {
    pending: '等待中',
    active: '生效中',
    expired: '已过期',
    revoked: '已撤销',
    failed: '失败'
  },
  unit: {
    days: '天',
    hours: '小时',
    minutes: '分',
    seconds: '秒',
    m15: '15 分钟',
    h1: '1 小时',
    h4: '4 小时',
    h12: '12 小时',
    h24: '24 小时'
  },
  msg: {
    ruleCreated: '规则已创建',
    ruleTerminated: '规则已终止',
    ruleExtended: '规则已延长',
    ruleDuplicated: '规则已复制',
    presetSaved: '预设已保存',
    presetDeleted: '预设已删除',
    loadFailed: '加载失败',
    invalidInput: '输入无效'
  }
}

export default {
  app: {
    title: 'PortPass',
    subtitle: 'On-demand server port opener'
  },
  menu: {
    home: 'Home',
    rules: 'Rules',
    history: 'History',
    settings: 'Settings'
  },
  action: {
    submit: 'Submit',
    cancel: 'Cancel',
    confirm: 'Confirm',
    create: 'Create rule',
    terminate: 'Terminate',
    extend: 'Extend',
    duplicate: 'Duplicate',
    delete: 'Delete',
    edit: 'Edit',
    save: 'Save',
    refresh: 'Refresh',
    login: 'Login',
    logout: 'Logout',
    search: 'Search'
  },
  home: {
    clientIP: 'Current client IP',
    sourceMode: 'Source IP',
    sourceCurrent: 'Use current IP',
    sourceAny: 'Any IP (0.0.0.0/0)',
    sourceManual: 'Manual CIDR',
    port: 'Port',
    portPlaceholder: '1-65535',
    presets: 'Presets',
    protocol: 'Protocol',
    duration: 'Duration',
    durationCustom: 'Custom expiry',
    note: 'Note',
    notePlaceholder: 'Optional memo',
    submitted: 'Rule applied',
    countdown: 'Remaining'
  },
  rules: {
    title: 'Active rules',
    empty: 'No active rules',
    id: 'ID',
    source: 'Source',
    port: 'Port',
    protocol: 'Protocol',
    remaining: 'Remaining',
    createdAt: 'Created at',
    note: 'Note',
    actions: 'Actions',
    terminateConfirm: 'Terminate this rule now?',
    extendDialog: 'Extend expiry',
    extendAmount: 'Extend by'
  },
  history: {
    title: 'History',
    status: 'Status',
    actor: 'Actor IP',
    terminatedAt: 'Terminated at',
    duration: 'Duration',
    filterFrom: 'From',
    filterTo: 'To'
  },
  settings: {
    title: 'Settings',
    tabPresets: 'Preset ports',
    tabDefaults: 'Defaults',
    tabProxies: 'Trusted proxies',
    tabAuth: 'Auth mode',
    authMode: 'Current auth mode',
    firewallDriver: 'Firewall driver',
    maxDuration: 'Max duration (hours)',
    historyRetention: 'History retention (days)',
    trustedProxies: 'Trusted proxy CIDR list'
  },
  login: {
    title: 'Sign in to PortPass',
    password: 'Admin password',
    passwordPlaceholder: 'Enter password to continue',
    failed: 'Login failed'
  },
  status: {
    pending: 'Pending',
    active: 'Active',
    expired: 'Expired',
    revoked: 'Revoked',
    failed: 'Failed'
  },
  unit: {
    days: 'd',
    hours: 'h',
    minutes: 'm',
    seconds: 's',
    m15: '15 minutes',
    h1: '1 hour',
    h4: '4 hours',
    h12: '12 hours',
    h24: '24 hours'
  },
  msg: {
    ruleCreated: 'Rule created',
    ruleTerminated: 'Rule terminated',
    ruleExtended: 'Rule extended',
    ruleDuplicated: 'Rule duplicated',
    presetSaved: 'Preset saved',
    presetDeleted: 'Preset deleted',
    loadFailed: 'Load failed',
    invalidInput: 'Invalid input'
  }
}

import axios, { type AxiosInstance } from 'axios'
import { Message } from '@/lib/toast'

// Shared axios instance. Base URL is empty so requests flow through the
// same origin (supports both dev proxy and embedded production serving).
const client: AxiosInstance = axios.create({
  baseURL: '/api',
  timeout: 15000
})

client.interceptors.request.use((cfg) => {
  const token = localStorage.getItem('portpass.token')
  if (token) {
    cfg.headers = cfg.headers ?? {}
    cfg.headers.Authorization = `Bearer ${token}`
  }
  return cfg
})

client.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const status = err?.response?.status
    const message = err?.response?.data?.error ?? err.message ?? '网络错误'
    if (status === 401) {
      localStorage.removeItem('portpass.token')
      if (!location.pathname.endsWith('/login')) {
        location.assign('/login')
      }
    } else if (status === 429) {
      Message.warning(message)
    } else if (status && status >= 500) {
      Message.error(message)
    } else if (status && status >= 400) {
      Message.warning(message)
    } else {
      Message.error(message)
    }
    return Promise.reject(err)
  }
)

export default client

import client from './client'

export async function login(password: string): Promise<{ token: string }> {
  const { data } = await client.post<{ token: string }>('/auth/login', { password })
  return data
}

export async function authStatus(): Promise<{ mode: string; required: boolean }> {
  const { data } = await client.get<{ mode: string; required: boolean }>('/auth/status')
  return data
}

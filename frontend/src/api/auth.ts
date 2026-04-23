import client from './client'
import type { Me, Role } from './types'

export interface LoginResponse {
  token: string
  username: string
  role: Role
}

export async function login(username: string, password: string): Promise<LoginResponse> {
  const { data } = await client.post<LoginResponse>('/auth/login', { username, password })
  return data
}

export async function authStatus(): Promise<{ mode: string; required: boolean }> {
  const { data } = await client.get<{ mode: string; required: boolean }>('/auth/status')
  return data
}

export async function getMe(): Promise<Me> {
  const { data } = await client.get<Me>('/auth/me')
  return data
}

export async function changeOwnPassword(oldPassword: string, newPassword: string) {
  await client.post('/auth/password', {
    old_password: oldPassword,
    new_password: newPassword
  })
}

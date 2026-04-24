import client from './client'
import type { ProtectedPort, UserAllowedRange } from './types'

export async function listProtectedPorts() {
  const { data } = await client.get<ProtectedPort[]>('/protected-ports')
  return data
}

export async function upsertProtectedPort(p: Partial<ProtectedPort>) {
  const { data } = await client.post<ProtectedPort>('/protected-ports', p)
  return data
}

export async function deleteProtectedPort(id: number) {
  await client.delete(`/protected-ports/${id}`)
}

export async function listUserRanges(userId: number) {
  const { data } = await client.get<UserAllowedRange[]>(`/users/${userId}/port-ranges`)
  return data
}

export async function upsertUserRange(userId: number, payload: Partial<UserAllowedRange>) {
  const { data } = await client.post<UserAllowedRange>(`/users/${userId}/port-ranges`, payload)
  return data
}

export async function deleteUserRange(userId: number, rangeId: number) {
  await client.delete(`/users/${userId}/port-ranges/${rangeId}`)
}

export async function clearUserRanges(userId: number) {
  await client.delete(`/users/${userId}/port-ranges`)
}

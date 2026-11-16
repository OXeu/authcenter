import { it, expect, vi } from 'vite-plus/test'
vi.mock('@yusufceng1z/ding', () => ({
  mountToaster: vi.fn(),
  toast: { configure: vi.fn(), success: vi.fn(() => 'id') },
}))
import { mountToaster, toast } from '@yusufceng1z/ding'
import { mountToasts, notify, setToastTheme } from '../src/toast.ts'
it('mounts once with the CSP nonce and retains the selected language', () => {
  mountToasts('zh-CN', 'nonce', 'light')
  notify('success', 'saved')
  expect(mountToaster).toHaveBeenCalledTimes(1)
  expect(mountToaster).toHaveBeenCalledWith({
    position: 'bottom-right',
    theme: 'light',
    nonce: 'nonce',
  })
  expect(toast.configure).toHaveBeenLastCalledWith(
    expect.objectContaining({ theme: 'light', labels: { region: '通知', dismiss: '关闭通知' } }),
  )
  setToastTheme('dark')
  expect(toast.configure).toHaveBeenLastCalledWith({ theme: 'dark' })
  expect(mountToaster).toHaveBeenCalledTimes(1)
})

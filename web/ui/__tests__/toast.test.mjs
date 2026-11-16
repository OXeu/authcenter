import { it, expect, vi } from 'vite-plus/test'
vi.mock('@yusufceng1z/ding', () => ({
  mountToaster: vi.fn(),
  toast: { configure: vi.fn(), success: vi.fn(() => 'id') },
}))
import { mountToaster, toast } from '@yusufceng1z/ding'
import { mountToasts, notify } from '../src/toast.js'
it('mounts once with the CSP nonce and retains the selected language', () => {
  mountToasts('zh-CN', 'nonce')
  notify('success', 'saved')
  expect(mountToaster).toHaveBeenCalledTimes(1)
  expect(mountToaster).toHaveBeenCalledWith({
    position: 'bottom-right',
    theme: 'dark',
    nonce: 'nonce',
  })
  expect(toast.configure).toHaveBeenLastCalledWith(
    expect.objectContaining({ labels: { region: '通知', dismiss: '关闭通知' } }),
  )
})

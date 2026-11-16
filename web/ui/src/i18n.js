/**
 * Client copy and the language switcher. Server-rendered text is translated by
 * Go and shipped in `PageData.messages`; here are only the ids the components
 * produce themselves (dialog titles, role badges, toasts, errors).
 * @type {Record<string, Record<string, string>>}
 */
export const fallbackMessages = {
  en: {
    'common.create': 'Create',
    'common.save': 'Save',
    'common.cancel': 'Cancel',
    'common.edit': 'Edit',
    'dialog.project': 'New project',
    'dialog.item': 'Save configuration',
    'dialog.key': 'New API key',
    'dialog.environment': 'New environment',
    'common.editDialog': 'Edit',
    'role.admin': 'Admin',
    'role.editor': 'Editor',
    'role.viewer': 'Viewer',
    'error.title': 'Something went wrong',
    'toast.region': 'Notifications',
    'toast.dismiss': 'Dismiss notification',
    'toast.itemDeleted': 'Deleted {{key}}',
    'toast.keyRevoked': 'The key was revoked',
    'toast.proposalApproved': 'Proposal approved',
    'toast.proposalRejected': 'Proposal rejected',
    'error.network': 'Network error, try again.',
    'error.500': 'Server error, try again.',
    'error.502': 'Server error, try again.',
    'error.conflict': 'That name is already taken.',
    'confirm.ok': 'OK',
  },
  'zh-CN': {
    'common.create': '创建',
    'common.save': '保存',
    'common.cancel': '取消',
    'common.edit': '编辑',
    'dialog.project': '新建项目',
    'dialog.item': '保存配置',
    'dialog.key': '新建 API 密钥',
    'dialog.environment': '新建环境',
    'common.editDialog': '编辑',
    'role.admin': '管理员',
    'role.editor': '编辑者',
    'role.viewer': '只读者',
    'error.title': '出错了',
    'toast.region': '通知',
    'toast.dismiss': '关闭通知',
    'toast.itemDeleted': '已删除 {{key}}',
    'toast.keyRevoked': '密钥已吊销',
    'toast.proposalApproved': '提案已批准',
    'toast.proposalRejected': '提案已拒绝',
    'error.network': '网络错误，请重试。',
    'error.500': '服务器出错，请重试。',
    'error.502': '服务器出错，请重试。',
    'error.conflict': '该名称已被占用。',
    'confirm.ok': '确定',
  },
}

/**
 * @typedef {object} Copy
 * @property {(id: string, vars?: Record<string, string>) => string} msg
 * @property {string} lang
 */

/**
 * @param {string} lang
 * @param {Record<string, string>} [serverMessages]
 * @returns {Copy}
 */
export function createCopy(lang, serverMessages = {}) {
  const table = { ...fallbackMessages.en, ...(fallbackMessages[lang] ?? {}), ...serverMessages }
  /**
   * @param {string} id
   * @param {Record<string, string>} [vars]
   * @returns {string}
   */
  const msg = (id, vars = {}) =>
    (table[id] ?? id).replace(
      /\{\{(\w+)\}\}/gu,
      (_, /** @type {string} */ name) => vars[name] ?? '',
    )
  return { msg, lang }
}

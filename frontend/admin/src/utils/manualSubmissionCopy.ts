// 后台订单详情「一键复制」用户提交信息时使用的固定格式。
// 客服需要把订单号、账号、密码和认证链接一次性粘给上游或工单系统，
// 因此按 orderid / Account / password / url 输出，缺失字段直接省略。

export type ManualSubmissionCopySource = Record<string, unknown> | null | undefined

const pickFirst = (source: Record<string, unknown>, keys: string[]): string => {
  for (const key of keys) {
    const value = source[key]
    if (value === undefined || value === null) continue
    const text = String(value).trim()
    if (text) return text
  }
  return ''
}

export const buildManualSubmissionCopy = (submission: ManualSubmissionCopySource, orderNo: string): string => {
  const source = submission && typeof submission === 'object' ? submission : {}
  const lines = [`orderid： ${String(orderNo || '').trim() || '-'}`]
  const account = pickFirst(source, ['google_account', 'account', 'claude_email', 'email'])
  const password = pickFirst(source, ['google_password', 'password', 'passwd', 'pwd'])
  const url = pickFirst(source, ['kyc_url', 'url', 'verify_url'])
  if (account) lines.push(`Account： ${account}`)
  if (password) lines.push(`password： ${password}`)
  if (url) lines.push(`url： ${url}`)
  return lines.join('\n')
}

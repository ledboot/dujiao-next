// 后台订单详情「一键复制」用户提交信息时使用的固定格式。
// 客服需要把订单号、账号、密码和认证链接一次性粘给上游或工单系统，
// 因此按 OrderId / KYC Type / Account / Password / Url 输出，缺失字段直接省略。

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

// KYC 认证类型（Persona / Portal）取自商品标题，识别不到则返回空串。
export const resolveKycType = (productTitle: string): string => {
  const title = String(productTitle || '')
  if (/persona/i.test(title)) return 'Persona'
  if (/portal/i.test(title)) return 'Portal'
  return ''
}

export const buildManualSubmissionCopy = (
  submission: ManualSubmissionCopySource,
  orderNo: string,
  kycType = ''
): string => {
  const source = submission && typeof submission === 'object' ? submission : {}
  const lines = [`OrderId： ${String(orderNo || '').trim() || '-'}`]
  const type = String(kycType || '').trim()
  if (type) lines.push(`KYC Type： ${type}`)
  const account = pickFirst(source, ['google_account', 'account', 'claude_email', 'email'])
  // 密码按提交的原文输出，不做任何脱敏，方便客服直接使用。
  const password = pickFirst(source, ['google_password', 'password', 'passwd', 'pwd'])
  const url = pickFirst(source, ['kyc_url', 'url', 'verify_url'])
  if (account) lines.push(`Account： ${account}`)
  if (password) lines.push(`Password： ${password}`)
  if (url) lines.push(`Url： ${url}`)
  return lines.join('\n')
}

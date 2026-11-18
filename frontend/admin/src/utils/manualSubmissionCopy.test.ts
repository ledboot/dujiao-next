import { describe, expect, it } from 'vitest'
import { buildManualSubmissionCopy, resolveKycType } from './manualSubmissionCopy'

describe('resolveKycType', () => {
  it('detects persona and portal from product title', () => {
    expect(resolveKycType('Claude Persona KYC 认证')).toBe('Persona')
    expect(resolveKycType('Claude Portal KYC 认证')).toBe('Portal')
    expect(resolveKycType('静态住宅IP 30天｜独享原生ISP（美国）')).toBe('')
  })
})

describe('buildManualSubmissionCopy', () => {
  it('formats portal kyc submission with google account and plaintext password', () => {
    const text = buildManualSubmissionCopy(
      {
        claude_email: 'buyer@example.com',
        kyc_url: 'https://portal.anthropic.com/verify?org=1&return_to=claude',
        google_account: 'buyer@gmail.com',
        google_password: 'p@ss&word',
      },
      'DJ20261117123900807747-01',
      'Portal'
    )
    expect(text).toBe(
      [
        'OrderId： DJ20261117123900807747-01',
        'KYC Type： Portal',
        'Account： buyer@gmail.com',
        'Password： p@ss&word',
        'Url： https://portal.anthropic.com/verify?org=1&return_to=claude',
      ].join('\n')
    )
  })

  it('falls back to claude email and skips missing password and kyc type', () => {
    const text = buildManualSubmissionCopy(
      { claude_email: 'buyer@example.com', kyc_url: 'https://withpersona.com/verify?inquiry-id=x' },
      'DJ1-01'
    )
    expect(text).toBe(
      [
        'OrderId： DJ1-01',
        'Account： buyer@example.com',
        'Url： https://withpersona.com/verify?inquiry-id=x',
      ].join('\n')
    )
  })

  it('outputs only order id when nothing was submitted', () => {
    expect(buildManualSubmissionCopy(null, 'DJ2')).toBe('OrderId： DJ2')
    expect(buildManualSubmissionCopy({ kyc_url: '   ' }, '')).toBe('OrderId： -')
  })
})

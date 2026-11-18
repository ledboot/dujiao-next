import { describe, expect, it } from 'vitest'
import { buildManualSubmissionCopy } from './manualSubmissionCopy'

describe('buildManualSubmissionCopy', () => {
  it('formats portal kyc submission with google account', () => {
    const text = buildManualSubmissionCopy(
      {
        claude_email: 'buyer@example.com',
        kyc_url: 'https://portal.anthropic.com/verify?org=1&return_to=claude',
        google_account: 'buyer@gmail.com',
        google_password: 'p@ss&word',
      },
      'DJ20261117123900807747-01'
    )
    expect(text).toBe(
      [
        'orderid： DJ20261117123900807747-01',
        'Account： buyer@gmail.com',
        'password： p@ss&word',
        'url： https://portal.anthropic.com/verify?org=1&return_to=claude',
      ].join('\n')
    )
  })

  it('falls back to claude email and skips missing password', () => {
    const text = buildManualSubmissionCopy(
      { claude_email: 'buyer@example.com', kyc_url: 'https://withpersona.com/verify?inquiry-id=x' },
      'DJ1-01'
    )
    expect(text).toBe(
      [
        'orderid： DJ1-01',
        'Account： buyer@example.com',
        'url： https://withpersona.com/verify?inquiry-id=x',
      ].join('\n')
    )
  })

  it('outputs only order id when nothing was submitted', () => {
    expect(buildManualSubmissionCopy(null, 'DJ2')).toBe('orderid： DJ2')
    expect(buildManualSubmissionCopy({ kyc_url: '   ' }, '')).toBe('orderid： -')
  })
})

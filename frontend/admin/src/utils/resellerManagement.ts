export type ResellerDomainHostingState = { managed?: boolean; approved?: boolean; removal_requested?: boolean }

const RESELLER_PROFILE_STATUS_PENDING_REVIEW = 'pending_review'
const RESELLER_PROFILE_STATUS_ACTIVE = 'active'
const RESELLER_PROFILE_STATUS_REJECTED = 'rejected'
const RESELLER_PROFILE_STATUS_DISABLED = 'disabled'

const RESELLER_DOMAIN_STATUS_PENDING_REVIEW = 'pending_review'
const RESELLER_DOMAIN_STATUS_ACTIVE = 'active'
const RESELLER_DOMAIN_STATUS_DISABLED = 'disabled'

export const getResellerProfileStatusKey = (status?: string) => {
  if (status === RESELLER_PROFILE_STATUS_PENDING_REVIEW) return 'pendingReview'
  if (status === RESELLER_PROFILE_STATUS_ACTIVE) return 'active'
  if (status === RESELLER_PROFILE_STATUS_REJECTED) return 'rejected'
  if (status === RESELLER_PROFILE_STATUS_DISABLED) return 'disabled'
  return 'unknown'
}

export const getResellerProfileActionState = (status?: string) => ({
  canApprove: status === RESELLER_PROFILE_STATUS_PENDING_REVIEW || status === RESELLER_PROFILE_STATUS_REJECTED,
  canReject: status === RESELLER_PROFILE_STATUS_PENDING_REVIEW,
  canDisable:
    status === RESELLER_PROFILE_STATUS_PENDING_REVIEW ||
    status === RESELLER_PROFILE_STATUS_ACTIVE ||
    status === RESELLER_PROFILE_STATUS_REJECTED,
  canRestore: status === RESELLER_PROFILE_STATUS_DISABLED,
})

export const getResellerDomainActionState = (status?: string, hosting?: ResellerDomainHostingState) => ({
  canApprove: !hosting?.removal_requested && ((status === RESELLER_DOMAIN_STATUS_PENDING_REVIEW && !(hosting?.managed && hosting.approved)) || status === RESELLER_DOMAIN_STATUS_DISABLED),
  canDisable: status === RESELLER_DOMAIN_STATUS_PENDING_REVIEW || status === RESELLER_DOMAIN_STATUS_ACTIVE,
})

export const isResellerDomainApprovedPending = (status?: string, hosting?: ResellerDomainHostingState) =>
  status === RESELLER_DOMAIN_STATUS_PENDING_REVIEW && hosting?.managed === true && hosting.approved === true

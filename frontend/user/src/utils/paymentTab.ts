// Keep the tab opened by the checkout click across the SPA navigation to /pay.
// Only the matching order may consume it.
let reserved: { orderNo: string; tab: Window } | null = null

export const handoffPaymentTab = (orderNo: string, tab: Window) => {
  reserved?.tab.close()
  reserved = { orderNo, tab }
}

export const takePaymentTab = (orderNo: string): Window | null => {
  if (!reserved || reserved.orderNo !== orderNo) return null
  const tab = reserved.tab
  reserved = null
  return tab.closed ? null : tab
}

export const filterInvoiceChannels = <T extends { supports_invoice?: boolean }>(channels: T[], invoiceRequested: boolean): T[] =>
  invoiceRequested ? channels.filter(channel => channel.supports_invoice === true) : channels

export const postTextLength = (value: string) => Array.from(value).length

export function postContentLength(value: string): number {
  const document = new DOMParser().parseFromString(value, 'text/html')
  return postTextLength(document.documentElement.textContent || '')
}

export function validateResellerPost(post: {
  title: Record<string, string>
  summary: Record<string, string>
  content: Record<string, string>
}): 'titleError' | 'summaryError' | 'contentError' | null {
  const titles = Object.values(post.title)
  if (!titles.some(value => value.trim()) || titles.some(value => postTextLength(value) > 120)) return 'titleError'
  if (Object.values(post.summary).some(value => postTextLength(value) > 300)) return 'summaryError'
  if (Object.values(post.content).some(value => postContentLength(value) > 10000)) return 'contentError'
  return null
}

export function downloadFilename(disposition = '') {
 const encoded = disposition.match(/filename\*\s*=\s*UTF-8''([^;]+)/i)
 const plain = disposition.match(/filename\s*=\s*(?:"([^"]+)"|([^;]+))/i)
 let name = encoded?.[1] || plain?.[1] || plain?.[2] || 'download'
 try { name = decodeURIComponent(name.trim()) } catch { /* Keep a malformed legacy filename readable. */ }
 return [...name].map(char => char==='/' || char==='\\' || char.charCodeAt(0)<32 ? '_' : char).join('')
}

export async function normalizeResponseBody(body: unknown, disposition = ''): Promise<unknown> {
 if (body instanceof Blob && !/attachment/i.test(disposition) && /json/i.test(body.type)) {
  return JSON.parse(await body.text())
 }
 return body
}

export function saveDownload(blob: Blob, disposition: string) {
 const url = URL.createObjectURL(blob), link = document.createElement('a')
 link.href = url; link.download = downloadFilename(disposition)
 document.body.appendChild(link); link.click(); link.remove()
 setTimeout(() => URL.revokeObjectURL(url), 1000)
}

// Preserve incomplete SSE frames between arbitrary network chunks.
export function createSSEDecoder(onEvent: (event: Record<string, any>) => void) {
 let pending = ''
 return {
  push(chunk: string) {
   pending += chunk
   let match: RegExpExecArray | null
   while ((match = /\r?\n\r?\n/.exec(pending))) {
    const frame = pending.slice(0, match.index); pending = pending.slice(match.index + match[0].length)
    const data = frame.split(/\r?\n/).filter(line => line.startsWith('data:')).map(line => line.slice(5).trimStart()).join('\n')
    if (data && data !== '[DONE]') onEvent(JSON.parse(data))
   }
  },
  finish() { if (pending.trim()) this.push('\n\n') },
 }
}

export function readUpload(file: File, excel: boolean): Promise<string> {
 return new Promise((resolve, reject) => {
  const reader = new FileReader()
  reader.onload = () => resolve(String(reader.result || ''))
  reader.onerror = () => reject(new Error('Unable to read the selected file'))
  if (excel) reader.readAsDataURL(file); else reader.readAsText(file)
 })
}

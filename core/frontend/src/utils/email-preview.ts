import { normalizeEmailHTML } from './email-content'

export interface EmailPreviewPalette { backgroundColor: string; textColor: string }
export function defaultPreviewPalette(dark = false): EmailPreviewPalette {
	return { backgroundColor: dark ? '#242424' : '#fff', textColor: dark ? '#fff' : '#222' }
}

// Presentation-only: never mutate the saved template or outgoing message.
export function emailPreviewDocument(value: string, palette: EmailPreviewPalette = defaultPreviewPalette()): string {
	const fenced = value.trim().match(/^```(?:html)?\s*\r?\n([\s\S]*?)\r?\n```\s*$/i)
	let html = normalizeEmailHTML(fenced ? fenced[1] : value)
	// Plain-text normalization has a known generated wrapper. Theme that wrapper,
	// but never invert or override colors authored in an actual email template.
	if (html.startsWith(`<div style="font-family:Arial,'Microsoft YaHei',sans-serif;font-size:14px;line-height:1.6;color:#222;">`)) {
		html = html.replace('color:#222;', `color:${palette.textColor};`)
	}
	const textColor = /(?:background(?:-color)?\s*:|\bbgcolor\s*=)/i.test(html) ? '#222' : palette.textColor
	const head = `<meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline' https: http:; img-src https: http: data: blob:; font-src https: http: data:; base-uri 'none'; form-action 'none'"><style>html,body{margin:0;padding:0;min-height:100%}html{background:${palette.backgroundColor}}body{background:transparent;font-family:Arial,'Microsoft YaHei',sans-serif;color:${textColor};overflow-wrap:anywhere}img{max-width:100%;height:auto}table{max-width:100%}</style>`
	// Insert before authored styles so template-specific styling still wins.
	if (/<head\b[^>]*>/i.test(html)) return html.replace(/<head\b[^>]*>/i, tag => tag + head)
	if (/<html\b[^>]*>/i.test(html)) return html.replace(/<html\b[^>]*>/i, tag => tag + '<head>' + head + '</head>')
	return '<!doctype html><html><head>' + head + '</head><body>' + html + '</body></html>'
}

export function previewLength(value: number | string): string {
	return typeof value === 'number' ? `${value}px` : value
}

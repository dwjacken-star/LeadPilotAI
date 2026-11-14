import { workspace } from './api'

// The endpoint applies department grants before choosing the preferred mailbox.
export async function getDefaultSender(): Promise<string> {
	try {
		const result = await workspace('list', { type: 'senders', page: 1, page_size: 1 })
		return typeof result.default_sender === 'string' ? result.default_sender : ''
	} catch {
		// Request failures are shown by the shared API interceptor.
		return ''
	}
}

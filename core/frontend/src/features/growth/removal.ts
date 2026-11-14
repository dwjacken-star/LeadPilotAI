import { confirm } from '@/utils'
export function confirmRemoval(title: string, content: string): Promise<boolean> {
	return new Promise(resolve =>
		confirm({
			title,
			content,
			confirmType: 'error',
			onConfirm: () => {
				resolve(true)
			},
			onCancel: () => {
				resolve(false)
			},
		})
	)
}

export function bindingFlag(value: unknown): boolean {
	return value === true || value === 1 || value === 'true' || value === 't'
}
export function mailboxBindingState(account: any): 'none' | 'invalid' | 'needs_grant' | 'ready' {
	if (!account?.account_id) return 'none'
	if (!bindingFlag(account.eligible)) return 'invalid'
	return bindingFlag(account.super) || bindingFlag(account.has_sender_grant) ? 'ready' : 'needs_grant'
}
export function mailboxBindingPayload(mailbox: string, account: any, grantRequested: boolean) {
	const grant = !!grantRequested && mailboxBindingState(account) === 'needs_grant'
	return {
		mailbox,
		account_id: Number(account?.account_id || 0),
		grant_department: grant,
		grant_dept_id: grant ? Number(account.dept_id) : 0,
	}
}

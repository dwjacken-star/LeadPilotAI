export function routeAllowed(account: { super?: boolean; roles?: { permissions?: string[] }[] } | null, path: string) {
 if (!account) return false
 if (account.super || path === '/workspace') return true
 const has = (code: string) => !!account.roles?.some(role => role.permissions?.includes(code) || role.permissions?.includes('*'))
 if (/^\/template\/ai-template\/[A-Za-z0-9_-]+$/.test(path)) return has('template:list') && has('template:update')
 if (path === '/market/task/edit') return has('campaign:list') && (has('campaign:create') || has('campaign:update'))
 if (/^\/market\/task\/analytics\/[1-9]\d*$/.test(path)) return has('campaign:list')
 const marketing = ['campaign:list']
 const required: Record<string, string[]> = { '/overview': ['overview:view'], '/market': marketing, '/market/task': marketing, '/template': ['template:list'], '/contacts': ['contact:list'], '/contacts/group': ['contact:list'], '/contacts/subscribers': ['contact:list'], '/contacts/tags': ['contact:list'], '/intelligence': ['intelligence:view'], '/social': ['social:plan:view','social:account:view','social:interaction:view','social:report:view'], '/crm': ['crm:list'], '/materials': ['company:list', 'material:list', 'product:list'], '/outreach': ['outreach:list', 'inquiry:list', 'inquiry:manage'] }
 return (required[path] || []).some(code => account.roles?.some(role => role.permissions?.includes(code) || role.permissions?.includes('*')))
}

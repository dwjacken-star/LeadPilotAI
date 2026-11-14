export type WorkspaceSection = 'marketing' | 'templates' | 'contacts'

export const sectionTabs: Record<WorkspaceSection, string[]> = {
	marketing: ['tasks', 'schedules', 'automations', 'reports'],
	templates: ['templates'],
	contacts: ['groups', 'contacts', 'tags'],
}

export function sectionEntry(section: WorkspaceSection, path: string) {
	if (section === 'contacts') {
		if (path === '/contacts/subscribers') return 'contacts'
		if (path === '/contacts/tags') return 'tags'
	}
	return sectionTabs[section][0]
}

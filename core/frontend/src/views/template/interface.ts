export interface Template {
	id: number
	temp_name: string
	add_type: number
	html_content: string
	drag_data: string
	create_time: number
	update_time: number
	can_update?: boolean
	can_delete?: boolean
	can_copy?: boolean
	chat_id:string
	isEdit?:boolean
	edit_name?:string
}

export interface TemplateParams {
	page: number
	page_size: number
	keyword: string
}

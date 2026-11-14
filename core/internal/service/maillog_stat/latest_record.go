package maillog_stat

// Keep the latest state for the existing queue-keyed statistics tables.
// Analysis scans logs backwards: the first line wins equal timestamps.
func keepLatestMailRecord[T interface {
	GetPostfixMessageID() string
	GetLogTimeMillis() int64
}](records []T, indexes map[string]int, record T) []T {
	key := record.GetPostfixMessageID()
	if index, exists := indexes[key]; exists {
		if record.GetLogTimeMillis() > records[index].GetLogTimeMillis() {
			records[index] = record
		}
		return records
	}
	indexes[key] = len(records)
	return append(records, record)
}

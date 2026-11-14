package maillog_stat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// postqueue -j emits one JSON object per line. A malformed/failed response is
// unknown, not evidence that the deferred queue is empty.
func parseDeferredQueue(output string) (int, error) {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var entry struct {
			QueueName string `json:"queue_name"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil || entry.QueueName == "" {
			return 0, fmt.Errorf("invalid postqueue JSON response")
		}
		if entry.QueueName == "deferred" {
			count++
		}
	}
	return count, nil
}

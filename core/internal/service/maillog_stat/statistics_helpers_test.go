package maillog_stat

import (
	"reflect"
	"strings"
	"testing"
)

type testMailLog struct {
	queue string
	time  int64
	state string
}

func (r testMailLog) GetPostfixMessageID() string { return r.queue }
func (r testMailLog) GetLogTimeMillis() int64     { return r.time }

func TestLatestMailRecordRetainsTerminalState(t *testing.T) {
	var records []testMailLog
	indexes := map[string]int{}
	for _, record := range []testMailLog{
		{"queue-a", 1000, "bounced"}, // reverse scan sees the latest line first
		{"queue-b", 1000, "sent"},
		{"queue-a", 1000, "deferred"}, // same second, older line must not overwrite it
		{"queue-b", 900, "deferred"},  // out-of-order older line
		{"queue-c", 2000, "deferred"},
		{"queue-c", 3000, "sent"},
	} {
		records = keepLatestMailRecord(records, indexes, record)
	}
	want := []testMailLog{{"queue-a", 1000, "bounced"}, {"queue-b", 1000, "sent"}, {"queue-c", 3000, "sent"}}
	if !reflect.DeepEqual(records, want) {
		t.Fatalf("latest mail states lost: %#v", records)
	}
}

func TestDeferredQueueParsingDistinguishesErrorsFromZero(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		count        int
		wantError    bool
	}{
		{"empty queue", "\n", 0, false},
		{"active only", `{"queue_name":"active"}`, 0, false},
		{"deferred and active", "{\"queue_name\":\"deferred\"}\r\n{\"queue_name\":\"active\"}\n{\"queue_name\":\"deferred\"}", 2, false},
		{"malformed output", "postqueue: fatal: service unavailable", 0, true},
		{"missing queue name", `{}`, 0, true},
		{"partial output", "{\"queue_name\":\"deferred\"}\ntruncated", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			count, err := parseDeferredQueue(tc.output)
			if (err != nil) != tc.wantError || count != tc.count {
				t.Fatalf("count=%d error=%v", count, err)
			}
		})
	}
}

func TestTrackedLinksAcceptHTMLAttributeForms(t *testing.T) {
	for _, anchor := range []string{
		`<a href="https://example.org/catalog?a=1&amp;b=2">Catalog</a>`,
		`<a href='https://example.org/catalog?a=1&amp;b=2'>Catalog</a>`,
		`<A HREF = 'https://example.org/catalog?a=1&amp;b=2'>Catalog</A>`,
		`<a href=https://example.org/catalog?a=1&amp;b=2>Catalog</a>`,
	} {
		var targets []string
		result := rewriteTrackedLinks(anchor, func(target string) string {
			targets = append(targets, target)
			return "https://tracking.example.org/pmta/demo"
		})
		if !reflect.DeepEqual(targets, []string{"https://example.org/catalog?a=1&b=2"}) || !strings.Contains(result, `/pmta/demo`) {
			t.Fatalf("link not instrumented correctly: %s %#v", result, targets)
		}
	}
}

func TestTrackedLinksDoNotRewriteResourcesOrUnsubscribe(t *testing.T) {
	for _, content := range []string{
		`<link href="https://example.org/style.css" rel="stylesheet">`,
		`<a data-href="https://example.org/private">Not a navigation</a>`,
		`<a href="javascript:alert(1)">Unsafe</a>`,
		`<a href="mailto:info@example.org">Email</a>`,
		`<a href="https://example.org/unsubscribe.html?id=1">Unsubscribe</a>`,
		`<a href="https://example.org/pmta/already-instrumented">Tracked</a>`,
	} {
		result := rewriteTrackedLinks(content, func(string) string { t.Fatal("unexpected tracking registration"); return "" })
		if result != content {
			t.Fatalf("non-tracking content changed: %s", result)
		}
	}
}

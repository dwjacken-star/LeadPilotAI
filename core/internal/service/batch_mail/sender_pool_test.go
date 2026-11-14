package batch_mail

import (
	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/governance"
	"context"
	"encoding/json"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
	"sync"
	"testing"
)

func exerciseSenderPools(t *testing.T, ctx context.Context, p *governance.Principal) {
	t.Helper()
	_, err := g.DB().Exec(ctx, `INSERT INTO mailbox(username,password,password_encode,full_name,maildir,local_part,domain,active) VALUES('second@example.com','test','6447567a64413d3d','Second','second/','second','example.com',1); INSERT INTO bm_contact_groups(name,token) VALUES('pool-group-one','pool-one'),('pool-group-two','pool-two');`)
	if err != nil {
		t.Fatal(err)
	}
	groups, err := g.DB().Model("bm_contact_groups").WhereLike("name", "pool-group-%").Order("id").All()
	if err != nil {
		t.Fatal(err)
	}
	ids := []int64{groups[0]["id"].Int64(), groups[1]["id"].Int64()}
	_, err = g.DB().Exec(ctx, `INSERT INTO bm_contacts(email,group_id,active,status) VALUES('one@client.test',$1,1,1),('two@client.test',$1,1,1),('one@client.test',$2,1,1),('three@client.test',$2,1,1)`, ids[0], ids[1])
	if err != nil {
		t.Fatal(err)
	}
	config := CampaignConfig{GroupIDs: ids, Senders: []string{"sender@example.com", "second@example.com"}, TemplateID: 1, Subject: "rotation", CCEmail: "copy@example.com", TrackOpen: true}
	count, err := PreviewAudience(ctx, p, config)
	if err != nil || count != 3 {
		t.Fatalf("audience dedup: %d %v", count, err)
	}
	var tagIDs []int64
	for i, group := range ids {
		tag, e := g.DB().Model("bm_tags").Data(g.Map{"name": "pool-tag", "group_id": group}).InsertAndGetId()
		if e != nil {
			t.Fatal(e)
		}
		tagIDs = append(tagIDs, tag)
		_, e = g.DB().Exec(ctx, `INSERT INTO bm_contact_tags(contact_id,tag_id) SELECT id,$1 FROM bm_contacts WHERE group_id=$2 AND email='one@client.test'`, tagIDs[i], group)
		if e != nil {
			t.Fatal(e)
		}
	}
	for logic, expected := range map[string]int{"AND": 1, "OR": 1, "NOT": 2} {
		filtered := config
		filtered.TagIDs = tagIDs
		filtered.TagLogic = logic
		count, e := PreviewAudience(ctx, p, filtered)
		if e != nil || count != expected {
			t.Fatalf("cross-group tags %s: %d %v", logic, count, e)
		}
	}
	var task int64
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		var e error
		task, e = CreateGovernedTask(ctx, tx, p, config, false, 0)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := g.DB().Model("recipient_info").Where("task_id", task).Order("id").All()
	if err != nil || len(rows) != 3 {
		t.Fatalf("recipient union: %v %v", rows, err)
	}
	for i, row := range rows {
		want := config.Senders[i%2]
		if row["assigned_sender"].String() != want {
			t.Fatalf("rotation %d: %s", i, row["assigned_sender"].String())
		}
	}
	row, _ := g.DB().Model("email_tasks").Where("id", task).One()
	if row["track_open"].Int() != 0 || row["cc_email"].String() != config.CCEmail {
		t.Fatal("CC must not attribute a shared pixel to one recipient")
	}
	encoded, _ := json.Marshal(config.Senders)
	api, err := g.DB().Model("api_templates").Data(g.Map{"api_name": "rotation", "api_key": "pool-key", "template_id": 1, "addresser": config.Senders[0], "senders": string(encoded), "subject": "pool", "active": 1}).InsertAndGetId()
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var stored entity.ApiTemplates
	if err = g.DB().Model("api_templates").Where("id", api).Scan(&stored); err != nil || len(stored.Senders) != 2 {
		t.Fatal("API sender pool cannot round-trip", err, stored.Senders)
	}
	var storedTask entity.EmailTask
	if err = g.DB().Model("email_tasks").Where("id", task).Scan(&storedTask); err != nil || len(storedTask.Senders) != 2 || len(storedTask.GroupIds) != 2 {
		t.Fatal("Task selections cannot round-trip", err)
	}
	var mu sync.Mutex
	counts := map[string]int{}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sender, e := NextAPISender(ctx, int(api), "")
			if e != nil {
				t.Error(e)
				return
			}
			mu.Lock()
			counts[sender]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if counts[config.Senders[0]] != 10 || counts[config.Senders[1]] != 10 {
		t.Fatalf("concurrent API rotation: %v", counts)
	}
	if _, e := NextAPISender(ctx, int(api), "outside@example.com"); e == nil {
		t.Fatal("outside sender accepted")
	}
	if err := ValidCC("ok@example.com\r\nBcc: bad@example.com"); err == nil {
		t.Fatal("CC header injection accepted")
	}
}

package contactimport

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"billionmail-core/internal/model/entity"
	"billionmail-core/internal/service/emailvalidation"
	"billionmail-core/internal/service/governance"
	"github.com/gogf/gf/v2/database/gdb"
	"github.com/gogf/gf/v2/frame/g"
)

type storedImportRow struct {
	Row     Row             `json:"row"`
	Raw     []string        `json:"raw"`
	Contact *entity.Contact `json:"contact,omitempty"`
}

func randomID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

// CreateJob parses the workbook once. Subsequent page/filter/progress requests
// use only this scoped identifier, never the original workbook or network probes.
func CreateJob(ctx context.Context, p *governance.Principal, data string, keywords []string, requested string) (id string, err error) {
	if !p.Has("contact:import") {
		return "", errors.New("Import access denied")
	}
	sender, err := ResolveSender(ctx, p, requested)
	if err != nil {
		return "", err
	}
	hash := sourceHash(data, keywords)
	if hash == "" || data == "" {
		return "", errors.New("import_preview_required")
	}
	lockHash := sha256.Sum256([]byte(fmt.Sprintf("%d:%d:%s:%s", p.ID, p.DeptID, hash, sender)))
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SELECT pg_advisory_xact_lock(?)", int64(binary.BigEndian.Uint64(lockHash[:8])&0x7fffffffffffffff)); e != nil {
			return e
		}
		old, e := tx.Model("bm_contact_import_jobs").Where("account_id", p.ID).Where("dept_id", p.DeptID).Where("source_hash", hash).Where("validation_sender", sender).Where("expires_at>now()").Order("created_at DESC").Value("id")
		if e != nil {
			return e
		}
		if !old.IsEmpty() {
			id = old.String()
			return nil
		}
		scan, e := ReadLocal(ctx, data, keywords)
		if e != nil {
			return e
		}
		id, e = randomID()
		if e != nil {
			return e
		}
		headers, _ := json.Marshal(scan.Headers)
		if _, e = tx.Model("bm_contact_import_jobs").Data(g.Map{"id": id, "account_id": p.ID, "dept_id": p.DeptID, "source_hash": hash, "validation_sender": sender, "headers": string(headers)}).Insert(); e != nil {
			return e
		}
		contacts := map[string]*entity.Contact{}
		for _, contact := range scan.Contacts {
			contacts[contact.Email] = contact
		}
		rows := make([]g.Map, 0, len(scan.Rows))
		for _, row := range scan.Rows {
			status, reason := row.Status, row.Reason
			if status == "valid" {
				status, reason = "queued", "verification_pending"
			}
			body, _ := json.Marshal(storedImportRow{Row: row, Raw: row.Raw, Contact: contacts[row.Email]})
			rows = append(rows, g.Map{"job_id": id, "source_row": row.Row, "email": row.Email, "status": status, "reason": reason, "payload": string(body)})
		}
		if len(rows) > 0 {
			_, e = tx.Model("bm_contact_import_rows").Data(rows).Batch(200).Insert()
		}
		return e
	})
	return
}

func loadJob(ctx context.Context, p *governance.Principal, id string) (gdb.Record, error) {
	if len(id) != 64 {
		return nil, errors.New("import_preview_changed")
	}
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != 32 || !p.Has("contact:import") {
		return nil, errors.New("import_preview_changed")
	}
	job, err := g.DB().Model("bm_contact_import_jobs").Ctx(ctx).Where("id", id).Where("account_id", p.ID).Where("dept_id", p.DeptID).Where("expires_at>now()").One()
	if err != nil {
		return nil, err
	}
	if len(job) == 0 {
		return nil, errors.New("import_preview_changed")
	}
	if _, err = ResolveSender(ctx, p, job["validation_sender"].String()); err != nil {
		return nil, err
	}
	return job, nil
}

func statusQuery(ctx context.Context, id, status string) *gdb.Model {
	q := g.DB().Model("bm_contact_import_rows").Ctx(ctx).Where("job_id", id)
	if status == "pending" {
		return q.WhereIn("status", []string{"queued", "checking", "unconfirmed"})
	}
	if status != "" && status != "all" {
		q = q.Where("status", status)
	}
	return q
}

func decodeImportRow(record gdb.Record) (storedImportRow, error) {
	var value storedImportRow
	if err := json.Unmarshal(record["payload"].Bytes(), &value); err != nil {
		return value, err
	}
	value.Row.Status, value.Row.Reason = record["status"].String(), record["reason"].String()
	value.Row.Raw = value.Raw
	return value, nil
}

// jobSnapshot freezes verified and terminal unconfirmed sets, so progress cannot
// silently expand an approved import. It is reused across pagination/polling.
func jobSnapshot(ctx context.Context, tx gdb.TX, p *governance.Principal, job gdb.Record, counts map[string]int) (string, *Result, error) {
	if counts["valid"] == 0 && counts["unconfirmed"] == 0 {
		return "", nil, nil
	}
	// Repeatable read gives the count, approved contacts and signature one view
	// even while the workers publish new verified addresses.
	var token string
	var snapshot *Result
	err := func() error {
		// Verified rows only accumulate. Unconfirmed rows may be retried, so their
		// identities, not only their count, must identify the frozen set. Compute a
		// small DB fingerprint without reloading workbook payloads on each poll.
		pendingFingerprint := ""
		if counts["unconfirmed"] > 0 {
			value, err := tx.GetValue("SELECT md5(string_agg(source_row::text, ',' ORDER BY source_row)) FROM bm_contact_import_rows WHERE job_id=? AND status='unconfirmed'", job["id"].String())
			if err != nil {
				return err
			}
			pendingFingerprint = value.String()
		}
		hash := sha256.Sum256([]byte(fmt.Sprintf("pending-opt-in:%s:%d:%s", job["id"].String(), counts["valid"], pendingFingerprint)))
		token = hex.EncodeToString(hash[:])
		existing, err := tx.Model("bm_contact_import_previews").Where("token", token).Where("account_id", p.ID).Where("dept_id", p.DeptID).Where("expires_at>?", time.Now().Add(time.Minute)).Count()
		if err != nil || existing > 0 {
			return err
		}
		records, err := tx.Model("bm_contact_import_rows").Where("job_id", job["id"]).WhereIn("status", []string{"valid", "unconfirmed"}).Order("source_row").All()
		if err != nil {
			return err
		}
		scan := &Result{Token: token, ValidationSender: job["validation_sender"].String(), Valid: counts["valid"], Invalid: counts["invalid"], Filtered: counts["filtered"], Duplicate: counts["duplicate"], Pending: counts["queued"] + counts["checking"] + counts["unconfirmed"], Contacts: []*entity.Contact{}}
		for _, n := range counts {
			scan.Total += n
		}
		for _, record := range records {
			row, err := decodeImportRow(record)
			if err != nil {
				return err
			}
			if row.Contact == nil {
				return errors.New("missing import contact payload")
			}
			if record["status"].String() == "valid" {
				scan.Contacts = append(scan.Contacts, row.Contact)
			} else {
				scan.PendingContacts = append(scan.PendingContacts, row.Contact)
			}
		}
		snapshot = scan
		return nil
	}()
	return token, snapshot, err
}

func JobPreview(ctx context.Context, p *governance.Principal, id, status string, page int, retry bool) (g.Map, error) {
	job, err := loadJob(ctx, p, id)
	if err != nil {
		return nil, err
	}
	retried := int64(0)
	if retry {
		result, e := g.DB().Model("bm_contact_import_rows").Ctx(ctx).Where("job_id", id).Where("status", "unconfirmed").Where("retry_after<=now()").Data(g.Map{"status": "queued", "reason": "verification_pending", "lease_token": nil, "lease_until": nil}).Update()
		if e != nil {
			return nil, e
		}
		retried, _ = result.RowsAffected()
	}
	// One read transaction keeps the counts, page and approval proof consistent.
	var response g.Map
	var snapshot *Result
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		if _, e := tx.Exec("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ"); e != nil {
			return e
		}
		groups, e := tx.Model("bm_contact_import_rows").Where("job_id", id).Fields("status,count(*) AS count").Group("status").All()
		if e != nil {
			return e
		}
		counts := map[string]int{}
		total := 0
		for _, row := range groups {
			n := row["count"].Int()
			counts[row["status"].String()] = n
			total += n
		}
		if page < 1 {
			page = 1
		}
		q := tx.Model("bm_contact_import_rows").Where("job_id", id)
		if status == "pending" {
			q = q.WhereIn("status", []string{"queued", "checking", "unconfirmed"})
		} else if status != "" && status != "all" {
			q = q.Where("status", status)
		}
		n, e := q.Count()
		if e != nil {
			return e
		}
		records, e := q.Order("source_row").Page(page, 20).All()
		if e != nil {
			return e
		}
		rows := []Row{}
		for _, record := range records {
			value, e := decodeImportRow(record)
			if e != nil {
				return e
			}
			rows = append(rows, value.Row)
		}
		token, approved, e := jobSnapshot(ctx, tx, p, job, counts)
		if e != nil {
			return e
		}
		snapshot = approved
		active := counts["queued"] + counts["checking"]
		response = g.Map{"job_id": id, "total": total, "valid": counts["valid"], "invalid": counts["invalid"], "filtered": counts["filtered"], "duplicate": counts["duplicate"], "pending": active + counts["unconfirmed"], "queued": counts["queued"], "checking": counts["checking"], "unconfirmed": counts["unconfirmed"], "active": active, "completed": total - active, "finished": active == 0, "retried": retried, "token": token, "validation_sender": job["validation_sender"].String(), "rows": rows, "row_total": n, "page": page, "page_size": 20}
		return nil
	})
	// Publish the frozen snapshot outside the read transaction, allowing parallel
	// page requests to safely upsert the same proof under READ COMMITTED.
	if err == nil && snapshot != nil {
		err = storePreviewHash(ctx, p, job["source_hash"].String(), snapshot)
	}
	return response, err
}

func JobWorkbook(ctx context.Context, p *governance.Principal, id, status string) ([]byte, error) {
	job, err := loadJob(ctx, p, id)
	if err != nil {
		return nil, err
	}
	scan := &Result{}
	if err = json.Unmarshal(job["headers"].Bytes(), &scan.Headers); err != nil {
		return nil, err
	}
	records, err := statusQuery(ctx, id, status).Order("source_row").All()
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		value, e := decodeImportRow(record)
		if e != nil {
			return nil, e
		}
		if status == "pending" {
			value.Row.Status = "pending"
		}
		scan.Rows = append(scan.Rows, value.Row)
	}
	return scan.StatusWorkbook(status)
}

type importJob struct {
	ID                   int64
	Email, Sender, Lease string
}

var importSlots = make(chan struct{}, 8)
var importPump sync.Mutex
var importMaintenance time.Time

func recoverImportRows(ctx context.Context) error {
	_, err := g.DB().Exec(ctx, `UPDATE bm_contact_import_rows SET status=CASE WHEN attempts<2 THEN 'queued' ELSE 'unconfirmed' END,reason=CASE WHEN attempts<2 THEN 'verification_pending' ELSE 'verification_interrupted' END,retry_after=now()+interval '5 minutes',lease_token=NULL,lease_until=NULL WHERE status='checking' AND lease_until<now()`)
	return err
}

func claimImportRow(ctx context.Context) (job *importJob, err error) {
	err = g.DB().Transaction(ctx, func(ctx context.Context, tx gdb.TX) error {
		row, e := tx.GetOne(`SELECT r.id,r.job_id,r.email,j.validation_sender FROM bm_contact_import_rows r JOIN bm_contact_import_jobs j ON j.id=r.job_id WHERE r.status='queued' AND j.expires_at>now() ORDER BY j.last_dispatch_at NULLS FIRST,r.id FOR UPDATE OF r SKIP LOCKED LIMIT 1`)
		if e != nil || len(row) == 0 {
			return e
		}
		lease, e := randomID()
		if e != nil {
			return e
		}
		_, e = tx.Model("bm_contact_import_rows").Where("id", row["id"]).Data(g.Map{"status": "checking", "reason": "verification_checking", "attempts": gdb.Raw("attempts+1"), "lease_token": lease, "lease_until": time.Now().Add(45 * time.Second)}).Update()
		if e == nil {
			_, e = tx.Model("bm_contact_import_jobs").Where("id", row["job_id"]).Data(g.Map{"last_dispatch_at": time.Now()}).Update()
		}
		if e == nil {
			job = &importJob{ID: row["id"].Int64(), Email: row["email"].String(), Sender: row["validation_sender"].String(), Lease: lease}
		}
		return e
	})
	return
}

func processImportRow(ctx context.Context, job *importJob) {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	r := emailvalidation.Check(ctx, job.Email, job.Sender, true)
	status := "unconfirmed"
	if r.Status == emailvalidation.Invalid {
		status = "invalid"
	}
	if r.Status == emailvalidation.Valid && r.Stage == "smtp" {
		status = "valid"
	}
	if r.Code == "" {
		r.Code = "smtp_unconfirmed"
	}
	if r.ExpiresAt.Before(time.Now()) {
		r.ExpiresAt = time.Now().Add(5 * time.Minute)
	}
	// Persist completion even when the network deadline has elapsed.
	saveCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	_, err := g.DB().Model("bm_contact_import_rows").Ctx(saveCtx).Where("id", job.ID).Where("lease_token", job.Lease).Data(g.Map{"status": status, "reason": r.Code, "checked_at": time.Now(), "retry_after": r.ExpiresAt, "lease_token": nil, "lease_until": nil}).Update()
	if err != nil {
		g.Log().Error(saveCtx, "Save import verification row:", err)
	}
}

// ProcessJobs uses persisted leases, so jobs resume after deployment. Completed
// uncertain rows are terminal until an explicit, backoff-limited manual retry.
func ProcessJobs(ctx context.Context) {
	if !importPump.TryLock() {
		return
	}
	defer importPump.Unlock()
	if time.Since(importMaintenance) > time.Minute {
		importMaintenance = time.Now()
		err := recoverImportRows(ctx)
		if err != nil {
			g.Log().Error(ctx, err)
			return
		}
		_, _ = g.DB().Exec(ctx, `DELETE FROM bm_contact_import_jobs WHERE expires_at<now()-interval '1 day'`)
		_, _ = g.DB().Exec(ctx, `DELETE FROM bm_contact_import_previews WHERE expires_at<now()`)
	}
	for {
		select {
		case importSlots <- struct{}{}:
		default:
			return
		}
		job, err := claimImportRow(ctx)
		if err != nil || job == nil {
			<-importSlots
			if err != nil {
				g.Log().Error(ctx, "Claim import verification row:", err)
			}
			return
		}
		go func() {
			defer func() {
				<-importSlots
				if value := recover(); value != nil {
					g.Log().Error(context.Background(), "Import verification worker:", value)
				}
			}()
			processImportRow(context.Background(), job)
		}()
	}
}

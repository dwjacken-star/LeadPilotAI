package growth

// r is always an already-authorized run row. These correlated aggregates are
// fetched in one statement for the 20 visible rows, without returning other scopes.
const runStatsSQL = `SELECT jsonb_build_object(
 'query_count',(SELECT count(*) FROM bm_growth_queries q WHERE q.run_id=r.id),
 'source_count',(SELECT coalesce(sum(CASE WHEN jsonb_typeof(q.sources)='array' THEN jsonb_array_length(q.sources) ELSE 0 END),0) FROM bm_growth_queries q WHERE q.run_id=r.id),
 'candidate_count',(SELECT count(*) FROM bm_growth_candidates c WHERE c.run_id=r.id),
 'email_count',(SELECT count(DISTINCT e.email) FROM bm_growth_candidates c CROSS JOIN LATERAL jsonb_array_elements_text(c.emails) e(email) WHERE c.run_id=r.id),
 'filtered_count',(SELECT coalesce(sum(c.filtered_count),0) FROM bm_growth_candidates c WHERE c.run_id=r.id),
 'imported_count',(SELECT coalesce(sum(c.imported_count),0) FROM bm_growth_candidates c WHERE c.run_id=r.id),
 'verified_count',(SELECT count(DISTINCT l.id) FROM bm_growth_jobs j JOIN bm_growth_leads l ON l.id::text=j.resource_id WHERE j.kind='verify' AND j.payload->>'run_id'=r.id::text AND l.state IN('verified','contacted','replied')),
 'unconfirmed_count',(SELECT count(DISTINCT l.id) FROM bm_growth_jobs j JOIN bm_growth_leads l ON l.id::text=j.resource_id WHERE j.kind='verify' AND j.payload->>'run_id'=r.id::text AND l.state='unconfirmed'),
 'invalid_count',(SELECT count(DISTINCT l.id) FROM bm_growth_jobs j JOIN bm_growth_leads l ON l.id::text=j.resource_id WHERE j.kind='verify' AND j.payload->>'run_id'=r.id::text AND l.state='invalid'),
 'draft_count',(SELECT count(*) FROM bm_growth_messages m WHERE m.run_id=r.id),
 'failed_jobs',(SELECT count(*) FROM bm_growth_jobs j WHERE j.payload->>'run_id'=r.id::text AND j.status IN('failed','configuration_required')),
 'failed_pages',(SELECT count(*) FROM bm_growth_candidates c WHERE c.run_id=r.id AND c.state IN('failed','partial'))
)`

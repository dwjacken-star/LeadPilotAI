//go:build ignore

// Run explicitly from the core working directory: go run scripts/meta-capability-probe.go
// Read-only: never calls InitDatabase, Migrate, Protect, reserveAPI or a publishing endpoint.
package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"billionmail-core/internal/consts"
	"billionmail-core/internal/service/aicredentials"
	"billionmail-core/internal/service/public"
	_ "github.com/lib/pq"
)

func decrypt(value string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	data, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(data) < aead.NonceSize() {
		return "", fmt.Errorf("invalid encrypted credential")
	}
	data, err = aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte("growth"))
	return string(data), err
}

type probeConfig struct {
	Facebook struct {
		ClientID string `json:"client_id"`
		Secret   string `json:"client_secret"`
		Version  string `json:"version"`
	} `json:"facebook"`
}
type probeAccount struct {
	ID        int64  `json:"id"`
	External  string `json:"external_id"`
	Encrypted string `json:"token"`
	Target    string `json:"target_type"`
}

func main() {
	root := flag.String("root", "", "Core runtime root; defaults to current directory, NOT probe executable directory")
	envFile := flag.String("env-file", "", "Existing environment file override")
	dbHost := flag.String("db-host", "", "Existing PostgreSQL socket directory or hostname override")
	keyFile := flag.String("key-file", "", "Existing encryption key path override")
	fromStdin := flag.Bool("stdin", false, "Read encrypted {config,accounts} JSON from stdin instead of opening the database")
	flag.Parse()
	if *root == "" {
		*root, _ = os.Getwd()
	}
	public.ROOT_PATH, _ = filepath.Abs(*root)
	if *envFile == "" {
		*envFile = public.AbsPath(consts.DEFAULT_DOCKER_ENV_FILE)
	}
	if *dbHost == "" {
		*dbHost = public.AbsPath(consts.POSTGRESQL_SOCK)
	}
	if *keyFile == "" {
		*keyFile = filepath.Join(aicredentials.StorageRoot(), ".ai-master-key")
	}
	if err := run(*envFile, *dbHost, *keyFile, *fromStdin); err != nil {
		b, _ := json.Marshal(map[string]string{"probe_error": err.Error()})
		fmt.Println(string(b))
		os.Exit(1)
	}
}

func run(envFile, dbHost, keyFile string, fromStdin bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	var config probeConfig
	var accounts []probeAccount
	if fromStdin {
		var input struct {
			Config   probeConfig    `json:"config"`
			Accounts []probeAccount `json:"accounts"`
		}
		if err := json.NewDecoder(io.LimitReader(os.Stdin, 1024*1024)).Decode(&input); err != nil || len(input.Accounts) > 20 {
			return fmt.Errorf("encrypted_stdin_invalid")
		}
		config, accounts = input.Config, input.Accounts
	} else {
		env, err := os.ReadFile(envFile)
		if err != nil {
			return fmt.Errorf("env_file_unavailable")
		}
		password := ""
		for _, line := range strings.Split(string(env), "\n") {
			name, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if ok && strings.TrimSpace(name) == "DBPASS" {
				password = strings.Trim(strings.TrimSpace(value), `"'`)
			}
		}
		if password == "" {
			return fmt.Errorf("db_password_missing")
		}
		quote := func(s string) string {
			return "'" + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), "'", `\'`) + "'"
		}
		db, err := sql.Open("postgres", "host="+quote(dbHost)+" user=billionmail dbname=billionmail password="+quote(password)+" sslmode=disable")
		if err != nil {
			return fmt.Errorf("db_open_failed")
		}
		defer db.Close()
		tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
		if err != nil {
			return fmt.Errorf("db_connect_readonly_failed")
		}
		defer tx.Rollback()
		var raw string
		if err = tx.QueryRowContext(ctx, "SELECT value FROM bm_options WHERE name='social_config'").Scan(&raw); err != nil {
			return fmt.Errorf("social_config_read_failed")
		}
		if json.Unmarshal([]byte(raw), &config) != nil {
			return fmt.Errorf("social_config_invalid")
		}
		rows, err := tx.QueryContext(ctx, "SELECT id,external_id,token,target_type FROM bm_social_accounts WHERE platform='facebook' ORDER BY id LIMIT 20")
		if err != nil {
			return fmt.Errorf("accounts_query_failed")
		}
		for rows.Next() {
			var a probeAccount
			if err = rows.Scan(&a.ID, &a.External, &a.Encrypted, &a.Target); err != nil {
				rows.Close()
				return fmt.Errorf("account_decode_failed")
			}
			accounts = append(accounts, a)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return fmt.Errorf("accounts_read_failed")
		}
		// Release the read-only transaction before the bounded remote reads.
		if err = tx.Rollback(); err != nil {
			return fmt.Errorf("readonly_transaction_close_failed")
		}
	}
	key, err := os.ReadFile(keyFile)
	if err != nil || len(key) != 32 {
		return fmt.Errorf("existing_key_unavailable_or_invalid")
	}
	secret, err := decrypt(config.Facebook.Secret, key)
	if err != nil {
		return fmt.Errorf("app_secret_decrypt_failed")
	}
	version := config.Facebook.Version
	if version == "" {
		version = "v24.0"
	}
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	get := func(path, token string) (map[string]any, map[string]any) {
		req, e := http.NewRequestWithContext(ctx, "GET", "https://graph.facebook.com/"+version+"/"+path, nil)
		if e != nil {
			return nil, map[string]any{"result": "request_unavailable"}
		}
		req.Header.Set("Authorization", "Bearer "+token)
		response, e := client.Do(req)
		if e != nil {
			return nil, map[string]any{"result": "network_unavailable"}
		}
		defer response.Body.Close()
		data, e := io.ReadAll(io.LimitReader(response.Body, 1024*1024))
		var result map[string]any
		if e != nil || json.Unmarshal(data, &result) != nil {
			return nil, map[string]any{"result": "invalid_response", "http_status": response.StatusCode}
		}
		if failure, ok := result["error"].(map[string]any); ok {
			return nil, map[string]any{"result": "provider_error", "http_status": response.StatusCode, "code": failure["code"], "subcode": failure["error_subcode"]}
		}
		return result, map[string]any{"result": "ok", "http_status": response.StatusCode}
	}
	emit := func(v any) { b, _ := json.Marshal(v); fmt.Println(string(b)) }
	emit(map[string]any{"probe": "read_only", "encryption": "AES-256-GCM/base64(nonce+ciphertext), AAD growth", "dashboard_access": "unverified"})
	for _, a := range accounts {
		out := map[string]any{"account_id": a.ID, "target_type": a.Target}
		if a.Encrypted == "" {
			out["token"] = "absent"
			emit(out)
			continue
		}
		token, e := decrypt(a.Encrypted, key)
		if e != nil {
			out["token"] = "decrypt_failed"
			emit(out)
			continue
		}
		out["token"] = "decrypted"
		info, status := get("debug_token?"+url.Values{"input_token": {token}}.Encode(), config.Facebook.ClientID+"|"+secret)
		out["debug"] = status
		if data, ok := info["data"].(map[string]any); ok {
			out["is_valid"] = data["is_valid"]
			out["app_matches"] = data["app_id"] == config.Facebook.ClientID
			out["token_type"] = data["type"]
			out["scopes"] = data["scopes"]
			out["expires_at"] = data["expires_at"]
			out["data_access_expires_at"] = data["data_access_expires_at"]
			if profile, ok := data["profile_id"].(string); ok {
				out["profile_matches"] = profile == a.External
			}
		}
		identity, status := get("me?fields=id", token)
		out["identity_probe"] = status
		out["asset_matches"] = identity["id"] == a.External
		out["create_content_task"] = "unverified"
		if a.Target == "page" {
			// A persisted Page token may not support /me/accounts. Failure is
			// inconclusive: do not infer missing tasks or invalid authorization.
			listed, status := get("me/accounts?fields=id,tasks&limit=100", token)
			out["tasks_probe"] = status
			if entries, ok := listed["data"].([]any); ok {
				for _, item := range entries {
					page, ok := item.(map[string]any)
					if !ok || page["id"] != a.External {
						continue
					}
					if tasks, ok := page["tasks"].([]any); ok {
						canCreate := false
						for _, task := range tasks {
							if task == "CREATE_CONTENT" || task == "MANAGE" {
								canCreate = true
							}
						}
						out["create_content_task"] = canCreate
						out["tasks"] = tasks
					}
				}
			}
		}
		emit(out)
	}
	return nil
}

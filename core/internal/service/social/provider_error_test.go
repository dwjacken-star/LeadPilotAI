package social

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestProviderErrorsKeepDiagnosticsAndRedactCredentials(t *testing.T) {
	prior := httpClient
	t.Cleanup(func() { httpClient = prior })
	for _, status := range []int{200, 400} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			secret, code, token := "fixture-client-secret", "fixture-code", "fixture-access-token"
			form := url.Values{"client_secret": {secret}, "code": {code}}
			req, err := http.NewRequest("POST", "https://graph.facebook.com/v24.0/oauth/access_token", strings.NewReader(form.Encode()))
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer "+token)
			payload, _ := json.Marshal(map[string]any{"error": map[string]any{"code": 190, "error_subcode": 463, "message": "Token expired " + secret + " " + code + " " + token}})
			httpClient = &http.Client{Transport: transportFunc(func(*http.Request) (*http.Response, error) { return response(status, string(payload)), nil })}
			_, _, err = execute(req, false)
			var remote *RemoteError
			if !errors.As(err, &remote) || remote.Kind != "authorization_expired" || remote.Code != 190 || remote.Subcode != 463 || !strings.Contains(remote.Detail, "Token expired") {
				t.Fatalf("missing diagnostics: %v", err)
			}
			for _, value := range []string{secret, code, token} {
				if strings.Contains(err.Error(), value) {
					t.Fatal("credential exposed")
				}
			}
		})
	}
}

func TestFacebookContentTasksAcrossPageExperiences(t *testing.T) {
	for _, role := range []string{"CREATE_CONTENT", "MANAGE", "PROFILE_PLUS_FULL_CONTROL", "PROFILE_PLUS_CREATE_CONTENT"} {
		if !facebookCanCreateContent([]string{role}) {
			t.Fatalf("content task rejected: %s", role)
		}
	}
	for _, tasks := range [][]string{nil, {"ANALYZE"}, {"ADVERTISE", "MESSAGING"}, {"PROFILE_PLUS_ANALYZE"}, {"PROFILE_PLUS_FACEBOOK_ACCESS"}} {
		if facebookCanCreateContent(tasks) {
			t.Fatalf("non-publishing access accepted: %v", tasks)
		}
	}
}

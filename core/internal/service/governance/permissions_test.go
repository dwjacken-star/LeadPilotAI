package governance

import (
	"reflect"
	"testing"
)

func TestRolePermissionAssignment(t *testing.T) {
	original := PermissionCatalog
	t.Cleanup(func() { PermissionCatalog = original })
	PermissionCatalog = append([]string(nil), original...)
	PermissionCatalog = append(PermissionCatalog, "growth:settings", "social:settings")
	for code, dependencies := range RolePermissionDependencies {
		PermissionCatalog = append(PermissionCatalog, code)
		PermissionCatalog = append(PermissionCatalog, dependencies...)
	}
	for _, tc := range []struct {
		name  string
		codes []string
		want  []string
	}{
		{"social account management includes view", []string{"social:account:manage"}, []string{"social:account:manage", "social:account:view"}},
		{"review includes plan view", []string{"social:content:review"}, []string{"social:content:review", "social:plan:view"}},
		{"intelligence execution includes view", []string{"intelligence:run"}, []string{"intelligence:run", "intelligence:view"}},
		{"admin settings are not delegated", []string{"growth:settings", "social:settings", "contact:list"}, []string{"contact:list"}},
		{"mailbox writing role stays restricted", []string{"campaign:create"}, []string{"campaign:create"}},
		{"campaign editing includes its menu", []string{"campaign:update"}, []string{"campaign:update", "campaign:list"}},
		{"campaign deletion includes its menu", []string{"campaign:delete"}, []string{"campaign:delete", "campaign:list"}},
		{"campaign publication includes its menu", []string{"campaign:publish"}, []string{"campaign:publish", "campaign:list"}},
		{"campaign pause includes its menu", []string{"campaign:pause"}, []string{"campaign:pause", "campaign:list"}},
		{"campaign cancellation includes its menu", []string{"campaign:cancel"}, []string{"campaign:cancel", "campaign:list"}},
		{"overview is independent of task access", []string{"overview:view"}, []string{"overview:view"}},
		{"duplicates and existing views", []string{"social:account:manage", "social:account:view", "social:account:manage"}, []string{"social:account:manage", "social:account:view"}},
		{"empty role", nil, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeRolePermissions(tc.codes)
			if err != nil || !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, %v; want %v", got, err, tc.want)
			}
			again, err := NormalizeRolePermissions(got)
			if err != nil || !reflect.DeepEqual(again, got) {
				t.Fatalf("normalization is not idempotent: %v, %v", again, err)
			}
		})
	}
	for _, code := range []string{"*", "unknown:permission"} {
		if _, err := NormalizeRolePermissions([]string{code}); err == nil {
			t.Fatalf("accepted invalid non-admin grant %q", code)
		}
	}
	for _, code := range AssignablePermissionCatalog() {
		if adminOnlyPermission(code) {
			t.Fatalf("administrator-only setting is assignable: %s", code)
		}
	}
}

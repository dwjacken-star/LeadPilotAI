package governance

import "fmt"

var RolePermissionDependencies = map[string][]string{
	"campaign:update":            {"campaign:list"},
	"campaign:delete":            {"campaign:list"},
	"campaign:publish":           {"campaign:list"},
	"campaign:pause":             {"campaign:list"},
	"campaign:cancel":            {"campaign:list"},
	"social:account:manage":      {"social:account:view"},
	"social:plan:manage":         {"social:plan:view"},
	"social:content:edit":        {"social:plan:view"},
	"social:content:review":      {"social:plan:view"},
	"social:publish":             {"social:plan:view"},
	"social:interaction:manage":  {"social:interaction:view"},
	"social:interaction:analyze": {"social:interaction:view"},
	"intelligence:manage":        {"intelligence:view"},
	"intelligence:run":           {"intelligence:view"},
	"intelligence:review":        {"intelligence:view"},
	"intelligence:publish":       {"intelligence:view"},
	"intelligence:settings":      {"intelligence:view"},
	"intelligence:export":        {"intelligence:view"},
}

func adminOnlyPermission(code string) bool {
	return code == "growth:settings" || code == "social:settings"
}

func AssignablePermissionCatalog() []string {
	catalog := make([]string, 0, len(PermissionCatalog))
	for _, code := range PermissionCatalog {
		if !adminOnlyPermission(code) {
			catalog = append(catalog, code)
		}
	}
	return catalog
}

func NormalizeRolePermissions(codes []string) ([]string, error) {
	known := make(map[string]bool, len(PermissionCatalog))
	for _, code := range PermissionCatalog {
		known[code] = true
	}
	result := make([]string, 0, len(codes))
	seen := make(map[string]bool)
	var add func(string) error
	add = func(code string) error {
		if !known[code] {
			return fmt.Errorf("未知权限：%s", code)
		}
		if seen[code] || adminOnlyPermission(code) {
			return nil
		}
		seen[code] = true
		result = append(result, code)
		for _, dependency := range RolePermissionDependencies[code] {
			if err := add(dependency); err != nil {
				return err
			}
		}
		return nil
	}
	for _, code := range codes {
		if err := add(code); err != nil {
			return nil, err
		}
	}
	return result, nil
}

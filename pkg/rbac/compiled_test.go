package rbac

import (
	"testing"

	"github.com/zxh326/kite/pkg/common"
	"github.com/zxh326/kite/pkg/model"
)

// --- Unit tests for compilePatterns / matchCompiled ---

func TestMatchCompiledWildcard(t *testing.T) {
	patterns := compilePatterns([]string{"*"})
	if !matchCompiled(patterns, "anything") {
		t.Error("wildcard should match any value")
	}
}

func TestMatchCompiledLiteralExact(t *testing.T) {
	patterns := compilePatterns([]string{"pods", "services"})
	if !matchCompiled(patterns, "pods") {
		t.Error("literal pattern should match exact value")
	}
	if !matchCompiled(patterns, "services") {
		t.Error("literal pattern should match exact value")
	}
	if matchCompiled(patterns, "deployments") {
		t.Error("literal pattern should not match different value")
	}
}

func TestMatchCompiledRegex(t *testing.T) {
	patterns := compilePatterns([]string{"dev.*"})
	if !matchCompiled(patterns, "dev-cluster") {
		t.Error("regex pattern should match")
	}
	if !matchCompiled(patterns, "dev") {
		t.Error("regex pattern should match exact prefix")
	}
	if matchCompiled(patterns, "prod-cluster") {
		t.Error("regex pattern should not match non-matching value")
	}
}

func TestMatchCompiledNegation(t *testing.T) {
	patterns := compilePatterns([]string{"!kube-system", "*"})
	if !matchCompiled(patterns, "default") {
		t.Error("negation+wildcard should match non-negated value")
	}
	if matchCompiled(patterns, "kube-system") {
		t.Error("negation should block exact match")
	}
}

func TestMatchCompiledEmpty(t *testing.T) {
	patterns := compilePatterns([]string{})
	if matchCompiled(patterns, "anything") {
		t.Error("empty patterns should match nothing")
	}
}

func TestMatchCompiledInvalidRegex(t *testing.T) {
	// Invalid regex should be treated as literal-only; should not panic.
	patterns := compilePatterns([]string{"[invalid"})
	if matchCompiled(patterns, "anything") {
		t.Error("invalid regex pattern should not match arbitrary values")
	}
	// But should still match the literal string itself
	if !matchCompiled(patterns, "[invalid") {
		t.Error("invalid regex should still match as literal")
	}
}

func TestMatchCompiledLiteralSkipsRegex(t *testing.T) {
	// Pattern "pods" has no metacharacters → re should be nil (Solution D)
	patterns := compilePatterns([]string{"pods"})
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].re != nil {
		t.Error("literal pattern 'pods' should not have a compiled regexp (Solution D)")
	}
	if !matchCompiled(patterns, "pods") {
		t.Error("literal pattern should still match via == comparison")
	}
}

func TestMatchCompiledRegexHasMeta(t *testing.T) {
	patterns := compilePatterns([]string{"dev-.*"})
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].re == nil {
		t.Error("pattern 'dev-.*' has metacharacters and should have compiled regexp")
	}
}

// --- Integration: CanAccessCluster / CanAccessNamespace with compiled roles ---

func TestCanAccessClusterCompiled(t *testing.T) {
	setTestRBACConfig(
		[]common.Role{{
			Name:       "dev-access",
			Clusters:   []string{"dev-.*"},
			Namespaces: []string{"*"},
			Resources:  []string{"*"},
			Verbs:      []string{"*"},
		}},
		[]common.RoleMapping{{Name: "dev-access", Users: []string{"alice"}}},
	)

	if !CanAccessCluster(model.User{Username: "alice"}, "dev-east") {
		t.Error("should match dev-east via regex")
	}
	if CanAccessCluster(model.User{Username: "alice"}, "prod-west") {
		t.Error("should NOT match prod-west")
	}
	if CanAccessCluster(model.User{Username: "bob"}, "dev-east") {
		t.Error("bob should have no roles")
	}
}

func TestCanAccessNamespaceCompiled(t *testing.T) {
	setTestRBACConfig(
		[]common.Role{{
			Name:       "ns-role",
			Clusters:   []string{"*"},
			Namespaces: []string{"!kube-system", "team-.*"},
			Resources:  []string{"*"},
			Verbs:      []string{"*"},
		}},
		[]common.RoleMapping{{Name: "ns-role", Users: []string{"*"}}},
	)

	if !CanAccessNamespace(model.User{Username: "anyone"}, "any-cluster", "team-alpha") {
		t.Error("should match team-alpha via regex")
	}
	if CanAccessNamespace(model.User{Username: "anyone"}, "any-cluster", "kube-system") {
		t.Error("kube-system should be negated")
	}
	if CanAccessNamespace(model.User{Username: "anyone"}, "any-cluster", "default") {
		t.Error("default does not match team-.* pattern")
	}
}

// --- Benchmarks: old sequential compile vs pre-compiled ---

func BenchmarkMatchCompiled(b *testing.B) {
	patterns := compilePatterns([]string{"dev-.*", "staging-.*", "!kube-system", "prod"})
	b.ResetTimer()
	for b.Loop() {
		matchCompiled(patterns, "dev-east")
	}
}

func BenchmarkMatchCompiledLiteral(b *testing.B) {
	patterns := compilePatterns([]string{"get", "create", "update", "delete"})
	b.ResetTimer()
	for b.Loop() {
		matchCompiled(patterns, "update")
	}
}

func BenchmarkMatchCompiledWildcard(b *testing.B) {
	patterns := compilePatterns([]string{"*"})
	b.ResetTimer()
	for b.Loop() {
		matchCompiled(patterns, "anything")
	}
}

func BenchmarkCanAccessFullCompiled(b *testing.B) {
	setTestRBACConfig(
		[]common.Role{
			{
				Name:       "dev",
				Clusters:   []string{"dev-.*"},
				Namespaces: []string{"!kube-system", "team-.*"},
				Resources:  []string{"pods", "deployments", "services"},
				Verbs:      []string{"get", "create", "update"},
			},
			{
				Name:       "admin",
				Clusters:   []string{"*"},
				Namespaces: []string{"*"},
				Resources:  []string{"*"},
				Verbs:      []string{"*"},
			},
		},
		[]common.RoleMapping{
			{Name: "dev", Users: []string{"dev-user"}},
			{Name: "admin", Users: []string{"admin-user"}},
		},
	)

	user := model.User{Username: "dev-user"}
	b.ResetTimer()
	for b.Loop() {
		CanAccess(user, "pods", "get", "dev-east", "team-alpha")
	}
}

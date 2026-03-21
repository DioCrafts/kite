package rbac

import (
	"regexp"
	"strings"

	"github.com/zxh326/kite/pkg/common"
	"k8s.io/klog/v2"
)

// compiledPattern holds a pre-compiled representation of a single RBAC pattern.
// Patterns are compiled once during role sync and reused for every access check,
// eliminating regexp.Compile from the hot path entirely.
type compiledPattern struct {
	raw      string         // original pattern string
	negate   bool           // true if prefixed with "!"
	wildcard bool           // true if pattern is "*"
	literal  string         // for exact string comparison (negation target or plain value)
	re       *regexp.Regexp // non-nil only when the pattern contains regex metacharacters
}

// compiledRole wraps a common.Role with pre-compiled patterns for each field.
type compiledRole struct {
	common.Role
	clusters   []compiledPattern
	namespaces []compiledPattern
	resources  []compiledPattern
	verbs      []compiledPattern
}

// regexpMetaDetector matches any regex metacharacter. If a pattern contains none
// of these, it is a literal string and regex matching can be skipped entirely (Solution D).
var regexpMetaDetector = regexp.MustCompile(`[\\.*+?^${}()|[\]]`)

// hasRegexMeta returns true if the pattern contains regex metacharacters.
func hasRegexMeta(p string) bool {
	return regexpMetaDetector.MatchString(p)
}

// compilePatterns converts a slice of raw pattern strings into pre-compiled patterns.
// Called once per sync cycle (every ~60s), not on the hot path.
func compilePatterns(patterns []string) []compiledPattern {
	out := make([]compiledPattern, 0, len(patterns))
	for _, p := range patterns {
		cp := compiledPattern{raw: p}

		switch {
		case len(p) > 1 && strings.HasPrefix(p, "!"):
			// Negation pattern: "!kube-system"
			cp.negate = true
			cp.literal = p[1:]

		case p == "*":
			// Wildcard: matches everything
			cp.wildcard = true

		default:
			// Store literal for exact comparison (always attempted first)
			cp.literal = p

			// Only compile regex if pattern has metacharacters (Solution D)
			if hasRegexMeta(p) {
				re, err := regexp.Compile(p)
				if err != nil {
					klog.Errorf("rbac: invalid regex pattern %q: %v", p, err)
					// Keep as literal-only (will still match via == check)
				} else {
					cp.re = re
				}
			}
		}

		out = append(out, cp)
	}
	return out
}

// compileRole converts a common.Role into a compiledRole with pre-compiled patterns.
func compileRole(r common.Role) compiledRole {
	return compiledRole{
		Role:       r,
		clusters:   compilePatterns(r.Clusters),
		namespaces: compilePatterns(r.Namespaces),
		resources:  compilePatterns(r.Resources),
		verbs:      compilePatterns(r.Verbs),
	}
}

// matchCompiled evaluates pre-compiled patterns against a value.
// Zero allocations, zero regexp.Compile calls on the hot path.
func matchCompiled(patterns []compiledPattern, val string) bool {
	for i := range patterns {
		p := &patterns[i]

		if p.negate {
			if p.literal == val {
				return false
			}
			continue
		}

		if p.wildcard || p.literal == val {
			return true
		}

		// Only invoke regex if a compiled regexp exists (pattern had metacharacters)
		if p.re != nil && p.re.MatchString(val) {
			return true
		}
	}
	return false
}

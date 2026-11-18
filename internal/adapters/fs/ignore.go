package fs

import (
	"errors"
	iofs "io/fs"
	"os"
	"strings"
)

// ignoreRule is one line of .gitignore, pre-parsed for matching.
type ignoreRule struct {
	pattern string
	anchor  bool // anchored to the root or contains a slash
	dirOnly bool
	negate  bool
}

func loadIgnoreRules(file string) ([]ignoreRule, error) {
	data, err := os.ReadFile(file)
	switch {
	case errors.Is(err, iofs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, wrapError(err, gitignoreFile)
	}

	return parseIgnoreRules(string(data)), nil
}

func parseIgnoreRules(content string) []ignoreRule {
	lines := strings.Split(content, "\n")
	rules := make([]ignoreRule, 0, len(lines))

	for _, line := range lines {
		rule, ok := parseIgnoreRule(line)
		if ok {
			rules = append(rules, rule)
		}
	}

	return rules
}

func parseIgnoreRule(line string) (ignoreRule, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}

	rule := ignoreRule{}
	if strings.HasPrefix(line, "!") {
		rule.negate = true
		line = strings.TrimPrefix(line, "!")
	}

	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")
	if strings.HasSuffix(line, "/") {
		rule.dirOnly = true
		line = strings.TrimSuffix(line, "/")
	}
	if line == "" || strings.ContainsRune(line, 0) {
		return ignoreRule{}, false
	}

	rule.pattern = line
	rule.anchor = anchored || strings.Contains(line, "/")

	return rule, true
}

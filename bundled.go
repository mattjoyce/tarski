package datalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// RuleSet is a named, versioned program with a hash over its text. The hash goes into
// every decision so a result can say which rules stood behind it after the rules have
// moved.
type RuleSet struct {
	Name    string
	Version int
	Hash    string // "sha256:<hex>" over the text
	Program Program
}

// NewRuleSet parses text and hashes it. It fails rather than returning a partial set:
// the rules are one reading, and a binary that lost half of it would answer wrongly
// with confidence. The name must be non-empty and free of spaces and semicolons, since
// ID is read back by people and joined with others; the version is not negative. The
// version is part of what an answer means, and moves when the rules' meaning does,
// with the hash catching any change the version did not.
func NewRuleSet(name string, version int, text string) (RuleSet, error) {
	if name == "" || strings.ContainsAny(name, " \t\r\n;/") {
		return RuleSet{}, fmt.Errorf("%w: rule set name %q must be non-empty, with no spaces, slash or semicolon", ErrInvalid, name)
	}
	if version < 0 {
		return RuleSet{}, fmt.Errorf("%w: rule set %s: version %d is negative", ErrInvalid, name, version)
	}
	p, err := Parse(text)
	if err != nil {
		return RuleSet{}, fmt.Errorf("rule set %s/%d: %w", name, version, err)
	}
	sum := sha256.Sum256([]byte(text))
	return RuleSet{Name: name, Version: version, Hash: "sha256:" + hex.EncodeToString(sum[:]), Program: p}, nil
}

// ID is "name/version hash", the string a decision carries.
func (s RuleSet) ID() string {
	return fmt.Sprintf("%s/%d %s", s.Name, s.Version, s.Hash)
}

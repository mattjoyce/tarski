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
// with confidence. The name must be non-empty and free of whitespace, slashes and
// semicolons: ID is "name/version hash", and several are joined with "; ", so any of
// them would make an ID ambiguous to read back. The version is not negative. The
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
	return RuleSet{Name: name, Version: version, Hash: textHash(text), Program: p}, nil
}

// ID is "name/version hash", the string a decision carries.
func (s RuleSet) ID() string {
	return fmt.Sprintf("%s/%d %s", s.Name, s.Version, s.Hash)
}

// Verify checks that the rule set is what its hash says: the hash is recomputed over
// the program's text and compared. A set built by hand, or edited after NewRuleSet,
// would otherwise put in a receipt a hash that matches neither the text nor the rules
// that ran. A program with no text cannot be verified and is refused.
func (s RuleSet) Verify() error {
	if s.Program.Text == "" {
		return fmt.Errorf("%w: rule set %s/%d has no program text to verify its hash against", ErrInvalid, s.Name, s.Version)
	}
	if want := textHash(s.Program.Text); s.Hash != want {
		return fmt.Errorf("%w: rule set %s/%d: hash %q is not the hash of its text (%s)", ErrInvalid, s.Name, s.Version, s.Hash, want)
	}
	return nil
}

// textHash is "sha256:<hex>" over text, the one shape a rule-set hash takes.
func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

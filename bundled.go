package datalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"reflect"
	"unicode"
	"unicode/utf8"
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
	if err := checkIdentity(name, version); err != nil {
		return RuleSet{}, err
	}
	if text == "" {
		return RuleSet{}, fmt.Errorf("%w: rule set %s/%d has no text; a rule set is its text, and Verify would refuse it", ErrInvalid, name, version)
	}
	p, err := Parse(text)
	if err != nil {
		return RuleSet{}, fmt.Errorf("rule set %s/%d: %w", name, version, err)
	}
	p.Name = fmt.Sprintf("%s/%d", name, version) // what Compile tags each rule with
	return RuleSet{Name: name, Version: version, Hash: textHash(text), Program: p}, nil
}

// ID is "name/version hash", the string a decision carries.
func (s RuleSet) ID() string {
	return fmt.Sprintf("%s/%d %s", s.Name, s.Version, s.Hash)
}

// Verify checks that the rule set is what its hash says, and that what runs is what was
// hashed. The hash is recomputed over the program's text and compared. The text is then
// parsed again and the result must equal the set's Program, with the name NewRuleSet
// would give it: the text is the one authoritative representation, so an AST edited
// after NewRuleSet, or built by hand around someone else's hash, is refused. A set
// built or edited otherwise would put in a receipt a hash that matches neither the text
// nor the rules that ran. A program with no text cannot be verified and is refused.
func (s RuleSet) Verify() error {
	if err := checkIdentity(s.Name, s.Version); err != nil {
		return err
	}
	if s.Program.Text == "" {
		return fmt.Errorf("%w: rule set %s/%d has no program text to verify its hash against", ErrInvalid, s.Name, s.Version)
	}
	if want := textHash(s.Program.Text); s.Hash != want {
		return fmt.Errorf("%w: rule set %s/%d: hash %q is not the hash of its text (%s)", ErrInvalid, s.Name, s.Version, s.Hash, want)
	}
	parsed, err := Parse(s.Program.Text)
	if err != nil {
		return fmt.Errorf("%w: rule set %s/%d: its text does not parse: %v", ErrInvalid, s.Name, s.Version, err)
	}
	parsed.Name = fmt.Sprintf("%s/%d", s.Name, s.Version)
	if !reflect.DeepEqual(parsed, s.Program) {
		return fmt.Errorf("%w: rule set %s/%d: the program does not match its text", ErrInvalid, s.Name, s.Version)
	}
	return nil
}

// checkIdentity holds a name and a version to what an ID can carry: a non-empty name
// with no whitespace or control character (so a receipt reads as one line and one
// token), no slash (which separates name from version) and no semicolon (which
// separates one ID from the next), and a version that is not negative. The
// constructor and Verify both ask, so a set edited after construction is held to the
// same rule as one built by it.
func checkIdentity(name string, version int) error {
	if name == "" {
		return fmt.Errorf("%w: rule set name must be non-empty", ErrInvalid)
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == '/' || r == ';' || r == utf8.RuneError {
			return fmt.Errorf("%w: rule set name %q must have no whitespace, control character, slash or semicolon", ErrInvalid, name)
		}
	}
	if version < 0 {
		return fmt.Errorf("%w: rule set %s: version %d is negative", ErrInvalid, name, version)
	}
	return nil
}

// textHash is "sha256:<hex>" over text, the one shape a rule-set hash takes.
func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return "sha256:" + hex.EncodeToString(sum[:])
}

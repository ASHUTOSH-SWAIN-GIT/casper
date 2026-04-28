package action

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// ProposalHash is the hex-encoded sha256 of a proposal's canonical form.
// It is the predictability anchor: every downstream artifact (policy verdict,
// approval, plan, audit event) references this hash, and the interpreter
// refuses to execute a plan whose source proposal hash does not match an
// approved record.
type ProposalHash string

// Canonicalize parses raw proposal JSON and re-emits it in canonical form:
// keys sorted alphabetically at every level, no insignificant whitespace,
// numbers normalized via Go's json.Marshal (float64 shortest representation).
//
// The output is byte-stable across reorderings and reformattings of the
// same logical document. Two proposals with identical content always
// produce identical canonical bytes regardless of how they were authored.
func Canonicalize(raw []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("canonicalize: parse: %w", err)
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, fmt.Errorf("canonicalize: encode: %w", err)
	}
	// json.Encoder appends a trailing newline; strip it so the canonical
	// form is exactly the document with no extra bytes.
	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

// Hash returns the proposal hash for raw proposal JSON.
// It does not validate against the schema — callers should call Validate
// first if they need that guarantee. Hashing an invalid-but-parseable
// proposal is still a deterministic operation; refusing to hash here
// would couple two unrelated concerns.
func Hash(raw []byte) (ProposalHash, error) {
	canon, err := Canonicalize(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canon)
	return ProposalHash(hex.EncodeToString(sum[:])), nil
}

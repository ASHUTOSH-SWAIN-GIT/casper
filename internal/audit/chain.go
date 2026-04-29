package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// computeHash returns the chained hash for an event:
//
//	hash = sha256(prevHash_bytes || canonical(coreFields))
//
// coreFields is everything the chain commits to: kind, proposal_hash,
// payload, and the timestamp. ID is excluded — it's a storage detail.
// PrevHash and Hash are excluded — chain inputs, not chain content.
//
// Canonicalization mirrors action.Canonicalize: parse → re-emit with
// sorted map keys and shortest float representation. Stable across
// reorderings and reformattings.
func computeHash(prevHash string, e Event) (string, error) {
	core := map[string]any{
		"kind":          string(e.Kind),
		"proposal_hash": string(e.ProposalHash),
		"payload":       e.Payload,
		"at":            e.At.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
	}
	canon, err := canonical(core)
	if err != nil {
		return "", err
	}
	prev, err := hex.DecodeString(prevHash)
	if err != nil {
		// First event has prevHash="" which decodes to empty, fine.
		return "", fmt.Errorf("prev hash not hex: %w", err)
	}
	h := sha256.New()
	h.Write(prev)
	h.Write(canon)
	return hex.EncodeToString(h.Sum(nil)), nil
}

func canonical(v any) ([]byte, error) {
	// Round-trip through json.Marshal to get sorted keys + normalized
	// numbers, the same trick action.Canonicalize uses.
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var roundTrip any
	if err := json.Unmarshal(b, &roundTrip); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	enc := json.NewEncoder(&out)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(roundTrip); err != nil {
		return nil, err
	}
	return bytes.TrimRight(out.Bytes(), "\n"), nil
}

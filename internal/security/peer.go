package security

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"time"
)

var (
	ErrUnauthorized    = errors.New("unauthorized")
	ErrReplayedRequest = errors.New("replayed_request")
	ErrExpiredRequest  = errors.New("expired_request")
)

type Peer struct {
	NodeID, OrganizationID string
	PublicKey              []byte
}
type PeerEnvelope struct {
	RequestID, SourceNodeID, SourceOrganizationID, TargetNodeID, HTTPMethod, RequestPath string
	BodyHash                                                                             [sha256.Size]byte
	IssuedAt, ExpiresAt                                                                  time.Time
	Nonce                                                                                string
	Signature                                                                            []byte
}

func peerBytes(e PeerEnvelope) []byte {
	values := []string{"healthtrust-peer-request-v1", e.RequestID, e.SourceNodeID, e.SourceOrganizationID, e.TargetNodeID, e.HTTPMethod, e.RequestPath, e.Nonce}
	var out []byte
	for _, v := range values {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(v)))
		out = append(out, size[:]...)
		out = append(out, []byte(v)...)
	}
	out = append(out, e.BodyHash[:]...)
	for _, v := range []int64{e.IssuedAt.UTC().UnixNano(), e.ExpiresAt.UTC().UnixNano()} {
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(v))
		out = append(out, b[:]...)
	}
	return out
}
func SignPeerEnvelope(e PeerEnvelope, key ed25519.PrivateKey) PeerEnvelope {
	e.Signature = ed25519.Sign(key, peerBytes(e))
	return e
}
func VerifyPeerEnvelope(e PeerEnvelope, peer Peer, targetNode, method, path string, body []byte, now time.Time) error {
	if peer.NodeID != e.SourceNodeID || peer.OrganizationID != e.SourceOrganizationID || e.TargetNodeID != targetNode {
		return ErrUnauthorized
	}
	if e.HTTPMethod != method || e.RequestPath != path || e.BodyHash != sha256.Sum256(body) {
		return ErrUnauthorized
	}
	if now.Before(e.IssuedAt.Add(-time.Minute)) || now.After(e.ExpiresAt) || e.ExpiresAt.Sub(e.IssuedAt) > 5*time.Minute {
		return ErrExpiredRequest
	}
	if len(peer.PublicKey) != ed25519.PublicKeySize || !ed25519.Verify(ed25519.PublicKey(peer.PublicKey), peerBytes(e), e.Signature) {
		return ErrUnauthorized
	}
	return nil
}

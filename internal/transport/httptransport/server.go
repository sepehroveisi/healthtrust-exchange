package httptransport

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
)

const maxRequestBytes = 1 << 20

type NodeInfoResponse struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Height         uint64 `json:"height"`
}
type ChainSummaryResponse struct {
	Height     uint64   `json:"height"`
	LatestHash [32]byte `json:"latestHash"`
}
type ErrorResponse struct {
	Error string `json:"error"`
}

func Handler(n *node.Node) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /node/info", func(w http.ResponseWriter, _ *http.Request) {
		latest, _ := n.Chain.GetLatestBlock()
		writeJSON(w, http.StatusOK, NodeInfoResponse{n.ID, n.OrganizationID, latest.Height})
	})
	mux.HandleFunc("GET /chain", func(w http.ResponseWriter, _ *http.Request) {
		latest, _ := n.Chain.GetLatestBlock()
		writeJSON(w, http.StatusOK, ChainSummaryResponse{latest.Height, latest.Hash})
	})
	mux.HandleFunc("GET /blocks", func(w http.ResponseWriter, r *http.Request) {
		from, err := strconv.ParseUint(r.URL.Query().Get("from"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid from height")
			return
		}
		blocks := n.Chain.Blocks()
		if from >= uint64(len(blocks)) {
			writeJSON(w, http.StatusOK, []blockchain.Block{})
			return
		}
		writeJSON(w, http.StatusOK, blocks[from:])
	})
	mux.HandleFunc("GET /blocks/{height}", func(w http.ResponseWriter, r *http.Request) {
		height, err := strconv.ParseUint(r.PathValue("height"), 10, 64)
		blocks := n.Chain.Blocks()
		if err != nil || height >= uint64(len(blocks)) {
			writeError(w, http.StatusNotFound, "block not found")
			return
		}
		writeJSON(w, http.StatusOK, blocks[height])
	})
	mux.HandleFunc("POST /blocks", func(w http.ResponseWriter, r *http.Request) {
		var block blockchain.Block
		if !decodeJSON(w, r, &block) {
			return
		}
		if err := n.ReceiveBlockContext(r.Context(), block); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"accepted": true, "height": block.Height})
	})
	mux.HandleFunc("POST /transactions", func(w http.ResponseWriter, r *http.Request) {
		var tx blockchain.Transaction
		if !decodeJSON(w, r, &tx) {
			return
		}
		if err := n.SubmitTransaction(tx); err != nil {
			status := http.StatusUnprocessableEntity
			if errors.Is(err, node.ErrDuplicateTransaction) {
				status = http.StatusConflict
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
	})
	mux.HandleFunc("POST /blocks/create", func(w http.ResponseWriter, r *http.Request) {
		block, err := n.CreateBlock(r.Context(), time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, block)
	})
	mux.HandleFunc("POST /sync", func(w http.ResponseWriter, r *http.Request) {
		var peer node.Peer
		if !decodeJSON(w, r, &peer) {
			return
		}
		if err := n.Sync(r.Context(), peer); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"synchronized": true})
	})
	return mux
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
		return false
	}
	return true
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{message})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

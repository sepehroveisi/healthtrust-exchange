package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
)

const maxResponseBytes = 4 << 20

type Client struct{ client *http.Client }

func NewClient(timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{client: &http.Client{Timeout: timeout}}
}

func (c *Client) BroadcastBlock(ctx context.Context, peers []node.Peer, block blockchain.Block) error {
	for _, peer := range peers {
		if err := c.postJSON(ctx, strings.TrimRight(peer.BaseURL, "/")+"/blocks", block); err != nil {
			return fmt.Errorf("broadcast to %s: %w", peer.ID, err)
		}
	}
	return nil
}

func (c *Client) FetchBlocks(ctx context.Context, peer node.Peer, fromHeight uint64) ([]blockchain.Block, error) {
	endpoint := strings.TrimRight(peer.BaseURL, "/") + "/blocks?from=" + url.QueryEscape(strconv.FormatUint(fromHeight, 10))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	reader := io.LimitReader(response.Body, maxResponseBytes+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > maxResponseBytes {
		return nil, errorsNew("response body too large")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("remote status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var blocks []blockchain.Block
	if err := json.Unmarshal(body, &blocks); err != nil {
		return nil, fmt.Errorf("decode blocks: %w", err)
	}
	return blocks, nil
}

func (c *Client) postJSON(ctx context.Context, endpoint string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("remote status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func errorsNew(message string) error { return fmt.Errorf("%s", message) }

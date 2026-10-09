package signaling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type SignalingClient struct {
	client   *http.Client
	relayURL string
}

// normalizeRelayURL ensures the relay endpoint has a scheme and no trailing
// slash. Bare "host:port" values (such as the address embedded in a session
// code) are assumed to be plain HTTP.
func normalizeRelayURL(raw string) string {
	u := strings.TrimSpace(raw)
	u = strings.TrimRight(u, "/")
	if u == "" {
		return "http://127.0.0.1:42425"
	}
	if !strings.Contains(u, "://") {
		u = "http://" + u
	}
	return u
}

// RelayURLFromEnv resolves the signaling relay endpoint. A relay must be
// reachable by BOTH peers for NAT hole punching to work, so deployments should
// point every client at the same shared relay (for example a self-hosted one
// started with `wormhole-sync -relay`). Set WORMHOLE_RELAY to a comma-separated
// list of URLs to override the built-in localhost fallback.
func RelayURLFromEnv() string {
	if v := strings.TrimSpace(os.Getenv("WORMHOLE_RELAY")); v != "" {
		for _, part := range strings.Split(v, ",") {
			if u := strings.TrimSpace(part); u != "" {
				return normalizeRelayURL(u)
			}
		}
	}
	return "http://127.0.0.1:42425"
}

func NewSignalingClient(relayURL string) *SignalingClient {
	return &SignalingClient{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		relayURL: normalizeRelayURL(relayURL),
	}
}

// PublishOffer posts the offer and direct address to the signaling server
func (c *SignalingClient) PublishOffer(ctx context.Context, code string, offerSDP string, directAddr string) error {
	payload, err := json.Marshal(map[string]string{
		"offer":      offerSDP,
		"directAddr": directAddr,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/signal/offer?code=%s", c.relayURL, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("publish offer returned status %d", resp.StatusCode)
	}
	return nil
}

// WaitForAnswer polls the relay until the peer posts their answer
func (c *SignalingClient) WaitForAnswer(ctx context.Context, code string) (string, error) {
	url := fmt.Sprintf("%s/signal/answer?code=%s", c.relayURL, code)

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", err
		}

		resp, err := c.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var body struct {
				Answer string `json:"answer"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if decodeErr == nil && body.Answer != "" {
				return body.Answer, nil
			}
		} else if resp != nil {
			resp.Body.Close()
		}

		time.Sleep(1 * time.Second)
	}
}

// FetchOffer retrieves the offer and direct address for the given room code
func (c *SignalingClient) FetchOffer(ctx context.Context, code string) (string, string, error) {
	url := fmt.Sprintf("%s/signal/offer?code=%s", c.relayURL, code)

	for {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return "", "", err
		}

		resp, err := c.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var body struct {
				Offer      string `json:"offer"`
				DirectAddr string `json:"directAddr"`
			}
			decodeErr := json.NewDecoder(resp.Body).Decode(&body)
			resp.Body.Close()
			if decodeErr == nil && (body.Offer != "" || body.DirectAddr != "") {
				return body.Offer, body.DirectAddr, nil
			}
		} else if resp != nil {
			resp.Body.Close()
		}

		time.Sleep(1 * time.Second)
	}
}

// PublishAnswer posts the receiver's SDP answer to the signaling server
func (c *SignalingClient) PublishAnswer(ctx context.Context, code string, answerSDP string) error {
	payload, err := json.Marshal(map[string]string{
		"answer": answerSDP,
	})
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/signal/answer?code=%s", c.relayURL, code)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("publish answer returned status %d", resp.StatusCode)
	}
	return nil
}

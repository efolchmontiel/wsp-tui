package giphy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const searchURL = "https://api.giphy.com/v1/gifs/search"

// Result is one GIF from a Giphy search.
type Result struct {
	ID    string
	Title string
	URL   string // downloadable GIF URL
}

type searchResponse struct {
	Data []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Images struct {
			Downsized struct {
				URL string `json:"url"`
			} `json:"downsized"`
			Original struct {
				URL string `json:"url"`
			} `json:"original"`
			FixedWidth struct {
				URL string `json:"url"`
			} `json:"fixed_width"`
		} `json:"images"`
	} `json:"data"`
	Meta struct {
		Status  int    `json:"status"`
		Message string `json:"msg"`
	} `json:"meta"`
}

// Search queries Giphy. apiKey is required; empty → error.
func Search(ctx context.Context, apiKey, query string, limit int) ([]Result, error) {
	apiKey = strings.TrimSpace(apiKey)
	query = strings.TrimSpace(query)
	if apiKey == "" {
		return nil, fmt.Errorf("giphy_api_key no configurada")
	}
	if query == "" {
		return nil, fmt.Errorf("escribí qué buscar")
	}
	if limit <= 0 || limit > 25 {
		limit = 12
	}
	u, err := url.Parse(searchURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("api_key", apiKey)
	q.Set("q", query)
	q.Set("limit", fmt.Sprintf("%d", limit))
	q.Set("rating", "pg-13")
	q.Set("lang", "es")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	var parsed searchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("giphy json: %w", err)
	}
	if parsed.Meta.Status != 0 && parsed.Meta.Status != 200 {
		return nil, fmt.Errorf("giphy: %s", parsed.Meta.Message)
	}
	out := make([]Result, 0, len(parsed.Data))
	for _, d := range parsed.Data {
		gifURL := d.Images.Downsized.URL
		if gifURL == "" {
			gifURL = d.Images.FixedWidth.URL
		}
		if gifURL == "" {
			gifURL = d.Images.Original.URL
		}
		if gifURL == "" {
			continue
		}
		title := strings.TrimSpace(d.Title)
		if title == "" {
			title = d.ID
		}
		out = append(out, Result{ID: d.ID, Title: title, URL: gifURL})
	}
	return out, nil
}

// DownloadToTemp fetches a GIF URL into a temp .gif file. Caller should remove it.
func DownloadToTemp(ctx context.Context, gifURL string) (string, error) {
	gifURL = strings.TrimSpace(gifURL)
	if gifURL == "" {
		return "", fmt.Errorf("url vacía")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, gifURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 45 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("download HTTP %d", res.StatusCode)
	}
	f, err := os.CreateTemp("", "wsp-tui-giphy-*.gif")
	if err != nil {
		return "", err
	}
	path := f.Name()
	_, copyErr := io.Copy(f, io.LimitReader(res.Body, 25<<20))
	closeErr := f.Close()
	if copyErr != nil {
		_ = os.Remove(path)
		return "", copyErr
	}
	if closeErr != nil {
		_ = os.Remove(path)
		return "", closeErr
	}
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".gif" {
		// CreateTemp already ends in .gif
	}
	return path, nil
}

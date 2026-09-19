//go:build windows

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// suggest.go adds live search-engine autocomplete to the address bar. Local
// history/bookmark suggestions (store.Suggest) always come first and appear
// instantly; the network suggestions are fetched from the selected engine's
// public autocomplete endpoint and merged in behind them.
//
// The queries are the same OpenSearch-style suggestion endpoints every major
// browser uses. They are only ever hit while the user is actively typing in
// the address bar and never in incognito mode.

// suggestClient is a short-timeout client: address-bar autocomplete must be
// snappy or not shown at all.
var suggestClient = &http.Client{Timeout: 2500 * time.Millisecond}

// fetchSearchSuggestions returns up to n search-query completions for q from
// the given engine. It returns nil on any error, empty query, or in
// incognito mode - the caller simply shows the local suggestions alone.
func fetchSearchSuggestions(engine, q string, n int) []suggestion {
	q = strings.TrimSpace(q)
	if q == "" || n <= 0 || incognitoMode {
		return nil
	}
	endpoint := suggestEndpoint(engine, q)
	if endpoint == "" {
		return nil
	}
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "OKBrowser/"+appVersion)
	resp, err := suggestClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil
	}
	terms := parseSuggestResponse(body)
	out := make([]suggestion, 0, n)
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		out = append(out, suggestion{URL: term, Title: term, Source: "s"})
		if len(out) >= n {
			break
		}
	}
	return out
}

// suggestEndpoint builds the autocomplete URL for the engine. All three use
// the OpenSearch suggestions JSON format: ["query",["s1","s2",...]].
func suggestEndpoint(engine, q string) string {
	e := url.QueryEscape(q)
	switch engine {
	case "Bing":
		return "https://www.bing.com/osjson.aspx?query=" + e
	case "DuckDuckGo":
		return "https://duckduckgo.com/ac/?type=list&q=" + e
	default: // Google
		return "https://suggestqueries.google.com/complete/search?client=firefox&q=" + e
	}
}

// parseSuggestResponse extracts the suggestion strings from an OpenSearch
// suggestions JSON array: element [1] is the list of completions. DuckDuckGo's
// type=list endpoint returns the identical shape.
func parseSuggestResponse(body []byte) []string {
	var raw []json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil || len(raw) < 2 {
		return nil
	}
	var terms []string
	if err := json.Unmarshal(raw[1], &terms); err != nil {
		return nil
	}
	return terms
}

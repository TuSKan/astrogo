package mast

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/TuSKan/astrogo/angle"
	"github.com/TuSKan/astrogo/catalog/provider"
	"github.com/TuSKan/astrogo/coord"
)

const mastAPI = "https://mast.stsci.edu/api/v0/invoke"

type Provider struct {
	client *provider.Client
	cache  provider.Cache
}

func New() *Provider {
	return &Provider{
		client: provider.NewClient(),
		cache:  provider.NewArrowCache(),
	}
}

func (p *Provider) Name() string { return "mast" }

func (p *Provider) Capabilities() []provider.Capability {
	return []provider.Capability{provider.CapObjectResolution, provider.CapConeSearch}
}

func (p *Provider) Resolve(query string) (provider.Target, bool) {
	targets := p.Search(query)
	if len(targets) > 0 {
		return targets[0], true
	}
	return provider.Target{}, false
}

func (p *Provider) Search(query string) []provider.Target {
	ctx := context.TODO()
	req := provider.ObjectRequest{Query: query, Limit: 10}

	iter := p.ResolveObject(ctx, req)
	var targets []provider.Target
	iter(func(t provider.Target, err error) bool {
		if err == nil {
			targets = append(targets, t)
		}
		return len(targets) < 10
	})
	return targets
}

func (p *Provider) ResolveObject(ctx context.Context, req provider.ObjectRequest) provider.SeqIterator[provider.Target] {
	cacheKey := "resolve:mast:" + provider.Normalize(req.Query)
	if seq, ok := p.cache.Get(cacheKey); ok {
		return seq
	}

	payload := map[string]interface{}{
		"service": "Mast.Name.Lookup",
		"format":  "json",
		"params": map[string]string{
			"input": req.Query,
		},
	}

	b, _ := json.Marshal(payload)
	v := url.Values{}
	v.Set("request", string(b))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, mastAPI, strings.NewReader(v.Encode()))
	if err != nil {
		return provider.SliceSeq([]provider.Target{})
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return func(yield func(provider.Target, error) bool) {
		resp, err := p.client.Do(httpReq)
		if err != nil {
			yield(provider.Target{}, err)
			return
		}
		defer resp.Body.Close()

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			yield(provider.Target{}, err)
			return
		}

		var targets []provider.Target

		if len(b) > 0 && b[0] == '{' {
			// Try parsing as JSON first
			var jsonPayload struct {
				Status             string `json:"status"`
				Msg                string `json:"msg"`
				ResolvedCoordinate []struct {
					CanonicalName string  `json:"canonicalName"`
					RA            float64 `json:"ra"`
					Decl          float64 `json:"decl"`
					Resolver      string  `json:"resolver"`
				} `json:"resolvedCoordinate"`
			}

			if err := json.Unmarshal(b, &jsonPayload); err == nil {
				if jsonPayload.Status == "ERROR" {
					yield(provider.Target{}, fmt.Errorf("mast: %s", jsonPayload.Msg))
					return
				}
				for _, match := range jsonPayload.ResolvedCoordinate {
					targets = append(targets, provider.Target{
						ID:      match.CanonicalName,
						Name:    match.CanonicalName,
						Coord:   coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
						Catalog: match.Resolver,
					})
				}
			}
		}

		if len(targets) == 0 {
			// Fallback to XML
			var xmlPayload struct {
				XMLName            xml.Name `xml:"resolvedItems"`
				ResolvedCoordinate []struct {
					CanonicalName string  `xml:"canonicalName"`
					RA            float64 `xml:"ra"`
					Decl          float64 `xml:"dec"` // XML uses 'dec', JSON uses 'decl'
					Resolver      string  `xml:"resolver"`
				} `xml:"resolvedCoordinate"`
			}

			if err := xml.Unmarshal(b, &xmlPayload); err != nil {
				yield(provider.Target{}, err)
				return
			}

			for _, match := range xmlPayload.ResolvedCoordinate {
				targets = append(targets, provider.Target{
					ID:      match.CanonicalName,
					Name:    match.CanonicalName,
					Coord:   coord.NewICRS(angle.Deg(match.RA), angle.Deg(match.Decl)),
					Catalog: match.Resolver,
				})
			}
		}

		if err := p.cache.Set(cacheKey, targets); err != nil {
			yield(provider.Target{}, err)
			return
		}

		for _, t := range targets {
			if !yield(t, nil) {
				return
			}
		}
	}
}

func (p *Provider) ConeSearch(ctx context.Context, req provider.ConeRequest) provider.SeqIterator[provider.Target] {
	// Minimal stub for CAOM spatial search
	return provider.SliceSeq([]provider.Target{})
}
